package tarayici

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"arsiv-indeks/internal/ayarlar"
	"arsiv-indeks/internal/indeks"
)

func agacKur(t *testing.T, dosyalar map[string]string) string {
	t.Helper()
	kok := t.TempDir()
	for goreli, icerik := range dosyalar {
		yol := filepath.Join(kok, filepath.FromSlash(goreli))
		if err := os.MkdirAll(filepath.Dir(yol), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yol, []byte(icerik), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return kok
}

func ayarKur(kok string) ayarlar.Ayarlar {
	ay := ayarlar.Varsayilan()
	ay.TaranacakDizinler = []string{kok}
	ay.Duzelt()
	return ay
}

func TestTaraTemelAkis(t *testing.T) {
	kok := agacKur(t, map[string]string{
		"a.txt":       "beton dokumu",
		"alt/b.txt":   "cimento teslimi",
		"alt/c.md":    "# rapor",
		"resim.jpg":   "sahte gorsel", // hariç uzantı
		"arsiv.zip":   "sahte arsiv",  // hariç uzantı
		".git/config": "atlanmali",    // hariç klasör
	})

	ix := indeks.Yeni()
	il := YeniIlerleme()
	ozet, err := Tara(context.Background(), ix, ayarKur(kok), t.TempDir(), il)
	if err != nil {
		t.Fatalf("Tara: %v", err)
	}

	if ozet.Islenen != 3 {
		t.Errorf("işlenen = %d, beklenen 3 (a.txt, b.txt, c.md)", ozet.Islenen)
	}
	if ozet.IcerikOkunan != 3 {
		t.Errorf("içeriği okunan = %d, beklenen 3", ozet.IcerikOkunan)
	}

	ix.Oku(func(o indeks.Okuyucu) {
		if o.CanliSayisi() != 3 {
			t.Errorf("indekste %d dosya var, beklenen 3", o.CanliSayisi())
		}
		if o.Df("beton") != 1 {
			t.Errorf("içerik indekslenmedi: df(beton) = %d", o.Df("beton"))
		}
		// Hariç tutulan uzantılar hiç indekslenmemeli.
		if len(o.AdGonderiler("resim")) != 0 || len(o.AdGonderiler("arsiv")) != 0 {
			t.Error("hariç tutulan uzantı indekslenmiş")
		}
		// Hariç klasör atlanmalı.
		if len(o.AdGonderiler("config")) != 0 {
			t.Error("hariç tutulan klasördeki dosya indekslenmiş")
		}
		// Tarama bitiş damgası atılmalı.
		if o.SonTarama().IsZero() {
			t.Error("tarama bitiş damgası atılmadı")
		}
	})
}

// Sayım geçişi ile tarama geçişi aynı uygunluk kurallarını kullanmalı,
// aksi halde ilerleme çubuğu hiç %100'e ulaşmaz.
func TestToplamSayimIslenenIleTutarli(t *testing.T) {
	kok := agacKur(t, map[string]string{
		"a.txt": "bir", "b.txt": "iki", "c.txt": "uc",
		"d.jpg": "hariç", "alt/e.txt": "bes",
	})

	ix := indeks.Yeni()
	il := YeniIlerleme()
	if _, err := Tara(context.Background(), ix, ayarKur(kok), t.TempDir(), il); err != nil {
		t.Fatal(err)
	}

	toplam := il.Toplam.Load()
	if toplam != 4 {
		t.Errorf("toplam = %d, beklenen 4 (.jpg hariç)", toplam)
	}
	if il.Islenen.Load() != toplam {
		t.Errorf("işlenen (%d) toplama (%d) eşit değil",
			il.Islenen.Load(), toplam)
	}
}

func TestTaraArtimliDegismemisiAtlar(t *testing.T) {
	kok := agacKur(t, map[string]string{
		"a.txt": "beton", "b.txt": "cimento",
	})
	ay := ayarKur(kok)
	veri := t.TempDir()
	ix := indeks.Yeni()

	if _, err := Tara(context.Background(), ix, ay, veri, YeniIlerleme()); err != nil {
		t.Fatal(err)
	}

	// İkinci tarama: hiçbir dosya değişmedi, hepsi atlanmalı.
	il2 := YeniIlerleme()
	ozet2, err := Tara(context.Background(), ix, ay, veri, il2)
	if err != nil {
		t.Fatal(err)
	}
	if ozet2.Islenen != 0 {
		t.Errorf("ikinci taramada işlenen = %d, beklenen 0", ozet2.Islenen)
	}
	if ozet2.Atlanan != 2 {
		t.Errorf("ikinci taramada atlanan = %d, beklenen 2", ozet2.Atlanan)
	}

	// Bir dosyayı değiştirince yeniden okunmalı.
	yol := filepath.Join(kok, "a.txt")
	if err := os.WriteFile(yol, []byte("beton ve demir degisti"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Değişim zamanının farklı olduğundan emin ol.
	yeni := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(yol, yeni, yeni); err != nil {
		t.Fatal(err)
	}

	ozet3, err := Tara(context.Background(), ix, ay, veri, YeniIlerleme())
	if err != nil {
		t.Fatal(err)
	}
	if ozet3.Islenen != 1 {
		t.Errorf("değişen dosya sayısı = %d, beklenen 1", ozet3.Islenen)
	}
	ix.Oku(func(o indeks.Okuyucu) {
		if o.Df("demir") != 1 {
			t.Error("değişen içerik yeniden indekslenmedi")
		}
		if o.CanliSayisi() != 2 {
			t.Errorf("canlı = %d, beklenen 2 (eskisi tombstone olmalı)",
				o.CanliSayisi())
		}
	})
}

func TestTaraSilinenDosyayiDuser(t *testing.T) {
	kok := agacKur(t, map[string]string{"a.txt": "beton", "b.txt": "cimento"})
	ay := ayarKur(kok)
	veri := t.TempDir()
	ix := indeks.Yeni()

	if _, err := Tara(context.Background(), ix, ay, veri, YeniIlerleme()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(kok, "b.txt")); err != nil {
		t.Fatal(err)
	}

	ozet, err := Tara(context.Background(), ix, ay, veri, YeniIlerleme())
	if err != nil {
		t.Fatal(err)
	}
	if ozet.Silinen != 1 {
		t.Errorf("silinen = %d, beklenen 1", ozet.Silinen)
	}
	ix.Oku(func(o indeks.Okuyucu) {
		if o.CanliSayisi() != 1 {
			t.Errorf("canlı = %d, beklenen 1", o.CanliSayisi())
		}
	})
}

// Taranmayan bir kökteki dosyalar silinmiş sayılmamalı.
func TestTaraBaskaKokuEtkilemez(t *testing.T) {
	kokA := agacKur(t, map[string]string{"a.txt": "beton"})
	kokB := agacKur(t, map[string]string{"b.txt": "cimento"})
	veri := t.TempDir()
	ix := indeks.Yeni()

	// İkisini birlikte tara.
	ikisi := ayarKur(kokA)
	ikisi.TaranacakDizinler = []string{kokA, kokB}
	if _, err := Tara(context.Background(), ix, ikisi, veri, YeniIlerleme()); err != nil {
		t.Fatal(err)
	}

	// Yalnızca A'yı tara: B'nin dosyası indekste kalmalı.
	ozet, err := Tara(context.Background(), ix, ayarKur(kokA), veri, YeniIlerleme())
	if err != nil {
		t.Fatal(err)
	}
	if ozet.Silinen != 0 {
		t.Errorf("silinen = %d, taranmayan kök etkilenmemeliydi", ozet.Silinen)
	}
	ix.Oku(func(o indeks.Okuyucu) {
		if o.CanliSayisi() != 2 {
			t.Errorf("canlı = %d, beklenen 2", o.CanliSayisi())
		}
	})
}

// Bozuk ve ikili dosyalar taramayı öldürmemeli; en kritik değişmez bu.
func TestTaraBozukDosyalarTaramayiOldurmez(t *testing.T) {
	kok := t.TempDir()
	yaz := func(ad string, b []byte) {
		if err := os.WriteFile(filepath.Join(kok, ad), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	yaz("saglam.txt", []byte("beton dokumu tamamlandi"))
	yaz("ikili.txt", []byte{0x00, 0x01, 0xFF, 0x00, 0x02})
	yaz("bozuk.pdf", []byte("%PDF-1.4 kesilmis"))
	yaz("bozuk.docx", []byte("bu bir zip degil"))
	yaz("bozuk.xlsx", []byte{0x50, 0x4B, 0x03, 0x04, 0xFF, 0xFF})
	yaz("eski.doc", []byte{0xD0, 0xCF, 0x11, 0xE0})

	ix := indeks.Yeni()
	ozet, err := Tara(context.Background(), ix, ayarKur(kok), t.TempDir(), YeniIlerleme())
	if err != nil {
		t.Fatalf("Tara bozuk dosyalarda hata verdi: %v", err)
	}

	if ozet.Islenen != 6 {
		t.Errorf("işlenen = %d, beklenen 6 (hepsi en az adıyla indekslenmeli)",
			ozet.Islenen)
	}
	ix.Oku(func(o indeks.Okuyucu) {
		if o.Df("beton") != 1 {
			t.Error("sağlam dosya bozuk dosyalar yüzünden indekslenmemiş")
		}
		if o.CanliSayisi() != 6 {
			t.Errorf("canlı = %d, beklenen 6", o.CanliSayisi())
		}
	})
}

func TestTaraIptalEdilebilir(t *testing.T) {
	// İptalin yakalanabilmesi için yeterince dosya üretiyoruz.
	dosyalar := make(map[string]string, 400)
	for i := 0; i < 400; i++ {
		dosyalar[filepath.Join("alt", "d"+strings.Repeat("x", i%20)+
			string(rune('a'+i%26))+string(rune('a'+i/26))+".txt")] =
			strings.Repeat("beton cimento demir kalip iskele ", 200)
	}
	kok := agacKur(t, dosyalar)

	ctx, iptal := context.WithCancel(context.Background())
	il := YeniIlerleme()

	bitti := make(chan error, 1)
	go func() {
		_, err := Tara(ctx, indeks.Yeni(), ayarKur(kok), t.TempDir(), il)
		bitti <- err
	}()

	// Tarama başladıktan sonra iptal et.
	basladi := time.Now()
	for il.Islenen.Load() == 0 && time.Since(basladi) < 5*time.Second {
		time.Sleep(2 * time.Millisecond)
	}
	iptal()

	select {
	case err := <-bitti:
		if err == nil {
			// Tarama iptalden önce bitmiş olabilir; bu bir hata değil.
			t.Log("tarama iptalden önce tamamlandı")
			return
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("hata = %v, iptal hatası beklenirdi", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("iptal sonrası tarama durmadı")
	}
}

func TestTaraDizinYokIseHata(t *testing.T) {
	ay := ayarlar.Varsayilan()
	ay.TaranacakDizinler = nil
	_, err := Tara(context.Background(), indeks.Yeni(), ay, t.TempDir(), YeniIlerleme())
	if err == nil {
		t.Error("dizin belirtilmediğinde hata beklenirdi")
	}
}

// Okunamayan bir klasör tüm yürüyüşü sonlandırmamalı.
func TestYuruyusHatasiKlasorAtlar(t *testing.T) {
	if h := yuruyusHatasi(nil, os.ErrPermission); h != nil {
		t.Errorf("dosya hatası = %v, nil beklenirdi (görmezden gelinmeli)", h)
	}
}

// ---------------------------------------------------------------- iş

func TestIsCiftBaslatmaReddedilir(t *testing.T) {
	kok := agacKur(t, map[string]string{"a.txt": strings.Repeat("beton ", 5000)})
	is := YeniIs()
	ix := indeks.Yeni()
	ay := ayarKur(kok)
	veri := t.TempDir()

	if err := is.Basla(ix, ay, veri); err != nil {
		t.Fatalf("ilk başlatma: %v", err)
	}
	if is.Calisiyor() {
		if err := is.Basla(ix, ay, veri); err != ErrTaramaSuruyor {
			t.Errorf("ikinci başlatma hatası = %v, beklenen ErrTaramaSuruyor", err)
		}
	}
	for is.Calisiyor() {
		time.Sleep(5 * time.Millisecond)
	}
	if d := is.Durum(); d.Durum != AsamaTamamlandi {
		t.Errorf("durum = %q, beklenen %q", d.Durum, AsamaTamamlandi)
	}
}

func TestIsBostaIptalHataDoner(t *testing.T) {
	if err := YeniIs().Iptal(); err != ErrTaramaYok {
		t.Errorf("hata = %v, beklenen ErrTaramaYok", err)
	}
}

func TestIsDurumBostaAlanlari(t *testing.T) {
	d := YeniIs().Durum()
	if d.Calisiyor {
		t.Error("boşta iş çalışıyor görünüyor")
	}
	if d.Durum != AsamaBosta {
		t.Errorf("durum = %q, beklenen %q", d.Durum, AsamaBosta)
	}
	if d.Asama == "" {
		t.Error("aşama açıklaması boş")
	}
}

// Artımlı taramada dosyaların çoğu "atlanan" olur; yüzde hesabı onları
// saymazsa ilerleme çubuğu hiç dolmaz.
func TestDurumYuzdesiAtlananlariSayar(t *testing.T) {
	il := YeniIlerleme()
	il.Toplam.Store(100)
	il.Islenen.Store(10)
	il.Atlanan.Store(90)

	is := &Is{ilerleme: il, baslangic: time.Now()}
	if d := is.Durum(); d.Yuzde != 100 {
		t.Errorf("yüzde = %d, beklenen 100", d.Yuzde)
	}
}
