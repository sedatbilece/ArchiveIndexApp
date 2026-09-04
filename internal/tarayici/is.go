package tarayici

import (
	"context"
	"errors"
	"sync"
	"time"

	"arsiv-indeks/internal/ayarlar"
	"arsiv-indeks/internal/indeks"
)

// ErrTaramaSuruyor ikinci bir tarama başlatılmaya çalışıldığında döner.
// HTTP katmanı bunu 409 Conflict olarak çevirir.
var ErrTaramaSuruyor = errors.New("tarama zaten sürüyor")

// ErrTaramaYok boştayken iptal istendiğinde döner.
var ErrTaramaYok = errors.New("sürmekte olan tarama yok")

// Is tek bir tarama işinin durum makinesidir.
//
//	Bosta -> Sayiliyor -> Taraniyor -> Kaydediliyor -> (Tamamlandi|Iptal|Hata)
//
// Durum sunucuda yaşadığı için tarayıcı sayfasını yenilemek bedavadır;
// istemci sadece /tarama/durum uç noktasını yoklar.
type Is struct {
	kilit     sync.Mutex
	ilerleme  *Ilerleme
	iptal     context.CancelFunc
	calisiyor bool
	baslangic time.Time
	bitis     time.Time
	hata      string
	ozet      Ozet
}

// YeniIs boşta bir tarama işi oluşturur.
func YeniIs() *Is {
	return &Is{ilerleme: YeniIlerleme()}
}

// Durum istemciye JSON olarak dönen anlık görüntüdür.
type Durum struct {
	Durum             string `json:"durum"`
	Asama             string `json:"asama"`
	Calisiyor         bool   `json:"calisiyor"`
	Toplam            int64  `json:"toplam"`
	Islenen           int64  `json:"islenen"`
	Atlanan           int64  `json:"atlanan"`
	Hatali            int64  `json:"hatali"`
	IcerikOkunan      int64  `json:"icerikOkunan"`
	SadeceAd          int64  `json:"sadeceAd"`
	Silinen           int64  `json:"silinen"`
	Yuzde             int    `json:"yuzde"`
	SonDosya          string `json:"sonDosya"`
	GecenSaniye       int64  `json:"gecenSaniye"`
	KalanTahminSaniye int64  `json:"kalanTahminSaniye"`
	Hata              string `json:"hata"`
	Bitis             string `json:"bitis"`
}

// Basla taramayı arka planda başlatır. Zaten bir tarama sürüyorsa
// ErrTaramaSuruyor döner.
func (is *Is) Basla(ix *indeks.Indeks, ay ayarlar.Ayarlar, veriDizini string) error {
	is.kilit.Lock()
	if is.calisiyor {
		is.kilit.Unlock()
		return ErrTaramaSuruyor
	}

	ctx, iptal := context.WithCancel(context.Background())
	is.calisiyor = true
	is.iptal = iptal
	is.baslangic = time.Now()
	is.bitis = time.Time{}
	is.hata = ""
	is.ozet = Ozet{}
	is.ilerleme = YeniIlerleme()
	il := is.ilerleme
	is.kilit.Unlock()

	go func() {
		defer iptal()
		ozet, err := Tara(ctx, ix, ay, veriDizini, il)

		is.kilit.Lock()
		defer is.kilit.Unlock()
		is.calisiyor = false
		is.bitis = time.Now()
		is.ozet = ozet
		switch {
		case errors.Is(err, context.Canceled):
			il.AsamaYaz(AsamaIptal)
		case err != nil:
			is.hata = err.Error()
			il.AsamaYaz(AsamaHata)
		default:
			il.AsamaYaz(AsamaTamamlandi)
		}
	}()

	return nil
}

// Iptal sürmekte olan taramayı durdurur.
func (is *Is) Iptal() error {
	is.kilit.Lock()
	defer is.kilit.Unlock()
	if !is.calisiyor || is.iptal == nil {
		return ErrTaramaYok
	}
	is.iptal()
	return nil
}

// Calisiyor tarama sürüyor mu.
func (is *Is) Calisiyor() bool {
	is.kilit.Lock()
	defer is.kilit.Unlock()
	return is.calisiyor
}

// Ozet biten son taramanın özetini döner.
func (is *Is) Ozet() Ozet {
	is.kilit.Lock()
	defer is.kilit.Unlock()
	return is.ozet
}

// Durum anlık görüntüyü üretir.
func (is *Is) Durum() Durum {
	is.kilit.Lock()
	il := is.ilerleme
	calisiyor := is.calisiyor
	baslangic := is.baslangic
	bitis := is.bitis
	hata := is.hata
	is.kilit.Unlock()

	asama := il.Asama()
	toplam := il.Toplam.Load()
	islenen := il.Islenen.Load()
	atlanan := il.Atlanan.Load()
	hatali := il.Hatali.Load()

	// İlerleme = işlenen + atlanan. Artımlı taramada dosyaların çoğu
	// "atlanan" olur; onları saymazsak çubuk hiç dolmaz.
	ilerlemis := islenen + atlanan
	yuzde := 0
	if toplam > 0 {
		yuzde = int(min(ilerlemis*100/toplam, 100))
	} else if !calisiyor {
		yuzde = 100
	}

	var gecen, kalan int64
	if !baslangic.IsZero() {
		bitisAn := time.Now()
		if !calisiyor && !bitis.IsZero() {
			bitisAn = bitis
		}
		gecen = int64(bitisAn.Sub(baslangic).Seconds())
		if calisiyor && ilerlemis > 0 && toplam > ilerlemis {
			hiz := float64(ilerlemis) / float64(max(gecen, 1))
			if hiz > 0 {
				kalan = int64(float64(toplam-ilerlemis) / hiz)
			}
		}
	}

	d := Durum{
		Durum:             asama,
		Asama:             AsamaAciklama(asama),
		Calisiyor:         calisiyor,
		Toplam:            toplam,
		Islenen:           islenen,
		Atlanan:           atlanan,
		Hatali:            hatali,
		IcerikOkunan:      il.IcerikOkunan.Load(),
		SadeceAd:          il.SadeceAd.Load(),
		Silinen:           il.Silinen.Load(),
		Yuzde:             yuzde,
		SonDosya:          il.SonDosya(),
		GecenSaniye:       gecen,
		KalanTahminSaniye: kalan,
		Hata:              hata,
	}
	if !bitis.IsZero() {
		d.Bitis = bitis.Format("02.01.2006 15:04:05")
	}
	return d
}
