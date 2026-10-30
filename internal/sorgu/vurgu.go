package sorgu

import (
	"slices"
	"strings"
	"unicode/utf8"

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

// EnFazlaPencere parçacıkta gösterilecek en fazla alıntı sayısıdır.
const EnFazlaPencere = 3

// Parcacik metin içinden, aranan parçaların geçtiği yerlerin çevresinden en
// fazla EnFazlaPencere alıntı çıkarır ve vurgular. Önce her parçanın ilk
// geçtiği yer seçilir, böylece farklı kelimeler ayrı ayrı görünür.
//
// pencere, tek alıntıda eşleşmenin her iki yanından alınacak yaklaşık
// karakter sayısıdır; birden çok alıntıda yarısı kullanılır.
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

	ilkler, hepsi := eslesmeKonumlari(normal, aranan)
	if len(ilkler) == 0 {
		// Eşleşme içerikte değil (ad veya yolda eşleşmiş olabilir):
		// baştan bir alıntı gösteriyoruz.
		return Vurgula(satirlariTemizle(string(asilRuneler[:min(len(asilRuneler), pencere*2)])), aranan)
	}

	yarim := pencere / 2
	var secilen []int
	ekle := func(r int) {
		if len(secilen) >= EnFazlaPencere {
			return
		}
		for _, s := range secilen {
			if abs(s-r) < 2*yarim {
				return
			}
		}
		secilen = append(secilen, r)
	}
	for _, r := range ilkler {
		ekle(r)
	}
	for _, r := range hepsi {
		ekle(r)
	}
	slices.Sort(secilen)
	if len(secilen) == 1 {
		yarim = pencere
	}

	var cikti []Parca
	for i, r := range secilen {
		bas := max(0, r-yarim)
		son := min(len(asilRuneler), r+yarim)
		alinti := satirlariTemizle(string(asilRuneler[bas:son]))
		if bas > 0 || i > 0 {
			alinti = "… " + alinti
		}
		if i == len(secilen)-1 && son < len(asilRuneler) {
			alinti = alinti + " …"
		} else if i < len(secilen)-1 {
			alinti = alinti + " "
		}
		cikti = append(cikti, Vurgula(alinti, aranan)...)
	}
	return cikti
}

// eslesmeKonumlari aranan parçaların normal metindeki rune konumlarını döner:
// her parçanın ilk konumu (sıralı) ve parça başına en fazla 20 konumun tümü.
func eslesmeKonumlari(normal string, aranan []string) (ilkler, hepsi []int) {
	for _, a := range aranan {
		hedef := metin.Normalize(strings.TrimSpace(a))
		if hedef == "" {
			continue
		}
		bayt, rune_ := 0, 0
		for n := 0; n < 20; n++ {
			i := strings.Index(normal[bayt:], hedef)
			if i < 0 {
				break
			}
			rune_ += utf8.RuneCountInString(normal[bayt : bayt+i])
			bayt += i
			if n == 0 {
				ilkler = append(ilkler, rune_)
			}
			hepsi = append(hepsi, rune_)
			bayt += len(hedef)
			rune_ += utf8.RuneCountInString(hedef)
		}
	}
	slices.Sort(ilkler)
	slices.Sort(hepsi)
	return ilkler, hepsi
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
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
