// Paket atomik, yarım yazılmış dosya bırakmayan dosya yazımı sağlar.
//
// Hem ayarlar.json hem indeks.gob bu deseni kullanır: geçici dosyaya yaz,
// diske indir (Sync), kapat, sonra hedefin üzerine taşı. Süreç ortada
// öldürülürse hedef dosya eski haliyle sağlam kalır — asla kırık bir gob
// yüklemeye çalışmayız.
package atomik

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Yaz, yazici'yı geçici bir dosyaya çalıştırır ve başarılıysa hedefin üzerine
// atomik olarak taşır. Geçici dosya hedefle AYNI dizinde oluşturulur; farklı
// bir birimde olursa os.Rename atomik olmaz.
func Yaz(hedef string, yazici func(io.Writer) error) error {
	dizin := filepath.Dir(hedef)
	if err := os.MkdirAll(dizin, 0o755); err != nil {
		return fmt.Errorf("dizin oluşturulamadı: %w", err)
	}

	gecici, err := os.CreateTemp(dizin, filepath.Base(hedef)+".tmp*")
	if err != nil {
		return fmt.Errorf("geçici dosya oluşturulamadı: %w", err)
	}
	geciciAd := gecici.Name()

	// Bu noktadan sonra her hata yolunda geçici dosyayı temizlemeliyiz.
	temizle := func() {
		gecici.Close()
		os.Remove(geciciAd)
	}

	if err := yazici(gecici); err != nil {
		temizle()
		return err
	}
	// Sync olmadan, elektrik kesintisinde Rename tamamlanmış ama içerik
	// diske inmemiş olabilir.
	if err := gecici.Sync(); err != nil {
		temizle()
		return fmt.Errorf("diske yazılamadı: %w", err)
	}
	if err := gecici.Close(); err != nil {
		os.Remove(geciciAd)
		return fmt.Errorf("geçici dosya kapatılamadı: %w", err)
	}
	// Windows'ta os.Rename, MoveFileEx REPLACE_EXISTING kullanır ve hedefin
	// üzerine yazar — ama hedefi AÇIK TUTAN bir okuyucu varsa başarısız olur.
	// Bu yüzden yükleme fonksiyonları dosyayı derhal kapatmalıdır.
	if err := os.Rename(geciciAd, hedef); err != nil {
		os.Remove(geciciAd)
		return fmt.Errorf("dosya yerine taşınamadı: %w", err)
	}
	return nil
}
