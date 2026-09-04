package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"arsiv-indeks/internal/ayarlar"
	"arsiv-indeks/internal/indeks"
	"arsiv-indeks/internal/sorgu"
	"arsiv-indeks/internal/tarayici"
)

// AramaVerisi arama sayfasının görünüm modelidir.
type AramaVerisi struct {
	Istek     sorgu.Istek
	Cikti     sorgu.Cikti
	Uzantilar []indeks.UzantiSayi
	Klasorler []string
	Sorgu     url.Values // sayfalama/sıralama bağlantıları için

	IndeksBos    bool
	CanliSayisi  int
	TaramaYarim  bool
	AramaYapildi bool
}

// aramaHandler arama sayfasını ve sonuçlarını gösterir.
//
// Form GET ile gönderilir: tüm durum URL'de yaşar, yani sonuçlar
// yer imlenebilir ve geri tuşu doğru çalışır.
func (u *Uygulama) aramaHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	istek := sorgu.Ayristir(q, u.istekAdedi())
	ay := u.Ayar()

	veri := AramaVerisi{
		Istek: istek,
		Sorgu: q,
	}

	u.ix.Oku(func(o indeks.Okuyucu) {
		veri.CanliSayisi = o.CanliSayisi()
		veri.IndeksBos = o.CanliSayisi() == 0
		veri.TaramaYarim = o.CanliSayisi() > 0 && o.SonTarama().IsZero()
		veri.Uzantilar = o.Uzantilar()
		veri.Klasorler = o.UstKlasorler(ay.TaranacakDizinler)
	})

	// Boş istek gelirse arama çalıştırmıyoruz; kullanıcıya boş form
	// gösteriyoruz. Aksi halde ilk açılışta 100.000 sonuç sıralanırdı.
	if !istek.BosMu() {
		veri.AramaYapildi = true
		veri.Cikti = sorgu.Calistir(u.ix, istek)
	}

	sv := SayfaVerisi{Baslik: "Arama", Veri: veri}
	if u.indeksHata != "" {
		sv.Hata = u.indeksHata
	}
	u.goster(w, "arama", sv)
}

// AyarlarVerisi ayarlar sayfasının görünüm modelidir.
type AyarlarVerisi struct {
	Ayar  ayarlar.Ayarlar
	Durum tarayici.Durum
	Ozet  tarayici.Ozet

	// Liste alanları formda satır başına bir öğe olarak düzenlenir.
	DizinlerMetni         string
	HaricKlasorlerMetni   string
	HaricUzantilarMetni   string
	IcerikUzantilariMetni string

	CanliSayisi int
	TerimSayisi int
	SonTarama   time.Time
	TaramaYarim bool
	VeriDizini  string
}

func (u *Uygulama) ayarlarVerisi(ay ayarlar.Ayarlar) AyarlarVerisi {
	v := AyarlarVerisi{
		Ayar:                  ay,
		Durum:                 u.is.Durum(),
		Ozet:                  u.is.Ozet(),
		DizinlerMetni:         strings.Join(ay.TaranacakDizinler, "\n"),
		HaricKlasorlerMetni:   strings.Join(ay.HaricKlasorler, "\n"),
		HaricUzantilarMetni:   strings.Join(ay.HaricUzantilar, "\n"),
		IcerikUzantilariMetni: strings.Join(ay.IcerikUzantilari, "\n"),
		VeriDizini:            u.veriDizini,
	}
	u.ix.Oku(func(o indeks.Okuyucu) {
		v.CanliSayisi = o.CanliSayisi()
		v.TerimSayisi = o.TerimSayisi()
		v.SonTarama = o.SonTarama()
		// İndekste doküman var ama tarama bitiş damgası yok: önceki tarama
		// yarım kalmış (süreç öldürülmüş veya iptal edilmiş).
		v.TaramaYarim = o.CanliSayisi() > 0 && o.SonTarama().IsZero()
	})
	return v
}

// ayarlarHandler ayarlar sayfasını gösterir.
//
// Tarama durumu sunucuda yaşadığı için sayfayı tarama ortasında yenilemek
// bedavadır: anlık görüntüyü şablona basıyoruz, JS oradan devam ediyor.
func (u *Uygulama) ayarlarHandler(w http.ResponseWriter, r *http.Request) {
	sv := SayfaVerisi{Baslik: "Ayarlar", Veri: u.ayarlarVerisi(u.Ayar())}
	if u.indeksHata != "" {
		sv.Hata = u.indeksHata
	}
	if r.URL.Query().Get("kaydedildi") == "1" {
		sv.Basari = "Ayarlar kaydedildi."
		// Kullanıcı indeksinin değiştiğini bilmeli.
		if n, err := strconv.Atoi(r.URL.Query().Get("temizlenen")); err == nil && n > 0 {
			sv.Basari += fmt.Sprintf(
				" Artık taranmayan dizinlerdeki %d dosya indeksten düşürüldü.", n)
		}
	}
	u.goster(w, "ayarlar", sv)
}

// ayarlarKaydetHandler ayarları doğrular ve kaydeder.
func (u *Uygulama) ayarlarKaydetHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form okunamadı", http.StatusBadRequest)
		return
	}
	if !u.jetonKontrol(r) {
		http.Error(w, "oturum jetonu geçersiz, sayfayı yenileyip tekrar deneyin",
			http.StatusForbidden)
		return
	}

	ay := u.Ayar()
	ay.TaranacakDizinler = satirlar(r.FormValue("dizinler"))
	ay.HaricKlasorler = satirlar(r.FormValue("haricKlasorler"))
	ay.HaricUzantilar = satirlar(r.FormValue("haricUzantilar"))
	ay.IcerikUzantilari = satirlar(r.FormValue("icerikUzantilari"))
	ay.IcerikCikarma = r.FormValue("icerikCikarma") == "1"
	ay.MaksDosyaBoyutuMB = formSayi(r, "maksDosyaBoyutuMB", ay.MaksDosyaBoyutuMB)
	ay.MaksMetinKarakter = formSayi(r, "maksMetinKarakter", ay.MaksMetinKarakter)
	ay.IsciSayisi = formSayi(r, "isciSayisi", ay.IsciSayisi)
	ay.SayfaBasiSonuc = formSayi(r, "sayfaBasiSonuc", ay.SayfaBasiSonuc)
	ay.DokumanBelirtecCap = formSayi(r, "dokumanBelirtecCap", ay.DokumanBelirtecCap)
	ay.Duzelt()

	if hatalar := ay.Dogrula(); len(hatalar) > 0 {
		// Hatalı ayarları kaydetmiyoruz ama formda kullanıcının yazdıklarını
		// koruyoruz, yeniden yazmak zorunda kalmasın.
		veri := u.ayarlarVerisi(ay)
		u.goster(w, "ayarlar", SayfaVerisi{
			Baslik: "Ayarlar",
			Hata:   strings.Join(hatalar, " "),
			Veri:   veri,
		})
		return
	}

	if err := ay.Kaydet(u.veriDizini); err != nil {
		u.goster(w, "ayarlar", SayfaVerisi{
			Baslik: "Ayarlar",
			Hata:   "Ayarlar kaydedilemedi: " + err.Error(),
			Veri:   u.ayarlarVerisi(ay),
		})
		return
	}
	u.AyarYaz(ay)

	// Taranacak dizin listesinden çıkarılan köklerin dokümanlarını HEMEN
	// düşürüyoruz; kullanıcının bunun için tarama yapmasını beklemek,
	// arada arama sonuçlarında açılamayan hayalet kayıtlar bırakırdı.
	temizlenen := u.ix.KoklerDisindakileriSil(ay.TaranacakDizinler)
	if temizlenen > 0 {
		if u.ix.TombstoneOrani() > 0.20 {
			u.ix.Sikistir()
		}
		if err := u.ix.Kaydet(u.veriDizini); err != nil {
			log.Printf("indeks kaydedilemedi: %v", err)
		}
	}

	// POST sonrası yönlendirme: kullanıcı sayfayı yenilediğinde form
	// yeniden gönderilmesin.
	http.Redirect(w, r,
		fmt.Sprintf("/ayarlar?kaydedildi=1&temizlenen=%d", temizlenen),
		http.StatusSeeOther)
}

// satirlar çok satırlı bir form alanını temizlenmiş dilime çevirir.
func satirlar(s string) []string {
	var cikti []string
	for _, satir := range strings.Split(s, "\n") {
		// Kullanıcılar yolları tırnak içinde yapıştırır (Explorer "yol olarak
		// kopyala" böyle verir); tırnakları temizliyoruz.
		satir = strings.Trim(strings.TrimSpace(satir), `"`)
		if satir != "" {
			cikti = append(cikti, satir)
		}
	}
	return cikti
}

func formSayi(r *http.Request, ad string, varsayilan int) int {
	s := strings.TrimSpace(r.FormValue(ad))
	if s == "" {
		return varsayilan
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return varsayilan
	}
	return n
}
