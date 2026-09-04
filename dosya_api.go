package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"arsiv-indeks/internal/cikarim"
	"arsiv-indeks/internal/sorgu"
)

// ParcacikZamanAsimi tek bir parçacık isteği için üst sınır.
const ParcacikZamanAsimi = 3 * time.Second

// dosyaIndirHandler bulunan dosyayı tarayıcıya gönderir.
func (u *Uygulama) dosyaIndirHandler(w http.ResponseWriter, r *http.Request) {
	yol, err := u.istenenYol(r.URL.Query().Get("yol"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	f, err := os.Open(yol)
	if err != nil {
		http.Error(w, "dosya açılamadı", http.StatusNotFound)
		return
	}
	defer f.Close()

	bilgi, err := f.Stat()
	if err != nil || bilgi.IsDir() {
		http.Error(w, "dosya okunamadı", http.StatusNotFound)
		return
	}

	ad := filepath.Base(yol)
	// RFC 5987: Türkçe karakterli dosya adları bozulmadan gitsin.
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		"attachment; filename*=UTF-8''%s", url.PathEscape(ad)))
	http.ServeContent(w, r, ad, bilgi.ModTime(), f)
}

// klasordeGosterHandler dosyayı Windows Gezgini'nde seçili olarak açar.
//
// Bu uç nokta bir SÜREÇ ÇALIŞTIRDIĞI için POST'tur ve yerelKontrol
// middleware'i ile jeton kontrolünden geçer.
func (u *Uygulama) klasordeGosterHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form okunamadı", http.StatusBadRequest)
		return
	}
	if !u.jetonKontrol(r) {
		http.Error(w, "oturum jetonu geçersiz, sayfayı yenileyin", http.StatusForbidden)
		return
	}

	yol, err := u.istenenYol(r.FormValue("yol"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := klasordeGoster(yol); err != nil {
		log.Printf("gezgin açılamadı: %v", err)
		http.Error(w, "gezgin açılamadı", http.StatusInternalServerError)
		return
	}

	// JS araya girdiyse (fetch, Accept: application/json) JSON dönüyoruz ve
	// kullanıcı sonuç listesinde kalıyor.
	//
	// JS kapalıysa istek düz bir HTML form gönderimidir; JSON döndürürsek
	// tarayıcı sayfadan ayrılıp ham {"tamam":true} gösterir. O yüzden
	// geldiği sayfaya geri yönlendiriyoruz.
	if jsonIstiyorMu(r) {
		jsonYaz(w, http.StatusOK, map[string]any{"tamam": true})
		return
	}
	http.Redirect(w, r, geriDonulecekYol(r), http.StatusSeeOther)
}

// jsonIstiyorMu istemcinin JSON beklediğini söyler.
func jsonIstiyorMu(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// geriDonulecekYol, Referer aynı kaynaktan geliyorsa onu, değilse arama
// sayfasını döner.
//
// Referer'ı doğrudan kullanmıyoruz: dışarıdan gelen bir değer açık
// yönlendirme (open redirect) aracına dönüşebilir. Yalnızca yol kısmını
// alıyoruz, host ve şemayı atıyoruz.
func geriDonulecekYol(r *http.Request) string {
	ref := r.Header.Get("Referer")
	if ref == "" {
		return "/arama"
	}
	u, err := url.Parse(ref)
	if err != nil || u.Path == "" || !strings.HasPrefix(u.Path, "/") {
		return "/arama"
	}
	geri := u.Path
	if u.RawQuery != "" {
		geri += "?" + u.RawQuery
	}
	return geri
}

// istenenYol kullanıcıdan gelen yolu güvenlik sınırından geçirir.
//
// Taranan kökler ayarlardan okunuyor: indekste olmayan ama kök altında olan
// bir dosya da açılabilir (tarama sonrası eklenmiş olabilir), ama kök
// dışındaki hiçbir şeye erişilemez.
func (u *Uygulama) istenenYol(ham string) (string, error) {
	kokler := u.Ayar().TaranacakDizinler
	if len(kokler) == 0 {
		return "", errors.New("taranacak dizin ayarlanmamış")
	}
	yol, err := GuvenliYol(ham, kokler)
	if err != nil {
		return "", fmt.Errorf("erişim reddedildi: %w", err)
	}
	return yol, nil
}

// ---------------------------------------------------------------- parçacık

// parcacikHandler tek bir sonuç satırı için vurgulu içerik alıntısı döner.
//
// Neden ayrı bir uç nokta: 300 sayfalık bir PDF'ten metin çıkarmak saf Go'da
// 1-5 saniye sürer. 20 sonucu arama handler'ında satır içi çıkarsaydık sayfa
// 10 saniyede açılırdı. Sonuçlar hemen basılıyor, parçacıklar satır satır
// buradan çekiliyor.
func (u *Uygulama) parcacikHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	yol, err := u.istenenYol(q.Get("yol"))
	if err != nil {
		jsonYaz(w, http.StatusForbidden, map[string]any{"hata": err.Error()})
		return
	}

	parcalar := sorgu.SorguParcalari(q.Get("q"))
	if len(parcalar) == 0 {
		jsonYaz(w, http.StatusOK, map[string]any{"parcalar": nil})
		return
	}

	icerik, tamam := u.dosyaMetni(r.Context(), yol)
	if !tamam {
		jsonYaz(w, http.StatusOK, map[string]any{"parcalar": nil})
		return
	}

	jsonYaz(w, http.StatusOK, map[string]any{
		"parcalar": sorgu.Parcacik(icerik, parcalar, 130),
	})
}

// dosyaMetni önbellekten veya diskten dosya metnini alır.
func (u *Uygulama) dosyaMetni(ctx context.Context, yol string) (string, bool) {
	if s, v := u.onbellek.Al(yol); v {
		return s, s != ""
	}

	// Eşzamanlı çıkarımı sınırlıyoruz: 20 satırın aynı anda büyük PDF açması
	// sunucuyu dize eder.
	select {
	case u.parcacikSinir <- struct{}{}:
		defer func() { <-u.parcacikSinir }()
	case <-ctx.Done():
		return "", false
	}

	dctx, dur := context.WithTimeout(ctx, ParcacikZamanAsimi)
	defer dur()

	ay := u.Ayar()
	icerik, durum := cikarim.GuvenliCikar(dctx, yol,
		cikarim.Secenekler{MaksKarakter: ay.MaksMetinKarakter})

	// Başarısız çıkarımı da önbelleğe alıyoruz (boş dize olarak): aynı bozuk
	// PDF her sayfa yenilemesinde yeniden denenmesin.
	u.onbellek.Yaz(yol, icerik)

	if durum != cikarim.DurumTam && durum != cikarim.DurumKirpildi {
		return "", false
	}
	return icerik, icerik != ""
}

// ---------------------------------------------------------------- önbellek

// metinOnbellek çıkarılmış dosya metinlerini sınırlı sayıda tutar.
//
// FIFO tahliye kullanıyoruz, LRU değil: parçacık istekleri tek bir sonuç
// sayfası için patlama halinde gelir, bu erişim deseninde FIFO ile LRU
// arasındaki fark ölçülemez ve FIFO çok daha az kod.
type metinOnbellek struct {
	kilit   sync.Mutex
	kayit   map[string]string
	sira    []string
	enFazla int
	// baytSiniri toplam bellek kullanımını sınırlar.
	baytSiniri int
	baytToplam int
}

func yeniMetinOnbellek(enFazla int) *metinOnbellek {
	return &metinOnbellek{
		kayit:      make(map[string]string, enFazla),
		enFazla:    enFazla,
		baytSiniri: 200 << 20,
	}
}

func (o *metinOnbellek) Al(anahtar string) (string, bool) {
	o.kilit.Lock()
	defer o.kilit.Unlock()
	s, v := o.kayit[anahtar]
	return s, v
}

func (o *metinOnbellek) Yaz(anahtar, deger string) {
	o.kilit.Lock()
	defer o.kilit.Unlock()

	if _, v := o.kayit[anahtar]; v {
		return
	}
	o.kayit[anahtar] = deger
	o.sira = append(o.sira, anahtar)
	o.baytToplam += len(deger)

	for len(o.sira) > o.enFazla || (o.baytToplam > o.baytSiniri && len(o.sira) > 1) {
		eski := o.sira[0]
		o.sira = o.sira[1:]
		o.baytToplam -= len(o.kayit[eski])
		delete(o.kayit, eski)
	}
}
