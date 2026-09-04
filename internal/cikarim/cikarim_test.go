package cikarim

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fixture'ları KOD İÇİNDE üretiyoruz. Böylece depoya ikili blob girmiyor ve
// test aynı zamanda biçimi belgeliyor.
func zipKur(t *testing.T, girdiler map[string]string) *bytes.Reader {
	t.Helper()
	var tampon bytes.Buffer
	zw := zip.NewWriter(&tampon)
	for ad, icerik := range girdiler {
		w, err := zw.Create(ad)
		if err != nil {
			t.Fatalf("zip girdisi oluşturulamadı: %v", err)
		}
		if _, err := w.Write([]byte(icerik)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(tampon.Bytes())
}

func cikar(t *testing.T, c Cikarici, r *bytes.Reader) (string, error) {
	t.Helper()
	return c.Cikar(context.Background(), r, int64(r.Len()), Secenekler{MaksKarakter: 100000})
}

func TestDocxCikarim(t *testing.T) {
	r := zipKur(t, map[string]string{
		"word/document.xml": `<?xml version="1.0"?>
			<w:document xmlns:w="http://x">
			  <w:body>
			    <w:p><w:r><w:t>Şantiye Beton Raporu</w:t></w:r></w:p>
			    <w:p><w:r><w:t>İkinci paragraf</w:t></w:r></w:p>
			  </w:body>
			</w:document>`,
	})
	g, err := cikar(t, ooxmlBelge{}, r)
	if err != nil {
		t.Fatalf("docx çıkarımı: %v", err)
	}
	for _, beklenen := range []string{"Şantiye Beton Raporu", "İkinci paragraf"} {
		if !strings.Contains(g, beklenen) {
			t.Errorf("çıktıda %q yok. çıktı: %q", beklenen, g)
		}
	}
	// Paragraf sonları satır sonuna dönmeli.
	if !strings.Contains(g, "\n") {
		t.Error("paragraf sonu satır sonuna çevrilmemiş")
	}
}

// <w:instrText> alan kodlarıdır (köprü hedefleri, sayfa numarası formülleri).
// İndekslersek arşiv sözlüğü köprü çöpüyle dolar.
func TestDocxAlanKodlariAtlanir(t *testing.T) {
	r := zipKur(t, map[string]string{
		"word/document.xml": `<w:document xmlns:w="http://x"><w:body><w:p>
			<w:r><w:t>Gerçek metin</w:t></w:r>
			<w:r><w:instrText>HYPERLINK "http://cop.example.com/xyz"</w:instrText></w:r>
		</w:p></w:body></w:document>`,
	})
	g, err := cikar(t, ooxmlBelge{}, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g, "Gerçek metin") {
		t.Errorf("gerçek metin kayboldu: %q", g)
	}
	if strings.Contains(g, "HYPERLINK") || strings.Contains(g, "cop.example.com") {
		t.Errorf("alan kodu indekse girdi: %q", g)
	}
}

func TestDocxUstbilgiDeOkunur(t *testing.T) {
	r := zipKur(t, map[string]string{
		"word/document.xml": `<w:document xmlns:w="http://x"><w:body>
			<w:p><w:r><w:t>Gövde</w:t></w:r></w:p></w:body></w:document>`,
		"word/header1.xml": `<w:hdr xmlns:w="http://x">
			<w:p><w:r><w:t>PROJE-4471</w:t></w:r></w:p></w:hdr>`,
	})
	g, err := cikar(t, ooxmlBelge{}, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g, "PROJE-4471") {
		t.Errorf("üstbilgi okunmadı: %q", g)
	}
}

func TestPptxCikarim(t *testing.T) {
	r := zipKur(t, map[string]string{
		"ppt/slides/slide1.xml": `<p:sld xmlns:a="http://y"><p:cSld><p:spTree>
			<p:sp><p:txBody><a:p><a:r><a:t>Sunum Başlığı</a:t></a:r></a:p></p:txBody></p:sp>
		</p:spTree></p:cSld></p:sld>`,
		"ppt/slides/slide2.xml": `<p:sld xmlns:a="http://y">
			<a:p><a:r><a:t>İkinci slayt</a:t></a:r></a:p></p:sld>`,
		// Seçilmemesi gereken bir girdi.
		"ppt/theme/theme1.xml": `<a:theme xmlns:a="http://y"><a:t>tema copu</a:t></a:theme>`,
	})
	g, err := cikar(t, ooxmlSunum{}, r)
	if err != nil {
		t.Fatalf("pptx çıkarımı: %v", err)
	}
	if !strings.Contains(g, "Sunum Başlığı") || !strings.Contains(g, "İkinci slayt") {
		t.Errorf("slayt metinleri eksik: %q", g)
	}
	if strings.Contains(g, "tema copu") {
		t.Errorf("tema dosyası indekse girdi: %q", g)
	}
}

// xlsx'in en kritik ayrıntısı: t="s" olan hücrede <v> METİN DEĞİL, paylaşılan
// dize havuzuna İNDEKSTİR. Havuzu çözmezsek sadece rakam listesi indekslenir.
func TestXlsxPaylasilanDizeHavuzu(t *testing.T) {
	r := zipKur(t, map[string]string{
		"xl/sharedStrings.xml": `<sst xmlns="http://z" count="3" uniqueCount="3">
			<si><t>Malzeme Adı</t></si>
			<si><t>Çimento</t></si>
			<si><t>Demir</t></si>
		</sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://z"><sheetData>
			<row r="1"><c r="A1" t="s"><v>0</v></c></row>
			<row r="2"><c r="A2" t="s"><v>1</v></c><c r="B2"><v>1250</v></c></row>
			<row r="3"><c r="A3" t="s"><v>2</v></c><c r="B3"><v>980</v></c></row>
		</sheetData></worksheet>`,
	})
	g, err := cikar(t, ooxmlTablo{}, r)
	if err != nil {
		t.Fatalf("xlsx çıkarımı: %v", err)
	}
	for _, beklenen := range []string{"Malzeme Adı", "Çimento", "Demir"} {
		if !strings.Contains(g, beklenen) {
			t.Errorf("havuz dizesi %q çözülmedi. çıktı: %q", beklenen, g)
		}
	}
	// Sayılar da indekslenir: sıklıkla fatura veya parça numarasıdır.
	if !strings.Contains(g, "1250") {
		t.Errorf("sayısal hücre indekslenmedi: %q", g)
	}
}

func TestXlsxSatirIciDize(t *testing.T) {
	r := zipKur(t, map[string]string{
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://z"><sheetData>
			<row r="1"><c r="A1" t="inlineStr"><is><t>Satır içi metin</t></is></c></row>
		</sheetData></worksheet>`,
	})
	g, err := cikar(t, ooxmlTablo{}, r)
	if err != nil {
		t.Fatalf("xlsx çıkarımı: %v", err)
	}
	if !strings.Contains(g, "Satır içi metin") {
		t.Errorf("inlineStr okunmadı: %q", g)
	}
}

func TestXlsxFormulMetniAtlanir(t *testing.T) {
	r := zipKur(t, map[string]string{
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://z"><sheetData>
			<row r="1"><c r="C1"><f>SUM(A1:B1)</f><v>42</v></c></row>
		</sheetData></worksheet>`,
	})
	g, err := cikar(t, ooxmlTablo{}, r)
	if err != nil {
		t.Fatalf("xlsx çıkarımı: %v", err)
	}
	if strings.Contains(g, "SUM") {
		t.Errorf("formül metni indekse girdi: %q", g)
	}
	if !strings.Contains(g, "42") {
		t.Errorf("formül sonucu indekslenmedi: %q", g)
	}
}

// Zengin metin: bir <si> birden çok <t> parçası içerebilir.
func TestXlsxZenginMetinBirlestirilir(t *testing.T) {
	r := zipKur(t, map[string]string{
		"xl/sharedStrings.xml": `<sst xmlns="http://z">
			<si><r><t>Beton</t></r><r><t> Dökümü</t></r></si>
		</sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://z"><sheetData>
			<row r="1"><c r="A1" t="s"><v>0</v></c></row>
		</sheetData></worksheet>`,
	})
	g, err := cikar(t, ooxmlTablo{}, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g, "Beton Dökümü") {
		t.Errorf("zengin metin parçaları birleştirilmedi: %q", g)
	}
}

func TestXlsxHavuzTasmasiCokmez(t *testing.T) {
	// Havuz aralığı dışında bir indeks bozuk dosyada olabilir; çökmemeli.
	r := zipKur(t, map[string]string{
		"xl/sharedStrings.xml": `<sst xmlns="http://z"><si><t>tek</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://z"><sheetData>
			<row r="1"><c r="A1" t="s"><v>999</v></c><c r="B1" t="s"><v>0</v></c></row>
		</sheetData></worksheet>`,
	})
	g, err := cikar(t, ooxmlTablo{}, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g, "tek") {
		t.Errorf("geçerli havuz kaydı okunmadı: %q", g)
	}
}

func TestOoxmlBosDosyaMetinYok(t *testing.T) {
	r := zipKur(t, map[string]string{"docProps/app.xml": `<Properties/>`})
	if _, err := cikar(t, ooxmlBelge{}, r); err != ErrMetinYok {
		t.Errorf("hata = %v, beklenen ErrMetinYok", err)
	}
}

func TestOoxmlBozukZip(t *testing.T) {
	r := bytes.NewReader([]byte("bu bir zip degil, rastgele bayt dizisi"))
	if _, err := cikar(t, ooxmlBelge{}, r); err == nil {
		t.Error("bozuk zip için hata beklenirdi")
	}
}

// ---------------------------------------------------------- GuvenliCikar

func gecici(t *testing.T, ad string, icerik []byte) string {
	t.Helper()
	yol := filepath.Join(t.TempDir(), ad)
	if err := os.WriteFile(yol, icerik, 0o644); err != nil {
		t.Fatal(err)
	}
	return yol
}

func TestGuvenliCikarDuzMetin(t *testing.T) {
	yol := gecici(t, "rapor.txt", []byte("Şantiye raporu\nikinci satır"))
	icerik, durum := GuvenliCikar(context.Background(), yol, Secenekler{})
	if durum != DurumTam {
		t.Fatalf("durum = %q, beklenen %q", durum, DurumTam)
	}
	if !strings.Contains(icerik, "Şantiye raporu") {
		t.Errorf("içerik = %q", icerik)
	}
}

// Eski Türkçe metin dosyaları cp1254'tür; UTF-8 varsayarsak aranamaz olurlar.
func TestGuvenliCikarCp1254(t *testing.T) {
	// "Ğüşİı" cp1254 baytlarıyla
	yol := gecici(t, "eski.txt", []byte{0xD0, 0xFC, 0xFE, 0xDD, 0xFD})
	icerik, durum := GuvenliCikar(context.Background(), yol, Secenekler{})
	if durum != DurumTam {
		t.Fatalf("durum = %q", durum)
	}
	if icerik != "Ğüşİı" {
		t.Errorf("içerik = %q, beklenen %q", icerik, "Ğüşİı")
	}
}

func TestGuvenliCikarIkiliDosyaTespiti(t *testing.T) {
	// .txt adı verilmiş ikili veri: indekslenmemeli.
	yol := gecici(t, "sahte.txt", []byte{0x00, 0x01, 0x02, 0xFF, 0x00, 0x7F, 0x00})
	_, durum := GuvenliCikar(context.Background(), yol, Secenekler{})
	if durum != DurumIkili {
		t.Errorf("durum = %q, beklenen %q", durum, DurumIkili)
	}
}

func TestGuvenliCikarEskiOfficeFormati(t *testing.T) {
	yol := gecici(t, "eski.doc", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	_, durum := GuvenliCikar(context.Background(), yol, Secenekler{})
	if durum != DurumEskiFormat {
		t.Errorf("durum = %q, beklenen %q", durum, DurumEskiFormat)
	}
}

func TestGuvenliCikarDesteklenmeyenUzanti(t *testing.T) {
	yol := gecici(t, "cizim.dwg", []byte("herhangi bir veri"))
	_, durum := GuvenliCikar(context.Background(), yol, Secenekler{})
	if durum != DurumAdOnly {
		t.Errorf("durum = %q, beklenen %q", durum, DurumAdOnly)
	}
}

// Bu testin ekmeğini hak eden kısmı: bozuk ve rastgele baytlı PDF'ler
// PANİK ATMAK YERİNE durum döndürmeli. Tarama asla ölmemeli.
func TestGuvenliCikarBozukPdfPanikAtmaz(t *testing.T) {
	testler := []struct {
		ad     string
		icerik []byte
	}{
		{"kesilmis.pdf", []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog")},
		{"rastgele.pdf", []byte{0x25, 0x50, 0x44, 0x46, 0xFF, 0x00, 0xAB, 0xCD, 0xEF, 0x12}},
		{"bos.pdf", []byte{}},
		{"sadecebaslik.pdf", []byte("%PDF-1.7\n%%EOF\n")},
	}
	for _, tt := range testler {
		t.Run(tt.ad, func(t *testing.T) {
			yol := gecici(t, tt.ad, tt.icerik)
			icerik, durum := GuvenliCikar(context.Background(), yol, Secenekler{})
			// Hangi durum döndüğü önemli değil; ÇÖKMEMESİ önemli.
			if icerik != "" && durum == "" {
				t.Errorf("tutarsız sonuç: icerik=%q durum=%q", icerik, durum)
			}
			t.Logf("%s -> durum=%q", tt.ad, durum)
		})
	}
}

func TestGuvenliCikarKirpma(t *testing.T) {
	uzun := strings.Repeat("kelime ", 5000) // ~35 KB
	yol := gecici(t, "uzun.txt", []byte(uzun))
	icerik, durum := GuvenliCikar(context.Background(), yol, Secenekler{MaksKarakter: 100})
	if durum != DurumKirpildi {
		t.Fatalf("durum = %q, beklenen %q", durum, DurumKirpildi)
	}
	if len(icerik) > 100 {
		t.Errorf("kırpma çalışmadı: %d bayt", len(icerik))
	}
}

// Kırpma rune sınırında olmalı; ortadan kesilen UTF-8 dizisi bozuk karakter
// üretir ve o kelime aranamaz hale gelir.
func TestGuvenliCikarKirpmaRuneSinirinda(t *testing.T) {
	// Her "ş" 2 bayt; tek bayt sınırında kesilirse bozulur.
	yol := gecici(t, "tr.txt", []byte(strings.Repeat("ş", 100)))
	icerik, _ := GuvenliCikar(context.Background(), yol, Secenekler{MaksKarakter: 11})
	for _, r := range icerik {
		if r == '�' {
			t.Fatalf("kırpma rune ortasından kesti: %q", icerik)
		}
	}
}

func TestGuvenliCikarIptal(t *testing.T) {
	ctx, iptal := context.WithCancel(context.Background())
	iptal()
	yol := gecici(t, "rapor.txt", []byte("içerik"))
	_, durum := GuvenliCikar(ctx, yol, Secenekler{})
	if durum != DurumBozuk {
		t.Errorf("durum = %q, iptal sonrası %q beklenirdi", durum, DurumBozuk)
	}
}

func TestGuvenliCikarOlmayanDosya(t *testing.T) {
	_, durum := GuvenliCikar(context.Background(),
		filepath.Join(t.TempDir(), "yok.txt"), Secenekler{})
	if durum != DurumKilitli {
		t.Errorf("durum = %q, beklenen %q", durum, DurumKilitli)
	}
}

func TestDestekli(t *testing.T) {
	for _, u := range []string{".pdf", ".docx", ".xlsx", ".pptx", ".txt", ".DOCX"} {
		if !Destekli(u) {
			t.Errorf("%s desteklenmiyor sanıldı", u)
		}
	}
	for _, u := range []string{".dwg", ".zip", ".exe", ""} {
		if Destekli(u) {
			t.Errorf("%s destekli sanıldı", u)
		}
	}
}
