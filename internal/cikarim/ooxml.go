package cikarim

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// OOXML dosyaları (docx/xlsx/pptx) aslında ZIP+XML'dir. Harici kütüphane
// gerekmez: archive/zip + encoding/xml yeterli.
//
// Struct unmarshal KULLANMIYORUZ; token akışı üzerinden gidiyoruz. Böylece
// 200 MB'lık bir sunumda bile bellek sabit kalır.

const (
	// zipGirdiSiniri tek bir zip girdisinden okunacak azami bayt.
	zipGirdiSiniri = 64 << 20
	// zipToplamSiniri seçilen girdilerin açılmış toplam boyut sınırı.
	// Zip bombalarına karşı ilk savunma.
	zipToplamSiniri = 512 << 20
	// havuzSiniri xlsx paylaşılan dize havuzunun azami kayıt sayısı.
	havuzSiniri = 500_000
)

func zipAc(r io.ReaderAt, boyut int64) (*zip.Reader, error) {
	zr, err := zip.NewReader(r, boyut)
	if err != nil {
		return nil, fmt.Errorf("zip açılamadı: %w", err)
	}
	var toplam uint64
	for _, f := range zr.File {
		toplam += f.UncompressedSize64
		if toplam > zipToplamSiniri {
			return nil, fmt.Errorf("açılmış boyut sınırı aşıldı (zip bombası olabilir)")
		}
	}
	return zr, nil
}

// zipMetinTopla, sec'in seçtiği zip girdilerinden verilen etiketin metin
// içeriğini toplar.
//
// docx ve pptx aynı yardımcıyı paylaşır: ikisinde de metin <w:t>/<a:t>
// yani Local adı "t", paragraf sonu ise </w:p>/</a:p> yani Local adı "p".
func zipMetinTopla(ctx context.Context, zr *zip.Reader, sec func(ad string) bool, etiket string, maks int) (string, error) {
	var sb strings.Builder
	bulundu := false

	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if !sec(f.Name) {
			continue
		}
		bulundu = true
		if err := xmlMetinTopla(ctx, f, etiket, &sb, maks); err != nil {
			// Tek bozuk parça yüzünden tüm dokümanı atmıyoruz; elimizdekiyle
			// devam ediyoruz.
			continue
		}
		if sb.Len() >= maks {
			break
		}
	}

	if !bulundu {
		return "", ErrMetinYok
	}
	if strings.TrimSpace(sb.String()) == "" {
		return "", ErrMetinYok
	}
	return sb.String(), nil
}

func xmlMetinTopla(ctx context.Context, f *zip.File, etiket string, sb *strings.Builder, maks int) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	cz := xml.NewDecoder(&ctxOkuyucu{r: io.LimitReader(rc, zipGirdiSiniri), ctx: ctx})
	topla := false
	for {
		jeton, err := cz.Token()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		switch t := jeton.(type) {
		case xml.StartElement:
			// Local adı karşılaştırıyoruz, ad alanı önekini değil. Bu sayede
			// <w:t> ve <a:t> aynı kodla yakalanır.
			//
			// <w:instrText> (alan kodları) doğal olarak dışarıda kalır çünkü
			// Local adı "instrText"; okusaydık köprü çöpü indeksi kirletirdi.
			if t.Name.Local == etiket {
				topla = true
			}
		case xml.EndElement:
			if t.Name.Local == etiket {
				topla = false
			}
			if t.Name.Local == "p" {
				sb.WriteByte('\n')
			}
		case xml.CharData:
			if topla {
				sb.Write(t)
			}
		}
		if sb.Len() >= maks {
			return nil
		}
	}
}

// ctxOkuyucu okuma sırasında iptali kontrol eder. Patolojik bir dosya bir
// sonraki okumada kopar, dakikalarca beklemez.
type ctxOkuyucu struct {
	r   io.Reader
	ctx context.Context
}

func (c *ctxOkuyucu) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// ---------------------------------------------------------------- docx

type ooxmlBelge struct{}

func (ooxmlBelge) Uzantilar() []string { return []string{".docx", ".docm"} }

func (ooxmlBelge) Cikar(ctx context.Context, r io.ReaderAt, boyut int64, sec Secenekler) (string, error) {
	zr, err := zipAc(r, boyut)
	if err != nil {
		return "", err
	}
	return zipMetinTopla(ctx, zr, func(ad string) bool {
		switch ad {
		case "word/document.xml", "word/footnotes.xml", "word/endnotes.xml":
			return true
		}
		// Üstbilgi ve altbilgiler proje adı / doküman numarası taşır.
		return strings.HasPrefix(ad, "word/header") ||
			strings.HasPrefix(ad, "word/footer")
	}, "t", sec.maksKarakter())
}

// ---------------------------------------------------------------- pptx

type ooxmlSunum struct{}

func (ooxmlSunum) Uzantilar() []string { return []string{".pptx", ".pptm"} }

func (ooxmlSunum) Cikar(ctx context.Context, r io.ReaderAt, boyut int64, sec Secenekler) (string, error) {
	zr, err := zipAc(r, boyut)
	if err != nil {
		return "", err
	}
	return zipMetinTopla(ctx, zr, func(ad string) bool {
		return strings.HasPrefix(ad, "ppt/slides/slide") ||
			strings.HasPrefix(ad, "ppt/notesSlides/notesSlide")
	}, "t", sec.maksKarakter())
}

// ---------------------------------------------------------------- xlsx

type ooxmlTablo struct{}

func (ooxmlTablo) Uzantilar() []string { return []string{".xlsx", ".xlsm"} }

// Cikar xlsx için iki adımlıdır.
//
// xl/sharedStrings.xml tekilleştirilmiş bir dize havuzudur. Bir hücre
// <c t="s"><v>17</v></c> şeklindeyse <v> METİN DEĞİL, havuza indekstir.
// Havuzu okumadan sayfaları taramak sadece rakam listesi verir.
func (ooxmlTablo) Cikar(ctx context.Context, r io.ReaderAt, boyut int64, sec Secenekler) (string, error) {
	zr, err := zipAc(r, boyut)
	if err != nil {
		return "", err
	}
	maks := sec.maksKarakter()

	havuz, err := havuzOku(ctx, zr, maks)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	bulundu := false
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if !strings.HasPrefix(f.Name, "xl/worksheets/") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		bulundu = true
		if err := sayfaOku(ctx, f, havuz, &sb, maks); err != nil {
			continue
		}
		if sb.Len() >= maks {
			break
		}
	}

	if !bulundu || strings.TrimSpace(sb.String()) == "" {
		return "", ErrMetinYok
	}
	return sb.String(), nil
}

func havuzOku(ctx context.Context, zr *zip.Reader, maks int) ([]string, error) {
	var f *zip.File
	for _, k := range zr.File {
		if k.Name == "xl/sharedStrings.xml" {
			f = k
			break
		}
	}
	if f == nil {
		return nil, nil // paylaşılan dize yok; her hücre satır içi olabilir
	}

	rc, err := f.Open()
	if err != nil {
		return nil, nil
	}
	defer rc.Close()

	cz := xml.NewDecoder(&ctxOkuyucu{r: io.LimitReader(rc, zipGirdiSiniri), ctx: ctx})
	var havuz []string
	var mevcut strings.Builder
	inSi, inT := false, false
	toplamBayt := 0

	for {
		jeton, err := cz.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return havuz, nil // elimizdeki havuzla devam
		}
		switch t := jeton.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSi = true
				mevcut.Reset()
			case "t":
				inT = true
			}
		case xml.CharData:
			// Bir <si> birden çok <t> içerebilir (zengin metin parçaları);
			// hepsini birleştiriyoruz.
			if inSi && inT {
				mevcut.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inT = false
			case "si":
				inSi = false
				s := mevcut.String()
				toplamBayt += len(s)
				havuz = append(havuz, s)
				if len(havuz) >= havuzSiniri || toplamBayt > maks*2 {
					return havuz, nil
				}
			}
		}
	}
	return havuz, nil
}

func sayfaOku(ctx context.Context, f *zip.File, havuz []string, sb *strings.Builder, maks int) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	cz := xml.NewDecoder(&ctxOkuyucu{r: io.LimitReader(rc, zipGirdiSiniri), ctx: ctx})
	var buf strings.Builder
	hucreTipi := ""
	topla, inFormul := false, false

	for {
		jeton, err := cz.Token()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		switch t := jeton.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "c":
				hucreTipi = ""
				for _, a := range t.Attr {
					if a.Name.Local == "t" {
						hucreTipi = a.Value
					}
				}
			case "f":
				// Formül metnini indekslemiyoruz; gürültü.
				inFormul = true
			case "v":
				if !inFormul {
					topla = true
					buf.Reset()
				}
			case "t":
				// t="inlineStr" durumunda metin <is><t> içindedir.
				topla = true
				buf.Reset()
			}
		case xml.CharData:
			if topla {
				buf.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "f":
				inFormul = false
			case "v":
				if topla {
					sb.WriteString(hucreCoz(buf.String(), hucreTipi, havuz))
					sb.WriteByte('\t')
					topla = false
				}
			case "t":
				if topla {
					sb.WriteString(buf.String())
					sb.WriteByte('\t')
					topla = false
				}
			case "row":
				sb.WriteByte('\n')
			}
		}
		if sb.Len() >= maks {
			return nil
		}
	}
}

// hucreCoz <v> içeriğini hücre tipine göre metne çevirir.
func hucreCoz(ham, tip string, havuz []string) string {
	if tip != "s" {
		// Sayı, formül sonucu (t="str") veya boolean. Sayıları da
		// indeksliyoruz: sıklıkla fatura veya parça numarasıdır.
		return ham
	}
	i, err := strconv.Atoi(strings.TrimSpace(ham))
	if err != nil || i < 0 || i >= len(havuz) {
		return ""
	}
	return havuz[i]
}
