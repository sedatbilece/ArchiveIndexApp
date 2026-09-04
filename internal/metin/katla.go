// Paket metin, arama için metin normalizasyonu, belirteçleme (tokenization),
// karakter kodlaması tespiti ve ikili dosya sezgisi sağlar.
//
// Buradaki en kritik değişmez: Normalize fonksiyonu HEM indeksleme HEM sorgu
// anında uygulanmalıdır. İkisi ayrışırsa arama sessizce sonuç vermez.
package metin

import (
	"strings"
	"unicode"
)

// Katla bir runeyi arama biçimine indirir.
//
// strings.ToLower burada yetmez: Türkçe'ye özgü harfleri (ı ş ğ ç ö ü) ASCII
// karşılıklarına katlamaz. Katlamazsak "SIRA", "sıra" ve "sira" üç ayrı terim
// olur ve kullanıcı hangisini yazdığını hatırlamak zorunda kalır.
//
// ı, i, I, İ hepsi 'i'ye katlanır. Bu bilinçli bir geri çağırma (recall)
// tercihidir: arşiv aramasında "sira" yazıp "sıra"yı bulmak istenir.
func Katla(r rune) rune {
	switch r {
	case 'ç', 'Ç':
		return 'c'
	case 'ğ', 'Ğ':
		return 'g'
	case 'ı', 'i', 'I', 'İ':
		return 'i'
	case 'ö', 'Ö':
		return 'o'
	case 'ş', 'Ş':
		return 's'
	case 'ü', 'Ü':
		return 'u'
	// Arşivlerde alıntı/yabancı adlarda karşımıza çıkan diğer aksanlar.
	case 'â', 'Â', 'à', 'À', 'á', 'Á', 'ä', 'Ä', 'å', 'Å', 'ã', 'Ã':
		return 'a'
	case 'é', 'É', 'è', 'È', 'ê', 'Ê', 'ë', 'Ë':
		return 'e'
	case 'î', 'Î', 'ï', 'Ï', 'í', 'Í', 'ì', 'Ì':
		return 'i'
	case 'ô', 'Ô', 'ó', 'Ó', 'ò', 'Ò', 'õ', 'Õ', 'ø', 'Ø':
		return 'o'
	case 'û', 'Û', 'ù', 'Ù', 'ú', 'Ú':
		return 'u'
	case 'ñ', 'Ñ':
		return 'n'
	case 'ß':
		return 's'
	}
	if r >= 'A' && r <= 'Z' {
		return r - 'A' + 'a'
	}
	if r < unicode.MaxASCII {
		return r
	}
	return unicode.ToLower(r)
}

// Normalize dizedeki her runeyi Katla ile geçirir.
func Normalize(s string) string {
	// Hızlı yol: zaten normal olan ASCII dizelerde ayırma yapmayalım.
	temiz := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= unicode.MaxASCII || (c >= 'A' && c <= 'Z') {
			temiz = false
			break
		}
	}
	if temiz {
		return s
	}
	return strings.Map(Katla, s)
}
