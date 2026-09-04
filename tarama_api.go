package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"arsiv-indeks/internal/tarayici"
)

// taramaBaslaHandler arka planda tarama başlatır.
//
// Zaten bir tarama sürüyorsa 409 Conflict döner; istemci bunu "Tarama zaten
// sürüyor" mesajına çevirir. Böylece kullanıcı butona iki kez bastığında
// ikinci bir tarama başlamaz.
func (u *Uygulama) taramaBaslaHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form okunamadı", http.StatusBadRequest)
		return
	}
	if !u.jetonKontrol(r) {
		http.Error(w, "oturum jetonu geçersiz, sayfayı yenileyin", http.StatusForbidden)
		return
	}

	ay := u.Ayar()
	if hatalar := ay.Dogrula(); len(hatalar) > 0 {
		jsonYaz(w, http.StatusBadRequest, map[string]any{
			"hata": hatalar[0],
		})
		return
	}

	// Tam yeniden tarama istendiyse indeksi boşaltıyoruz; aksi halde
	// artımlı tarama yapılır.
	if r.FormValue("tamTarama") == "1" {
		u.ix.Temizle()
	}

	if err := u.is.Basla(u.ix, ay, u.veriDizini); err != nil {
		if errors.Is(err, tarayici.ErrTaramaSuruyor) {
			jsonYaz(w, http.StatusConflict, map[string]any{
				"hata": "Tarama zaten sürüyor.",
			})
			return
		}
		jsonYaz(w, http.StatusInternalServerError, map[string]any{"hata": err.Error()})
		return
	}

	log.Printf("tarama başladı: %v", ay.TaranacakDizinler)
	jsonYaz(w, http.StatusAccepted, u.is.Durum())
}

// taramaIptalHandler sürmekte olan taramayı durdurur.
func (u *Uygulama) taramaIptalHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form okunamadı", http.StatusBadRequest)
		return
	}
	if !u.jetonKontrol(r) {
		http.Error(w, "oturum jetonu geçersiz, sayfayı yenileyin", http.StatusForbidden)
		return
	}
	if err := u.is.Iptal(); err != nil {
		jsonYaz(w, http.StatusConflict, map[string]any{
			"hata": "Sürmekte olan tarama yok.",
		})
		return
	}
	jsonYaz(w, http.StatusOK, u.is.Durum())
}

// taramaDurumHandler ilerleme durumunu JSON olarak döner. İstemci saniyede
// bir yoklar.
func (u *Uygulama) taramaDurumHandler(w http.ResponseWriter, r *http.Request) {
	d := u.is.Durum()
	// İndeks istatistiklerini de ekliyoruz ki tarama bitince sayfayı
	// yenilemeden güncel sayılar görünsün.
	jsonYaz(w, http.StatusOK, d)
}

func jsonYaz(w http.ResponseWriter, kod int, govde any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(kod)
	if err := json.NewEncoder(w).Encode(govde); err != nil {
		log.Printf("json yazılamadı: %v", err)
	}
}
