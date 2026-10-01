package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"arsiv-indeks/internal/indeks"
	"arsiv-indeks/internal/sorgu"
)

// ---------------------------------------------------------------- yardımcı

// kurulum bir uygulama örneği ve içinde örnek dosyalar olan bir arşiv dizini
// oluşturur.
func kurulum(t *testing.T) (*Uygulama, string) {
	t.Helper()

	veriDizini := t.TempDir()
	arsiv := t.TempDir()
	ornekArsivKur(t, arsiv)

	u, err := yeniUygulama(veriDizini, false)
	if err != nil {
		t.Fatalf("uygulama kurulamadı: %v", err)
	}
	return u, arsiv
}

// ornekArsivKur planda listelenen doğrulama senaryolarını karşılayan bir
// dosya ağacı üretir.
func ornekArsivKur(t *testing.T, kok string) {
	t.Helper()

	yaz := func(göreli string, icerik []byte) {
		yol := filepath.Join(kok, göreli)
		if err := os.MkdirAll(filepath.Dir(yol), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yol, icerik, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Türkçe adlı düz metin
	yaz(`2023/Şantiye Raporu.txt`,
		[]byte("Beton dökümü tamamlandı.\nProje kodu ENK-4471.\n"))
	// cp1254 kodlu eski metin dosyası: "Ğüşİı"
	yaz(`2023/eski kodlama.txt`, []byte{0xD0, 0xFC, 0xFE, 0xDD, 0xFD})
	// Farklı klasör ve uzantı
	yaz(`2024/teklif.md`, []byte("# Teklif\nCimento fiyati 1250 TL.\n"))
	// İçinde HTML/script geçen dosya: XSS testinin kaynağı
	yaz(`2024/zararli.txt`,
		[]byte("burada <script>alert('xss')</script> geciyor ve beton kelimesi var"))
	// .txt adı verilmiş ikili dosya
	yaz(`2024/sahte.txt`, []byte{0x00, 0x01, 0x02, 0xFF, 0x00, 0x7F, 0x00})
	// Hariç tutulan uzantı
	yaz(`2024/arsiv.zip`, []byte("PK\x03\x04 sahte"))
	// Eski Office biçimi
	yaz(`2022/eski.doc`, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	// Metin katmanı olmayan PDF
	yaz(`2022/taranmis.pdf`, []byte("%PDF-1.7\n%%EOF\n"))

	// Gerçek bir docx
	yaz(`2023/Sozlesme.docx`, docxKur(t, "Sozlesme metni: taseron ile anlasma"))
	// Gerçek bir xlsx (paylaşılan dize havuzu ile)
	yaz(`2023/Maliyet.xlsx`, xlsxKur(t))

	// Uzun yol: toplam yol 260 karakteri aşsın.
	//
	// Tek bir bileşen 255 karakteri geçemez (NTFS sınırı), o yüzden iç içe
	// klasörlerle uzatıyoruz. Go'nun os katmanı \?\ önekini yalnızca
	// mutlak+temiz yollara uyguladığı için bu test tarayıcının kökü
	// filepath.Abs ile mutlaklaştırdığını da doğruluyor.
	derinParcalar := []string{"derin"}
	for i := 0; i < 6; i++ {
		derinParcalar = append(derinParcalar, strings.Repeat("uzunklasoradi", 4))
	}
	derinParcalar = append(derinParcalar, "derindeki.txt")
	yaz(filepath.Join(derinParcalar...), []byte("derin dosyada beton var"))
}

func docxKur(t *testing.T, metin string) []byte {
	t.Helper()
	return zipBayt(t, map[string]string{
		"word/document.xml": `<w:document xmlns:w="http://x"><w:body>` +
			`<w:p><w:r><w:t>` + metin + `</w:t></w:r></w:p></w:body></w:document>`,
	})
}

func xlsxKur(t *testing.T) []byte {
	t.Helper()
	return zipBayt(t, map[string]string{
		"xl/sharedStrings.xml": `<sst xmlns="http://z">` +
			`<si><t>Malzeme</t></si><si><t>Cimento</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://z"><sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>0</v></c></row>` +
			`<row r="2"><c r="A2" t="s"><v>1</v></c><c r="B2"><v>1250</v></c></row>` +
			`</sheetData></worksheet>`,
	})
}

func zipBayt(t *testing.T, girdiler map[string]string) []byte {
	t.Helper()
	var tampon bytes.Buffer
	zw := zip.NewWriter(&tampon)
	for ad, icerik := range girdiler {
		w, err := zw.Create(ad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(icerik)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return tampon.Bytes()
}

// istek yerel kontrolden geçecek bir istek üretir.
func istek(metot, adres string, form url.Values) *http.Request {
	var r *http.Request
	if form == nil {
		r = httptest.NewRequest(metot, adres, nil)
	} else {
		r = httptest.NewRequest(metot, adres, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	// yerelKontrol middleware'i Host'un yerel olmasını şart koşuyor.
	r.Host = "127.0.0.1:8080"
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	return r
}

func cagir(u *Uygulama, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	u.rotalar().ServeHTTP(w, r)
	return w
}

// ---------------------------------------------------------------- rotalar

func TestAnaSayfaAramayaYonlendirir(t *testing.T) {
	u, _ := kurulum(t)
	w := cagir(u, istek("GET", "/", nil))
	if w.Code != http.StatusFound {
		t.Fatalf("kod = %d, beklenen 302", w.Code)
	}
	if k := w.Header().Get("Location"); k != "/arama" {
		t.Errorf("Location = %q, beklenen /arama", k)
	}
}

func TestMetotKontrolu(t *testing.T) {
	u, _ := kurulum(t)
	testler := []struct{ metot, adres string }{
		{"POST", "/arama"},
		{"GET", "/tarama/basla"},
		{"GET", "/dosya/klasorde-goster"},
		{"POST", "/tarama/durum"},
	}
	for _, tt := range testler {
		w := cagir(u, istek(tt.metot, tt.adres, nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s -> %d, beklenen 405", tt.metot, tt.adres, w.Code)
		}
	}
}

// ---------------------------------------------------------------- güvenlik

func TestYerelKontrolCaprazKaynakReddeder(t *testing.T) {
	u, _ := kurulum(t)

	r := httptest.NewRequest("GET", "/arama", nil)
	r.Host = "127.0.0.1:8080"
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if w := cagir(u, r); w.Code != http.StatusForbidden {
		t.Errorf("çapraz kaynaklı istek kodu = %d, beklenen 403", w.Code)
	}
}

// Host kontrolü DNS rebinding saldırısını kapatır: saldırganın alanı
// 127.0.0.1'e çözülse bile Host başlığı onun alanı olur.
func TestYerelKontrolYabanciHostReddeder(t *testing.T) {
	u, _ := kurulum(t)

	r := httptest.NewRequest("GET", "/arama", nil)
	r.Host = "kotu.example.com"
	if w := cagir(u, r); w.Code != http.StatusForbidden {
		t.Errorf("yabancı host kodu = %d, beklenen 403", w.Code)
	}
}

func TestGuvenliYol(t *testing.T) {
	kok := t.TempDir()
	icDosya := filepath.Join(kok, "ic.txt")
	if err := os.WriteFile(icDosya, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Kök ile aynı öneki paylaşan ama ALTINDA olmayan bir dizin.
	komsu := kok + "-gizli"
	if err := os.MkdirAll(komsu, 0o755); err != nil {
		t.Fatal(err)
	}
	komsuDosya := filepath.Join(komsu, "sizinti.txt")
	if err := os.WriteFile(komsuDosya, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	kokler := []string{kok}

	testler := []struct {
		ad      string
		istenen string
		gecerli bool
	}{
		{"kök altındaki dosya", icDosya, true},
		{"harf duyarsız kök", strings.ToUpper(icDosya), true},
		{"kök dışı sistem dosyası", `C:\Windows\win.ini`, false},
		{"ayırıcı sınırı olmayan komşu dizin", komsuDosya, false},
		{"göreli yol", "ic.txt", false},
		{"üst dizine kaçış", filepath.Join(kok, "..", "kacis.txt"), false},
		{"boş yol", "", false},
		{"NUL baytı", icDosya + "\x00.png", false},
		{"UNC yolu", `\\sunucu\pay\dosya.txt`, false},
		{"alternatif veri akışı", icDosya + ":gizliakis", false},
	}

	for _, tt := range testler {
		_, err := GuvenliYol(tt.istenen, kokler)
		if tt.gecerli && err != nil {
			t.Errorf("%s: beklenmeyen hata: %v", tt.ad, err)
		}
		if !tt.gecerli && err == nil {
			t.Errorf("%s: reddedilmesi gerekirdi (%q)", tt.ad, tt.istenen)
		}
	}
}

func TestDosyaIndirKokDisiniReddeder(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)

	w := cagir(u, istek("GET", "/dosya/indir?yol="+
		url.QueryEscape(`C:\Windows\win.ini`), nil))
	if w.Code != http.StatusForbidden {
		t.Errorf("kod = %d, beklenen 403", w.Code)
	}
}

func TestKlasordeGosterJetonsuzReddeder(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)

	form := url.Values{"yol": {filepath.Join(arsiv, "2024", "teklif.md")}}
	w := cagir(u, istek("POST", "/dosya/klasorde-goster", form))
	if w.Code != http.StatusForbidden {
		t.Errorf("jetonsuz istek kodu = %d, beklenen 403", w.Code)
	}
}

// ---------------------------------------------------------------- ayarlar

func ayarKaydet(t *testing.T, u *Uygulama, arsiv string) {
	t.Helper()
	form := url.Values{
		"jeton":             {u.jeton},
		"dizinler":          {arsiv},
		"icerikCikarma":     {"1"},
		"icerikUzantilari":  {".txt\n.md\n.docx\n.xlsx\n.pdf"},
		"haricUzantilar":    {".zip\n.exe"},
		"haricKlasorler":    {".git"},
		"maksDosyaBoyutuMB": {"64"},
		"sayfaBasiSonuc":    {"25"},
	}
	w := cagir(u, istek("POST", "/ayarlar", form))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("ayar kaydetme kodu = %d, beklenen 303. gövde: %s",
			w.Code, w.Body.String())
	}
}

func TestAyarKaydetVeYukle(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)

	ay := u.Ayar()
	if len(ay.TaranacakDizinler) != 1 {
		t.Fatalf("dizinler = %v", ay.TaranacakDizinler)
	}
	// Diske de yazılmış olmalı: yeni bir uygulama örneği aynı ayarı görmeli.
	u2, err := yeniUygulama(u.veriDizini, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(u2.Ayar().TaranacakDizinler) != 1 {
		t.Errorf("ayarlar diske yazılmamış: %v", u2.Ayar().TaranacakDizinler)
	}
}

func TestAyarKaydetGecersizDizinReddeder(t *testing.T) {
	u, _ := kurulum(t)
	form := url.Values{
		"jeton":    {u.jeton},
		"dizinler": {`Z:\boyle-bir-dizin-yok-12345`},
	}
	w := cagir(u, istek("POST", "/ayarlar", form))
	if w.Code != http.StatusOK {
		t.Fatalf("kod = %d, beklenen 200 (form hatayla yeniden gösterilir)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bulunamadı") {
		t.Errorf("hata mesajı gösterilmedi")
	}
	if len(u.Ayar().TaranacakDizinler) != 0 {
		t.Errorf("geçersiz ayar kaydedilmiş")
	}
}

func TestAyarKaydetJetonsuzReddeder(t *testing.T) {
	u, arsiv := kurulum(t)
	form := url.Values{"dizinler": {arsiv}}
	w := cagir(u, istek("POST", "/ayarlar", form))
	if w.Code != http.StatusForbidden {
		t.Errorf("kod = %d, beklenen 403", w.Code)
	}
}

// ---------------------------------------------------------------- tarama

// taramaCalistir taramayı başlatır ve bitmesini bekler.
func taramaCalistir(t *testing.T, u *Uygulama, tamTarama bool) {
	t.Helper()

	form := url.Values{"jeton": {u.jeton}}
	if tamTarama {
		form.Set("tamTarama", "1")
	}
	w := cagir(u, istek("POST", "/tarama/basla", form))
	if w.Code != http.StatusAccepted {
		t.Fatalf("tarama başlatma kodu = %d, gövde: %s", w.Code, w.Body.String())
	}

	bitis := time.Now().Add(30 * time.Second)
	for time.Now().Before(bitis) {
		if !u.is.Calisiyor() {
			d := u.is.Durum()
			if d.Durum == "hata" {
				t.Fatalf("tarama hata verdi: %s", d.Hata)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("tarama 30 saniyede bitmedi")
}

func TestCiftTaramaBaslatma409Doner(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)

	// İlk taramayı başlat ama bitmesini bekleme.
	form := url.Values{"jeton": {u.jeton}}
	if w := cagir(u, istek("POST", "/tarama/basla", form)); w.Code != http.StatusAccepted {
		t.Fatalf("ilk tarama kodu = %d", w.Code)
	}

	// Sürüyorsa ikinci istek 409 dönmeli. Tarama çok hızlı bitebileceği için
	// yalnızca sürüyorken kontrol ediyoruz.
	if u.is.Calisiyor() {
		w := cagir(u, istek("POST", "/tarama/basla", form))
		if w.Code != http.StatusConflict {
			t.Errorf("ikinci tarama kodu = %d, beklenen 409", w.Code)
		}
	}

	for u.is.Calisiyor() {
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTaramaIptalBostaykenConflict(t *testing.T) {
	u, _ := kurulum(t)
	form := url.Values{"jeton": {u.jeton}}
	w := cagir(u, istek("POST", "/tarama/iptal", form))
	if w.Code != http.StatusConflict {
		t.Errorf("kod = %d, beklenen 409", w.Code)
	}
}

func TestTaramaDurumJsonSekli(t *testing.T) {
	u, _ := kurulum(t)
	w := cagir(u, istek("GET", "/tarama/durum", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("kod = %d", w.Code)
	}
	var d map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatalf("JSON ayrıştırılamadı: %v", err)
	}
	for _, alan := range []string{"durum", "asama", "calisiyor", "toplam",
		"islenen", "yuzde", "sonDosya"} {
		if _, v := d[alan]; !v {
			t.Errorf("%q alanı eksik", alan)
		}
	}
}

// ---------------------------------------------------------------- uçtan uca

func TestUctanUcaTaramaVeArama(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)
	taramaCalistir(t, u, false)

	ozet := u.is.Ozet()
	if ozet.Islenen == 0 {
		t.Fatal("hiç dosya işlenmedi")
	}
	t.Logf("özet: islenen=%d icerikOkunan=%d sadeceAd=%d atlanan=%d hatali=%d",
		ozet.Islenen, ozet.IcerikOkunan, ozet.SadeceAd, ozet.Atlanan, ozet.Hatali)

	testler := []struct {
		ad        string
		sorgu     string
		beklenen  string
		beklenmez string
	}{
		{
			ad:       "içerikte arama",
			sorgu:    "?q=beton&kapsam=icerik",
			beklenen: "Şantiye Raporu.txt",
		},
		{
			ad:       "dosya adında arama",
			sorgu:    "?q=teklif&kapsam=ad",
			beklenen: "teklif.md",
		},
		{
			ad: "Türkçe katlama: sorgu diakritiksiz yazılsa da bulmalı",
			// "santiye" yazıp "Şantiye"yi bulmak katlamanın asıl amacı.
			sorgu:    "?q=santiye&kapsam=ad",
			beklenen: "Şantiye Raporu.txt",
		},
		{
			ad:       "docx içeriği",
			sorgu:    "?q=taseron&kapsam=icerik",
			beklenen: "Sozlesme.docx",
		},
		{
			ad:       "xlsx paylaşılan dize havuzu",
			sorgu:    "?q=cimento&kapsam=icerik",
			beklenen: "Maliyet.xlsx",
		},
		{
			ad:       "uzantı filtresi",
			sorgu:    "?q=&uzanti=.md",
			beklenen: "teklif.md",
			// .txt dosyaları filtre dışında kalmalı
			beklenmez: "Şantiye Raporu.txt",
		},
		{
			ad:        "klasör filtresi",
			sorgu:     "?q=&klasor=" + url.QueryEscape(filepath.Join(arsiv, "2024")),
			beklenen:  "teklif.md",
			beklenmez: "Şantiye Raporu.txt",
		},
		{
			ad: "hariç tutulan uzantı hiç indekslenmemeli",
			// .zip hariç listesinde: adıyla bile bulunmamalı.
			sorgu:     "?q=arsiv&kapsam=ad",
			beklenmez: "arsiv.zip",
		},
		{
			ad:       "uzun yol (260 karakter üstü) indekslenmeli",
			sorgu:    "?q=derindeki&kapsam=ad",
			beklenen: "derindeki.txt",
		},
	}

	for _, tt := range testler {
		t.Run(tt.ad, func(t *testing.T) {
			w := cagir(u, istek("GET", "/arama"+tt.sorgu, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("kod = %d", w.Code)
			}
			govde := w.Body.String()
			if tt.beklenen != "" && !strings.Contains(govde, tt.beklenen) {
				t.Errorf("sonuçlarda %q yok", tt.beklenen)
			}
			if tt.beklenmez != "" && strings.Contains(govde, tt.beklenmez) {
				t.Errorf("sonuçlarda olmaması gereken %q var", tt.beklenmez)
			}
		})
	}
}

// cp1254 kodlu eski Türkçe dosyaların içeriği de aranabilir olmalı.
func TestUctanUcaEskiKodlamaAranabilir(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)
	taramaCalistir(t, u, false)

	// Dosya cp1254 ile "Ğüşİı" içeriyor; katlanmış hali "gusii".
	w := cagir(u, istek("GET", "/arama?q=gusii&kapsam=icerik", nil))
	if !strings.Contains(w.Body.String(), "eski kodlama.txt") {
		t.Error("cp1254 dosyanın içeriği aranamadı")
	}
}

// Durum etiketleri kullanıcıya içeriğin NEDEN aranamadığını söylemeli.
func TestUctanUcaDurumEtiketleri(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)
	taramaCalistir(t, u, false)

	testler := []struct{ sorgu, etiket string }{
		{"?q=taranmis&kapsam=ad", "taranmış PDF"},
		{"?q=sahte&kapsam=ad", "ikili dosya"},
	}
	for _, tt := range testler {
		w := cagir(u, istek("GET", "/arama"+tt.sorgu, nil))
		if !strings.Contains(w.Body.String(), tt.etiket) {
			t.Errorf("%s için %q etiketi gösterilmedi", tt.sorgu, tt.etiket)
		}
	}
}

// Artımlı tarama: ikinci tarama dosyaların içeriğini yeniden okumamalı.
func TestArtimliTaramaIcerigiYenidenOkumaz(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)

	taramaCalistir(t, u, false)
	birinci := u.is.Ozet()

	taramaCalistir(t, u, false)
	ikinci := u.is.Ozet()

	if birinci.IcerikOkunan == 0 {
		t.Fatal("ilk taramada hiç içerik okunmadı")
	}
	if ikinci.Islenen != 0 {
		t.Errorf("ikinci tarama %d dosya işledi, hiç işlememeliydi", ikinci.Islenen)
	}
	if ikinci.Atlanan < birinci.IcerikOkunan {
		t.Errorf("ikinci taramada atlanan = %d, en az %d olmalıydı",
			ikinci.Atlanan, birinci.IcerikOkunan)
	}
}

// Silinen dosya indeksten düşmeli.
func TestSilinenDosyaIndekstenDuser(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)
	taramaCalistir(t, u, false)

	w := cagir(u, istek("GET", "/arama?q=teklif&kapsam=ad", nil))
	if !strings.Contains(w.Body.String(), "teklif.md") {
		t.Fatal("dosya ilk taramada bulunamadı")
	}

	if err := os.Remove(filepath.Join(arsiv, "2024", "teklif.md")); err != nil {
		t.Fatal(err)
	}
	taramaCalistir(t, u, false)

	if u.is.Ozet().Silinen != 1 {
		t.Errorf("silinen = %d, beklenen 1", u.is.Ozet().Silinen)
	}
	w = cagir(u, istek("GET", "/arama?q=teklif&kapsam=ad", nil))
	if strings.Contains(w.Body.String(), "teklif.md") {
		t.Error("silinen dosya hâlâ sonuçlarda")
	}
}

// ---------------------------------------------------------------- XSS

// Bu testin koruduğu şey: dosya içeriği ve adı asla HTML olarak
// yorumlanmamalı. Arşivde <script> içeren tek bir dosya varsa ve bunu
// kaçırmazsak depolanmış XSS deliği açılır.
func TestParcacikScriptKacirilir(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)
	taramaCalistir(t, u, false)

	yol := filepath.Join(arsiv, "2024", "zararli.txt")
	adres := "/api/parcacik?yol=" + url.QueryEscape(yol) + "&q=beton"
	w := cagir(u, istek("GET", adres, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("kod = %d, gövde: %s", w.Code, w.Body.String())
	}

	var cevap struct {
		Parcalar []struct {
			Metin   string
			Vurgulu bool
		} `json:"parcalar"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cevap); err != nil {
		t.Fatalf("JSON ayrıştırılamadı: %v — %s", err, w.Body.String())
	}
	if len(cevap.Parcalar) == 0 {
		t.Fatal("parçacık dönmedi")
	}

	// Parçacık YAPISAL dönmeli: metin alanları ham metin taşır, HTML değil.
	// İstemci bunları createTextNode ile basıyor, innerHTML ile değil.
	var tumMetin string
	vurguVar := false
	for _, p := range cevap.Parcalar {
		tumMetin += p.Metin
		if p.Vurgulu {
			vurguVar = true
		}
	}
	if !vurguVar {
		t.Error("eşleşme vurgulanmadı")
	}
	if !strings.Contains(tumMetin, "beton") {
		t.Errorf("parçacık eşleşmeyi içermiyor: %q", tumMetin)
	}
	// Sunucu <mark> gibi HTML üretmemeli; vurgu Vurgulu bayrağıyla taşınır.
	if strings.Contains(tumMetin, "<mark>") {
		t.Error("sunucu parçacık içine HTML gömmüş")
	}
}

// Dosya adı da aynı yoldan geçer: sonuç sayfasında script kaçırılmalı.
func TestSonucSayfasiScriptKacirir(t *testing.T) {
	u, arsiv := kurulum(t)

	// Windows dosya adında < > : " | ? * kullanılamaz; & ve ' kullanılabilir
	// ve HTML'de kaçırılmaları şart.
	kotuAd := `rapor & sirket'in.txt`
	if err := os.WriteFile(filepath.Join(arsiv, kotuAd),
		[]byte("beton"), 0o644); err != nil {
		t.Fatal(err)
	}

	ayarKaydet(t, u, arsiv)
	taramaCalistir(t, u, false)

	w := cagir(u, istek("GET", "/arama?q=rapor&kapsam=ad", nil))
	govde := w.Body.String()

	// & ve ' ham biçimde HTML gövdesine çıkmamalı.
	if strings.Contains(govde, "rapor & sirket'in") {
		t.Error("dosya adı kaçırılmadan basılmış")
	}
	if !strings.Contains(govde, "&amp;") || !strings.Contains(govde, "&#39;") {
		t.Error("dosya adı kaçırılmış biçimde bulunamadı")
	}
}

// ---------------------------------------------------------------- gözat

func TestGozatSuruculeriListeler(t *testing.T) {
	u, _ := kurulum(t)
	w := cagir(u, istek("GET", "/api/gozat", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("kod = %d", w.Code)
	}
	var c GozatCevabi
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Suruculer) == 0 {
		t.Error("hiç sürücü listelenmedi")
	}
}

func TestGozatKlasorleriListeler(t *testing.T) {
	u, arsiv := kurulum(t)
	w := cagir(u, istek("GET", "/api/gozat?yol="+url.QueryEscape(arsiv), nil))

	var c GozatCevabi
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if !c.Okunabilir {
		t.Fatalf("okunabilir değil: %s", c.Hata)
	}
	adlar := make(map[string]bool)
	for _, k := range c.Klasorler {
		adlar[k.Ad] = true
	}
	for _, beklenen := range []string{"2022", "2023", "2024", "derin"} {
		if !adlar[beklenen] {
			t.Errorf("%q klasörü listelenmedi", beklenen)
		}
	}
}

func TestGozatOlmayanYolHataDoner(t *testing.T) {
	u, _ := kurulum(t)
	w := cagir(u, istek("GET", "/api/gozat?yol="+
		url.QueryEscape(`Z:\yok-boyle-bir-yer-98765`), nil))
	// Hatalar gövdede, HTTP 200 ile dönüyor: istemci tek yoldan okuyor.
	if w.Code != http.StatusOK {
		t.Fatalf("kod = %d, beklenen 200", w.Code)
	}
	var c GozatCevabi
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c.Hata == "" {
		t.Error("hata mesajı beklenirdi")
	}
}

// ---------------------------------------------------------------- biçimleme

func TestInsanBoyut(t *testing.T) {
	testler := []struct {
		b        int64
		beklenen string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1,0 KB"},
		{1536, "1,5 KB"},
		{1048576, "1,0 MB"},
		{5 * 1024 * 1024, "5,0 MB"},
	}
	for _, tt := range testler {
		if g := insanBoyut(tt.b); g != tt.beklenen {
			t.Errorf("insanBoyut(%d) = %q, beklenen %q", tt.b, g, tt.beklenen)
		}
	}
}

func TestSayiFormat(t *testing.T) {
	testler := []struct {
		deger    any
		beklenen string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.000"},
		{int64(98213), "98.213"},
		{1234567, "1.234.567"},
		{-4500, "-4.500"},
	}
	for _, tt := range testler {
		if g := sayiFormat(tt.deger); g != tt.beklenen {
			t.Errorf("sayiFormat(%v) = %q, beklenen %q", tt.deger, g, tt.beklenen)
		}
	}
}

func TestSorguDegisFiltreleriKorur(t *testing.T) {
	mevcut := url.Values{
		"q":      {"beton"},
		"uzanti": {".pdf", ".docx"},
		"sayfa":  {"2"},
	}
	sonuc := string(sorguDegis(mevcut, "sayfa", "3"))

	for _, beklenen := range []string{"q=beton", "uzanti=.pdf", "uzanti=.docx", "sayfa=3"} {
		if !strings.Contains(sonuc, beklenen) {
			t.Errorf("%q korunmadı: %s", beklenen, sonuc)
		}
	}
	if strings.Contains(sonuc, "sayfa=2") {
		t.Errorf("eski sayfa değeri kaldı: %s", sonuc)
	}
	// Orijinal değiştirilmemeli.
	if mevcut.Get("sayfa") != "2" {
		t.Error("sorguDegis girdiyi değiştirdi")
	}
}

func TestFiltreCipleri(t *testing.T) {
	q := url.Values{
		"q":         {"beton"},
		"kapsam":    {"ad"},
		"klasor":    {`C:\Arsiv\2024`},
		"baslangic": {"2024-01-05"},
		"uzanti":    {".pdf", ".docx"},
		"sirala":    {"tarih"},
		"sayfa":     {"3"},
	}
	cipler, panel := filtreCipleri(q, sorgu.Ayristir(q, 20))

	if len(cipler) != 5 {
		t.Fatalf("5 çip bekleniyordu (kapsam, klasör, başlangıç, 2 uzantı), gelen %d: %+v", len(cipler), cipler)
	}
	if panel != 4 {
		t.Errorf("panel sayacı kapsamı saymamalı: beklenen 4, gelen %d", panel)
	}

	var pdf FiltreCipi
	for _, c := range cipler {
		if c.Deger == ".pdf" {
			pdf = c
		}
	}
	adres := string(pdf.KaldirURL)
	if strings.Contains(adres, "uzanti=.pdf") || !strings.Contains(adres, "uzanti=.docx") {
		t.Errorf(".pdf çipi yalnızca kendi değerini kaldırmalı: %s", adres)
	}
	if strings.Contains(adres, "sayfa=") {
		t.Errorf("filtre kaldırınca 1. sayfaya dönülmeli: %s", adres)
	}
	for _, korunan := range []string{"q=beton", "kapsam=ad", "sirala=tarih"} {
		if !strings.Contains(adres, korunan) {
			t.Errorf("%q korunmadı: %s", korunan, adres)
		}
	}

	hepsi := string(filtresizAdres(q))
	if hepsi != "?q=beton&sirala=tarih" {
		t.Errorf("filtresiz adres yanlış: %s", hepsi)
	}
}

// ---------------------------------------------------------------- kök değişimi

// Kullanıcı taranacak dizini değiştirdiğinde eski dizinin dokümanları
// indekste KALMAMALI.
//
// Kalırlarsa en kötü kombinasyon oluşur: arama sonuçlarında görünürler ama
// açılamazlar, çünkü GuvenliYol o kökü artık tanımaz (403).
func TestKokDegisinceEskiVeriTemizlenir(t *testing.T) {
	u, arsivA := kurulum(t)
	ayarKaydet(t, u, arsivA)
	taramaCalistir(t, u, false)

	w := cagir(u, istek("GET", "/arama?q=teklif&kapsam=ad", nil))
	if !strings.Contains(w.Body.String(), "teklif.md") {
		t.Fatal("A kökündeki dosya ilk taramada bulunamadı")
	}

	// Tamamen başka bir dizine geç.
	arsivB := t.TempDir()
	if err := os.WriteFile(filepath.Join(arsivB, "yenidosya.txt"),
		[]byte("yeni icerik"), 0o644); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"jeton":            {u.jeton},
		"dizinler":         {arsivB},
		"icerikCikarma":    {"1"},
		"icerikUzantilari": {".txt\n.md"},
	}
	if w := cagir(u, istek("POST", "/ayarlar", form)); w.Code != http.StatusSeeOther {
		t.Fatalf("ayar kaydetme kodu = %d", w.Code)
	}

	// Temizlik ayar kaydedilir kaydedilmez olmalı; tarama beklemeye gerek yok.
	w = cagir(u, istek("GET", "/arama?q=teklif&kapsam=ad", nil))
	if strings.Contains(w.Body.String(), "teklif.md") {
		t.Error("eski kökteki dosya ayar değişince temizlenmedi")
	}

	// Yeni kök taranınca yeni dosya bulunmalı.
	taramaCalistir(t, u, false)
	w = cagir(u, istek("GET", "/arama?q=yenidosya&kapsam=ad", nil))
	if !strings.Contains(w.Body.String(), "yenidosya.txt") {
		t.Error("yeni kökteki dosya bulunamadı")
	}
}

// Çok köklü kurulumda bir kök çıkarılınca YALNIZCA onun dokümanları düşmeli.
func TestKokCikarilincaDigerKokKorunur(t *testing.T) {
	u, arsivA := kurulum(t)
	arsivB := t.TempDir()
	if err := os.WriteFile(filepath.Join(arsivB, "bkokdosyasi.txt"),
		[]byte("beton"), 0o644); err != nil {
		t.Fatal(err)
	}

	temel := url.Values{
		"jeton":            {u.jeton},
		"icerikCikarma":    {"1"},
		"icerikUzantilari": {".txt\n.md"},
	}

	// İkisini birlikte tara.
	ikisi := url.Values{}
	for k, v := range temel {
		ikisi[k] = v
	}
	ikisi.Set("dizinler", arsivA+"\n"+arsivB)
	if w := cagir(u, istek("POST", "/ayarlar", ikisi)); w.Code != http.StatusSeeOther {
		t.Fatalf("kod = %d", w.Code)
	}
	taramaCalistir(t, u, false)

	// A kökünü çıkar, B kalsın.
	sadeceB := url.Values{}
	for k, v := range temel {
		sadeceB[k] = v
	}
	sadeceB.Set("dizinler", arsivB)
	if w := cagir(u, istek("POST", "/ayarlar", sadeceB)); w.Code != http.StatusSeeOther {
		t.Fatalf("kod = %d", w.Code)
	}

	w := cagir(u, istek("GET", "/arama?q=bkokdosyasi&kapsam=ad", nil))
	if !strings.Contains(w.Body.String(), "bkokdosyasi.txt") {
		t.Error("kalan kökün dosyası da silinmiş")
	}
	w = cagir(u, istek("GET", "/arama?q=teklif&kapsam=ad", nil))
	if strings.Contains(w.Body.String(), "teklif.md") {
		t.Error("çıkarılan kökün dosyası temizlenmedi")
	}
}

// Güvenlik freni: kök listesi boşken hiçbir şey silinmemeli. Bozuk bir ayar
// dosyası yüzünden tüm indeksin sessizce silinmesini istemiyoruz.
func TestBosKokListesiIndeksiSilmez(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)
	taramaCalistir(t, u, false)

	var once int
	u.ix.Oku(func(o indeks.Okuyucu) { once = o.CanliSayisi() })
	if once == 0 {
		t.Fatal("tarama sonrası indeks boş")
	}

	if n := u.ix.KoklerDisindakileriSil(nil); n != 0 {
		t.Errorf("boş kök listesiyle %d doküman silindi", n)
	}
	var sonra int
	u.ix.Oku(func(o indeks.Okuyucu) { sonra = o.CanliSayisi() })
	if sonra != once {
		t.Errorf("doküman sayısı %d -> %d değişti", once, sonra)
	}
}

// ---------------------------------------------------------------- satır eylemi

// Kullanıcı raporu: "Klasörde göster" tıklanınca tarayıcı sayfadan ayrılıp
// ham {"tamam":true} JSON'unu gösteriyordu.
//
// Artık: fetch (Accept: application/json) JSON alır ve sayfada kalır; düz
// HTML form gönderimi ise geldiği sayfaya yönlendirilir.
func TestKlasordeGosterDuzFormGeriYonlendirir(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)

	yol := filepath.Join(arsiv, "2024", "teklif.md")
	form := url.Values{"jeton": {u.jeton}, "yol": {yol}}

	// Düz form gönderimi (Accept: application/json YOK).
	r := istek("POST", "/dosya/klasorde-goster", form)
	r.Header.Set("Referer", "http://127.0.0.1:8080/arama?q=teklif&kapsam=ad")
	w := cagir(u, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("kod = %d, beklenen 303 (geri yönlendirme)", w.Code)
	}
	if k := w.Header().Get("Location"); k != "/arama?q=teklif&kapsam=ad" {
		t.Errorf("Location = %q, arama sayfasına dönmeliydi", k)
	}
	if strings.Contains(w.Body.String(), "tamam") {
		t.Error("düz form gönderimine ham JSON döndü")
	}
}

func TestKlasordeGosterFetchJsonDoner(t *testing.T) {
	u, arsiv := kurulum(t)
	ayarKaydet(t, u, arsiv)

	form := url.Values{
		"jeton": {u.jeton},
		"yol":   {filepath.Join(arsiv, "2024", "teklif.md")},
	}
	r := istek("POST", "/dosya/klasorde-goster", form)
	r.Header.Set("Accept", "application/json")
	w := cagir(u, r)

	if w.Code != http.StatusOK {
		t.Fatalf("kod = %d, beklenen 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "tamam") {
		t.Errorf("JSON dönmedi: %s", w.Body.String())
	}
}

// Referer dışarıdan gelen bir değerdir; açık yönlendirme aracına
// dönüşmemeli.
func TestGeriDonulecekYolAcikYonlendirmeyiEngeller(t *testing.T) {
	testler := []struct {
		referer  string
		beklenen string
	}{
		{"", "/arama"},
		{"http://127.0.0.1:8080/arama?q=x", "/arama?q=x"},
		{"https://kotu.example.com/tuzak", "/tuzak"},
		{"http://127.0.0.1:8080/ayarlar", "/ayarlar"},
	}
	for _, tt := range testler {
		r := httptest.NewRequest("POST", "/dosya/klasorde-goster", nil)
		if tt.referer != "" {
			r.Header.Set("Referer", tt.referer)
		}
		if g := geriDonulecekYol(r); g != tt.beklenen {
			t.Errorf("geriDonulecekYol(%q) = %q, beklenen %q",
				tt.referer, g, tt.beklenen)
		}
	}
}
