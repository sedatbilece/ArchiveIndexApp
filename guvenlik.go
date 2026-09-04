package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"arsiv-indeks/internal/indeks"
)

// ErrYolDisi istenen yolun taranan köklerin dışında olduğunu belirtir.
var ErrYolDisi = errors.New("yol taranan dizinlerin dışında")

// GuvenliYol istenen yolu doğrular ve yalnızca taranan köklerden birinin
// altındaysa temizlenmiş mutlak halini döner.
//
// Bu fonksiyon uygulamanın güvenlik sınırıdır: /dosya/indir ve
// /dosya/klasorde-goster uç noktaları buradan geçmeyen hiçbir yola
// dokunmamalıdır.
func GuvenliYol(istenen string, kokler []string) (string, error) {
	istenen = strings.TrimSpace(istenen)
	if istenen == "" {
		return "", errors.New("yol boş")
	}
	// NUL baytı yol ayrıştırıcılarını şaşırtmak için kullanılır.
	if strings.ContainsRune(istenen, 0) {
		return "", errors.New("yol geçersiz karakter içeriyor")
	}
	if !filepath.IsAbs(istenen) {
		return "", errors.New("yol mutlak değil")
	}

	temiz := filepath.Clean(istenen)

	// Alternatif veri akışı (dosya.txt:gizli) reddedilir. Sürücü harfindeki
	// iki nokta (C:) meşru, ondan sonrakiler değil.
	if i := strings.Index(temiz, ":"); i >= 0 {
		if strings.Contains(temiz[i+1:], ":") {
			return "", errors.New("yol geçersiz")
		}
	}

	// UNC yolları yalnızca kökün kendisi UNC ise kabul edilir.
	if strings.HasPrefix(temiz, `\\`) && !herhangiUNC(kokler) {
		return "", errors.New("ağ yolları desteklenmiyor")
	}

	// Symlink/junction çözümlemesi: kök içindeki bir junction'ın dışarıya
	// (örn. C:\Windows) kaçmasını engelleyen adım budur. Dosya yoksa
	// çözümleme başarısız olur; o durumda temiz yolla devam ediyoruz ve
	// aşağıdaki Lstat kontrolü işi bitiriyor.
	if cozulmus, err := filepath.EvalSymlinks(temiz); err == nil {
		temiz = cozulmus
	}

	for _, kok := range kokler {
		kokTemiz := filepath.Clean(kok)
		if cozulmus, err := filepath.EvalSymlinks(kokTemiz); err == nil {
			kokTemiz = cozulmus
		}
		if _, altinda := indeks.AltindaMi(temiz, kokTemiz); altinda {
			// Reparse noktalarını reddediyoruz: hedefi kökün dışına
			// çıkabilir.
			bilgi, err := os.Lstat(temiz)
			if err != nil {
				return "", fmt.Errorf("dosya bulunamadı: %w", err)
			}
			if bilgi.Mode()&os.ModeSymlink != 0 {
				return "", errors.New("bağlantı dosyaları açılamaz")
			}
			return temiz, nil
		}
	}
	return "", ErrYolDisi
}

func herhangiUNC(kokler []string) bool {
	for _, k := range kokler {
		if strings.HasPrefix(filepath.Clean(k), `\\`) {
			return true
		}
	}
	return false
}

// jetonUret süreç başına rastgele bir CSRF jetonu üretir.
func jetonUret() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand.Read pratikte başarısız olmaz; olursa jeton kontrolünü
		// devre dışı bırakmak yerine sabit bir değere düşmek daha kötü
		// olurdu, o yüzden panik atıyoruz.
		panic("rastgele jeton üretilemedi: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// yerelKontrol, durum değiştiren ve süreç çalıştıran uç noktaları çapraz
// kaynaklı isteklere karşı korur.
//
// Bu gerçek bir risktir: bir HTML form POST'u çapraz kaynaklı olarak
// izinlidir, preflight istemez ve /dosya/klasorde-goster bir süreç
// çalıştırır. Katmanlı savunma:
//
//  1. Sunucu yalnızca 127.0.0.1 üzerinde dinler (bkz. main).
//  2. Sec-Fetch-Site same-origin/none olmalı (eski istemciler için Origin).
//  3. Host localhost/127.0.0.1 olmalı — bu DNS rebinding'i kapatır.
func yerelKontrol(sonraki http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !yerelHostMu(r.Host) {
			http.Error(w, "yalnızca yerel erişim", http.StatusForbidden)
			return
		}
		if !ayniKaynakMi(r) {
			http.Error(w, "çapraz kaynaklı istek reddedildi", http.StatusForbidden)
			return
		}
		sonraki.ServeHTTP(w, r)
	})
}

func yerelHostMu(host string) bool {
	makine, _, err := net.SplitHostPort(host)
	if err != nil {
		makine = host
	}
	switch strings.ToLower(makine) {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return false
}

func ayniKaynakMi(r *http.Request) bool {
	// Modern tarayıcılar bu başlığı gönderir ve sahtelenemez.
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "cross-site", "same-site":
		return false
	}
	// Başlık yoksa Origin'e düşüyoruz. Origin de yoksa (curl, eski
	// istemciler) GET'lere izin veriyoruz ama yazma isteklerini
	// reddetmiyoruz — yerel bir araçta curl ile test yapmak meşru.
	kaynak := r.Header.Get("Origin")
	if kaynak == "" {
		return true
	}
	u, err := url.Parse(kaynak)
	if err != nil {
		return false
	}
	return yerelHostMu(u.Host)
}

// jetonKontrol form gönderimlerinde CSRF jetonunu doğrular.
func (u *Uygulama) jetonKontrol(r *http.Request) bool {
	return r.FormValue("jeton") == u.jeton
}
