// Paket tarayici, arşiv dizinlerini gezer, dosya içeriklerini çıkarır ve
// indekse yazar.
//
// # İş hattı
//
//	1 üretici (WalkDir)  ->  N çıkarıcı işçi  ->  1 indeks yazıcı
//
// Kanal tamponları küçük (256) tutuluyor: bu doğal geri baskı sağlar, yani
// çıkarım yavaşladığında yürüyüş bekler ve 100.000 Gorev asla aynı anda
// bellekte durmaz.
//
// İndekse yazan TEK goroutine yazıcıdır. Yazmalar partiler halinde yapılır
// (bkz. PartiBoyutu), böylece yazma kilidi kısa aralıklarla tutulur ve
// tarama sürerken yapılan aramalar hissedilir şekilde yavaşlamaz.
package tarayici

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"arsiv-indeks/internal/ayarlar"
	"arsiv-indeks/internal/cikarim"
	"arsiv-indeks/internal/indeks"
	"arsiv-indeks/internal/metin"
)

const (
	// KanalTamponu üretici ve işçiler arasındaki tampon boyutu.
	KanalTamponu = 256
	// PartiBoyutu tek seferde indekse yazılacak doküman sayısı.
	PartiBoyutu = 512
	// KontrolNoktasi kaç dokümanda bir indeksin diske yazılacağı.
	// 90.000. dosyada çöken bir tarama boşa gitmesin.
	KontrolNoktasi = 20000
	// DosyaZamanAsimi tek bir dosyanın çıkarımına ayrılan azami süre.
	DosyaZamanAsimi = 60 * time.Second
	// SikistirmaEsigi tombstone oranı bunu geçince indeks sıkıştırılır.
	SikistirmaEsigi = 0.20
)

// Gorev işçiye verilen tek dosyalık iştir.
type Gorev struct {
	Yol       string
	Boyut     int64
	Zaman     int64
	IcerikOku bool
	Durum     string // içerik okunmayacaksa nedenini taşır
}

type isSonuc struct {
	Gorev
	Belirtecler []string
	Durum       string
}

// Tara verilen ayarlardaki kökleri tarar ve indeksi güncelleyip diske yazar.
//
// Artımlıdır: boyutu ve değişim zamanı aynı kalan dosyaların içeriği yeniden
// çıkarılmaz. Taranan köklerin altında olup bu taramada görülmeyen dosyalar
// indeksten düşürülür.
func Tara(
	ctx context.Context,
	ix *indeks.Indeks,
	ay ayarlar.Ayarlar,
	veriDizini string,
	il *Ilerleme,
) (Ozet, error) {
	baslangic := time.Now()
	kokler := ay.TaranacakDizinler
	if len(kokler) == 0 {
		return Ozet{}, errors.New("taranacak dizin belirtilmemiş")
	}

	// --- Geçiş 1: sayım ---------------------------------------------------
	// Yalnızca dizin girdilerine bakar; Stat yok, dosya açma yok. NTFS'te
	// 100.000 dosya için ~1-3 saniye. Karşılığında gerçek bir yüzde
	// gösterebiliyoruz ki belirsiz ilerleme çubuğundan çok daha iyi.
	il.AsamaYaz(AsamaSayiliyor)
	toplam := say(ctx, kokler, ay)
	if err := ctx.Err(); err != nil {
		return il.OzetAl(time.Since(baslangic)), err
	}
	il.Toplam.Store(toplam)

	// --- Geçiş 2: tarama --------------------------------------------------
	il.AsamaYaz(AsamaTaraniyor)

	isKanal := make(chan Gorev, KanalTamponu)
	ciktiKanal := make(chan isSonuc, KanalTamponu)
	gorulen := make(map[string]struct{}, toplam)

	// Yazıcı goroutine: indekse yazan tek yer.
	yaziciBitti := make(chan struct{})
	go func() {
		defer close(yaziciBitti)
		yazici(ix, il, veriDizini, ciktiKanal)
	}()

	// Çıkarıcı işçiler.
	var wg sync.WaitGroup
	for i := 0; i < ay.IsciAdedi(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			isci(ctx, ay, il, isKanal, ciktiKanal)
		}()
	}

	// Üretici ana goroutine'de çalışır.
	yuru(ctx, kokler, ay, ix, il, gorulen, isKanal)
	close(isKanal)
	wg.Wait()
	close(ciktiKanal)
	<-yaziciBitti

	if err := ctx.Err(); err != nil {
		// İptal edildi: yarım da olsa elimizdekini kaydediyoruz, ama
		// TaramaBitti damgalanmıyor — böylece Ayarlar sayfası "tarama yarım
		// kaldı" uyarısı gösterebiliyor.
		_ = ix.Kaydet(veriDizini)
		return il.OzetAl(time.Since(baslangic)), err
	}

	// --- Silinen dosyaları düşür -----------------------------------------
	// Yalnızca BU taramanın köklerinin altındaki, artık var olmayan
	// dosyaları düşürüyoruz. Ayarlardan çıkarılmış köklerin yetim
	// dokümanları burada değil, kök kümesinin gerçekten değiştiği yerlerde
	// temizlenir (ayar kaydetme ve uygulama açılışı) — böylece Tara'nın
	// sözleşmesi "sana verdiğim kökleri uzlaştır" olarak dar kalıyor.
	il.Silinen.Store(int64(ix.GorulmeyenleriSil(gorulen, kokler)))

	if ix.TombstoneOrani() > SikistirmaEsigi {
		ix.Sikistir()
	}

	// --- Kaydet ------------------------------------------------------------
	il.AsamaYaz(AsamaKaydediliyor)
	ix.TaramaBitti(time.Now())
	if err := ix.Kaydet(veriDizini); err != nil {
		return il.OzetAl(time.Since(baslangic)), err
	}

	return il.OzetAl(time.Since(baslangic)), nil
}

// ---------------------------------------------------------------- geçiş 1

func say(ctx context.Context, kokler []string, ay ayarlar.Ayarlar) int64 {
	var n int64
	for _, kok := range kokler {
		filepath.WalkDir(kok, func(yol string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return fs.SkipAll
			}
			if err != nil {
				return yuruyusHatasi(d, err)
			}
			if d.IsDir() {
				if yol != kok && klasorAtlanirMi(ay, d) {
					return fs.SkipDir
				}
				return nil
			}
			if dosyaUygunMu(ay, d.Name()) {
				n++
			}
			return nil
		})
	}
	return n
}

// ---------------------------------------------------------------- üretici

func yuru(
	ctx context.Context,
	kokler []string,
	ay ayarlar.Ayarlar,
	ix *indeks.Indeks,
	il *Ilerleme,
	gorulen map[string]struct{},
	isKanal chan<- Gorev,
) {
	maksBayt := ay.MaksDosyaBayt()

	for _, kok := range kokler {
		filepath.WalkDir(kok, func(yol string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return fs.SkipAll
			}
			if err != nil {
				// İzin reddedilen bir klasör yürüyüşü SONLANDIRMAMALI.
				return yuruyusHatasi(d, err)
			}

			if d.IsDir() {
				if yol == kok {
					return nil
				}
				if klasorAtlanirMi(ay, d) {
					return fs.SkipDir
				}
				// Junction/symlink döngülerini burada kesiyoruz.
				if bilgi, e := d.Info(); e == nil && ReparseNoktasiMi(bilgi) {
					return fs.SkipDir
				}
				return nil
			}

			if !dosyaUygunMu(ay, d.Name()) {
				il.Atlanan.Add(1)
				return nil
			}

			bilgi, e := d.Info()
			if e != nil {
				il.Hatali.Add(1)
				return nil
			}
			if ReparseNoktasiMi(bilgi) {
				il.Atlanan.Add(1)
				return nil
			}

			boyut := bilgi.Size()
			zaman := bilgi.ModTime().UnixNano()

			// Bu dosyayı gördük: silme tespitinde kullanılacak.
			gorulen[indeks.Anahtar(yol)] = struct{}{}

			// Artımlı taramanın kalbi: boyut ve zaman aynıysa çıkarımı
			// tamamen atlıyoruz. Değişmemiş bir arşivin yeniden taranması
			// böylece dakikalar yerine saniyeler sürüyor.
			if ix.Degismemis(yol, boyut, zaman) {
				il.Atlanan.Add(1)
				return nil
			}

			g := Gorev{Yol: yol, Boyut: boyut, Zaman: zaman}
			uzanti := strings.ToLower(filepath.Ext(yol))
			switch {
			case BulutYerTutucuMu(bilgi):
				// Okumak dosyayı buluttan indirir; yalnızca metadata.
				g.Durum = cikarim.DurumBulut
			case boyut > maksBayt:
				g.Durum = cikarim.DurumBuyuk
			case cikarim.EskiOfficeMi(uzanti):
				// Genel "ad indeksli" etiketi yerine nedeni söyleyelim.
				g.Durum = cikarim.DurumEskiFormat
			case !ay.IcerikOkunacakMi(uzanti):
				g.Durum = cikarim.DurumAdOnly
			default:
				g.IcerikOku = true
			}

			select {
			case isKanal <- g:
			case <-ctx.Done():
				return fs.SkipAll
			}
			return nil
		})
	}
}

// yuruyusHatasi WalkDir'in hata argümanını ele alır.
//
// Bu, izin reddedilen bir klasörün tüm taramayı sonlandırmasını engelleyen
// koddur. Klasörse o alt ağacı atla, dosyaysa görmezden gel.
func yuruyusHatasi(d fs.DirEntry, err error) error {
	if d != nil && d.IsDir() {
		return fs.SkipDir
	}
	return nil
}

func klasorAtlanirMi(ay ayarlar.Ayarlar, d fs.DirEntry) bool {
	if ay.HaricKlasorMu(d.Name()) {
		return true
	}
	// Yalnızca SİSTEM klasörlerini atlıyoruz, gizli olanları atlamıyoruz:
	// kullanıcının arşiv klasörü gizli işaretlenmiş olabilir ve onu sessizce
	// atlamak sessiz veri kaybı olurdu. $RECYCLE.BIN gibi gerçek çöpler
	// zaten ad kara listesinde.
	if bilgi, err := d.Info(); err == nil {
		if ozellikSistemMi(bilgi) {
			return true
		}
	}
	return false
}

func dosyaUygunMu(ay ayarlar.Ayarlar, ad string) bool {
	if strings.HasPrefix(ad, "~$") {
		return false // Office geçici kilit dosyaları
	}
	return !ay.HaricUzantiMi(strings.ToLower(filepath.Ext(ad)))
}

// ---------------------------------------------------------------- işçi

func isci(
	ctx context.Context,
	ay ayarlar.Ayarlar,
	il *Ilerleme,
	isKanal <-chan Gorev,
	ciktiKanal chan<- isSonuc,
) {
	sec := cikarim.Secenekler{MaksKarakter: ay.MaksMetinKarakter}

	for g := range isKanal {
		if ctx.Err() != nil {
			return
		}
		il.SonDosyaYaz(g.Yol)

		s := isSonuc{Gorev: g, Durum: g.Durum}
		if g.IcerikOku {
			// Dosya başına zaman aşımı: patolojik tek bir dosya taramayı
			// süresiz kilitlemesin.
			dctx, dur := context.WithTimeout(ctx, DosyaZamanAsimi)
			icerik, durum := cikarim.GuvenliCikar(dctx, g.Yol, sec)
			dur()

			s.Durum = durum
			if icerik != "" {
				s.Belirtecler = metin.TekilBelirtecler(icerik, ay.DokumanBelirtecCap)
			}
		}

		select {
		case ciktiKanal <- s:
		case <-ctx.Done():
			return
		}
	}
}

// ---------------------------------------------------------------- yazıcı

func yazici(
	ix *indeks.Indeks,
	il *Ilerleme,
	veriDizini string,
	ciktiKanal <-chan isSonuc,
) {
	parti := make([]indeks.Girdi, 0, PartiBoyutu)
	sonKontrol := int64(0)

	bosalt := func() {
		if len(parti) == 0 {
			return
		}
		ix.EkleToplu(parti)
		parti = parti[:0]
	}

	for s := range ciktiKanal {
		parti = append(parti, indeks.Girdi{
			Yol:         s.Yol,
			Boyut:       s.Boyut,
			Zaman:       s.Zaman,
			Durum:       s.Durum,
			Belirtecler: s.Belirtecler,
		})

		islenen := il.Islenen.Add(1)
		switch s.Durum {
		case cikarim.DurumTam, cikarim.DurumKirpildi:
			il.IcerikOkunan.Add(1)
		case cikarim.DurumBozuk:
			il.Hatali.Add(1)
			il.SadeceAd.Add(1)
		default:
			il.SadeceAd.Add(1)
		}

		if len(parti) >= PartiBoyutu {
			bosalt()
		}
		if islenen-sonKontrol >= KontrolNoktasi {
			sonKontrol = islenen
			bosalt()
			// Ara kayıt: çökme hâlinde 20.000 dosyadan fazlasını kaybetmeyiz.
			_ = ix.Kaydet(veriDizini)
		}
	}
	bosalt()
}
