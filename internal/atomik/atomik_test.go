package atomik

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestYazOlusturur(t *testing.T) {
	hedef := filepath.Join(t.TempDir(), "alt", "veri.txt")
	err := Yaz(hedef, func(w io.Writer) error {
		_, err := w.Write([]byte("merhaba"))
		return err
	})
	if err != nil {
		t.Fatalf("Yaz hata verdi: %v", err)
	}
	b, err := os.ReadFile(hedef)
	if err != nil {
		t.Fatalf("hedef okunamadı: %v", err)
	}
	if string(b) != "merhaba" {
		t.Fatalf("içerik = %q", b)
	}
}

// En önemli davranış: yazma başarısız olursa eski dosya bozulmadan kalmalı.
func TestYazHatadaEskiDosyayiKorur(t *testing.T) {
	hedef := filepath.Join(t.TempDir(), "veri.txt")
	if err := os.WriteFile(hedef, []byte("eski"), 0o644); err != nil {
		t.Fatal(err)
	}

	beklenenHata := errors.New("yazıcı patladı")
	err := Yaz(hedef, func(w io.Writer) error {
		w.Write([]byte("yarim"))
		return beklenenHata
	})
	if !errors.Is(err, beklenenHata) {
		t.Fatalf("hata = %v, beklenen %v", err, beklenenHata)
	}

	b, _ := os.ReadFile(hedef)
	if string(b) != "eski" {
		t.Fatalf("eski dosya bozuldu: %q", b)
	}
	// Geçici dosya da ortada kalmamalı.
	girdiler, _ := os.ReadDir(filepath.Dir(hedef))
	if len(girdiler) != 1 {
		for _, g := range girdiler {
			t.Logf("kalan: %s", g.Name())
		}
		t.Fatalf("geçici dosya temizlenmedi: %d dosya var", len(girdiler))
	}
}

func TestYazUzerineYazar(t *testing.T) {
	hedef := filepath.Join(t.TempDir(), "veri.txt")
	for _, icerik := range []string{"bir", "iki-daha-uzun", "uc"} {
		err := Yaz(hedef, func(w io.Writer) error {
			_, err := w.Write([]byte(icerik))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(hedef)
		if string(b) != icerik {
			t.Fatalf("içerik = %q, beklenen %q", b, icerik)
		}
	}
}
