package sorgu

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"arsiv-indeks/internal/metin"
)

// OtomatikOnekEnKisa bu kadar ve daha uzun kelimeler kendiliğinden önek
// olarak aranır ("rapor" → raporu, raporlar). Daha kısası için "ra*" yazılır.
const OtomatikOnekEnKisa = 3

// Kelime sorgudaki tek bir arama birimidir: düz kelime, "tam ifade" veya
// ad: ile sınırlanmış parça.
type Kelime struct {
	Kaynak   string   // kullanıcının yazdığı hali (öneri üretirken değiştirilir)
	Ham      string   // normalize; alt dize araması ve vurgu için
	Terimler []string // ters indeks belirteçleri; durak kelimede boş
	Onek     bool     // terimler önek olarak genişletilsin mi
	SadeceAd bool     // ad: ile yazıldı
	Ifade    string   // tam ifadeyse metin.IfadeBicimi hali, değilse ""
}

// Cozulmus ayrıştırılmış sorgu metnidir.
type Cozulmus struct {
	Gruplar   [][]Kelime // gruplar VE ile, grup içindekiler VEYA ile bağlı
	Haric     []Kelime
	Klasorler []string // klasor: değerleri (normalize, alt dize)
	Uzantilar []string // uzanti: değerleri (".pdf")
}

// Coz sorgu metnini ayrıştırır. Desteklenen sözdizimi:
//
//	rapor*  "tam ifade"  -hariç  a VEYA b (veya, or, |)  ad:x  klasor:x  uzanti:pdf
func Coz(s string) Cozulmus {
	var c Cozulmus
	veyaBekliyor := false
	for _, p := range sozcukle(s) {
		if !p.tirnakli && !p.eksi && p.alan == "" && veyaMi(p.deger) {
			veyaBekliyor = len(c.Gruplar) > 0
			continue
		}
		switch p.alan {
		case "klasor":
			if v := metin.Normalize(strings.TrimSpace(p.deger)); v != "" {
				c.Klasorler = append(c.Klasorler, v)
			}
			veyaBekliyor = false
			continue
		case "uzanti":
			for _, v := range strings.Split(p.deger, ",") {
				if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
					c.Uzantilar = append(c.Uzantilar, "."+strings.TrimPrefix(v, "."))
				}
			}
			veyaBekliyor = false
			continue
		}

		k, tamam := kelimeKur(p)
		if !tamam {
			continue
		}
		if p.eksi {
			// Hariç tutmada otomatik önek yok: "-rapor" raporlama'yı da atmasın.
			k.Onek = strings.HasSuffix(p.deger, "*")
			c.Haric = append(c.Haric, k)
			veyaBekliyor = false
			continue
		}
		if veyaBekliyor {
			son := len(c.Gruplar) - 1
			c.Gruplar[son] = append(c.Gruplar[son], k)
		} else {
			c.Gruplar = append(c.Gruplar, []Kelime{k})
		}
		veyaBekliyor = false
	}
	return c
}

// Kelimeler tüm pozitif kelimeleri sırayla döner.
func (c Cozulmus) Kelimeler() []Kelime {
	var cikti []Kelime
	for _, g := range c.Gruplar {
		cikti = append(cikti, g...)
	}
	return cikti
}

// Vurgular sonuçta işaretlenecek parçaları döner.
func (c Cozulmus) Vurgular() []string {
	var cikti []string
	for _, k := range c.Kelimeler() {
		cikti = append(cikti, k.Ham)
	}
	return cikti
}

func veyaMi(s string) bool {
	switch metin.Normalize(s) {
	case "veya", "or", "|":
		return true
	}
	return false
}

func kelimeKur(p sozcuk) (Kelime, bool) {
	deger := strings.TrimSpace(p.deger)
	acikOnek := !p.tirnakli && strings.HasSuffix(deger, "*")
	deger = strings.TrimRight(deger, "*")
	ham := strings.Join(strings.Fields(metin.Normalize(deger)), " ")
	if ham == "" {
		return Kelime{}, false
	}
	k := Kelime{
		Kaynak:   p.ham,
		Ham:      ham,
		Terimler: metin.Belirtecler(deger),
		SadeceAd: p.alan == "ad",
	}
	if p.tirnakli {
		k.Ifade = metin.IfadeBicimi(deger)
		if !strings.Contains(k.Ifade, " ") {
			k.Ifade = "" // tek kelimelik tırnak: düz tam kelime araması yeter
		}
		return k, true
	}
	k.Onek = acikOnek || utf8.RuneCountInString(ham) >= OtomatikOnekEnKisa
	return k, true
}

// sozcuk sorgu metninin boşlukla (tırnak içi hariç) ayrılmış bir parçasıdır.
type sozcuk struct {
	ham      string // kaynaktaki hali
	deger    string // önek ve tırnaklar atılmış
	alan     string // "", "ad", "klasor", "uzanti"
	eksi     bool
	tirnakli bool
}

func sozcukle(s string) []sozcuk {
	var cikti []sozcuk
	r := []rune(s)
	for i := 0; i < len(r); {
		if unicode.IsSpace(r[i]) {
			i++
			continue
		}
		bas := i
		tirnakta := false
		for i < len(r) && (tirnakta || !unicode.IsSpace(r[i])) {
			if r[i] == '"' {
				tirnakta = !tirnakta
			}
			i++
		}
		if p, tamam := sozcukCoz(string(r[bas:i])); tamam {
			cikti = append(cikti, p)
		}
	}
	return cikti
}

func sozcukCoz(ham string) (sozcuk, bool) {
	p := sozcuk{ham: ham}
	d := ham
	if len(d) > 1 && d[0] == '-' {
		p.eksi = true
		d = d[1:]
	}
	if i := strings.IndexByte(d, ':'); i > 0 {
		switch metin.Normalize(d[:i]) {
		case "ad":
			p.alan = "ad"
		case "klasor":
			p.alan = "klasor"
		case "uzanti":
			p.alan = "uzanti"
		}
		if p.alan != "" {
			d = d[i+1:]
		}
	}
	if strings.HasPrefix(d, `"`) {
		p.tirnakli = true
		d = strings.TrimSuffix(d[1:], `"`)
	}
	p.deger = d
	return p, strings.TrimSpace(d) != ""
}
