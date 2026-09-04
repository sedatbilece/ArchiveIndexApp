// Paket cikarim, dosyalardan aranabilir düz metin çıkarır.
//
// Buradaki en kritik değişmez: GÜVENİLMEZ BİR DOSYANIN ÇIKARIMI TARAMAYI ASLA
// ÖLDÜRMEZ. Arşivde bozuk PDF, uzantısı yanlış ikili dosya, zip bombası ve
// şifreli doküman bulunacağı kesindir. Her çıkarım recover() içinde, boyut
// sınırlı ve iptal edilebilir çalışır.
package cikarim

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"arsiv-indeks/internal/metin"
)

// Durum kodları. Sonuç satırında kullanıcıya etiket olarak gösterilir; bir
// dosyanın içeriğinin NEDEN aranamadığını bilmek kullanıcı için kritiktir.
const (
	DurumTam         = "tam"          // içerik tamamen okundu
	DurumAdOnly      = "ad-only"      // içerik okunmadı (ayarlarda kapsam dışı)
	DurumBuyuk       = "buyuk"        // boyut sınırını aşıyor
	DurumIkili       = "ikili"        // metin değil, ikili veri
	DurumBozuk       = "bozuk"        // çıkarım hata verdi
	DurumEskiFormat  = "eski-format"  // .doc/.xls/.ppt (OLE2)
	DurumPdfMetinsiz = "pdf-metinsiz" // taranmış PDF, metin katmanı yok
	DurumKirpildi    = "kirpildi"     // metin sınırında kesildi
	DurumBulut       = "bulut"        // OneDrive yer tutucu (tarayıcı atar)
	DurumKilitli     = "kilitli"      // başka süreç kilitlemiş (tarayıcı atar)
)

// DurumAciklama durum kodunun kullanıcıya gösterilecek Türkçe karşılığı.
func DurumAciklama(durum string) string {
	switch durum {
	case DurumTam:
		return ""
	case DurumAdOnly:
		return "yalnızca ad indeksli"
	case DurumBuyuk:
		return "çok büyük — içerik okunmadı"
	case DurumIkili:
		return "ikili dosya — içerik okunmadı"
	case DurumBozuk:
		return "okunamadı"
	case DurumEskiFormat:
		return "eski Office biçimi — içerik okunmadı"
	case DurumPdfMetinsiz:
		return "taranmış PDF — içerik aranamaz"
	case DurumKirpildi:
		return "içerik kısmen indeksli"
	case DurumBulut:
		return "bulutta (indirilmedi)"
	case DurumKilitli:
		return "dosya kilitli"
	}
	return durum
}

// ErrMetinYok içerik çıkarılamadığını ama bunun bir hata olmadığını belirtir
// (örn. taranmış PDF).
var ErrMetinYok = errors.New("metin katmanı bulunamadı")

// ErrEskiFormat OLE2 tabanlı eski Office biçimleri için döner.
var ErrEskiFormat = errors.New("eski Office biçimi desteklenmiyor")

// ErrIkili dosyanın uzantısına rağmen metin değil ikili veri olduğunu belirtir
// (örn. ".txt" adı verilmiş bir disk imajı).
var ErrIkili = errors.New("ikili dosya")

// Secenekler çıkarım sınırlarıdır.
type Secenekler struct {
	// MaksKarakter çıkarılan metnin üst sınırı. Aşılırsa metin kırpılır.
	MaksKarakter int
}

func (s Secenekler) maksKarakter() int {
	if s.MaksKarakter <= 0 {
		return 1_000_000
	}
	return s.MaksKarakter
}

// Cikarici tek bir dosya biçiminden metin çıkarır.
//
// io.ReaderAt istiyoruz çünkü hem archive/zip hem PDF okuyucusu rastgele
// erişim gerektirir; *os.File ve bytes.Reader ikisi de bunu sağlar.
type Cikarici interface {
	// Uzantilar bu çıkarıcının ilgilendiği uzantıları döner (nokta dahil).
	Uzantilar() []string
	// Cikar metni çıkarır. Metin yoksa ErrMetinYok döner.
	Cikar(ctx context.Context, r io.ReaderAt, boyut int64, sec Secenekler) (string, error)
}

// cikaricilar uzantı -> çıkarıcı haritası. init() içinde her çıkarıcı kendini
// kaydeder; böylece kapsamı tek bakışta görebiliyoruz ve dağıtım O(1).
var cikaricilar = map[string]Cikarici{}

func kaydet(c Cikarici) {
	for _, u := range c.Uzantilar() {
		cikaricilar[strings.ToLower(u)] = c
	}
}

func init() {
	kaydet(duzMetin{})
	kaydet(ooxmlBelge{})  // docx, docm
	kaydet(ooxmlSunum{})  // pptx, pptm
	kaydet(ooxmlTablo{})  // xlsx, xlsm
	kaydet(pdfCikarici{}) // pdf
	kaydet(eskiOffice{})  // doc, xls, ppt
}

// Destekli uzantının bir çıkarıcısı olup olmadığını söyler.
func Destekli(uzanti string) bool {
	_, v := cikaricilar[strings.ToLower(uzanti)]
	return v
}

// EskiOfficeMi uzantının OLE2 tabanlı eski Office biçimi olup olmadığını
// söyler.
//
// Tarayıcı bunu, içeriği okunmayan bir dosyaya genel "yalnızca ad indeksli"
// etiketi yerine daha bilgilendirici "eski Office biçimi" etiketini vermek
// için kullanır. Bu uzantıları ayarlardaki "içeriği okunacak uzantılar"
// listesine koymak yanıltıcı olurdu: içerikleri gerçekten okunmuyor.
func EskiOfficeMi(uzanti string) bool {
	switch strings.ToLower(uzanti) {
	case ".doc", ".xls", ".ppt":
		return true
	}
	return false
}

// GuvenliCikar dosyayı açar, uzantısına göre çıkarıcıya dağıtır ve metni
// döner. Hiçbir koşulda panik yaymaz.
//
// Dönen durum, dosyanın içeriğinin aranabilir olup olmadığını anlatır ve
// doğrudan kullanıcıya gösterilir.
func GuvenliCikar(ctx context.Context, yol string, sec Secenekler) (icerik string, durum string) {
	// Çıkarıcılar üçüncü parti kod (PDF) veya güvenilmez veri üzerinde
	// çalışıyor; panik yakalamak pazarlık konusu değil.
	defer func() {
		if p := recover(); p != nil {
			icerik = ""
			durum = DurumBozuk
		}
	}()

	uzanti := strings.ToLower(filepath.Ext(yol))
	c, v := cikaricilar[uzanti]
	if !v {
		return "", DurumAdOnly
	}

	f, err := os.Open(yol)
	if err != nil {
		// Kilitli dosya, silinmiş dosya, izin reddi... hiçbiri ölümcül değil.
		return "", DurumKilitli
	}
	defer f.Close()

	bilgi, err := f.Stat()
	if err != nil {
		return "", DurumBozuk
	}

	metinIcerik, err := c.Cikar(ctx, f, bilgi.Size(), sec)
	switch {
	case errors.Is(err, ErrEskiFormat):
		return "", DurumEskiFormat
	case errors.Is(err, ErrIkili):
		return "", DurumIkili
	case errors.Is(err, ErrMetinYok):
		if uzanti == ".pdf" {
			return "", DurumPdfMetinsiz
		}
		return "", DurumAdOnly
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "", DurumBozuk
	case err != nil:
		return "", DurumBozuk
	}

	if strings.TrimSpace(metinIcerik) == "" {
		if uzanti == ".pdf" {
			return "", DurumPdfMetinsiz
		}
		return "", DurumTam
	}

	maks := sec.maksKarakter()
	if len(metinIcerik) > maks {
		return kirp(metinIcerik, maks), DurumKirpildi
	}
	return metinIcerik, DurumTam
}

// kirp metni bayt sınırında değil, rune sınırında keser.
func kirp(s string, maks int) string {
	if len(s) <= maks {
		return s
	}
	kesim := maks
	for kesim > 0 && !utf8BaslangicMi(s[kesim]) {
		kesim--
	}
	return s[:kesim]
}

func utf8BaslangicMi(b byte) bool { return b&0xC0 != 0x80 }

// ---------------------------------------------------------------- düz metin

type duzMetin struct{}

func (duzMetin) Uzantilar() []string {
	return []string{
		".txt", ".md", ".csv", ".tsv", ".log", ".json", ".xml", ".yaml",
		".yml", ".ini", ".cfg", ".conf", ".sql", ".htm", ".html", ".rtf",
		".go", ".py", ".js", ".ts", ".java", ".cs", ".c", ".h", ".cpp",
		".hpp", ".rb", ".php", ".sh", ".ps1", ".bat", ".vb", ".css",
	}
}

func (duzMetin) Cikar(ctx context.Context, r io.ReaderAt, boyut int64, sec Secenekler) (string, error) {
	// Sınırdan biraz fazla okuyoruz ki kırpma durumunu tespit edebilelim.
	limit := int64(sec.maksKarakter()) + 1
	if boyut < limit {
		limit = boyut
	}
	b := make([]byte, limit)
	n, err := r.ReadAt(b, 0)
	if err != nil && err != io.EOF && n == 0 {
		return "", fmt.Errorf("okunamadı: %w", err)
	}
	b = b[:n]
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// İkili sezgisi çıkarımdan ÖNCE: ".txt" adı verilmiş bir disk imajını
	// indekslemek sözlüğü çöple doldurur.
	if metin.IkiliMi(b) {
		return "", ErrIkili
	}
	return metin.Coz(b), nil
}

// ---------------------------------------------------------------- eski Office

// eskiOffice OLE2 bileşik ikili biçimleri (.doc/.xls/.ppt) için yer tutucudur.
//
// İçeriklerini okumuyoruz. Gerçek BIFF/WordDocument-stream ayrıştırması
// stdlib desteği olmayan çok haftalık bir iştir; ASCII/UTF-16 dizi kazımak ise
// indeksi zehirleyen çöp belirteçler üretir. Dosya yine adıyla indekslenir ve
// arayüzde "eski Office biçimi" etiketi gösterilir.
type eskiOffice struct{}

func (eskiOffice) Uzantilar() []string { return []string{".doc", ".xls", ".ppt"} }

func (eskiOffice) Cikar(context.Context, io.ReaderAt, int64, Secenekler) (string, error) {
	return "", ErrEskiFormat
}
