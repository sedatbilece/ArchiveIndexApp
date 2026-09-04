package indeks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func ornek(yol string, belirtecler ...string) Girdi {
	return Girdi{
		Yol:         yol,
		Boyut:       100,
		Zaman:       time.Date(2023, 5, 1, 12, 0, 0, 0, time.UTC).UnixNano(),
		Durum:       "tam",
		Belirtecler: belirtecler,
	}
}

func TestEkleVeAra(t *testing.T) {
	ix := Yeni()
	ix.EkleToplu([]Girdi{
		ornek(`C:\Arsiv\rapor.txt`, "santiye", "beton"),
		ornek(`C:\Arsiv\teklif.txt`, "beton", "fiyat"),
	})

	ix.Oku(func(o Okuyucu) {
		if o.CanliSayisi() != 2 {
			t.Fatalf("canlı sayısı = %d, beklenen 2", o.CanliSayisi())
		}
		if g := o.Df("beton"); g != 2 {
			t.Errorf("df(beton) = %d, beklenen 2", g)
		}
		if g := o.Df("santiye"); g != 1 {
			t.Errorf("df(santiye) = %d, beklenen 1", g)
		}
		// Dosya adı belirteçleri AYRI haritaya girmeli.
		if g := o.Df("rapor"); g != 0 {
			t.Errorf("df(rapor) = %d, ad belirteci içerik haritasına karışmış", g)
		}
		if g := len(o.AdGonderiler("rapor")); g != 1 {
			t.Errorf("adGonderiler(rapor) = %d, beklenen 1", g)
		}
	})
}

// Gönderi listeleri artan sıralı olmalı; AND birleştirmesi buna dayanıyor.
func TestGonderilerSirali(t *testing.T) {
	ix := Yeni()
	var girdiler []Girdi
	for i := 0; i < 50; i++ {
		girdiler = append(girdiler, ornek(filepath.Join(`C:\A`, string(rune('a'+i%26))+".txt"), "ortak"))
	}
	ix.EkleToplu(girdiler)
	ix.Oku(func(o Okuyucu) {
		g := o.Gonderiler("ortak")
		for i := 1; i < len(g); i++ {
			if g[i] <= g[i-1] {
				t.Fatalf("gönderiler sıralı değil: %v", g[:i+1])
			}
		}
	})
}

// Aynı terim hem yolda hem içerikte geçerse her haritada birer kez olmalı.
func TestGonderiTekrarEklenmez(t *testing.T) {
	ix := Yeni()
	ix.EkleToplu([]Girdi{ornek(`C:\Arsiv\beton.txt`, "beton", "santiye")})
	ix.Oku(func(o Okuyucu) {
		if g := o.Gonderiler("beton"); len(g) != 1 {
			t.Fatalf("içerik gönderileri = %v, beklenen tek kayıt", g)
		}
		if g := o.AdGonderiler("beton"); len(g) != 1 {
			t.Fatalf("ad gönderileri = %v, beklenen tek kayıt", g)
		}
	})
}

// Kullanıcı "sadece içerikte ara" dediğinde dosya adı eşleşmeleri
// gelmemeli; içerik ve yol gönderilerinin ayrılması o isteğin temelidir.
func TestIcerikVeAdGonderileriAyri(t *testing.T) {
	ix := Yeni()
	ix.EkleToplu([]Girdi{ornek(`C:\Arsiv\teklif.txt`, "beton")})
	ix.Oku(func(o Okuyucu) {
		// "teklif" yalnızca dosya adında var.
		if len(o.Gonderiler("teklif")) != 0 {
			t.Error("ad terimi içerik haritasına girdi")
		}
		if len(o.AdGonderiler("teklif")) != 1 {
			t.Error("ad terimi ad haritasına girmedi")
		}
		// "beton" yalnızca içerikte var.
		if len(o.Gonderiler("beton")) != 1 {
			t.Error("içerik terimi içerik haritasına girmedi")
		}
		if len(o.AdGonderiler("beton")) != 0 {
			t.Error("içerik terimi ad haritasına girdi")
		}
		// idf hesabı ikisini toplamalı.
		if o.ToplamDf("teklif") != 1 {
			t.Errorf("toplamDf(teklif) = %d, beklenen 1", o.ToplamDf("teklif"))
		}
	})
}

func TestAyniYolTekrarEklenirseEskisiSilinir(t *testing.T) {
	ix := Yeni()
	ix.EkleToplu([]Girdi{ornek(`C:\Arsiv\rapor.txt`, "eski")})
	ix.EkleToplu([]Girdi{ornek(`C:\Arsiv\rapor.txt`, "yeni")})

	ix.Oku(func(o Okuyucu) {
		if o.CanliSayisi() != 1 {
			t.Fatalf("canlı sayısı = %d, beklenen 1", o.CanliSayisi())
		}
		if o.BelgeSayisi() != 2 {
			t.Fatalf("yuva sayısı = %d, beklenen 2 (biri tombstone)", o.BelgeSayisi())
		}
	})
}

// Windows dosya sistemi büyük/küçük harf duyarsız: aynı dosya farklı yazımla
// gelirse iki kayıt olmamalı.
func TestYolKimligiBuyukKucukDuyarsiz(t *testing.T) {
	ix := Yeni()
	ix.EkleToplu([]Girdi{ornek(`C:\Arsiv\Rapor.txt`, "bir")})
	ix.EkleToplu([]Girdi{ornek(`c:\arsiv\rapor.txt`, "iki")})
	ix.Oku(func(o Okuyucu) {
		if o.CanliSayisi() != 1 {
			t.Fatalf("canlı sayısı = %d, beklenen 1", o.CanliSayisi())
		}
	})
}

// Kimlik anahtarı arama normalizasyonundan ayrı olmalı: Normalize ı/i/İ
// harflerini katlar, bu iki dosya AYNI kayda düşerse biri kaybolur.
func TestKimlikAramaNormalizasyonundanAyri(t *testing.T) {
	ix := Yeni()
	ix.EkleToplu([]Girdi{
		ornek(`C:\Arsiv\İş.txt`, "bir"),
		ornek(`C:\Arsiv\is.txt`, "iki"),
	})
	ix.Oku(func(o Okuyucu) {
		if o.CanliSayisi() != 2 {
			t.Fatalf("canlı sayısı = %d, beklenen 2 (kimlik katlandı, dosya kayboldu)",
				o.CanliSayisi())
		}
	})
}

func TestDegismemis(t *testing.T) {
	ix := Yeni()
	g := ornek(`C:\Arsiv\rapor.txt`, "x")
	ix.EkleToplu([]Girdi{g})

	if !ix.Degismemis(g.Yol, g.Boyut, g.Zaman) {
		t.Error("aynı boyut ve zamanla değişmemiş sayılmalı")
	}
	if ix.Degismemis(g.Yol, g.Boyut+1, g.Zaman) {
		t.Error("boyut değişince değişmiş sayılmalı")
	}
	if ix.Degismemis(g.Yol, g.Boyut, g.Zaman+1) {
		t.Error("zaman değişince değişmiş sayılmalı")
	}
	if ix.Degismemis(`C:\Arsiv\yok.txt`, 1, 1) {
		t.Error("indekste olmayan dosya değişmemiş sayılmamalı")
	}
}

func TestGorulmeyenleriSil(t *testing.T) {
	ix := Yeni()
	ix.EkleToplu([]Girdi{
		ornek(`C:\Arsiv\kalan.txt`, "a"),
		ornek(`C:\Arsiv\silinen.txt`, "b"),
		ornek(`D:\Baska\dokunulmaz.txt`, "c"),
	})

	gorulen := map[string]struct{}{Anahtar(`C:\Arsiv\kalan.txt`): {}}
	n := ix.GorulmeyenleriSil(gorulen, []string{`C:\Arsiv`})

	if n != 1 {
		t.Fatalf("silinen = %d, beklenen 1", n)
	}
	ix.Oku(func(o Okuyucu) {
		if o.CanliSayisi() != 2 {
			t.Fatalf("canlı = %d, beklenen 2", o.CanliSayisi())
		}
	})
	// Taranmayan kökteki dosya silinmemeli.
	if !ix.Degismemis(`D:\Baska\dokunulmaz.txt`, 100,
		time.Date(2023, 5, 1, 12, 0, 0, 0, time.UTC).UnixNano()) {
		t.Error("taranmayan kökteki dosya silinmiş")
	}
}

func TestAltindaMi(t *testing.T) {
	testler := []struct {
		yol, kok string
		kalan    string
		tamam    bool
		neden    string
	}{
		{`C:\Arsiv\a\b.txt`, `C:\Arsiv`, `a\b.txt`, true, "normal"},
		{`C:\Arsiv\a\b.txt`, `C:\Arsiv\`, `a\b.txt`, true, "kökte fazla ayırıcı"},
		{`c:\arsiv\a\b.txt`, `C:\Arsiv`, `a\b.txt`, true, "harf duyarsız"},
		{`C:\Arsiv`, `C:\Arsiv`, ``, true, "kökün kendisi"},
		{`C:\Arsiv-gizli\a.txt`, `C:\Arsiv`, ``, false, "ayırıcı sınırı yok"},
		{`C:\Baska\a.txt`, `C:\Arsiv`, ``, false, "başka kök"},
		{`C:\A`, `C:\Arsiv`, ``, false, "yol kökten kısa"},
	}
	for _, tt := range testler {
		kalan, tamam := AltindaMi(tt.yol, tt.kok)
		if tamam != tt.tamam || kalan != tt.kalan {
			t.Errorf("AltindaMi(%q, %q) = (%q, %v), beklenen (%q, %v) — %s",
				tt.yol, tt.kok, kalan, tamam, tt.kalan, tt.tamam, tt.neden)
		}
	}
}

func TestSikistir(t *testing.T) {
	ix := Yeni()
	ix.EkleToplu([]Girdi{
		ornek(`C:\A\bir.txt`, "ortak", "birinci"),
		ornek(`C:\A\iki.txt`, "ortak", "ikinci"),
		ornek(`C:\A\uc.txt`, "ortak", "ucuncu"),
	})
	ix.SilYollar([]string{`C:\A\iki.txt`})

	if o := ix.TombstoneOrani(); o <= 0 {
		t.Fatalf("tombstone oranı = %v, beklenen > 0", o)
	}

	ix.Sikistir()

	ix.Oku(func(o Okuyucu) {
		if o.BelgeSayisi() != 2 || o.CanliSayisi() != 2 {
			t.Fatalf("sıkıştırma sonrası yuva=%d canlı=%d, beklenen 2/2",
				o.BelgeSayisi(), o.CanliSayisi())
		}
		if g := o.Df("ortak"); g != 2 {
			t.Errorf("df(ortak) = %d, beklenen 2", g)
		}
		// Ölü dokümanın tek terimi sözlükten tamamen düşmeli.
		if g := o.Df("ikinci"); g != 0 {
			t.Errorf("df(ikinci) = %d, beklenen 0", g)
		}
		// Yol haritası da sıkıştırılmalı: silinen dosyanın adı kalmamalı.
		if g := len(o.AdGonderiler("iki")); g != 0 {
			t.Errorf("adGonderiler(iki) = %d, ad haritası sıkıştırılmadı", g)
		}
		// Kalan gönderiler yeni kimliklere doğru eşlenmeli.
		for _, terim := range []string{"birinci", "ucuncu"} {
			g := o.Gonderiler(terim)
			if len(g) != 1 || g[0] >= uint32(o.BelgeSayisi()) {
				t.Errorf("gönderiler(%s) = %v, geçersiz kimlik", terim, g)
			}
		}
	})
	if o := ix.TombstoneOrani(); o != 0 {
		t.Errorf("sıkıştırma sonrası tombstone oranı = %v, beklenen 0", o)
	}
}

// Sıkıştırmadan sonra artımlı tarama hâlâ çalışmalı: YolID yeni kimliklere
// göre yeniden kurulmuş olmalı.
func TestSikistirSonrasiDegismemisCalisir(t *testing.T) {
	ix := Yeni()
	g := ornek(`C:\A\kalan.txt`, "x")
	ix.EkleToplu([]Girdi{g, ornek(`C:\A\gidecek.txt`, "y")})
	ix.SilYollar([]string{`C:\A\gidecek.txt`})
	ix.Sikistir()

	if !ix.Degismemis(g.Yol, g.Boyut, g.Zaman) {
		t.Error("sıkıştırma sonrası YolID bozuldu")
	}
}

func TestGobGidisDonus(t *testing.T) {
	dizin := t.TempDir()
	ix := Yeni()
	ix.EkleToplu([]Girdi{
		ornek(`C:\Arsiv\Şantiye Raporu.pdf`, "beton", "santiye"),
		ornek(`C:\Arsiv\eski.txt`, "arsiv"),
	})
	ix.SilYollar([]string{`C:\Arsiv\eski.txt`})
	bitis := time.Now().Truncate(time.Second)
	ix.TaramaBitti(bitis)

	if err := ix.Kaydet(dizin); err != nil {
		t.Fatalf("Kaydet: %v", err)
	}
	geri, err := Yukle(dizin)
	if err != nil {
		t.Fatalf("Yukle: %v", err)
	}

	geri.Oku(func(o Okuyucu) {
		if o.CanliSayisi() != 1 {
			t.Errorf("canlı = %d, beklenen 1", o.CanliSayisi())
		}
		if g := o.Df("beton"); g != 1 {
			t.Errorf("df(beton) = %d, beklenen 1", g)
		}
		// Türetilmiş alanlar yükleme sonrası yeniden üretilmiş olmalı.
		if o.AdNorm(0) != "santiye raporu.pdf" {
			t.Errorf("adNorm = %q, beklenen %q", o.AdNorm(0), "santiye raporu.pdf")
		}
		if !o.SonTarama().Equal(bitis) {
			t.Errorf("sonTarama = %v, beklenen %v", o.SonTarama(), bitis)
		}
		// Tombstone durumu korunmalı.
		if !o.SilinmisMi(1) {
			t.Error("tombstone gob gidiş dönüşünde kayboldu")
		}
	})
}

func TestYukleDosyaYokIseBosIndeks(t *testing.T) {
	ix, err := Yukle(t.TempDir())
	if err != nil {
		t.Fatalf("dosya yokken hata dönmemeli: %v", err)
	}
	ix.Oku(func(o Okuyucu) {
		if o.BelgeSayisi() != 0 {
			t.Fatalf("boş indeks beklenirken %d belge var", o.BelgeSayisi())
		}
	})
}

func TestYukleBozukDosyaAtilir(t *testing.T) {
	dizin := t.TempDir()
	yol := filepath.Join(dizin, DosyaAdi)
	if err := os.WriteFile(yol, []byte("bu gob degil, rastgele metin"), 0o644); err != nil {
		t.Fatal(err)
	}
	ix, err := Yukle(dizin)
	if err == nil {
		t.Error("bozuk dosyada hata beklenirdi")
	}
	if ix == nil {
		t.Fatal("bozuk dosyada bile kullanılabilir boş indeks dönmeli")
	}
	ix.Oku(func(o Okuyucu) {
		if o.BelgeSayisi() != 0 {
			t.Error("bozuk indeks atılmamış")
		}
	})
}

func TestUzantilarVeUstKlasorler(t *testing.T) {
	ix := Yeni()
	ix.EkleToplu([]Girdi{
		ornek(`C:\Arsiv\2023\a.pdf`),
		ornek(`C:\Arsiv\2023\b.pdf`),
		ornek(`C:\Arsiv\2024\c.docx`),
		ornek(`C:\Arsiv\kok.txt`),
	})
	ix.Oku(func(o Okuyucu) {
		u := o.Uzantilar()
		if len(u) == 0 || u[0].Uzanti != ".pdf" || u[0].Sayi != 2 {
			t.Errorf("uzantılar = %+v, beklenen ilk sırada .pdf/2", u)
		}
		k := o.UstKlasorler([]string{`C:\Arsiv`})
		if len(k) != 2 {
			t.Fatalf("üst klasörler = %v, beklenen 2 tane", k)
		}
	})
}
