// Arşiv İndeks: arşivlenmiş dosyaları adına, içeriğine, uzantısına, tarihine
// ve bulunduğu klasöre göre aramak için yerel bir web uygulaması.
//
// Kullanım:
//
//	go run . -gelistirme          geliştirme (şablonlar diskten okunur)
//	go build -o arsiv-indeks.exe .
//
// Bayraklar: -adres, -veri, -gelistirme
package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"arsiv-indeks/internal/ayarlar"
	"arsiv-indeks/internal/indeks"
	"arsiv-indeks/internal/tarayici"
)

// Şablonlar ve statik dosyalar binary'ye gömülür.
//
// Bunu embed ile yapmamızın sebebi: göreli "templates/" yolu uygulamanın
// proje kökünden çalıştırılmasını zorunlu kılar. Bu araç her gün tek bir
// .exe olarak, herhangi bir dizinden çalıştırılacak. -gelistirme bayrağı
// düzenle-yenile döngüsü için diskten okumaya geri döner.
//
//go:embed templates static
var gomulu embed.FS

//go:embed version.txt
var versiyonGomulu string

// SayfaVerisi tüm sayfaların paylaştığı görünüm modelidir.
type SayfaVerisi struct {
	Aktif    string // gezinti vurgusu için
	Baslik   string
	Hata     string
	Basari   string
	Jeton    string
	Versiyon string
	Veri     any // sayfaya özel veri
}

// Uygulama sunucu durumunu tutar.
type Uygulama struct {
	ix         *indeks.Indeks
	is         *tarayici.Is
	veriDizini string
	gelistirme bool
	jeton      string

	// indeksHata açılışta indeks yüklenemediyse kullanıcıya gösterilir.
	indeksHata string

	ayarKilit sync.RWMutex
	ayar      ayarlar.Ayarlar

	sablonlar map[string]*template.Template
	statik    http.Handler

	// parcacikSinir eşzamanlı parçacık çıkarımını sınırlar: 20 sonuç
	// satırının aynı anda büyük PDF açması sunucuyu dize eder.
	parcacikSinir chan struct{}
	onbellek      *metinOnbellek
}

func main() {
	adres := flag.String("adres", "127.0.0.1:8080",
		"dinlenecek adres (yalnızca yerel arayüzde dinleyin)")
	veriBayrak := flag.String("veri", "",
		"veri dizini (boşsa %APPDATA%\\ArsivIndeks)")
	gelistirme := flag.Bool("gelistirme", false,
		"şablonları ve statik dosyaları diskten oku")
	flag.Parse()

	veriDizini := *veriBayrak
	if veriDizini == "" {
		v, err := ayarlar.VarsayilanVeriDizini()
		if err != nil {
			log.Fatalf("veri dizini belirlenemedi: %v", err)
		}
		veriDizini = v
	}
	if err := os.MkdirAll(veriDizini, 0o755); err != nil {
		log.Fatalf("veri dizini oluşturulamadı: %v", err)
	}
	gunlukAyarla(veriDizini)

	u, err := yeniUygulama(veriDizini, *gelistirme)
	if err != nil {
		log.Fatalf("uygulama başlatılamadı: %v", err)
	}

	log.Printf("Arşiv İndeks çalışıyor:  http://%s", *adres)
	log.Printf("Veri dizini:             %s", veriDizini)
	u.ix.Oku(func(o indeks.Okuyucu) {
		log.Printf("İndeks:                  %d dosya, %d terim",
			o.CanliSayisi(), o.TerimSayisi())
	})
	if u.indeksHata != "" {
		log.Printf("UYARI: %s", u.indeksHata)
	}

	sunucu := &http.Server{
		Addr:              *adres,
		Handler:           u.rotalar(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := sunucu.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// gunlukAyarla loglari <veriDizini>/arsiv-indeks.log dosyasina yonlendirir.
//
// -H=windowsgui ile derlenen bir binary'de konsol yoktur; os.Stderr yazma
// hatasi doner. sessizYazici bu hatayi yutar ki dosyaya yazma, stderr'e
// yazmanin basarisiz olmasindan etkilenmesin (io.MultiWriter ilk hatada
// sonraki yazicilara hic ugramaz). Elle terminalden calistirildiginda cikti
// yine ekrana da duser.
func gunlukAyarla(veriDizini string) {
	yol := filepath.Join(veriDizini, "arsiv-indeks.log")
	if bilgi, err := os.Stat(yol); err == nil && bilgi.Size() > 2*1024*1024 {
		eski := filepath.Join(veriDizini, "arsiv-indeks.log.eski")
		os.Remove(eski)
		os.Rename(yol, eski)
	}
	dosya, err := os.OpenFile(yol, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(sessizYazici{os.Stderr}, dosya))
}

// sessizYazici bir yazicinin hatalarini yutar.
type sessizYazici struct{ w io.Writer }

func (s sessizYazici) Write(p []byte) (int, error) {
	s.w.Write(p)
	return len(p), nil
}

func yeniUygulama(veriDizini string, gelistirme bool) (*Uygulama, error) {
	ay, ayarHata := ayarlar.Yukle(veriDizini)

	ix, ixHata := indeks.Yukle(veriDizini)

	u := &Uygulama{
		ix:            ix,
		is:            tarayici.YeniIs(),
		veriDizini:    veriDizini,
		gelistirme:    gelistirme,
		jeton:         jetonUret(),
		ayar:          ay,
		parcacikSinir: make(chan struct{}, 4),
		onbellek:      yeniMetinOnbellek(300),
	}
	if ixHata != nil {
		u.indeksHata = ixHata.Error()
	} else if ayarHata != nil {
		u.indeksHata = ayarHata.Error()
	}

	if err := u.sablonlariYukle(); err != nil {
		return nil, err
	}
	u.statikHazirla()

	// Açılışta yetim dokümanları düşür.
	//
	// Kullanıcı ayarlar.json dosyasını uygulama kapalıyken elle düzenleyip
	// bir dizini çıkarmış olabilir. Temizlemezsek o dizinin dosyaları arama
	// sonuçlarında görünür ama açılamaz (GuvenliYol o kökü tanımaz).
	if n := ix.KoklerDisindakileriSil(ay.TaranacakDizinler); n > 0 {
		log.Printf("artık taranmayan dizinlerdeki %d doküman indeksten düşürüldü", n)
		if ix.TombstoneOrani() > 0.20 {
			ix.Sikistir()
		}
		if err := ix.Kaydet(veriDizini); err != nil {
			log.Printf("indeks kaydedilemedi: %v", err)
		}
	}

	return u, nil
}

// ---------------------------------------------------------------- şablonlar

// sayfalar her sayfa için şablon dosyası adı.
var sayfalar = []string{"arama", "ayarlar"}

func (u *Uygulama) sablonlariYukle() error {
	u.sablonlar = make(map[string]*template.Template, len(sayfalar))
	for _, s := range sayfalar {
		t := template.New("layout.html").Funcs(sablonFonksiyonlari())
		var err error
		if u.gelistirme {
			t, err = t.ParseFiles(
				filepath.Join("templates", "layout.html"),
				filepath.Join("templates", s+".html"))
		} else {
			t, err = t.ParseFS(gomulu,
				"templates/layout.html", "templates/"+s+".html")
		}
		if err != nil {
			return fmt.Errorf("%s şablonu yüklenemedi: %w", s, err)
		}
		u.sablonlar[s] = t
	}
	return nil
}

// surum başlıkta gösterilen sürüm damgasını döner; -gelistirme modunda
// version.txt'yi diskten okur ki kur.ps1 çalıştırmadan da güncel görünsün.
func (u *Uygulama) surum() string {
	if u.gelistirme {
		if b, err := os.ReadFile("version.txt"); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return strings.TrimSpace(versiyonGomulu)
}

func (u *Uygulama) statikHazirla() {
	if u.gelistirme {
		u.statik = http.StripPrefix("/static/", http.FileServer(http.Dir("static")))
		return
	}
	alt, err := fs.Sub(gomulu, "static")
	if err != nil {
		log.Fatalf("gömülü statik dosyalar okunamadı: %v", err)
	}
	u.statik = http.StripPrefix("/static/", http.FileServer(http.FS(alt)))
}

// goster sayfayı render eder.
func (u *Uygulama) goster(w http.ResponseWriter, sayfa string, sv SayfaVerisi) {
	// Geliştirme modunda her istekte yeniden yükle: düzenle-yenile döngüsü.
	if u.gelistirme {
		if err := u.sablonlariYukle(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	t, v := u.sablonlar[sayfa]
	if !v {
		http.Error(w, "şablon bulunamadı: "+sayfa, http.StatusInternalServerError)
		return
	}
	sv.Aktif = sayfa
	sv.Jeton = u.jeton
	sv.Versiyon = u.surum()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", sv); err != nil {
		// Gövdenin bir kısmı yazılmış olabilir; en azından kaydet.
		log.Printf("şablon hatası (%s): %v", sayfa, err)
	}
}

func sablonFonksiyonlari() template.FuncMap {
	return template.FuncMap{
		"insanBoyut":   insanBoyut,
		"tarih":        tarihFormat,
		"tarihKisa":    func(t time.Time) string { return t.Format("2006-01-02") },
		"sayi":         sayiFormat,
		"sure":         sureFormat,
		"sorguDegis":   sorguDegis,
		"icerir":       icerir,
		"artiBir":      func(n int) int { return n + 1 },
		"eksiBir":      func(n int) int { return n - 1 },
		"sayfaAraligi": sayfaAraligi,
	}
}

// insanBoyut bayt sayısını okunabilir hale getirir (Türkçe ondalık virgülle).
func insanBoyut(b int64) string {
	const birim = 1024.0
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	deger := float64(b)
	adlar := []string{"KB", "MB", "GB", "TB"}
	for _, ad := range adlar {
		deger /= birim
		if deger < birim {
			return strings.Replace(fmt.Sprintf("%.1f %s", deger, ad), ".", ",", 1)
		}
	}
	return strings.Replace(fmt.Sprintf("%.1f PB", deger), ".", ",", 1)
}

func tarihFormat(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("02.01.2006 15:04")
}

// sayiFormat binlik ayırıcı olarak nokta kullanır (Türkçe biçim).
//
// Şablonlardan hem int hem int64 geliyor; her çağrıda dönüşüm yazmak yerine
// burada karşılıyoruz.
func sayiFormat(deger any) string {
	var n int64
	switch v := deger.(type) {
	case int:
		n = int64(v)
	case int32:
		n = int64(v)
	case int64:
		n = v
	case float64:
		n = int64(v)
	default:
		return fmt.Sprint(deger)
	}
	s := strconv.FormatInt(n, 10)
	eksi := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var parcalar []string
	for len(s) > 3 {
		parcalar = append([]string{s[len(s)-3:]}, parcalar...)
		s = s[:len(s)-3]
	}
	parcalar = append([]string{s}, parcalar...)
	cikti := strings.Join(parcalar, ".")
	if eksi {
		return "-" + cikti
	}
	return cikti
}

func sureFormat(d time.Duration) string {
	if d < time.Millisecond {
		return "1 ms altı"
	}
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return strings.Replace(fmt.Sprintf("%.1f sn", d.Seconds()), ".", ",", 1)
}

// sorguDegis mevcut sorgu parametrelerinin bir kopyasında tek bir anahtarı
// değiştirip sorgu dizesi döner. Sayfalama ve sıralama bağlantıları bunu
// kullanır; böylece filtreler korunur.
func sorguDegis(mevcut url.Values, anahtar, deger string) template.URL {
	yeni := url.Values{}
	for k, v := range mevcut {
		yeni[k] = append([]string(nil), v...)
	}
	if deger == "" {
		yeni.Del(anahtar)
	} else {
		yeni.Set(anahtar, deger)
	}
	return template.URL("?" + yeni.Encode())
}

func icerir(liste []string, deger string) bool {
	for _, s := range liste {
		if strings.EqualFold(s, deger) {
			return true
		}
	}
	return false
}

// sayfaAraligi sayfalama bağlantıları için gösterilecek sayfa numaralarını
// üretir (mevcut sayfanın çevresinde bir pencere).
func sayfaAraligi(mevcut, toplam int) []int {
	const pencere = 3
	bas := max(1, mevcut-pencere)
	son := min(toplam, mevcut+pencere)
	cikti := make([]int, 0, son-bas+1)
	for i := bas; i <= son; i++ {
		cikti = append(cikti, i)
	}
	return cikti
}

// ---------------------------------------------------------------- rotalar

func (u *Uygulama) rotalar() http.Handler {
	mux := http.NewServeMux()

	// Go 1.22+ metot desenleri: yanlış metot otomatik 405 döner.
	// "GET /{$}" yalnızca tam "/" yoluna eşleşir. Düz "GET /" yazsaydık
	// her yolu yakalar ve yanlış metotlar 405 yerine 404 dönerdi.
	mux.HandleFunc("GET /{$}", u.anaSayfaHandler)
	mux.HandleFunc("GET /arama", u.aramaHandler)
	mux.HandleFunc("GET /ayarlar", u.ayarlarHandler)
	mux.HandleFunc("POST /ayarlar", u.ayarlarKaydetHandler)

	mux.HandleFunc("GET /api/gozat", u.gozatHandler)
	mux.HandleFunc("GET /api/parcacik", u.parcacikHandler)

	mux.HandleFunc("POST /tarama/basla", u.taramaBaslaHandler)
	mux.HandleFunc("POST /tarama/iptal", u.taramaIptalHandler)
	mux.HandleFunc("GET /tarama/durum", u.taramaDurumHandler)

	mux.HandleFunc("GET /dosya/indir", u.dosyaIndirHandler)
	mux.HandleFunc("POST /dosya/klasorde-goster", u.klasordeGosterHandler)

	mux.Handle("GET /static/", u.statik)

	// Her şey yerel kontrolden geçer: DNS rebinding ve çapraz kaynaklı
	// istekler burada kesilir.
	return yerelKontrol(mux)
}

func (u *Uygulama) anaSayfaHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/arama", http.StatusFound)
}

// ---------------------------------------------------------------- yardımcı

// Ayar mevcut ayarların bir kopyasını döner.
func (u *Uygulama) Ayar() ayarlar.Ayarlar {
	u.ayarKilit.RLock()
	defer u.ayarKilit.RUnlock()
	return u.ayar
}

// AyarYaz ayarları değiştirir.
func (u *Uygulama) AyarYaz(a ayarlar.Ayarlar) {
	u.ayarKilit.Lock()
	defer u.ayarKilit.Unlock()
	u.ayar = a
}

// istekAdedi sayfa başına sonuç sayısını ayarlardan alır.
func (u *Uygulama) istekAdedi() int {
	return u.Ayar().SayfaBasiSonuc
}
