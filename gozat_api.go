package main

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GozatCevabi dizin gezgini yanitidir.
type GozatCevabi struct {
	Yol        string        `json:"yol"`
	Ust        string        `json:"ust"`
	Suruculer  []string      `json:"suruculer"`
	Klasorler  []GozatKlasor `json:"klasorler"`
	Okunabilir bool          `json:"okunabilir"`
	Hata       string        `json:"hata"`
}

// GozatKlasor tek bir alt klasordur.
type GozatKlasor struct {
	Ad  string `json:"ad"`
	Yol string `json:"yol"`
}

// gozatHandler sunucu tarafli dizin gezginidir.
//
// Tarayicinin <input type=file> ogesi gercek dosya sistemi yolu vermez, bu
// yuzden dizin secimini sunucudan sunuyoruz. Metin girisi yetkili kalir:
// yol yapistirmak hizli yol, gezgin kesif yoludur.
func (u *Uygulama) gozatHandler(w http.ResponseWriter, r *http.Request) {
	istenen := strings.TrimSpace(r.URL.Query().Get("yol"))

	cevap := GozatCevabi{
		Yol:       istenen,
		Suruculer: suruculeriBul(),
		Klasorler: []GozatKlasor{},
	}

	// Yol bos: yalnizca surucu harflerini gosteriyoruz.
	if istenen == "" {
		cevap.Okunabilir = true
		jsonYaz(w, http.StatusOK, cevap)
		return
	}

	mutlak, err := filepath.Abs(istenen)
	if err != nil {
		cevap.Hata = "Gecerli bir yol degil."
		jsonYaz(w, http.StatusOK, cevap)
		return
	}
	cevap.Yol = mutlak
	if ust := filepath.Dir(mutlak); ust != mutlak {
		cevap.Ust = ust
	}

	bilgi, err := os.Stat(mutlak)
	if err != nil {
		if os.IsNotExist(err) {
			cevap.Hata = "Klasor bulunamadi."
		} else {
			cevap.Hata = "Klasor okunamadi: " + err.Error()
		}
		jsonYaz(w, http.StatusOK, cevap)
		return
	}
	if !bilgi.IsDir() {
		cevap.Hata = "Bu bir klasor degil, dosya."
		jsonYaz(w, http.StatusOK, cevap)
		return
	}

	girdiler, err := os.ReadDir(mutlak)
	if err != nil {
		cevap.Hata = "Klasor icerigi listelenemedi (izin sorunu olabilir)."
		jsonYaz(w, http.StatusOK, cevap)
		return
	}
	cevap.Okunabilir = true
	for _, g := range girdiler {
		if !g.IsDir() {
			continue
		}
		cevap.Klasorler = append(cevap.Klasorler, GozatKlasor{
			Ad:  g.Name(),
			Yol: filepath.Join(mutlak, g.Name()),
		})
	}
	sort.Slice(cevap.Klasorler, func(i, j int) bool {
		return strings.ToLower(cevap.Klasorler[i].Ad) < strings.ToLower(cevap.Klasorler[j].Ad)
	})

	jsonYaz(w, http.StatusOK, cevap)
}

// suruculeriBul mevcut surucu harflerini bulur.
//
// WMI veya CGO gerekmez: A-Z arasi harfleri Stat ile deniyoruz.
func suruculeriBul() []string {
	var cikti []string
	for h := 'A'; h <= 'Z'; h++ {
		kok := string(h) + `:\`
		if _, err := os.Stat(kok); err == nil {
			cikti = append(cikti, string(h)+":")
		}
	}
	return cikti
}
