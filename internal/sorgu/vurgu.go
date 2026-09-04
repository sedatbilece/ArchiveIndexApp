package sorgu

import (
	"strings"

	"arsiv-indeks/internal/metin"
)

// Parca vurgulanmış veya düz bir metin parçasıdır.
//
// Eşleşmeyi HTML olarak değil YAPISAL olarak döndürmemiz kritik. Şablon
// tarafında her Metin alanı html/template ile kaçırılır, <mark> etiketleri
// ise yazar kontrolündedir:
//
//	{{range .Parcalar}}{{if .Vurgulu}}<mark>{{.Metin}}</mark>{{else}}{{.Metin}}{{end}}{{end}}
//
// Cazip görünen alternatif — strings.ReplaceAll(s, q, "<mark>"+q+"</mark>")
// ardından template.HTML — içinde <script> geçen HERHANGİ bir dosyanın
// tetikleyeceği depolanmış XSS deliğidir. Dosya adları da aynı yoldan geçer.
type Parca struct {
	Metin   string
	Vurgulu bool
}

// Vurgula orijinal metinde aranan parçaların geçtiği yerleri işaretler.
//
// Karşılaştırma normalize edilmiş biçimde yapılır (Türkçe harf katlamalı),
// ama döndürülen metin ORİJİNALDİR — kullanıcı dosya adını yazıldığı gibi
// görmeli.
//
// Rune bazında çalışıyoruz: metin.Katla her runeyi tam bir runeye eşlediği
// için normalize edilmiş dizenin rune sayısı orijinaliyle aynıdır, ama BAYT
// uzunlukları farklıdır ('ş' 2 bayt, 's' 1 bayt). Bayt indeksiyle çalışmak
// Türkçe adlarda yanlış yere vurgu koyardı.
func Vurgula(orijinal string, aranan []string) []Parca {
	if orijinal == "" {
		return nil
	}
	temizAranan := make([]string, 0, len(aranan))
	for _, a := range aranan {
		if a = strings.TrimSpace(a); a != "" {
			temizAranan = append(temizAranan, a)
		}
	}
	if len(temizAranan) == 0 {
		return []Parca{{Metin: orijinal}}
	}

	asilRuneler := []rune(orijinal)
	normalRuneler := []rune(metin.Normalize(orijinal))
	if len(asilRuneler) != len(normalRuneler) {
		// Beklenmiyor (Katla rune sayısını korur), ama olursa vurgusuz
		// dönmek yanlış yere vurgu koymaktan iyidir.
		return []Parca{{Metin: orijinal}}
	}

	// Vurgulanacak rune aralıklarını işaretle.
	isaret := make([]bool, len(asilRuneler))
	normal := string(normalRuneler)
	for _, a := range temizAranan {
		hedef := metin.Normalize(a)
		if hedef == "" {
			continue
		}
		hedefRuneAdedi := len([]rune(hedef))
		// Bayt indeksinden rune indeksine çevirmek için ilerledikçe sayıyoruz.
		bayt := 0
		for {
			i := strings.Index(normal[bayt:], hedef)
			if i < 0 {
				break
			}
			mutlakBayt := bayt + i
			runeBasi := len([]rune(normal[:mutlakBayt]))
			for k := runeBasi; k < runeBasi+hedefRuneAdedi && k < len(isaret); k++ {
				isaret[k] = true
			}
			bayt = mutlakBayt + len(hedef)
			if bayt >= len(normal) {
				break
			}
		}
	}

	return isaretleriParcala(asilRuneler, isaret)
}

func isaretleriParcala(runeler []rune, isaret []bool) []Parca {
	var parcalar []Parca
	basla := 0
	for i := 1; i <= len(runeler); i++ {
		if i == len(runeler) || isaret[i] != isaret[basla] {
			parcalar = append(parcalar, Parca{
				Metin:   string(runeler[basla:i]),
				Vurgulu: isaret[basla],
			})
			basla = i
		}
	}
	return parcalar
}

// Parcacik metin içinde aranan parçaların ilk geçtiği yerin çevresinden bir
// alıntı çıkarır ve vurgular.
//
// pencere, eşleşmenin her iki yanından alınacak yaklaşık karakter sayısıdır.
func Parcacik(icerik string, aranan []string, pencere int) []Parca {
	if icerik == "" {
		return nil
	}
	if pencere <= 0 {
		pencere = 120
	}

	asilRuneler := []rune(icerik)
	normalRuneler := []rune(metin.Normalize(icerik))
	if len(asilRuneler) != len(normalRuneler) {
		return []Parca{{Metin: kisalt(icerik, pencere*2)}}
	}
	normal := string(normalRuneler)

	// İlk eşleşmenin rune konumunu bul.
	enIyi := -1
	for _, a := range aranan {
		hedef := metin.Normalize(strings.TrimSpace(a))
		if hedef == "" {
			continue
		}
		if i := strings.Index(normal, hedef); i >= 0 {
			r := len([]rune(normal[:i]))
			if enIyi < 0 || r < enIyi {
				enIyi = r
			}
		}
	}
	if enIyi < 0 {
		// Eşleşme içerikte değil (ad veya yolda eşleşmiş olabilir):
		// baştan bir alıntı gösteriyoruz.
		return Vurgula(satirlariTemizle(string(asilRuneler[:min(len(asilRuneler), pencere*2)])), aranan)
	}

	bas := max(0, enIyi-pencere)
	son := min(len(asilRuneler), enIyi+pencere)
	alinti := satirlariTemizle(string(asilRuneler[bas:son]))
	if bas > 0 {
		alinti = "… " + alinti
	}
	if son < len(asilRuneler) {
		alinti = alinti + " …"
	}
	return Vurgula(alinti, aranan)
}

// satirlariTemizle çok satırlı içeriği tek satıra indirir; sonuç satırı
// listede tek satır kaplasın.
func satirlariTemizle(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}

func kisalt(s string, maks int) string {
	r := []rune(s)
	if len(r) <= maks {
		return satirlariTemizle(s)
	}
	return satirlariTemizle(string(r[:maks])) + " …"
}
