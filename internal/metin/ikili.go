package metin

import (
	"bytes"
	"unicode/utf8"
)

// SezgiBoyutu ikili tespiti için okunacak baş bayt sayısı.
const SezgiBoyutu = 8 << 10

// IkiliMi baytların metin değil ikili veri olup olduğunu tahmin eder.
//
// Bu, ".txt" adı verilmiş bir disk imajını veya uzantısı yanlış bir zip'i
// indekslemekten kurtaran kontroldür. İki sinyal kullanıyoruz:
//   - NUL baytı (UTF-16 BOM yoksa) — metin dosyalarında bulunmaz
//   - yazdırılamayan karakter oranı > %30
func IkiliMi(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	// UTF-16 metni NUL baytı içerir; BOM'u varsa ikili sayma.
	if bytes.HasPrefix(b, []byte{0xFF, 0xFE}) || bytes.HasPrefix(b, []byte{0xFE, 0xFF}) {
		return false
	}
	if len(b) > SezgiBoyutu {
		b = b[:SezgiBoyutu]
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return true
	}

	// Kontrol karakteri oranı. Geçerli UTF-8 ise rune bazında, değilse bayt
	// bazında sayıyoruz — ikinci durumda cp1254 üst bölgesi metin sayılır.
	kotu, toplam := 0, 0
	if utf8.Valid(b) {
		for _, r := range string(b) {
			toplam++
			if kontrolMu(r) {
				kotu++
			}
		}
	} else {
		for _, c := range b {
			toplam++
			if c < 0x20 && !yaziBoslugu(rune(c)) {
				kotu++
			}
		}
	}
	if toplam == 0 {
		return false
	}
	return kotu*100/toplam > 30
}

func kontrolMu(r rune) bool {
	if r == utf8.RuneError {
		return true
	}
	if r < 0x20 {
		return !yaziBoslugu(r)
	}
	return r == 0x7F
}

func yaziBoslugu(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
}
