// Paket sorgu, arama isteklerini ayrıştırır, indeks üzerinde çalıştırır,
// skorlar, sıralar ve sayfalar.
package sorgu

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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

	// IcerikOku tam ifade doğrulamasında dosya metnini getirir; nil ise
	// ifadeler yalnızca ad ve yolda doğrulanır.
	IcerikOku func(ctx context.Context, yol string) (string, bool)
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
	// Alanlar eşleşmenin nerede olduğunu söyler (AlanAd, AlanKlasor, AlanIcerik).
	Alanlar []string
	// Benzerlik puanın tüm sonuçlardaki en yüksek puana oranıdır (0-100);
	// alaka sıralaması dışında ve puansız (yalnızca filtre) aramada 0.
	Benzerlik int
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
	// Oneri sonuç çıkmadığında yazımı düzeltilmiş sorgu; yoksa "".
	Oneri string
	// Uyari kullanıcıya gösterilecek not (ör. doğrulanamayan tam ifade).
	Uyari string
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

// SorguParcalari kullanıcının yazdığı metinden vurgulanacak parçaları çıkarır.
//
// Bu, metin.Belirtecler'den KASITLI olarak farklıdır: burada durak kelimeleri
// ve uzun sayıları atmıyoruz. Kullanıcı "1698240000" veya "the" yazdıysa onu
// dosya adında alt dize olarak aramak istiyor demektir; ters indekste o terim
// bulunmaz ama alt dize taraması bulur. Operatörler (-x, ad:, VEYA) atılır.
func SorguParcalari(s string) []string {
	return Coz(s).Vurgular()
}

// Alan adları: sonuç satırında eşleşmenin nerede olduğunu gösteren rozetler.
const (
	AlanAd     = "ad"
	AlanKlasor = "klasör"
	AlanIcerik = "içerik"
)

// Önek genişletmesinin üst sınırı: "co*" gibi kısa bir önek binlerce terime
// açılıp birleşimi pahalılaştırmasın.
const enFazlaGenisleme = 400

// Tam ifade içerik doğrulaması için bütçe; dosya metni diskten yeniden okunur.
const (
	ifadeDogrulamaSuresi = 2500 * time.Millisecond
	ifadeDogrulamaAdedi  = 300
)

// kelimeKumesi bir kelimenin indeksteki eşleşmeleridir (hepsi artan sıralı).
type kelimeKumesi struct {
	k      Kelime
	hepsi  []uint32 // kapsama göre eşleşen tüm dokümanlar
	icerik []uint32 // içerikte eşleşenler (önek dahil)
	tam    []uint32 // içerikte önek olmadan, tam terimle eşleşenler
	idf    float64
}

func iceriyor(l []uint32, id uint32) bool {
	_, v := slices.BinarySearch(l, id)
	return v
}

// Calistir aramayı yürütür.
func Calistir(ix *indeks.Indeks, is Istek) Cikti {
	basla := time.Now()
	coz := Coz(is.Metin)
	cikti := Cikti{Sayfa: is.Sayfa, Parcalar: coz.Vurgular()}

	// uzanti: operatörü paneldeki uzantı filtresiyle VEYA'lanır.
	etkin := is
	if len(coz.Uzantilar) > 0 {
		etkin.Uzantilar = append(slices.Clone(is.Uzantilar), coz.Uzantilar...)
	}

	var toplandi []puanliBelge
	var gruplar [][]*kelimeKumesi
	var haric []*kelimeKumesi

	ix.Oku(func(o indeks.Okuyucu) {
		n := float64(max(o.CanliSayisi(), 1))
		gruplar, haric = kumeleriKur(o, coz, is.Kapsam)
		for _, g := range gruplar {
			for _, kk := range g {
				kk.idf = math.Log(1 + n/float64(max(len(kk.hepsi), 1)))
			}
		}

		adaylar := adaylariBul(o, gruplar)
		for _, h := range haric {
			if h.k.Ifade == "" {
				adaylar = cikar(adaylar, h.hepsi)
			}
		}
		toplandi = make([]puanliBelge, 0, min(len(adaylar), 4096))

		for _, id := range adaylar {
			if o.SilinmisMi(id) {
				continue
			}
			b := o.Belge(id)
			if !filtreleriGecer(b, etkin) || !klasorlerdeMi(o, id, coz.Klasorler) {
				continue
			}
			pb := puanla(o, id, gruplar)
			pb.belge = b
			toplandi = append(toplandi, pb)
		}

		if len(toplandi) == 0 {
			cikti.Oneri = oneriUret(o, is.Metin, gruplar)
		}
	})

	sirala(toplandi, is.Sirala)
	toplandi, cikti.Uyari = ifadeleriDogrula(toplandi, gruplar, haric, is.IcerikOku)

	cikti.Toplam = len(toplandi)

	// Ham puan sorgudan sorguya ölçeksiz; kullanıcıya en iyi sonuca oranla gösteriyoruz.
	enYuksek := 0.0
	if is.Sirala == SiralaSkor && len(toplandi) > 0 {
		enYuksek = toplandi[0].puan
	}

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
			AdParcalari:      Vurgula(ad, cikti.Parcalar),
			IcerikAranabilir: icerikAranabilirMi(pb.belge.Durum),
			Alanlar:          pb.alanlar,
			Benzerlik:        benzerlik(pb.puan, enYuksek),
		})
	}

	cikti.Sure = time.Since(basla)
	return cikti
}

func benzerlik(puan, enYuksek float64) int {
	if enYuksek <= 0 {
		return 0
	}
	return max(1, int(math.Round(puan/enYuksek*100)))
}

func icerikAranabilirMi(durum string) bool {
	switch durum {
	case cikarim.DurumTam, cikarim.DurumKirpildi:
		return true
	}
	return false
}

type puanliBelge struct {
	id      uint32
	belge   indeks.Belge
	puan    float64
	alanlar []string
}

// ---------------------------------------------------------------- adaylar

// kumeleriKur her kelimenin eşleşme kümesini hesaplar.
//
// Terimi olmayan kelimeler (durak kelime, tek harf, uzun sayı) yalnızca dosya
// adında alt dize olarak aranabilir; sorguda terimli bir kelime de varsa onları
// zorunlu tutmuyoruz, yoksa "rapor ve fatura" adında "ve" geçmeyeni elerdi.
func kumeleriKur(o indeks.Okuyucu, c Cozulmus, kapsam Kapsam) (gruplar [][]*kelimeKumesi, haric []*kelimeKumesi) {
	terimliVar := false
	for _, k := range c.Kelimeler() {
		if len(k.Terimler) > 0 || k.SadeceAd {
			terimliVar = true
			break
		}
	}
	for _, g := range c.Gruplar {
		var kg []*kelimeKumesi
		for _, k := range g {
			if terimliVar && len(k.Terimler) == 0 && !k.SadeceAd {
				continue
			}
			kg = append(kg, kelimeKumesiKur(o, k, kapsam))
		}
		if len(kg) > 0 {
			gruplar = append(gruplar, kg)
		}
	}
	for _, k := range c.Haric {
		haric = append(haric, kelimeKumesiKur(o, k, KapsamHepsi))
	}
	return gruplar, haric
}

func kelimeKumesiKur(o indeks.Okuyucu, k Kelime, kapsam Kapsam) *kelimeKumesi {
	kk := &kelimeKumesi{k: k}
	if k.SadeceAd {
		kk.hepsi = adAdaylari(o, k)
		return kk
	}
	kk.icerik = terimKumesi(o, k.Terimler, k.Onek, o.Gonderiler)
	if k.Onek {
		kk.tam = terimKumesi(o, k.Terimler, false, o.Gonderiler)
	} else {
		kk.tam = kk.icerik
	}
	switch kapsam {
	case KapsamIcerik:
		kk.hepsi = kk.icerik
	case KapsamAd:
		kk.hepsi = birlestir(adAdaylari(o, k), terimKumesi(o, k.Terimler, k.Onek, o.AdGonderiler))
	default:
		kk.hepsi = birlestir(
			birlestir(kk.icerik, terimKumesi(o, k.Terimler, k.Onek, o.AdGonderiler)),
			adAdaylari(o, k))
	}
	return kk
}

// terimKumesi terimlerin hepsini içeren dokümanları döner (VE). onek doğruysa
// her terim sözlükteki o önekle başlayan terimlerin birleşimidir.
func terimKumesi(o indeks.Okuyucu, terimler []string, onek bool, gonderiler func(string) []uint32) []uint32 {
	if len(terimler) == 0 {
		return nil
	}
	listeler := make([][]uint32, 0, len(terimler))
	for _, t := range terimler {
		var g []uint32
		if onek {
			for _, a := range o.OnekliTerimler(t, enFazlaGenisleme) {
				g = birlestir(g, gonderiler(a))
			}
		} else {
			g = gonderiler(t)
		}
		if len(g) == 0 {
			return nil // VE: bir terim hiç yoksa sonuç boş
		}
		listeler = append(listeler, g)
	}
	return kesisim(listeler)
}

// kesisim listelerin kesişimini alır. En küçük listeden başlıyoruz: kesişimin
// boyutu en küçük listeyle sınırlı olduğu için gereksiz karşılaştırma olmaz.
func kesisim(listeler [][]uint32) []uint32 {
	if len(listeler) == 0 {
		return nil
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

// adaylariBul grupları birleştirir: grup içi VEYA, gruplar arası VE.
func adaylariBul(o indeks.Okuyucu, gruplar [][]*kelimeKumesi) []uint32 {
	// Hiç arama kelimesi yok: filtreler tek başına çalışsın, tüm dokümanlar
	// aday olsun. 100.000 doküman üzerinde doğrusal tarama yeterince hızlı.
	if len(gruplar) == 0 {
		hepsi := make([]uint32, o.BelgeSayisi())
		for i := range hepsi {
			hepsi[i] = uint32(i)
		}
		return hepsi
	}
	listeler := make([][]uint32, 0, len(gruplar))
	for _, g := range gruplar {
		var u []uint32
		for _, kk := range g {
			u = birlestir(u, kk.hepsi)
		}
		if len(u) == 0 {
			return nil
		}
		listeler = append(listeler, u)
	}
	return kesisim(listeler)
}

// adAdaylari dosya adında alt dize araması yapar.
//
// Ters indeks alt dize aramaz. n-gram indeksi kurmak yerine önceden
// normalize edilmiş ad önbelleği üzerinde doğrusal strings.Contains
// yapıyoruz: 100.000 dosyada ~5-10 ms, yerini alacağı indeksten daha ucuz.
func adAdaylari(o indeks.Okuyucu, k Kelime) []uint32 {
	var cikti []uint32
	for id := 0; id < o.BelgeSayisi(); id++ {
		kid := uint32(id)
		if o.SilinmisMi(kid) {
			continue
		}
		if adEslesir(o.AdNorm(kid), k) {
			cikti = append(cikti, kid)
		}
	}
	return cikti
}

// adEslesir normalize edilmiş adın ya da yolun kelimeyi içerip içermediğini söyler.
func adEslesir(norm string, k Kelime) bool {
	if k.Ifade != "" {
		return strings.Contains(metin.IfadeBicimi(norm), k.Ifade)
	}
	return strings.Contains(norm, k.Ham)
}

// cikar a'dan b'de olanları atar (ikisi de artan sıralı).
func cikar(a, b []uint32) []uint32 {
	if len(b) == 0 {
		return a
	}
	cikti := make([]uint32, 0, len(a))
	j := 0
	for _, x := range a {
		for j < len(b) && b[j] < x {
			j++
		}
		if j < len(b) && b[j] == x {
			continue
		}
		cikti = append(cikti, x)
	}
	return cikti
}

// klasorlerdeMi klasor: operatörlerini uygular: dosyanın klasör yolu her
// değeri alt dize olarak içermeli.
func klasorlerdeMi(o indeks.Okuyucu, id uint32, klasorler []string) bool {
	if len(klasorler) == 0 {
		return true
	}
	yol, ad := o.YolNorm(id), o.AdNorm(id)
	klasor := yol[:len(yol)-len(ad)]
	for _, k := range klasorler {
		if !strings.Contains(klasor, k) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- tam ifade

// ifadeleriDogrula tırnaklı ifadeleri, kelimeleri yan yana geçen dokümanlara indirir.
//
// İndeks konum tutmadığı için (bkz. REHBER §12) ifade ancak metin yeniden
// okunarak doğrulanabilir. Alaka sırasıyla ve süre/adet bütçesiyle yapıyoruz;
// bütçe biterse kalanlar elenmez, kullanıcıya uyarı gösterilir.
func ifadeleriDogrula(
	liste []puanliBelge,
	gruplar [][]*kelimeKumesi,
	haric []*kelimeKumesi,
	oku func(context.Context, string) (string, bool),
) ([]puanliBelge, string) {
	ifadeVar := false
	for _, g := range gruplar {
		for _, kk := range g {
			ifadeVar = ifadeVar || kk.k.Ifade != ""
		}
	}
	for _, h := range haric {
		ifadeVar = ifadeVar || h.k.Ifade != ""
	}
	if !ifadeVar {
		return liste, ""
	}

	ctx, iptal := context.WithTimeout(context.Background(), ifadeDogrulamaSuresi)
	defer iptal()
	okunan, dogrulanamayan := 0, 0

	cikti := liste[:0]
	for _, pb := range liste {
		yolBicimi := metin.IfadeBicimi(pb.belge.Yol)
		var icerik *string
		butceBitti := false
		// icerikte metni en fazla bir kez okur; bütçe bittiyse "geçer" sayar.
		icerikte := func(ifade string) bool {
			if icerik == nil {
				s := ""
				switch {
				case oku == nil || !icerikAranabilirMi(pb.belge.Durum):
				case ctx.Err() != nil || okunan >= ifadeDogrulamaAdedi:
					butceBitti = true
				default:
					okunan++
					if m, tamam := oku(ctx, pb.belge.Yol); tamam {
						s = metin.IfadeBicimi(m)
					} else if ctx.Err() != nil {
						butceBitti = true
					}
				}
				icerik = &s
			}
			return butceBitti || strings.Contains(*icerik, ifade)
		}
		gecer := func(kk *kelimeKumesi) bool {
			if !iceriyor(kk.hepsi, pb.id) {
				return false
			}
			if kk.k.Ifade == "" || strings.Contains(yolBicimi, kk.k.Ifade) {
				return true
			}
			return !kk.k.SadeceAd && icerikte(kk.k.Ifade)
		}

		tut := true
		for _, g := range gruplar {
			if !slices.ContainsFunc(g, gecer) {
				tut = false
				break
			}
		}
		for _, h := range haric {
			if tut && h.k.Ifade != "" && gecer(h) && !butceBitti {
				tut = false
			}
		}
		if butceBitti {
			dogrulanamayan++
		}
		if tut {
			cikti = append(cikti, pb)
		}
	}

	if dogrulanamayan > 0 {
		return cikti, fmt.Sprintf(
			"Tam ifade %d dosyada süre sınırı yüzünden doğrulanamadı; bu dosyalar sonuçlarda bırakıldı.",
			dogrulanamayan)
	}
	return cikti, ""
}

// ---------------------------------------------------------------- öneri

// oneriUret sonuç çıkmayan sorgu için yazım düzeltmesi önerir: bulunamayan
// her kelimeyi sözlükte düzenleme mesafesi en küçük (eşitse en yaygın)
// terimle değiştirir. Hiçbir kelime düzeltilemezse "" döner.
func oneriUret(o indeks.Okuyucu, metinHam string, gruplar [][]*kelimeKumesi) string {
	oneri := metinHam
	degisti := false
	for _, g := range gruplar {
		for _, kk := range g {
			k := kk.k
			if len(kk.hepsi) > 0 || k.Ifade != "" || len(k.Terimler) != 1 {
				continue
			}
			hedef := k.Terimler[0]
			if len(hedef) < 3 {
				continue
			}
			esik := 2
			if len(hedef) <= 4 {
				esik = 1
			}
			enIyi, enIyiMesafe, enIyiDf := "", esik+1, 0
			o.TerimleriGez(func(t string) bool {
				if fark := len(t) - len(hedef); fark > esik || -fark > esik {
					return true
				}
				m := duzenlemeMesafesi(hedef, t, esik)
				if m > esik {
					return true
				}
				df := o.ToplamDf(t)
				if m < enIyiMesafe || (m == enIyiMesafe && df > enIyiDf) {
					enIyi, enIyiMesafe, enIyiDf = t, m, df
				}
				return true
			})
			if enIyi != "" {
				oneri = strings.Replace(oneri, k.Kaynak, enIyi, 1)
				degisti = true
			}
		}
	}
	if !degisti {
		return ""
	}
	return oneri
}

// duzenlemeMesafesi Levenshtein mesafesini hesaplar; esik aşılınca erken çıkar.
func duzenlemeMesafesi(a, b string, esik int) int {
	ar, br := []rune(a), []rune(b)
	onceki := make([]int, len(br)+1)
	simdi := make([]int, len(br)+1)
	for j := range onceki {
		onceki[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		simdi[0] = i
		satirMin := simdi[0]
		for j := 1; j <= len(br); j++ {
			maliyet := 1
			if ar[i-1] == br[j-1] {
				maliyet = 0
			}
			simdi[j] = min(onceki[j]+1, simdi[j-1]+1, onceki[j-1]+maliyet)
			satirMin = min(satirMin, simdi[j])
		}
		if satirMin > esik {
			return esik + 1
		}
		onceki, simdi = simdi, onceki
	}
	return onceki[len(br)]
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

// puanla alaka puanını hesaplar ve eşleşmenin hangi alanlarda olduğunu bulur.
//
// Skor = Σ idf(kelime) × alan çarpanı. Konum (position) ve terim frekansı
// TUTMUYORUZ; ad ağırlıklı bir arşivde bu basit formül BM25'i döverken
// indeksi ~80 MB küçük tutuyor.
//
// Alan çarpanları: ad eşleşmesi ×3, klasör/yol ×2, içerik ×1 (yalnızca önekle
// eşleşen içerik ×0.7). Arşivde dosya adı, içerikten çok daha güçlü bir sinyaldir.
func puanla(o indeks.Okuyucu, id uint32, gruplar [][]*kelimeKumesi) puanliBelge {
	adNorm := o.AdNorm(id)
	yolNorm := o.YolNorm(id)
	pb := puanliBelge{id: id}

	var adda, klasorde, icerikte bool
	var hamlar []string
	for _, g := range gruplar {
		for _, kk := range g {
			if !iceriyor(kk.hepsi, id) {
				continue
			}
			k := kk.k
			hamlar = append(hamlar, k.Ham)
			w := kk.idf
			switch {
			case adEslesir(adNorm, k):
				pb.puan += w * 3 * sinirCarpani(adNorm, k)
				adda = true
			case !k.SadeceAd && adEslesir(yolNorm, k):
				pb.puan += w * 2 * sinirCarpani(yolNorm, k)
				klasorde = true
			default:
				if k.Onek && !iceriyor(kk.tam, id) {
					w *= onekCarpani
				}
				pb.puan += w
			}
			if iceriyor(kk.icerik, id) {
				icerikte = true
			}
		}
	}

	// Tam sorgu dizesi dosya adında geçiyorsa bu çok güçlü bir sinyaldir.
	if len(hamlar) > 1 && strings.Contains(adNorm, strings.Join(hamlar, " ")) {
		pb.puan *= 1.5
	}
	for _, g := range gurultuKlasorleri {
		if strings.Contains(yolNorm, g) {
			pb.puan *= 0.5
			break
		}
	}

	if adda {
		pb.alanlar = append(pb.alanlar, AlanAd)
	}
	if klasorde {
		pb.alanlar = append(pb.alanlar, AlanKlasor)
	}
	if icerikte {
		pb.alanlar = append(pb.alanlar, AlanIcerik)
	}
	return pb
}

// Önek ve alt dize eşleşmeleri tam kelimeden zayıftır: "proje" araması
// "proje.docx"i "projects.html"in, "rapor" araması "rapor.md"yi
// "deneme-raporu.md"nin önüne koymalı.
const (
	onekCarpani    = 0.7 // kelimenin başında: rapor → raporu
	altDizeCarpani = 0.5 // kelimenin içinde: apor → rapor
)

// sinirCarpani kelimenin normalize metinde tam kelime olarak mı, bir
// kelimenin başı olarak mı yoksa içinde mi geçtiğine göre çarpan döner.
func sinirCarpani(norm string, k Kelime) float64 {
	hedef := k.Ham
	if k.Ifade != "" {
		norm, hedef = metin.IfadeBicimi(norm), k.Ifade
	}
	enIyi := 0.0
	for bas := 0; bas < len(norm); {
		i := strings.Index(norm[bas:], hedef)
		if i < 0 {
			break
		}
		i += bas
		son := i + len(hedef)
		solda := i == 0 || !harfRakamMi(norm[:i], true)
		sagda := son == len(norm) || !harfRakamMi(norm[son:], false)
		switch {
		case solda && sagda:
			return 1
		case solda:
			enIyi = max(enIyi, onekCarpani)
		default:
			enIyi = max(enIyi, altDizeCarpani)
		}
		bas = i + 1
	}
	return enIyi
}

// harfRakamMi s'nin son (sondan=true) ya da ilk runesinin harf/rakam olduğunu söyler.
func harfRakamMi(s string, sondan bool) bool {
	var r rune
	if sondan {
		r, _ = utf8.DecodeLastRuneInString(s)
	} else {
		r, _ = utf8.DecodeRuneInString(s)
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// gurultuKlasorleri üçüncü taraf belge ve paket klasörleridir; çoğu zaman
// aranan şey değildir ama sayıca ilk sayfayı doldururlar. Elenmez, geri itilir.
var gurultuKlasorleri = []string{
	`\docs\api\`, `\apidocs\`, `\javadoc\`, `\vendor\`,
	`\third_party\`, `\thirdparty\`, `\site-packages\`, `\packages\`,
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
