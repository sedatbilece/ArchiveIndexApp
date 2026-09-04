package metin

import "testing"

func TestNormalizeTurkce(t *testing.T) {
	testler := []struct{ girdi, beklenen string }{
		{"İSTANBUL Çğüşiı", "istanbul cgusii"},
		{"SIRA", "sira"},
		{"sıra", "sira"},
		{"sira", "sira"},
		{"İıIi", "iiii"},
		{"ÖĞRENCİ ŞUBAT", "ogrenci subat"},
		{"zaten kucuk", "zaten kucuk"},
		{"", ""},
		{"Rapor_2023-Rev.02.docx", "rapor_2023-rev.02.docx"},
	}
	for _, tt := range testler {
		if g := Normalize(tt.girdi); g != tt.beklenen {
			t.Errorf("Normalize(%q) = %q, beklenen %q", tt.girdi, g, tt.beklenen)
		}
	}
}

// Katlamanın asıl amacı: kullanıcı hangi harfi yazdığını hatırlamak zorunda
// kalmasın. Bu üç yazım aynı terime düşmeli.
func TestKatlamaAyniTerimeDuser(t *testing.T) {
	a := Normalize("ŞİRKET")
	b := Normalize("sirket")
	c := Normalize("şirket")
	if a != b || b != c {
		t.Fatalf("katlama ayrıştı: %q %q %q", a, b, c)
	}
}

func TestKabul(t *testing.T) {
	testler := []struct {
		b        string
		beklenen bool
		neden    string
	}{
		{"a", false, "tek karakter"},
		{"ab", true, "en kısa sınırda"},
		{"rapor", true, "normal kelime"},
		{"ve", false, "türkçe durak kelime"},
		{"the", false, "ingilizce durak kelime"},
		{"2023", true, "yıl: 4 haneli sayı tutulur"},
		{"03", true, "revizyon: 2 haneli sayı tutulur"},
		{"1698240000", false, "timestamp: 7+ haneli sayı atılır"},
		{"a4", true, "alfanümerik her zaman tutulur"},
		{"enk", true, "proje kodu parçası"},
		{"rev2", true, "alfanümerik"},
		{"m25x120", true, "parça kodu"},
		{"abcdefghijabcdefghijabcdefghijabcd", false, "32 karakterden uzun"},
	}
	for _, tt := range testler {
		if g := Kabul(tt.b); g != tt.beklenen {
			t.Errorf("Kabul(%q) = %v, beklenen %v (%s)", tt.b, g, tt.beklenen, tt.neden)
		}
	}
}

func TestBelirtecler(t *testing.T) {
	// "ENK-1234" iki belirteç olur; sorgu anında da aynı ayrıldığı için
	// arama yine eşleşir.
	g := Belirtecler("ENK-1234 Şantiye Raporu.pdf")
	beklenen := []string{"enk", "1234", "santiye", "raporu", "pdf"}
	esitMi(t, g, beklenen)
}

func TestBelirteclerTasanAtilir(t *testing.T) {
	// 40 karakterlik bir hash bloğu tamamen atılmalı, KIRPILMAMALI.
	uzun := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	g := Belirtecler("rapor " + uzun + " son")
	esitMi(t, g, []string{"rapor", "son"})
}

func TestTekilBelirteclerCap(t *testing.T) {
	// Cap, tek bozuk PDF'in sözlüğü zehirlemesine karşı tek korumamız.
	metin := ""
	for i := 0; i < 100; i++ {
		metin += " kelime" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	g := TekilBelirtecler(metin, 10)
	if len(g) != 10 {
		t.Fatalf("cap uygulanmadı: %d belirteç döndü", len(g))
	}
}

func TestTekilBelirteclerTekrarTemizler(t *testing.T) {
	g := TekilBelirtecler("rapor rapor RAPOR Rapor", 0)
	esitMi(t, g, []string{"rapor"})
}

func TestCozBOM(t *testing.T) {
	testler := []struct {
		ad       string
		b        []byte
		beklenen string
	}{
		{"utf8 BOM", append([]byte{0xEF, 0xBB, 0xBF}, []byte("şantiye")...), "şantiye"},
		{"utf8 BOM'suz", []byte("şantiye"), "şantiye"},
		{"utf16 LE", append([]byte{0xFF, 0xFE}, 0x61, 0x00, 0x62, 0x00), "ab"},
		{"utf16 BE", append([]byte{0xFE, 0xFF}, 0x00, 0x61, 0x00, 0x62), "ab"},
	}
	for _, tt := range testler {
		if g := Coz(tt.b); g != tt.beklenen {
			t.Errorf("%s: Coz = %q, beklenen %q", tt.ad, g, tt.beklenen)
		}
	}
}

// Eski Türkçe metin dosyaları cp1254'tür. UTF-8 varsayarsak "ı" içeren her
// kelime U+FFFD'ye döner ve aranamaz hale gelir.
func TestCozCp1254(t *testing.T) {
	// Ğ=0xD0 ü=0xFC ş=0xFE İ=0xDD ı=0xFD
	b := []byte{0xD0, 0xFC, 0xFE, 0xDD, 0xFD}
	if g := Coz(b); g != "Ğüşİı" {
		t.Errorf("Coz(cp1254) = %q, beklenen %q", g, "Ğüşİı")
	}
	// Katlama sonrası aranabilir olmalı.
	if n := Normalize(Coz(b)); n != "gusii" {
		t.Errorf("Normalize(Coz(cp1254)) = %q, beklenen %q", n, "gusii")
	}
}

func TestIkiliMi(t *testing.T) {
	testler := []struct {
		ad       string
		b        []byte
		beklenen bool
	}{
		{"düz metin", []byte("Şantiye raporu\r\nsatır iki\n"), false},
		{"boş", []byte{}, false},
		{"NUL içeren", []byte("rapor\x00\x00binary"), true},
		{"utf16 BOM'lu NUL", append([]byte{0xFF, 0xFE}, 'a', 0, 'b', 0), false},
		{"cp1254 metin", []byte{0xD0, 0xFC, 0xFE, 'r', 'a', 'p', 'o', 'r'}, false},
		{"yoğun kontrol karakteri", []byte("\x01\x02\x03\x04\x05\x06\x07ab"), true},
	}
	for _, tt := range testler {
		if g := IkiliMi(tt.b); g != tt.beklenen {
			t.Errorf("%s: IkiliMi = %v, beklenen %v", tt.ad, g, tt.beklenen)
		}
	}
}

func esitMi(t *testing.T, g, beklenen []string) {
	t.Helper()
	if len(g) != len(beklenen) {
		t.Fatalf("belirteçler = %q, beklenen %q", g, beklenen)
	}
	for i := range g {
		if g[i] != beklenen[i] {
			t.Fatalf("belirteçler = %q, beklenen %q", g, beklenen)
		}
	}
}
