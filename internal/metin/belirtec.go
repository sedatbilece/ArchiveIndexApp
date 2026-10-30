package metin

import (
	"strings"
	"unicode"
)

// Sözlük (vocabulary) denetim kaldıraçları.
//
// Bu sabitler indeksin bellek kullanımını belirleyen tek şeydir. Mühendislik
// arşivleri "hapax-ağır"dır: proje kodları, parça numaraları, revizyon
// dizeleri, zaman damgaları... tek bir dokümanda görünen on binlerce terim
// üretir. Aşağıdaki sınırlar olmadan sözlük kontrolsüz büyür.
const (
	EnKisaBelirtec = 2  // altı gürültü
	EnUzunBelirtec = 32 // üstü base64/hash bloğu; KIRPILMAZ, ATILIR
	EnUzunSayi     = 6  // salt sayısal: yıl/revizyon kalsın, timestamp/ID gitsin
	VarsayilanCap  = 2000
)

// durakKelimeler indekslenmeyecek kelimelerdir. Gönderi (posting) sayısını
// %30-40 düşürürler, sözlüğe etkileri ihmal edilebilir.
//
// Not: anahtarlar Normalize edilmiş biçimde tutulur (ı/i/İ hepsi 'i').
var durakKelimeler = map[string]bool{}

func init() {
	// Türkçe
	tr := `bir bu ve ile için gibi kadar daha çok az var yok olarak ise de da
		mi mı mu mü ki ne ya ama fakat lakin ancak veya yada hem her hiç
		bazı tüm bütün şey şu o ben sen biz siz onlar bunlar şunlar
		ben benim senin onun bizim sizin onların
		olan olduğu olur oldu olmak etmek yapmak eden edilen
		üzere göre sonra önce beri diye sanki nasıl neden niçin
		çünkü ayrıca yani ancak halde rağmen dolayı itibaren
		acaba belki galiba tabii elbette evet hayır değil
		burada orada şurada nerede kim kime kimin hangi
		iki üç dört beş altı yedi sekiz dokuz on yüz bin`
	// İngilizce (arşivlerde İngilizce doküman da bol)
	en := `the a an and or but if then else for of to in on at by with from
		as is are was were be been being have has had do does did
		this that these those it its it's i you he she we they them
		his her their our your my me him us not no nor so than too very
		can will just should now also into over under after before
		about above below between out off up down again further
		there here where when why how what which who whom
		one two three four five six seven eight nine ten
		page pages figure table section chapter appendix
		www http https com net org html htm`
	for _, alan := range []string{tr, en} {
		for _, k := range strings.Fields(alan) {
			durakKelimeler[Normalize(k)] = true
		}
	}
}

// DurakMi kelimenin durak kelime olup olmadığını söyler.
// Girdi Normalize edilmiş olmalıdır.
func DurakMi(b string) bool { return durakKelimeler[b] }

// Belirtecler metni normalize eder ve indekslenebilir belirteçlere ayırır.
// Sırayı korur, tekrarları temizlemez. Sorgu ayrıştırmada kullanılır.
func Belirtecler(s string) []string {
	var cikti []string
	Gez(s, func(b string) bool {
		cikti = append(cikti, b)
		return true
	})
	return cikti
}

// TekilBelirtecler indeksleme için ilk-görülen sırada tekil belirteçler döner.
// maks 0'dan büyükse o kadar tekil belirteçten sonra durur — tek bir bozuk
// PDF'in sözlüğe 50 bin çöp terim eklemesine karşı tek korumamız budur.
func TekilBelirtecler(s string, maks int) []string {
	cikti, _ := TekilBelirteclerSinirli(s, maks)
	return cikti
}

// TekilBelirteclerSinirli TekilBelirtecler gibidir; ek olarak sınıra takılıp
// metnin geri kalanının atlanıp atlanmadığını söyler.
//
// Sorgudan farklı olarak CamelCase parçaları da üretir ("UserService" →
// userservice, user, service): sorgu bütün kelimeyi, indeks ise hem bütünü
// hem parçaları tuttuğu için iki yazım da eşleşir.
func TekilBelirteclerSinirli(s string, maks int) (cikti []string, kesildi bool) {
	if maks <= 0 {
		maks = VarsayilanCap
	}
	gorulen := make(map[string]struct{}, 256)
	cikti = make([]string, 0, 256)
	gez(s, true, func(b string) bool {
		if _, v := gorulen[b]; v {
			return true
		}
		if len(cikti) >= maks {
			kesildi = true
			return false
		}
		gorulen[b] = struct{}{}
		cikti = append(cikti, b)
		return true
	})
	return cikti, kesildi
}

// Gez metni tek geçişte belirteçlere ayırır ve her kabul edilen belirteç için
// geri çağırmayı çalıştırır. Geri çağırma false dönerse gezinme durur.
//
// Belirteç = harf ve rakamların kesintisiz dizisi (katlama sonrası). Diğer her
// şey ayırıcıdır. "ENK-1234" iki belirteç olur: "enk" ve "1234"; aynı ayırma
// sorgu anında da olduğu için arama yine eşleşir.
func Gez(s string, geri func(belirtec string) bool) {
	gez(s, false, geri)
}

// gez, parcala doğruysa küçük→büyük harf ve harf↔rakam geçişlerinde bölünen
// parçaları da (bütün belirteçten sonra) bildirir.
func gez(s string, parcala bool, geri func(belirtec string) bool) {
	var sb strings.Builder
	sb.Grow(EnUzunBelirtec + 1)
	tastiMi := false // belirteç EnUzunBelirtec'i aştı mı?
	var sinirlar []int
	var onceki rune

	bitir := func() bool {
		if sb.Len() == 0 && !tastiMi {
			return true
		}
		b := sb.String()
		sb.Reset()
		asti := tastiMi
		tastiMi = false
		parcaSinir := sinirlar
		sinirlar = sinirlar[:0]
		// Taşan belirteci kırpmıyoruz — kırpmak yanlış eşleşme üretir
		// ("a1b2c3..." gibi bir hash'in ilk 32 karakteri sahte bir terim
		// olur). Tamamen atıyoruz.
		if asti {
			return true
		}
		if Kabul(b) && !geri(b) {
			return false
		}
		bas := 0
		for i, son := range append(parcaSinir, len(b)) {
			if i == len(parcaSinir) && bas == 0 {
				break // hiç sınır yok: parça = bütün
			}
			if p := b[bas:son]; Kabul(p) && !geri(p) {
				return false
			}
			bas = son
		}
		return true
	}

	for _, r := range s {
		k := Katla(r)
		if harfMi(k) || rakamMi(k) {
			// Builder'ı sınırda tutuyoruz: ayırıcısız 10 MB'lık bir bayt
			// dizisi (bozuk dosya, base64 blob) belleği şişirmesin.
			if sb.Len() > EnUzunBelirtec {
				tastiMi = true
				continue
			}
			if parcala && sb.Len() > 0 && parcaSiniriMi(onceki, r) {
				sinirlar = append(sinirlar, sb.Len())
			}
			sb.WriteRune(k)
			onceki = r
			continue
		}
		if !bitir() {
			return
		}
	}
	bitir()
}

// parcaSiniriMi iki ardışık özgün rune arasında bir kelime sınırı olup
// olmadığını söyler: "userService" (küçük→büyük), "rapor2024" (harf↔rakam).
func parcaSiniriMi(onceki, simdiki rune) bool {
	if unicode.IsLower(onceki) && unicode.IsUpper(simdiki) {
		return true
	}
	return unicode.IsDigit(onceki) != unicode.IsDigit(simdiki)
}

// IfadeBicimi tam ifade karşılaştırması için metni katlar ve harf/rakam
// dışındaki her diziyi tek boşluğa indirir: "Proje-Planı  v2" → "proje plani v2".
func IfadeBicimi(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	bosluk := true
	for _, r := range s {
		k := Katla(r)
		if harfMi(k) || rakamMi(k) {
			sb.WriteRune(k)
			bosluk = false
			continue
		}
		if !bosluk {
			sb.WriteByte(' ')
			bosluk = true
		}
	}
	return strings.TrimRight(sb.String(), " ")
}

// Kabul bir belirtecin indekslenip indekslenmeyeceğine karar verir.
// Girdi Normalize edilmiş olmalıdır.
func Kabul(b string) bool {
	n := len(b)
	if n < EnKisaBelirtec || n > EnUzunBelirtec {
		return false
	}
	if DurakMi(b) {
		return false
	}
	// Salt sayısal belirteçler en büyük alan kaldıracıdır: 2020 (yıl) ve
	// 03 (revizyon) işe yarar, 1698240000 (timestamp) yaramaz.
	// Harf içeren alfanümerikler (a4, rev2, m25) HER ZAMAN tutulur.
	if saltSayiMi(b) && n > EnUzunSayi {
		return false
	}
	return true
}

func saltSayiMi(b string) bool {
	for i := 0; i < len(b); i++ {
		if b[i] < '0' || b[i] > '9' {
			return false
		}
	}
	return true
}

func harfMi(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	}
	// Katlama sonrası kalan ASCII dışı harfler (Kiril, Yunan, CJK...).
	return r > 127 && !boslukMu(r) && !noktalamaMi(r)
}

func rakamMi(r rune) bool { return r >= '0' && r <= '9' }

func boslukMu(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == 0xA0
}

func noktalamaMi(r rune) bool {
	switch r {
	case '–', '—', '‘', '’', '“', '”',
		'…', '«', '»', '•', '·':
		return true
	}
	return false
}
