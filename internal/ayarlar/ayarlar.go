// Paket ayarlar, uygulama ayarlarını %APPDATA%\ArsivIndeks\ayarlar.json
// dosyasında saklar ve doğrular.
package ayarlar

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"arsiv-indeks/internal/atomik"
)

// UygulamaAdi veri dizini adı olarak kullanılır.
const UygulamaAdi = "ArsivIndeks"

// DosyaAdi ayar dosyasının adı.
const DosyaAdi = "ayarlar.json"

// Ayarlar kullanıcının yapılandırmasıdır.
type Ayarlar struct {
	// TaranacakDizinler birden çok arşiv kökü destekler.
	TaranacakDizinler []string `json:"taranacakDizinler"`
	HaricKlasorler    []string `json:"haricKlasorler"`
	HaricUzantilar    []string `json:"haricUzantilar"`

	// IcerikCikarma kapalıysa yalnızca dosya adı ve metadata indekslenir.
	IcerikCikarma    bool     `json:"icerikCikarma"`
	IcerikUzantilari []string `json:"icerikUzantilari"`

	MaksDosyaBoyutuMB  int `json:"maksDosyaBoyutuMB"`
	MaksMetinKarakter  int `json:"maksMetinKarakter"`
	IsciSayisi         int `json:"isciSayisi"` // 0 = min(NumCPU, 8)
	SayfaBasiSonuc     int `json:"sayfaBasiSonuc"`
	DokumanBelirtecCap int `json:"dokumanBelirtecCap"`

	SonTarama string `json:"sonTarama"`
}

// Varsayilan makul başlangıç ayarlarını döner.
func Varsayilan() Ayarlar {
	return Ayarlar{
		TaranacakDizinler: nil,
		HaricKlasorler: []string{
			"$RECYCLE.BIN", "System Volume Information", "$WinREAgent",
			".git", ".svn", "node_modules", "__pycache__", ".venv",
		},
		HaricUzantilar: []string{
			".exe", ".dll", ".sys", ".msi", ".iso", ".img", ".vhd", ".vhdx",
			".zip", ".rar", ".7z", ".gz", ".tar", ".cab",
			".mp4", ".mkv", ".avi", ".mov", ".mp3", ".wav", ".flac",
			".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tif", ".tiff", ".webp",
			".pdb", ".obj", ".lib", ".so", ".dylib",
		},
		IcerikCikarma: true,
		IcerikUzantilari: []string{
			// Düz metin ve veri
			".txt", ".md", ".csv", ".tsv", ".log", ".json", ".xml", ".yaml",
			".yml", ".ini", ".cfg", ".conf", ".sql", ".htm", ".html", ".rtf",
			// Kod
			".go", ".py", ".js", ".ts", ".java", ".cs", ".c", ".h", ".cpp",
			".hpp", ".rb", ".php", ".sh", ".ps1", ".bat", ".vb",
			// Office: archive/zip + encoding/xml ile, bağımlılıksız
			".docx", ".xlsx", ".pptx", ".docm", ".xlsm", ".pptm",
			// PDF
			".pdf",
		},
		MaksDosyaBoyutuMB:  64,
		MaksMetinKarakter:  1_000_000,
		IsciSayisi:         0,
		SayfaBasiSonuc:     25,
		DokumanBelirtecCap: 2000,
	}
}

// VarsayilanVeriDizini %APPDATA%\ArsivIndeks yolunu döner.
//
// Exe yanına yazmak yanlış olurdu: `go run` binary'yi geçici bir dizine
// koyar ve Program Files yazılabilir değildir.
func VarsayilanVeriDizini() (string, error) {
	kok, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("kullanıcı yapılandırma dizini bulunamadı: %w", err)
	}
	return filepath.Join(kok, UygulamaAdi), nil
}

// Yukle ayar dosyasını okur. Dosya yoksa bu bir hata değildir;
// varsayılanlar döner (ilk çalıştırma).
func Yukle(veriDizini string) (Ayarlar, error) {
	a := Varsayilan()
	b, err := os.ReadFile(filepath.Join(veriDizini, DosyaAdi))
	if err != nil {
		if os.IsNotExist(err) {
			return a, nil
		}
		return a, fmt.Errorf("ayarlar okunamadı: %w", err)
	}
	// Varsayılanların üzerine yazıyoruz, böylece sonradan eklenen bir alan
	// eski bir ayar dosyasında sıfır değer almaz.
	if err := json.Unmarshal(b, &a); err != nil {
		return Varsayilan(), fmt.Errorf("ayarlar dosyası bozuk: %w", err)
	}
	a.Duzelt()
	return a, nil
}

// Kaydet ayarları atomik olarak yazar.
func (a Ayarlar) Kaydet(veriDizini string) error {
	a.Duzelt()
	return atomik.Yaz(filepath.Join(veriDizini, DosyaAdi), func(w io.Writer) error {
		kodlayici := json.NewEncoder(w)
		kodlayici.SetIndent("", "  ")
		return kodlayici.Encode(a)
	})
}

// Duzelt sayısal alanları makul aralığa çeker, uzantı ve dizin listelerini
// normalleştirir. Hem yükleme hem kaydetme sırasında çalışır.
func (a *Ayarlar) Duzelt() {
	v := Varsayilan()
	if a.MaksDosyaBoyutuMB <= 0 || a.MaksDosyaBoyutuMB > 2048 {
		a.MaksDosyaBoyutuMB = v.MaksDosyaBoyutuMB
	}
	if a.MaksMetinKarakter <= 0 || a.MaksMetinKarakter > 50_000_000 {
		a.MaksMetinKarakter = v.MaksMetinKarakter
	}
	if a.IsciSayisi < 0 || a.IsciSayisi > 64 {
		a.IsciSayisi = 0
	}
	if a.SayfaBasiSonuc < 5 || a.SayfaBasiSonuc > 200 {
		a.SayfaBasiSonuc = v.SayfaBasiSonuc
	}
	if a.DokumanBelirtecCap < 100 || a.DokumanBelirtecCap > 100_000 {
		a.DokumanBelirtecCap = v.DokumanBelirtecCap
	}
	a.HaricUzantilar = uzantilariDuzelt(a.HaricUzantilar)
	a.IcerikUzantilari = uzantilariDuzelt(a.IcerikUzantilari)
	a.TaranacakDizinler = dizinleriDuzelt(a.TaranacakDizinler)
}

// IsciAdedi kullanılacak işçi goroutine sayısını hesaplar.
//
// Çıkarım, inflate + XML ayrıştırma (CPU) ile disk okumanın karışımıdır;
// tek bir diskte 8'den fazla işçi kafa gezinmesi yüzünden ters teper.
func (a Ayarlar) IsciAdedi() int {
	if a.IsciSayisi > 0 {
		return a.IsciSayisi
	}
	return min(runtime.NumCPU(), 8)
}

// MaksDosyaBayt boyut sınırını bayta çevirir.
func (a Ayarlar) MaksDosyaBayt() int64 {
	return int64(a.MaksDosyaBoyutuMB) << 20
}

// HaricKlasorMu bir klasör adının atlanacağını söyler. Yol değil ad
// karşılaştırması yapar, büyük/küçük harf duyarsız.
func (a Ayarlar) HaricKlasorMu(ad string) bool {
	for _, h := range a.HaricKlasorler {
		if strings.EqualFold(ad, h) {
			return true
		}
	}
	return false
}

// HaricUzantiMi dosyanın hiç indekslenmeyeceğini söyler.
func (a Ayarlar) HaricUzantiMi(uzanti string) bool {
	uzanti = strings.ToLower(uzanti)
	for _, h := range a.HaricUzantilar {
		if uzanti == h {
			return true
		}
	}
	return false
}

// IcerikOkunacakMi dosyanın içeriğinin çıkarılacağını söyler. Hayır ise
// dosya yine indekslenir, ama yalnızca adı ve metadata'sıyla.
func (a Ayarlar) IcerikOkunacakMi(uzanti string) bool {
	if !a.IcerikCikarma {
		return false
	}
	uzanti = strings.ToLower(uzanti)
	for _, h := range a.IcerikUzantilari {
		if uzanti == h {
			return true
		}
	}
	return false
}

// Dogrula kullanıcıya gösterilecek Türkçe hata metinleri döner. Boş dilim,
// ayarların geçerli olduğu anlamına gelir.
func (a Ayarlar) Dogrula() []string {
	var hatalar []string
	if len(a.TaranacakDizinler) == 0 {
		hatalar = append(hatalar, "En az bir taranacak dizin girmelisiniz.")
	}
	for _, d := range a.TaranacakDizinler {
		if h := DizinDogrula(d); h != "" {
			hatalar = append(hatalar, h)
		}
	}
	if a.IcerikCikarma && len(a.IcerikUzantilari) == 0 {
		hatalar = append(hatalar,
			"İçerik indeksleme açık ama hiç içerik uzantısı seçilmemiş.")
	}
	return hatalar
}

// DizinDogrula tek bir dizini kontrol eder ve Türkçe hata metni döner
// (geçerliyse boş dize).
func DizinDogrula(yol string) string {
	if strings.TrimSpace(yol) == "" {
		return "Boş dizin yolu girildi."
	}
	mutlak, err := filepath.Abs(yol)
	if err != nil {
		return fmt.Sprintf("%q geçerli bir yol değil.", yol)
	}
	bilgi, err := os.Stat(mutlak)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("%s bulunamadı.", mutlak)
		}
		return fmt.Sprintf("%s okunamadı: %v", mutlak, err)
	}
	if !bilgi.IsDir() {
		return fmt.Sprintf("%s bir klasör değil, dosya.", mutlak)
	}
	// Okunabilirliği deneyerek anlıyoruz; Windows'ta izinleri Stat çıkışından
	// çıkarmak güvenilir değil.
	d, err := os.Open(mutlak)
	if err != nil {
		return fmt.Sprintf("%s açılamadı (izin sorunu olabilir): %v", mutlak, err)
	}
	defer d.Close()
	if _, err := d.ReadDir(1); err != nil && err != io.EOF {
		return fmt.Sprintf("%s içeriği listelenemedi: %v", mutlak, err)
	}
	return ""
}

func uzantilariDuzelt(liste []string) []string {
	gorulen := make(map[string]bool, len(liste))
	cikti := make([]string, 0, len(liste))
	for _, u := range liste {
		u = strings.ToLower(strings.TrimSpace(u))
		if u == "" {
			continue
		}
		if !strings.HasPrefix(u, ".") {
			u = "." + u
		}
		if gorulen[u] {
			continue
		}
		gorulen[u] = true
		cikti = append(cikti, u)
	}
	return cikti
}

func dizinleriDuzelt(liste []string) []string {
	gorulen := make(map[string]bool, len(liste))
	cikti := make([]string, 0, len(liste))
	for _, d := range liste {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		// Uzun yol desteği için mutlak ve temiz yol şart: Go'nun os katmanı
		// \\?\ düzeltmesini yalnızca mutlak+temiz yollara uygular.
		if mutlak, err := filepath.Abs(d); err == nil {
			d = mutlak
		}
		anahtar := strings.ToLower(d)
		if gorulen[anahtar] {
			continue
		}
		gorulen[anahtar] = true
		cikti = append(cikti, d)
	}
	return cikti
}
