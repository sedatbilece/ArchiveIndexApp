package tarayici

import (
	"sync/atomic"
	"time"
)

// Aşama adları. Ayarlar sayfasındaki ilerleme göstergesinde gösterilir.
const (
	AsamaBosta        = "bosta"
	AsamaSayiliyor    = "sayiliyor"
	AsamaTaraniyor    = "taraniyor"
	AsamaKaydediliyor = "kaydediliyor"
	AsamaTamamlandi   = "tamamlandi"
	AsamaIptal        = "iptal"
	AsamaHata         = "hata"
)

// AsamaAciklama aşamanın kullanıcıya gösterilecek Türkçe karşılığı.
func AsamaAciklama(a string) string {
	switch a {
	case AsamaBosta:
		return "Bekliyor"
	case AsamaSayiliyor:
		return "Dosyalar sayılıyor"
	case AsamaTaraniyor:
		return "İçerik çıkarılıyor ve indeksleniyor"
	case AsamaKaydediliyor:
		return "İndeks diske yazılıyor"
	case AsamaTamamlandi:
		return "Tamamlandı"
	case AsamaIptal:
		return "İptal edildi"
	case AsamaHata:
		return "Hata"
	}
	return a
}

// Ilerleme tarama sayaçlarını tutar.
//
// Tamamen atomic; kilit yok. Durum uç noktası saniyede bir okuyor, tarayıcı
// işçileri sürekli yazıyor — burada mutex kullanmak gereksiz çekişme olurdu.
type Ilerleme struct {
	Toplam       atomic.Int64 // birinci geçişte sayılan uygun dosya sayısı
	Islenen      atomic.Int64
	Atlanan      atomic.Int64 // değişmemiş (artımlı tarama), hariç tutulan
	Hatali       atomic.Int64
	IcerikOkunan atomic.Int64
	SadeceAd     atomic.Int64 // içeriği okunamayan ama adıyla indekslenen
	Silinen      atomic.Int64

	asama    atomic.Value // string
	sonDosya atomic.Value // string
}

// YeniIlerleme boş bir sayaç kümesi oluşturur.
func YeniIlerleme() *Ilerleme {
	il := &Ilerleme{}
	il.AsamaYaz(AsamaBosta)
	il.SonDosyaYaz("")
	return il
}

// AsamaYaz mevcut aşamayı günceller.
func (il *Ilerleme) AsamaYaz(a string) { il.asama.Store(a) }

// Asama mevcut aşamayı döner.
func (il *Ilerleme) Asama() string {
	if v, ok := il.asama.Load().(string); ok {
		return v
	}
	return AsamaBosta
}

// SonDosyaYaz işlenen son dosyayı kaydeder ("şu an: ..." satırı için).
func (il *Ilerleme) SonDosyaYaz(y string) { il.sonDosya.Store(y) }

// SonDosya işlenen son dosyayı döner.
func (il *Ilerleme) SonDosya() string {
	if v, ok := il.sonDosya.Load().(string); ok {
		return v
	}
	return ""
}

// Ozet tarama sonunda kullanıcıya gösterilecek toplamlardır.
type Ozet struct {
	Islenen      int64 `json:"islenen"`
	IcerikOkunan int64 `json:"icerikOkunan"`
	SadeceAd     int64 `json:"sadeceAd"`
	Atlanan      int64 `json:"atlanan"`
	Hatali       int64 `json:"hatali"`
	Silinen      int64 `json:"silinen"`
	SureSaniye   int64 `json:"sureSaniye"`
}

// OzetAl sayaçlardan bir özet üretir.
func (il *Ilerleme) OzetAl(sure time.Duration) Ozet {
	return Ozet{
		Islenen:      il.Islenen.Load(),
		IcerikOkunan: il.IcerikOkunan.Load(),
		SadeceAd:     il.SadeceAd.Load(),
		Atlanan:      il.Atlanan.Load(),
		Hatali:       il.Hatali.Load(),
		Silinen:      il.Silinen.Load(),
		SureSaniye:   int64(sure.Seconds()),
	}
}
