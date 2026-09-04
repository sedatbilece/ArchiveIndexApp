package cikarim

import (
	"context"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// pdfCikarici, PDF metin katmanını çıkarır.
//
// Bu uygulamanın TEK harici bağımlılığıdır: github.com/ledongthuc/pdf
// (rsc.io/pdf'in GetPlainText ekleyen çatalı). Saf Go, CGO yok — sistemde gcc
// olmadığı ve CGO kapalı olduğu için bu şart.
//
// Bilinen zayıflığı bozuk dosyalarda PANİK ATMASI. Sorun değil: GuvenliCikar
// her çıkarımı recover() içinde çalıştırıyor. Yine de burada da bir savunma
// katmanı bırakıyoruz ki hata mesajı anlamlı olsun.
//
// Değerlendirilen alternatifler:
//   - rsc.io/pdf: fiilen donmuş, yalnızca düşük seviyeli Content() veriyor;
//     yerleşim birleştirmesini elle yazmak gerekirdi.
//   - pdfcpu: bakımlı ama ağır ve katı doğrulayıcısı gerçek dünyanın özensiz
//     PDF'lerinde hata veriyor.
//
// Cikarici arayüzünün arkasında olduğu için ileride takas tek dosyalık iştir.
type pdfCikarici struct{}

func (pdfCikarici) Uzantilar() []string { return []string{".pdf"} }

func (pdfCikarici) Cikar(ctx context.Context, r io.ReaderAt, boyut int64, sec Secenekler) (icerik string, hata error) {
	defer func() {
		if p := recover(); p != nil {
			icerik = ""
			hata = ErrMetinYok
		}
	}()

	if err := ctx.Err(); err != nil {
		return "", err
	}

	okuyucu, err := pdf.NewReader(r, boyut)
	if err != nil {
		// Şifreli veya bozuk PDF. Dosya yine adıyla indekslenir.
		return "", ErrMetinYok
	}

	maks := sec.maksKarakter()
	var sb strings.Builder

	// Sayfa sayfa gidiyoruz (GetPlainText yerine): iptali kontrol edebiliyor,
	// sınıra ulaşınca durabiliyor ve tek bozuk sayfa tüm dokümanı düşürmüyor.
	sayfaSayisi := okuyucu.NumPage()
	for i := 1; i <= sayfaSayisi; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if sb.Len() >= maks {
			break
		}
		sayfaMetni := sayfaCikar(okuyucu, i)
		if sayfaMetni == "" {
			continue
		}
		sb.WriteString(sayfaMetni)
		sb.WriteByte('\n')
	}

	if strings.TrimSpace(sb.String()) == "" {
		// Taranmış/görüntü PDF: metin katmanı yok. OCR kapsam dışı.
		// Kullanıcıya "taranmış PDF — içerik aranamaz" etiketi gösterilecek.
		return "", ErrMetinYok
	}
	return sb.String(), nil
}

// sayfaCikar tek bir sayfanın metnini alır. Sayfa bozuksa boş döner; panik
// bu seviyede yakalanır ki bir sayfa yüzünden 300 sayfalık dokümanı
// kaybetmeyelim.
func sayfaCikar(okuyucu *pdf.Reader, no int) (metinIcerik string) {
	defer func() {
		if p := recover(); p != nil {
			metinIcerik = ""
		}
	}()

	sayfa := okuyucu.Page(no)
	if sayfa.V.IsNull() {
		return ""
	}
	s, err := sayfa.GetPlainText(nil)
	if err != nil {
		return ""
	}
	return s
}
