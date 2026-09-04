// Paket sorgu, arama isteklerini ayrıştırır, indeks üzerinde çalıştırır,
// skorlar, sıralar ve sayfalar.
package sorgu

import (
	"math"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"arsiv-indeks/internal/cikarim"
	"arsiv-indeks/internal/indeks"
	"arsiv-indeks/internal/metin"
)

// Kapsam aramanın nerede yapılacağını belirler.
type Kapsam string

const (
	KapsamHepsi  Kapsam = "hepsi"
	KapsamAd     Kapsam = "ad"
	KapsamIcerik Kapsam = "icerik"
)

// Sirala sonuç sıralamasını belirler.
type Sirala string

const (
	SiralaSkor       Sirala = "skor"
	SiralaTarihYeni  Sirala = "tarih"
	SiralaTarihEski  Sirala = "tarih-eski"
	SiralaAd         Sirala = "ad"
	SiralaBoyutBuyuk Sirala = "boyut"
)

// EnFazlaSayfa çok derin sayfalamayı engeller.
const EnFazlaSayfa = 400

// UzantisizEtiket uzantısı olmayan dosyalar için filtre değeri.
const UzantisizEtiket = "(uzantısız)"

// Istek tek bir arama isteğidir.
type Istek struct {
	Metin     string
	Kapsam    Kapsam
	Uzantilar []string
	Klasor    string
	Baslangic time.Time
	Bitis     time.Time
	Sirala    Sirala
	Sayfa     int
	Adet      int
}

// BosMu istekte hiç arama ölçütü olup olmadığını söyler.
func (is Istek) BosMu() bool {
	return strings.TrimSpace(is.Metin) == "" &&
		len(is.Uzantilar) == 0 &&
		is.Klasor == "" &&
		is.Baslangic.IsZero() &&
		is.Bitis.IsZero()
}

// Sonuc tek bir sonuç satırıdır.
type Sonuc struct {
	Yol           string
	Ad            string
	Klasor        string
	Durum         string
	DurumAciklama string
	Boyut         int64
	Zaman         time.Time
	Puan          float64
	AdParcalari   []Parca
	// IcerikAranabilir false ise parçacık istemeye çalışmıyoruz.
	IcerikAranabilir bool
}

// Cikti bir aramanın sonucudur.
type Cikti struct {
	Sonuclar    []Sonuc
	Toplam      int
	Sayfa       int
	SayfaSayisi int
	Sure        time.Duration
	// Parcalar istemcinin vurgulaması için sorgu parçaları.
	Parcalar []string
}

// Ayristir URL sorgu parametrelerinden bir istek üretir.
//
// Tüm durum query param'da tutuluyor: sonuçlar yer imlenebilir, geri tuşu
// doğru çalışır ve POST/redirect dansına gerek kalmaz.
func Ayristir(q url.Values, varsayilanAdet int) Istek {
	is := Istek{
		Metin:     strings.TrimSpace(q.Get("q")),
		Kapsam:    kapsamCoz(q.Get("kapsam")),
		Uzantilar: temizListe(q["uzanti"]),
		Klasor:    strings.TrimSpace(q.Get("klasor")),
		Baslangic: tarihCoz(q.Get("baslangic"), false),
		Bitis:     tarihCoz(q.Get("bitis"), true),
		Sirala:    siralaCoz(q.Get("sirala")),
		Sayfa:     sayiCoz(q.Get("sayfa"), 1),
		Adet:      varsayilanAdet,
	}
	if is.Sayfa < 1 {
		is.Sayfa = 1
	}
	if is.Sayfa > EnFazlaSayfa {
		is.Sayfa = EnFazlaSayfa
	}
	if is.Adet <= 0 {
		is.Adet = 25
	}
	return is
}

func kapsamCoz(s string) Kapsam {
	switch Kapsam(s) {
	case KapsamAd:
		return KapsamAd
	case KapsamIcerik:
		return KapsamIcerik
	}
	return KapsamHepsi
}

func siralaCoz(s string) Sirala {
	switch Sirala(s) {
	case SiralaTarihYeni:
		return SiralaTarihYeni
	case SiralaTarihEski:
		return SiralaTarihEski
	case SiralaAd:
		return SiralaAd
	case SiralaBoyutBuyuk:
		return SiralaBoyutBuyuk
	}
	return SiralaSkor
}

func sayiCoz(s string, varsayilan int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return varsayilan
}

// tarihCoz "2024-01-31" biçimini ayrıştırır. gunSonu true ise günün sonuna
// çeker, böylece "bitiş" tarihi o günü de kapsar.
func tarihCoz(s string, gunSonu bool) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}
	}
	if gunSonu {
		return t.Add(24*time.Hour - time.Nanosecond)
	}
	return t
}

func temizListe(l []string) []string {
	cikti := make([]string, 0, len(l))
	for _, s := range l {
		if s = strings.TrimSpace(s); s != "" {
			cikti = append(cikti, strings.ToLower(s))
		}
	}
	return cikti
}

// SorguParcalari kullanıcının yazdığı metni alt dize araması için parçalara
// ayırır.
//
// Bu, metin.Belirtecler'den KASITLI olarak farklıdır: burada durak kelimeleri
// ve uzun sayıları atmıyoruz. Kullanıcı "1698240000" veya "the" yazdıysa onu
// dosya adında alt dize olarak aramak istiyor demektir; ters indekste o terim
// bulunmaz ama alt dize taraması bulur.
func SorguParcalari(s string) []string {
	var cikti []string
	for _, p := range strings.Fields(metin.Normalize(s)) {
		if p != "" {
			cikti = append(cikti, p)
		}
	}
	return cikti
}

// Calistir aramayı yürütür.
func Calistir(ix *indeks.Indeks, is Istek) Cikti {
	basla := time.Now()
	cikti := Cikti{Sayfa: is.Sayfa, Parcalar: SorguParcalari(is.Metin)}

	terimler := metin.Belirtecler(is.Metin)
	parcalar := cikti.Parcalar

	var toplandi []puanliBelge

	ix.Oku(func(o indeks.Okuyucu) {
		adaylar := adaylariBul(o, is.Kapsam, terimler, parcalar)
		toplandi = make([]puanliBelge, 0, min(len(adaylar), 4096))

		n := float64(max(o.CanliSayisi(), 1))
		// idf'i terim başına bir kez hesaplıyoruz. Hem içerik hem yol
		// frekansını topluyoruz: "rapor" her dosya adında geçiyorsa ayırt
		// edici değildir ve düşük ağırlık almalıdır.
		idf := make(map[string]float64, len(terimler))
		for _, t := range terimler {
			df := float64(max(o.ToplamDf(t), 1))
			idf[t] = math.Log(1 + n/df)
		}

		for _, id := range adaylar {
			if o.SilinmisMi(id) {
				continue
			}
			b := o.Belge(id)
			if !filtreleriGecer(b, is) {
				continue
			}
			toplandi = append(toplandi, puanliBelge{
				belge: b,
				puan:  puanla(o, id, b, terimler, parcalar, idf),
			})
		}
	})

	cikti.Toplam = len(toplandi)
	sirala(toplandi, is.Sirala)

	// Sayfalama: sıralama zaten tüm aday kümesi üzerinde yapıldığı için
	// derin sayfalama ek maliyet getirmiyor, sadece dilimliyoruz.
	basIndeks := (is.Sayfa - 1) * is.Adet
	if basIndeks > len(toplandi) {
		basIndeks = len(toplandi)
	}
	sonIndeks := min(basIndeks+is.Adet, len(toplandi))

	cikti.SayfaSayisi = (len(toplandi) + is.Adet - 1) / is.Adet
	cikti.Sonuclar = make([]Sonuc, 0, sonIndeks-basIndeks)
	for _, pb := range toplandi[basIndeks:sonIndeks] {
		ad := pb.belge.Ad()
		cikti.Sonuclar = append(cikti.Sonuclar, Sonuc{
			Yol:              pb.belge.Yol,
			Ad:               ad,
			Klasor:           pb.belge.Klasor(),
			Durum:            pb.belge.Durum,
			DurumAciklama:    cikarim.DurumAciklama(pb.belge.Durum),
			Boyut:            pb.belge.Boyut,
			Zaman:            pb.belge.Vakit(),
			Puan:             pb.puan,
			AdParcalari:      Vurgula(ad, parcalar),
			IcerikAranabilir: icerikAranabilirMi(pb.belge.Durum),
		})
	}

	cikti.Sure = time.Since(basla)
	return cikti
}

func icerikAranabilirMi(durum string) bool {
	switch durum {
	case cikarim.DurumTam, cikarim.DurumKirpildi:
		return true
	}
	return false
}

type puanliBelge struct {
	belge indeks.Belge
	puan  float64
}

// ---------------------------------------------------------------- adaylar

// adaylariBul kapsama göre aday doküman kimliklerini üretir.
func adaylariBul(o indeks.Okuyucu, kapsam Kapsam, terimler, parcalar []string) []uint32 {
	// Hiç arama metni yok: filtreler tek başına çalışsın, tüm dokümanlar
	// aday olsun. 100.000 doküman üzerinde doğrusal tarama yeterince hızlı.
	if len(terimler) == 0 && len(parcalar) == 0 {
		hepsi := make([]uint32, o.BelgeSayisi())
		for i := range hepsi {
			hepsi[i] = uint32(i)
		}
		return hepsi
	}

	switch kapsam {
	case KapsamAd:
		// Dosya adı araması iki yoldan yürür: alt dize taraması (kullanıcı
		// adın bir PARÇASINI yazdıysa) ve yol ters indeksi (tam belirteç
		// yazdıysa; klasör adlarını da kapsar).
		return birlestir(
			adAdaylari(o, parcalar),
			indeksAdaylari(o, terimler, o.AdGonderiler),
		)
	case KapsamIcerik:
		// YALNIZCA içerik gönderileri. Yol terimleri ayrı haritada olduğu
		// için buraya dosya adı eşleşmesi karışmıyor.
		return indeksAdaylari(o, terimler, o.Gonderiler)
	}

	// KapsamHepsi: üç kaynağın birleşimi.
	return birlestir(
		birlestir(
			indeksAdaylari(o, terimler, o.Gonderiler),
			indeksAdaylari(o, terimler, o.AdGonderiler),
		),
		adAdaylari(o, parcalar),
	)
}

// indeksAdaylari ters indeks üzerinden AND birleştirmesi yapar.
//
// En küçük gönderi listesinden başlıyoruz: kesişimin boyutu en küçük listeyle
// sınırlı olduğu için bu, gereksiz karşılaştırmaları en aza indirir.
func indeksAdaylari(o indeks.Okuyucu, terimler []string, gonderiler func(string) []uint32) []uint32 {
	if len(terimler) == 0 {
		return nil
	}
	listeler := make([][]uint32, 0, len(terimler))
	for _, t := range terimler {
		g := gonderiler(t)
		if len(g) == 0 {
			return nil // AND: bir terim hiç yoksa sonuç boş
		}
		listeler = append(listeler, g)
	}
	sort.Slice(listeler, func(i, j int) bool {
		return len(listeler[i]) < len(listeler[j])
	})

	sonuc := slices.Clone(listeler[0])
	for _, l := range listeler[1:] {
		sonuc = kesistir(sonuc, l)
		if len(sonuc) == 0 {
			return nil
		}
	}
	return sonuc
}

// kesistir iki artan sıralı listenin kesişimini alır.
func kesistir(a, b []uint32) []uint32 {
	cikti := a[:0]
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			cikti = append(cikti, a[i])
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return cikti
}

// adAdaylari dosya adında alt dize araması yapar.
//
// Ters indeks alt dize aramaz. n-gram indeksi kurmak yerine önceden
// normalize edilmiş ad önbelleği üzerinde doğrusal strings.Contains
// yapıyoruz: 100.000 dosyada ~5-10 ms, yerini alacağı indeksten daha ucuz.
func adAdaylari(o indeks.Okuyucu, parcalar []string) []uint32 {
	if len(parcalar) == 0 {
		return nil
	}
	var cikti []uint32
	for id := 0; id < o.BelgeSayisi(); id++ {
		kid := uint32(id)
		if o.SilinmisMi(kid) {
			continue
		}
		ad := o.AdNorm(kid)
		hepsiVar := true
		for _, p := range parcalar {
			if !strings.Contains(ad, p) {
				hepsiVar = false
				break
			}
		}
		if hepsiVar {
			cikti = append(cikti, kid)
		}
	}
	return cikti
}

// birlestir iki artan sıralı listenin birleşimini alır.
func birlestir(a, b []uint32) []uint32 {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	cikti := make([]uint32, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			cikti = append(cikti, a[i])
			i++
			j++
		case a[i] < b[j]:
			cikti = append(cikti, a[i])
			i++
		default:
			cikti = append(cikti, b[j])
			j++
		}
	}
	cikti = append(cikti, a[i:]...)
	cikti = append(cikti, b[j:]...)
	return cikti
}

// ---------------------------------------------------------------- filtreler

// filtreleriGecer metadata filtrelerini uygular.
//
// Bunlar aday kümesi üzerinde SON filtre olarak çalışır, ön filtre olarak
// değil: içerik AND'i genelde birkaç yüz doküman döndürür, o kümeyi süzmek
// 100.000 dokümanı önceden süzmekten çok daha ucuz.
func filtreleriGecer(b indeks.Belge, is Istek) bool {
	if len(is.Uzantilar) > 0 {
		u := b.Uzanti()
		if u == "" {
			u = UzantisizEtiket
		}
		if !slices.Contains(is.Uzantilar, u) {
			return false
		}
	}
	if is.Klasor != "" {
		if _, altinda := indeks.AltindaMi(b.Yol, is.Klasor); !altinda {
			return false
		}
	}
	if !is.Baslangic.IsZero() && b.Vakit().Before(is.Baslangic) {
		return false
	}
	if !is.Bitis.IsZero() && b.Vakit().After(is.Bitis) {
		return false
	}
	return true
}

// ---------------------------------------------------------------- skorlama

// puanla alaka puanını hesaplar.
//
// Skor = Σ idf(terim) × alan çarpanı. Konum (position) ve terim frekansı
// TUTMUYORUZ; ad ağırlıklı bir arşivde bu basit formül BM25'i döverken
// indeksi ~80 MB küçük tutuyor.
//
// Alan çarpanları: ad eşleşmesi ×3, klasör/yol ×2, içerik ×1. Arşivde dosya
// adı, içerikten çok daha güçlü bir sinyaldir.
func puanla(
	o indeks.Okuyucu,
	id uint32,
	b indeks.Belge,
	terimler, parcalar []string,
	idf map[string]float64,
) float64 {
	adNorm := o.AdNorm(id)
	yolNorm := o.YolNorm(id)

	var puan float64
	for _, t := range terimler {
		w := idf[t]
		switch {
		case strings.Contains(adNorm, t):
			puan += w * 3
		case strings.Contains(yolNorm, t):
			puan += w * 2
		default:
			puan += w
		}
	}

	// Ters indekste yer almayan parçalar (durak kelime, uzun sayı) yalnızca
	// alt dize olarak eşleşir; onları da ödüllendiriyoruz.
	for _, p := range parcalar {
		if _, indeksli := idf[p]; indeksli {
			continue
		}
		if strings.Contains(adNorm, p) {
			puan += 1.5
		}
	}

	// Tam sorgu dizesi dosya adında geçiyorsa bu çok güçlü bir sinyaldir.
	if len(parcalar) > 1 {
		tam := strings.Join(parcalar, " ")
		if strings.Contains(adNorm, tam) {
			puan *= 1.5
		}
	}
	return puan
}

func sirala(liste []puanliBelge, s Sirala) {
	switch s {
	case SiralaTarihYeni:
		slices.SortStableFunc(liste, func(a, b puanliBelge) int {
			return -cmpInt64(a.belge.Zaman, b.belge.Zaman)
		})
	case SiralaTarihEski:
		slices.SortStableFunc(liste, func(a, b puanliBelge) int {
			return cmpInt64(a.belge.Zaman, b.belge.Zaman)
		})
	case SiralaAd:
		slices.SortStableFunc(liste, func(a, b puanliBelge) int {
			return strings.Compare(
				strings.ToLower(filepath.Base(a.belge.Yol)),
				strings.ToLower(filepath.Base(b.belge.Yol)))
		})
	case SiralaBoyutBuyuk:
		slices.SortStableFunc(liste, func(a, b puanliBelge) int {
			return -cmpInt64(a.belge.Boyut, b.belge.Boyut)
		})
	default: // SiralaSkor
		slices.SortStableFunc(liste, func(a, b puanliBelge) int {
			if a.puan != b.puan {
				if a.puan > b.puan {
					return -1
				}
				return 1
			}
			// Eşitlik bozucu: yeni dosya önce.
			return -cmpInt64(a.belge.Zaman, b.belge.Zaman)
		})
	}
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
