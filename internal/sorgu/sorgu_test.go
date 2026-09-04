package sorgu

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"arsiv-indeks/internal/indeks"
)

func indeksKur(girdiler ...indeks.Girdi) *indeks.Indeks {
	ix := indeks.Yeni()
	ix.EkleToplu(girdiler)
	return ix
}

func girdi(yol string, gun int, belirtecler ...string) indeks.Girdi {
	return indeks.Girdi{
		Yol:         yol,
		Boyut:       1000,
		Zaman:       time.Date(2023, 1, gun, 12, 0, 0, 0, time.UTC).UnixNano(),
		Durum:       "tam",
		Belirtecler: belirtecler,
	}
}

func yollar(c Cikti) []string {
	var cikti []string
	for _, s := range c.Sonuclar {
		cikti = append(cikti, s.Ad)
	}
	return cikti
}

func icerirYol(c Cikti, ad string) bool {
	for _, s := range c.Sonuclar {
		if s.Ad == ad {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- kapsam

// Kullanıcı "sadece içerik" dediğinde dosya adı eşleşmesi GELMEMELİ.
// Bu, isteğin ayrı ayrı listelediği "adına" ve "içindeki veriye" göre arama
// ayrımının test edildiği yer.
func TestKapsamIcerikAdEslesmesiGetirmez(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\A\beton.txt`, 1, "cimento"), // "beton" yalnızca ADDA
		girdi(`C:\A\rapor.txt`, 2, "beton"),   // "beton" yalnızca İÇERİKTE
	)

	c := Calistir(ix, Istek{Metin: "beton", Kapsam: KapsamIcerik, Sayfa: 1, Adet: 25})
	if !icerirYol(c, "rapor.txt") {
		t.Error("içerikte geçen dosya bulunamadı")
	}
	if icerirYol(c, "beton.txt") {
		t.Error("yalnızca adında geçen dosya içerik aramasında geldi")
	}
}

func TestKapsamAdIcerikEslesmesiGetirmez(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\A\beton.txt`, 1, "cimento"),
		girdi(`C:\A\rapor.txt`, 2, "beton"),
	)

	c := Calistir(ix, Istek{Metin: "beton", Kapsam: KapsamAd, Sayfa: 1, Adet: 25})
	if !icerirYol(c, "beton.txt") {
		t.Error("adında geçen dosya bulunamadı")
	}
	if icerirYol(c, "rapor.txt") {
		t.Error("yalnızca içeriğinde geçen dosya ad aramasında geldi")
	}
}

func TestKapsamHepsiIkisiniDeGetirir(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\A\beton.txt`, 1, "cimento"),
		girdi(`C:\A\rapor.txt`, 2, "beton"),
	)
	c := Calistir(ix, Istek{Metin: "beton", Kapsam: KapsamHepsi, Sayfa: 1, Adet: 25})
	if c.Toplam != 2 {
		t.Errorf("toplam = %d, beklenen 2. sonuçlar: %v", c.Toplam, yollar(c))
	}
}

// Ters indeks alt dize aramaz; dosya adının bir PARÇASINI yazmak da
// çalışmalı ("apor" -> "rapor.txt").
func TestAdAltDizeAramasi(t *testing.T) {
	ix := indeksKur(girdi(`C:\A\rapor2023.txt`, 1, "icerik"))
	c := Calistir(ix, Istek{Metin: "apor", Kapsam: KapsamAd, Sayfa: 1, Adet: 25})
	if c.Toplam != 1 {
		t.Errorf("alt dize aramasında sonuç yok: %d", c.Toplam)
	}
}

// Türkçe katlama: kullanıcı hangi harfi yazdığını hatırlamak zorunda kalmasın.
func TestTurkceKatlamaAramada(t *testing.T) {
	ix := indeksKur(girdi(`C:\A\Şantiye Raporu.txt`, 1, "beton"))

	for _, sorguMetni := range []string{"santiye", "Şantiye", "ŞANTIYE", "santıye"} {
		c := Calistir(ix, Istek{Metin: sorguMetni, Kapsam: KapsamAd, Sayfa: 1, Adet: 25})
		if c.Toplam != 1 {
			t.Errorf("%q ile arama sonuç vermedi", sorguMetni)
		}
	}
}

// ---------------------------------------------------------------- AND

func TestCokTerimliAramaANDdir(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\A\bir.txt`, 1, "beton", "cimento"),
		girdi(`C:\A\iki.txt`, 2, "beton"),
	)
	c := Calistir(ix, Istek{Metin: "beton cimento", Kapsam: KapsamIcerik, Sayfa: 1, Adet: 25})
	if c.Toplam != 1 || !icerirYol(c, "bir.txt") {
		t.Errorf("AND çalışmadı: %v", yollar(c))
	}
}

func TestBulunmayanTerimBosSonuc(t *testing.T) {
	ix := indeksKur(girdi(`C:\A\bir.txt`, 1, "beton"))
	c := Calistir(ix, Istek{Metin: "kesinlikleyokboyle", Sayfa: 1, Adet: 25})
	if c.Toplam != 0 {
		t.Errorf("toplam = %d, beklenen 0", c.Toplam)
	}
}

// ---------------------------------------------------------------- filtreler

func TestUzantiFiltresi(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\A\bir.pdf`, 1, "beton"),
		girdi(`C:\A\iki.docx`, 2, "beton"),
		girdi(`C:\A\uc.txt`, 3, "beton"),
	)
	c := Calistir(ix, Istek{
		Metin:     "beton",
		Kapsam:    KapsamIcerik,
		Uzantilar: []string{".pdf", ".docx"},
		Sayfa:     1, Adet: 25,
	})
	if c.Toplam != 2 {
		t.Fatalf("toplam = %d, beklenen 2: %v", c.Toplam, yollar(c))
	}
	if icerirYol(c, "uc.txt") {
		t.Error("filtre dışı uzantı geldi")
	}
}

func TestKlasorFiltresiAyiriciSinirinaSaygiliDir(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\Arsiv\a.txt`, 1, "beton"),
		girdi(`C:\Arsiv-gizli\b.txt`, 2, "beton"),
	)
	c := Calistir(ix, Istek{
		Metin: "beton", Kapsam: KapsamIcerik,
		Klasor: `C:\Arsiv`,
		Sayfa:  1, Adet: 25,
	})
	if c.Toplam != 1 || !icerirYol(c, "a.txt") {
		t.Errorf("klasör filtresi sınır tanımadı: %v", yollar(c))
	}
}

func TestTarihAraligiFiltresi(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\A\ocak5.txt`, 5, "beton"),
		girdi(`C:\A\ocak15.txt`, 15, "beton"),
		girdi(`C:\A\ocak25.txt`, 25, "beton"),
	)
	bas := time.Date(2023, 1, 10, 0, 0, 0, 0, time.UTC)
	son := time.Date(2023, 1, 20, 23, 59, 59, 0, time.UTC)

	c := Calistir(ix, Istek{
		Metin: "beton", Kapsam: KapsamIcerik,
		Baslangic: bas, Bitis: son,
		Sayfa: 1, Adet: 25,
	})
	if c.Toplam != 1 || !icerirYol(c, "ocak15.txt") {
		t.Errorf("tarih filtresi: %v", yollar(c))
	}
}

// Metin olmadan yalnızca filtreyle arama da çalışmalı.
func TestSadeceFiltreIleArama(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\A\bir.pdf`, 1),
		girdi(`C:\A\iki.txt`, 2),
	)
	c := Calistir(ix, Istek{Uzantilar: []string{".pdf"}, Sayfa: 1, Adet: 25})
	if c.Toplam != 1 || !icerirYol(c, "bir.pdf") {
		t.Errorf("sadece filtreyle arama: %v", yollar(c))
	}
}

// ---------------------------------------------------------------- sıralama

// Ad eşleşmesi içerik eşleşmesinden güçlü sinyaldir (×3 çarpan).
func TestAdEslesmesiDahaYuksekPuanAlir(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\A\alakasiz.txt`, 2, "beton"),
		girdi(`C:\A\beton.txt`, 1, "alakasiz"),
	)
	c := Calistir(ix, Istek{Metin: "beton", Kapsam: KapsamHepsi, Sirala: SiralaSkor, Sayfa: 1, Adet: 25})
	if len(c.Sonuclar) != 2 {
		t.Fatalf("sonuç sayısı = %d", len(c.Sonuclar))
	}
	if c.Sonuclar[0].Ad != "beton.txt" {
		t.Errorf("ilk sonuç = %q, ad eşleşmesi önce gelmeliydi", c.Sonuclar[0].Ad)
	}
}

func TestTariheGoreSiralama(t *testing.T) {
	ix := indeksKur(
		girdi(`C:\A\eski.txt`, 1, "beton"),
		girdi(`C:\A\yeni.txt`, 20, "beton"),
	)
	c := Calistir(ix, Istek{Metin: "beton", Kapsam: KapsamIcerik, Sirala: SiralaTarihYeni, Sayfa: 1, Adet: 25})
	if c.Sonuclar[0].Ad != "yeni.txt" {
		t.Errorf("yeni→eski sıralama bozuk: %v", yollar(c))
	}

	c = Calistir(ix, Istek{Metin: "beton", Kapsam: KapsamIcerik, Sirala: SiralaTarihEski, Sayfa: 1, Adet: 25})
	if c.Sonuclar[0].Ad != "eski.txt" {
		t.Errorf("eski→yeni sıralama bozuk: %v", yollar(c))
	}
}

// ---------------------------------------------------------------- sayfalama

func TestSayfalama(t *testing.T) {
	var girdiler []indeks.Girdi
	for i := 1; i <= 12; i++ {
		girdiler = append(girdiler, girdi(`C:\A\dosya`+string(rune('a'+i))+`.txt`, i, "beton"))
	}
	ix := indeksKur(girdiler...)

	c := Calistir(ix, Istek{Metin: "beton", Kapsam: KapsamIcerik, Sayfa: 1, Adet: 5})
	if len(c.Sonuclar) != 5 || c.Toplam != 12 || c.SayfaSayisi != 3 {
		t.Errorf("sayfa 1: len=%d toplam=%d sayfaSayisi=%d",
			len(c.Sonuclar), c.Toplam, c.SayfaSayisi)
	}

	c = Calistir(ix, Istek{Metin: "beton", Kapsam: KapsamIcerik, Sayfa: 3, Adet: 5})
	if len(c.Sonuclar) != 2 {
		t.Errorf("son sayfa uzunluğu = %d, beklenen 2", len(c.Sonuclar))
	}

	// Aralık dışı sayfa çökmemeli.
	c = Calistir(ix, Istek{Metin: "beton", Kapsam: KapsamIcerik, Sayfa: 99, Adet: 5})
	if len(c.Sonuclar) != 0 {
		t.Errorf("aralık dışı sayfa = %d sonuç", len(c.Sonuclar))
	}
}

// ---------------------------------------------------------------- ayrıştırma

func TestAyristir(t *testing.T) {
	q, _ := url.ParseQuery(
		"q=beton+cimento&kapsam=icerik&uzanti=.pdf&uzanti=.DOCX" +
			"&baslangic=2023-01-01&bitis=2023-12-31&klasor=C%3A%5CArsiv" +
			"&sirala=tarih&sayfa=3")
	is := Ayristir(q, 25)

	if is.Metin != "beton cimento" {
		t.Errorf("metin = %q", is.Metin)
	}
	if is.Kapsam != KapsamIcerik {
		t.Errorf("kapsam = %q", is.Kapsam)
	}
	if len(is.Uzantilar) != 2 || is.Uzantilar[1] != ".docx" {
		t.Errorf("uzantılar = %v (küçük harfe çevrilmeli)", is.Uzantilar)
	}
	if is.Klasor != `C:\Arsiv` {
		t.Errorf("klasör = %q", is.Klasor)
	}
	if is.Sirala != SiralaTarihYeni {
		t.Errorf("sırala = %q", is.Sirala)
	}
	if is.Sayfa != 3 {
		t.Errorf("sayfa = %d", is.Sayfa)
	}
	// Bitiş tarihi günün SONUNU kapsamalı, aksi halde o güne ait dosyalar
	// filtrenin dışında kalır.
	if is.Bitis.Hour() != 23 {
		t.Errorf("bitiş tarihi gün sonuna çekilmemiş: %v", is.Bitis)
	}
}

func TestAyristirBozukGirdiVarsayilanaDuser(t *testing.T) {
	q, _ := url.ParseQuery("kapsam=uydurma&sirala=uydurma&sayfa=abc&baslangic=bozuk")
	is := Ayristir(q, 25)

	if is.Kapsam != KapsamHepsi {
		t.Errorf("kapsam = %q, beklenen hepsi", is.Kapsam)
	}
	if is.Sirala != SiralaSkor {
		t.Errorf("sırala = %q, beklenen skor", is.Sirala)
	}
	if is.Sayfa != 1 {
		t.Errorf("sayfa = %d, beklenen 1", is.Sayfa)
	}
	if !is.Baslangic.IsZero() {
		t.Errorf("bozuk tarih sıfır olmalıydı: %v", is.Baslangic)
	}
}

func TestAyristirDerinSayfaSinirlanir(t *testing.T) {
	q, _ := url.ParseQuery("sayfa=999999")
	if is := Ayristir(q, 25); is.Sayfa != EnFazlaSayfa {
		t.Errorf("sayfa = %d, beklenen %d", is.Sayfa, EnFazlaSayfa)
	}
}

// SorguParcalari, ters indekste bulunmayacak parçaları da korumalı:
// kullanıcı uzun bir numara veya durak kelime yazdıysa onu dosya adında
// alt dize olarak aramak ister.
func TestSorguParcalariDurakKelimeyiAtmaz(t *testing.T) {
	p := SorguParcalari("the 1698240000")
	if len(p) != 2 {
		t.Fatalf("parçalar = %v, beklenen 2 parça", p)
	}
}

func TestBosMu(t *testing.T) {
	if !(Istek{}).BosMu() {
		t.Error("boş istek boş sayılmalı")
	}
	if (Istek{Metin: "x"}).BosMu() {
		t.Error("metinli istek boş sayılmamalı")
	}
	if (Istek{Uzantilar: []string{".pdf"}}).BosMu() {
		t.Error("filtreli istek boş sayılmamalı")
	}
}

// ---------------------------------------------------------------- vurgulama

func TestVurgula(t *testing.T) {
	p := Vurgula("Rapor 2023.txt", []string{"rapor"})
	if len(p) == 0 || !p[0].Vurgulu || p[0].Metin != "Rapor" {
		t.Fatalf("parçalar = %+v", p)
	}
	// Vurgulanan metin ORİJİNAL yazımı korumalı, normalize edilmiş hali değil.
	if p[0].Metin != "Rapor" {
		t.Errorf("orijinal yazım korunmadı: %q", p[0].Metin)
	}
}

// Türkçe karakterlerde bayt uzunluğu değişir ('ş' 2 bayt, 's' 1 bayt).
// Bayt indeksiyle çalışsaydık vurgu yanlış yere kayardı.
func TestVurgulaTurkceRuneHizalamasi(t *testing.T) {
	testler := []struct {
		asil, aranan, beklenenVurgu string
	}{
		{"Şantiye Raporu.txt", "santiye", "Şantiye"},
		{"Şantiye Raporu.txt", "raporu", "Raporu"},
		{"İstanbul Şubesi.docx", "istanbul", "İstanbul"},
		{"ÖĞRENCİ listesi.txt", "ogrenci", "ÖĞRENCİ"},
	}
	for _, tt := range testler {
		p := Vurgula(tt.asil, []string{tt.aranan})
		var vurgulu string
		for _, k := range p {
			if k.Vurgulu {
				vurgulu += k.Metin
			}
		}
		if vurgulu != tt.beklenenVurgu {
			t.Errorf("Vurgula(%q, %q) vurgusu = %q, beklenen %q",
				tt.asil, tt.aranan, vurgulu, tt.beklenenVurgu)
		}
		// Parçaların birleşimi orijinali tam olarak vermeli — hiçbir
		// karakter kaybolmamalı veya çoğalmamalı.
		var hepsi string
		for _, k := range p {
			hepsi += k.Metin
		}
		if hepsi != tt.asil {
			t.Errorf("parçalar birleşimi = %q, beklenen %q", hepsi, tt.asil)
		}
	}
}

func TestVurgulaCokluParca(t *testing.T) {
	p := Vurgula("beton ve cimento raporu", []string{"beton", "cimento"})
	sayac := 0
	for _, k := range p {
		if k.Vurgulu {
			sayac++
		}
	}
	if sayac != 2 {
		t.Errorf("vurgulu parça sayısı = %d, beklenen 2. parçalar: %+v", sayac, p)
	}
}

func TestVurgulaEslesmeYoksaTekParca(t *testing.T) {
	p := Vurgula("rapor.txt", []string{"kesinlikleyok"})
	if len(p) != 1 || p[0].Vurgulu {
		t.Errorf("parçalar = %+v, beklenen tek vurgusuz parça", p)
	}
}

// ---------------------------------------------------------------- parçacık

func TestParcacikEslesmeCevresindenAlinti(t *testing.T) {
	icerik := strings.Repeat("dolgu metni ", 50) +
		"burada BETON dökümü var " + strings.Repeat("daha dolgu ", 50)

	p := Parcacik(icerik, []string{"beton"}, 40)

	var hepsi, vurgulu string
	for _, k := range p {
		hepsi += k.Metin
		if k.Vurgulu {
			vurgulu += k.Metin
		}
	}
	if vurgulu != "BETON" {
		t.Errorf("vurgulanan = %q, beklenen BETON", vurgulu)
	}
	if !strings.Contains(hepsi, "…") {
		t.Errorf("kırpma göstergesi yok: %q", hepsi)
	}
	// Tüm içerik değil, yalnızca çevresi dönmeli.
	if len(hepsi) > 300 {
		t.Errorf("parçacık çok uzun (%d karakter)", len(hepsi))
	}
}

// Parçacık tek satıra indirilmeli; sonuç listesinde satır kaymasın.
func TestParcacikSatirlariTekSatiraIndirir(t *testing.T) {
	p := Parcacik("birinci satir\nikinci satirda beton var\nucuncu", []string{"beton"}, 60)
	var hepsi string
	for _, k := range p {
		hepsi += k.Metin
	}
	if strings.ContainsAny(hepsi, "\n\r\t") {
		t.Errorf("parçacıkta satır sonu kaldı: %q", hepsi)
	}
}

// Eşleşme içerikte değilse (ad veya yolda eşleşmişse) baştan alıntı gösterilir.
func TestParcacikEslesmeYoksaBastanAlinti(t *testing.T) {
	p := Parcacik("bu dosyanin icerigi tamamen alakasiz", []string{"beton"}, 20)
	if len(p) == 0 {
		t.Fatal("parçacık dönmedi")
	}
	var hepsi string
	for _, k := range p {
		hepsi += k.Metin
	}
	if !strings.HasPrefix(hepsi, "bu dosyanin") {
		t.Errorf("baştan alıntı beklenirdi: %q", hepsi)
	}
}

func TestParcacikBosIcerik(t *testing.T) {
	if p := Parcacik("", []string{"beton"}, 40); p != nil {
		t.Errorf("boş içerik için nil beklenirdi: %+v", p)
	}
}
