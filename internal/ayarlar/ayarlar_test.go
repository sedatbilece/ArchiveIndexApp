package ayarlar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestYukleDosyaYokIseVarsayilan(t *testing.T) {
	a, err := Yukle(t.TempDir())
	if err != nil {
		t.Fatalf("dosya yokken hata dönmemeli: %v", err)
	}
	if !a.IcerikCikarma || a.SayfaBasiSonuc == 0 {
		t.Errorf("varsayılanlar yüklenmedi: %+v", a)
	}
}

func TestKaydetYukleGidisDonus(t *testing.T) {
	dizin := t.TempDir()
	arsiv := t.TempDir()

	a := Varsayilan()
	a.TaranacakDizinler = []string{arsiv}
	a.MaksDosyaBoyutuMB = 128
	a.IcerikCikarma = false

	if err := a.Kaydet(dizin); err != nil {
		t.Fatalf("Kaydet: %v", err)
	}
	geri, err := Yukle(dizin)
	if err != nil {
		t.Fatalf("Yukle: %v", err)
	}
	if len(geri.TaranacakDizinler) != 1 || geri.TaranacakDizinler[0] != arsiv {
		t.Errorf("dizinler = %v", geri.TaranacakDizinler)
	}
	if geri.MaksDosyaBoyutuMB != 128 {
		t.Errorf("maksDosyaBoyutuMB = %d", geri.MaksDosyaBoyutuMB)
	}
	if geri.IcerikCikarma {
		t.Error("icerikCikarma=false korunmadı")
	}
}

// Eski bir ayar dosyasında olmayan alanlar sıfır değil varsayılan almalı;
// aksi halde sonradan eklenen bir ayar sessizce bozuk gelir.
func TestYukleEksikAlanlarVarsayilanaDuser(t *testing.T) {
	dizin := t.TempDir()
	eski := `{"taranacakDizinler":["C:\\Arsiv"],"icerikCikarma":true}`
	if err := os.WriteFile(filepath.Join(dizin, DosyaAdi), []byte(eski), 0o644); err != nil {
		t.Fatal(err)
	}

	a, err := Yukle(dizin)
	if err != nil {
		t.Fatal(err)
	}
	if a.SayfaBasiSonuc != Varsayilan().SayfaBasiSonuc {
		t.Errorf("sayfaBasiSonuc = %d, varsayılana düşmeliydi", a.SayfaBasiSonuc)
	}
	if len(a.IcerikUzantilari) == 0 {
		t.Error("icerikUzantilari varsayılana düşmedi")
	}
}

func TestYukleBozukDosyaHataVeVarsayilan(t *testing.T) {
	dizin := t.TempDir()
	if err := os.WriteFile(filepath.Join(dizin, DosyaAdi),
		[]byte("{bu json degil"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Yukle(dizin)
	if err == nil {
		t.Error("bozuk dosyada hata beklenirdi")
	}
	if a.SayfaBasiSonuc == 0 {
		t.Error("bozuk dosyada bile kullanılabilir varsayılan dönmeli")
	}
}

func TestDuzeltSinirlariCeker(t *testing.T) {
	v := Varsayilan()
	testler := []struct {
		ad       string
		degistir func(*Ayarlar)
		kontrol  func(Ayarlar) bool
	}{
		{"negatif boyut", func(a *Ayarlar) { a.MaksDosyaBoyutuMB = -5 },
			func(a Ayarlar) bool { return a.MaksDosyaBoyutuMB == v.MaksDosyaBoyutuMB }},
		{"aşırı boyut", func(a *Ayarlar) { a.MaksDosyaBoyutuMB = 99999 },
			func(a Ayarlar) bool { return a.MaksDosyaBoyutuMB == v.MaksDosyaBoyutuMB }},
		{"sıfır sayfa boyutu", func(a *Ayarlar) { a.SayfaBasiSonuc = 0 },
			func(a Ayarlar) bool { return a.SayfaBasiSonuc == v.SayfaBasiSonuc }},
		{"negatif işçi", func(a *Ayarlar) { a.IsciSayisi = -1 },
			func(a Ayarlar) bool { return a.IsciSayisi == 0 }},
		{"aşırı belirteç cap", func(a *Ayarlar) { a.DokumanBelirtecCap = 5 },
			func(a Ayarlar) bool { return a.DokumanBelirtecCap == v.DokumanBelirtecCap }},
	}
	for _, tt := range testler {
		a := Varsayilan()
		tt.degistir(&a)
		a.Duzelt()
		if !tt.kontrol(a) {
			t.Errorf("%s: sınır çekilmedi: %+v", tt.ad, a)
		}
	}
}

func TestDuzeltUzantilariNormallestirir(t *testing.T) {
	a := Varsayilan()
	a.IcerikUzantilari = []string{"TXT", " .PDF ", ".txt", "", ".docx"}
	a.Duzelt()

	beklenen := []string{".txt", ".pdf", ".docx"}
	if len(a.IcerikUzantilari) != len(beklenen) {
		t.Fatalf("uzantılar = %v, beklenen %v", a.IcerikUzantilari, beklenen)
	}
	for i, u := range beklenen {
		if a.IcerikUzantilari[i] != u {
			t.Errorf("uzantılar = %v, beklenen %v", a.IcerikUzantilari, beklenen)
			break
		}
		_ = i
	}
}

// Aynı dizin farklı yazımla girilirse tek kayıt olmalı.
func TestDuzeltDizinTekrarlariniTemizler(t *testing.T) {
	dizin := t.TempDir()
	a := Varsayilan()
	a.TaranacakDizinler = []string{dizin, strings.ToUpper(dizin), dizin, "  "}
	a.Duzelt()
	if len(a.TaranacakDizinler) != 1 {
		t.Errorf("dizinler = %v, beklenen tek kayıt", a.TaranacakDizinler)
	}
}

func TestDogrula(t *testing.T) {
	arsiv := t.TempDir()

	a := Varsayilan()
	a.TaranacakDizinler = []string{arsiv}
	if h := a.Dogrula(); len(h) != 0 {
		t.Errorf("geçerli ayarda hata döndü: %v", h)
	}

	bos := Varsayilan()
	bos.TaranacakDizinler = nil
	if h := bos.Dogrula(); len(h) == 0 {
		t.Error("dizinsiz ayarda hata beklenirdi")
	}

	yok := Varsayilan()
	yok.TaranacakDizinler = []string{filepath.Join(arsiv, "boyle-bir-yer-yok")}
	if h := yok.Dogrula(); len(h) == 0 {
		t.Error("olmayan dizinde hata beklenirdi")
	}
}

func TestDizinDogrulaDosyayiReddeder(t *testing.T) {
	dosya := filepath.Join(t.TempDir(), "dosya.txt")
	if err := os.WriteFile(dosya, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if h := DizinDogrula(dosya); h == "" {
		t.Error("dosya yolu klasör olarak kabul edildi")
	}
}

func TestIcerikOkunacakMi(t *testing.T) {
	a := Varsayilan()
	if !a.IcerikOkunacakMi(".PDF") {
		t.Error(".PDF büyük harfle tanınmadı")
	}
	if a.IcerikOkunacakMi(".dwg") {
		t.Error(".dwg içerik listesinde olmamalı")
	}

	// İçerik indeksleme kapalıysa hiçbir uzantı okunmaz.
	a.IcerikCikarma = false
	if a.IcerikOkunacakMi(".txt") {
		t.Error("içerik indeksleme kapalıyken .txt okunuyor")
	}
}

func TestHaricKontrolleri(t *testing.T) {
	a := Varsayilan()
	if !a.HaricUzantiMi(".ZIP") {
		t.Error(".ZIP hariç listesinde tanınmadı")
	}
	if !a.HaricKlasorMu("node_modules") || !a.HaricKlasorMu("NODE_MODULES") {
		t.Error("hariç klasör adı harf duyarsız tanınmadı")
	}
	if a.HaricKlasorMu("2023") {
		t.Error("normal klasör hariç sayıldı")
	}
}

func TestIsciAdedi(t *testing.T) {
	a := Varsayilan()
	if n := a.IsciAdedi(); n < 1 || n > 8 {
		t.Errorf("otomatik işçi sayısı = %d, 1-8 arası beklenirdi", n)
	}
	a.IsciSayisi = 3
	if n := a.IsciAdedi(); n != 3 {
		t.Errorf("işçi sayısı = %d, beklenen 3", n)
	}
}
