package metin

import (
	"bytes"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// cp1254 tablosu: Windows-1254 (Türkçe) baytlarının 0x80-0xFF aralığının
// Unicode karşılıkları. 0x00-0x7F aralığı ASCII ile birebir aynıdır.
//
// golang.org/x/text yerine bu 128 elemanlı tabloyu elle yazıyoruz: ihtiyacımız
// olan tek eski kod sayfası bu ve karşılığında bağımlılık sayımızı 1'de
// tutuyoruz. 1252'den ayrıldığı yerler Türkçe harflerdir:
// 0xD0=Ğ 0xDD=İ 0xDE=Ş 0xF0=ğ 0xFD=ı 0xFE=ş
var cp1254 = [128]rune{
	'€', '�', '‚', 'ƒ', '„', '…', '†', '‡', // 80-87
	'ˆ', '‰', '�', '‹', 'Œ', '�', '�', '�', // 88-8F
	'�', '‘', '’', '“', '”', '•', '–', '—', // 90-97
	'˜', '™', '�', '›', 'œ', '�', '�', 'Ÿ', // 98-9F
	' ', '¡', '¢', '£', '¤', '¥', '¦', '§', // A0-A7
	'¨', '©', 'ª', '«', '¬', '­', '®', '¯', // A8-AF
	'°', '±', '²', '³', '´', 'µ', '¶', '·', // B0-B7
	'¸', '¹', 'º', '»', '¼', '½', '¾', '¿', // B8-BF
	'À', 'Á', 'Â', 'Ã', 'Ä', 'Å', 'Æ', 'Ç', // C0-C7
	'È', 'É', 'Ê', 'Ë', 'Ì', 'Í', 'Î', 'Ï', // C8-CF
	'Ğ', 'Ñ', 'Ò', 'Ó', 'Ô', 'Õ', 'Ö', '×', // D0-D7  (D0=Ğ)
	'Ø', 'Ù', 'Ú', 'Û', 'Ü', 'İ', 'Ş', 'ß', // D8-DF  (DD=İ DE=Ş)
	'à', 'á', 'â', 'ã', 'ä', 'å', 'æ', 'ç', // E0-E7
	'è', 'é', 'ê', 'ë', 'ì', 'í', 'î', 'ï', // E8-EF
	'ğ', 'ñ', 'ò', 'ó', 'ô', 'õ', 'ö', '÷', // F0-F7  (F0=ğ)
	'ø', 'ù', 'ú', 'û', 'ü', 'ı', 'ş', 'ÿ', // F8-FF  (FD=ı FE=ş)
}

// Coz ham baytları metne çevirir. Tespit sırası: BOM -> geçerli UTF-8 -> cp1254.
//
// Yedek olarak cp1254 seçmemizin sebebi: eski Türkçe metin dosyalarında bare
// 0xFD baytı 'ı' demektir. UTF-8 varsayarsak U+FFFD olur ve o dosyadaki her
// "ı" içeren kelime aranamaz hale gelir.
func Coz(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:])
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return utf16Coz(b[2:], true)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return utf16Coz(b[2:], false)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return cp1254Coz(b)
}

func cp1254Coz(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b) + len(b)/4)
	for _, c := range b {
		if c < 0x80 {
			sb.WriteByte(c)
			continue
		}
		sb.WriteRune(cp1254[c-0x80])
	}
	return sb.String()
}

func utf16Coz(b []byte, kucukUclu bool) string {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		if kucukUclu {
			u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
		} else {
			u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		}
	}
	return string(utf16.Decode(u))
}
