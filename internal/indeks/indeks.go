// Paket indeks, arşiv dosyalarının metadata ve ters indeksini (inverted
// index) bellekte tutar ve encoding/gob ile diske yazar.
//
// # Eşzamanlılık modeli
//
// Tek bir *Indeks süreç boyunca yaşar ve kendi RWMutex'i ile korunur.
// Tarayıcı yazmaları PARTİLER halinde yapar (bkz. EkleToplu): yazma kilidi
// her partide ~1 ms tutulur, taramanın toplam süresinin binde birinden azı.
// Aramalar bu arada RLock ile kesintisiz çalışır.
//
// Alternatif olarak "değişmez anlık görüntü + atomic.Pointer takası" da
// düşünüldü ve REDDEDİLDİ: artımlı tarama ile uyumsuz. Değişmemiş bir
// dosyanın gönderilerini yeni bir indekse taşıyabilmek için doküman başına
// terim listesi (ileri indeks) saklamak gerekirdi ve bu belleği ikiye
// katlardı. Tombstone'lu yerinde değişiklik bu maliyeti tamamen kaldırıyor.
//
// # Temsil
//
// v1 temsili düz map[string][]uint32'dir: okunması ve ayıklanması kolay,
// ~30.000 dosyaya kadar rahat. 100.000 dosyada ~1 GB RSS'e çıkar. Bu yüzden
// dışarıya yalnızca Okuyucu arayüzü açılıyor — temsili paketlenmiş bir
// arena'ya çevirmek yalnızca bu paketi etkiler, çağıranları etkilemez.
package indeks

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"arsiv-indeks/internal/atomik"
	"arsiv-indeks/internal/metin"
)

// SurumNo indeks şema sürümü. Uyuşmazlıkta indeks migrate edilmez, atılır ve
// yeniden tarama istenir — arşiv indeksini yeniden üretmek her zaman mümkün.
const SurumNo = 1

// DosyaAdi indeks dosyasının adı.
const DosyaAdi = "indeks.gob"

// Belge indekslenmiş tek bir dosyadır.
//
// Yol tek gerçek veridir; Ad, Uzanti ve Klasor bundan dilimlenir. Alt dilimler
// aynı arka diziyi paylaştığı için ek veri maliyeti sıfırdır — 100.000 dosyada
// metadata toplamı ~30 MB.
type Belge struct {
	Yol   string
	Boyut int64
	Zaman int64 // Unix nano
	Durum string
}

// Ad dosya adını döner.
func (b Belge) Ad() string { return filepath.Base(b.Yol) }

// Uzanti küçük harfli uzantıyı döner (nokta dahil, örn ".pdf").
func (b Belge) Uzanti() string { return strings.ToLower(filepath.Ext(b.Yol)) }

// Klasor dosyanın bulunduğu klasörü döner.
func (b Belge) Klasor() string { return filepath.Dir(b.Yol) }

// Vakit değişim zamanını time.Time olarak döner.
func (b Belge) Vakit() time.Time { return time.Unix(0, b.Zaman) }

// Girdi indekse eklenecek bir dosyayı tarif eder.
type Girdi struct {
	Yol         string
	Boyut       int64
	Zaman       int64
	Durum       string
	Belirtecler []string // içerikten çıkarılmış tekil belirteçler
}

// UzantiSayi arama formundaki uzantı filtresini beslemek için kullanılır.
type UzantiSayi struct {
	Uzanti string
	Sayi   int
}

// Indeks bellekteki arama yapısıdır. Doğrudan alanlarına dokunulmaz;
// okumak için Oku, yazmak için EkleToplu/Sil* kullanılır.
type Indeks struct {
	kilit sync.RWMutex

	// Aşağıdaki alanlar gob ile diske yazılır. Adları büyük harfle başlıyor
	// çünkü gob yalnızca dışa açık alanları kodlar.
	Surum    int
	Belgeler []Belge
	YolID    map[string]uint32 // Anahtar(yol) -> doküman kimliği

	// Gonderiler İÇERİKTEN çıkarılmış terimleri tutar.
	// AdGonderiler ise dosya YOLUNDAN çıkarılmış terimleri tutar.
	//
	// İkisini ayrı tutmamızın sebebi: kullanıcı "sadece içerikte ara"
	// dediğinde dosya adı eşleşmeleri gelmemeli. Tek haritada tutsaydık
	// bunu ayırt etmenin yolu olmazdı. Yol terimleri doküman başına 5-15
	// tanedir, yani ek bellek maliyeti içeriğin yanında ihmal edilebilir.
	Gonderiler   map[string][]uint32 // terim -> artan sıralı doküman kimlikleri
	AdGonderiler map[string][]uint32

	Silinmis  []uint64 // tombstone bit kümesi
	SilinmisN int      // tombstone sayısı (bit saymamak için)
	SonTarama int64    // biten son taramanın zamanı; 0 = yarım kaldı

	// Türetilmiş alanlar: diske yazılmaz, yüklemeden sonra Hazirla üretir.
	adNorm  []string
	yolNorm []string
}

// Yeni boş bir indeks oluşturur.
func Yeni() *Indeks {
	return &Indeks{
		Surum:        SurumNo,
		YolID:        make(map[string]uint32),
		Gonderiler:   make(map[string][]uint32),
		AdGonderiler: make(map[string][]uint32),
	}
}

// ---------------------------------------------------------------- okuma

// Okuyucu, Oku çağrısı süresince geçerli olan salt-okunur indeks görünümüdür.
// Bu arayüzün dışına referans kaçırmayın; Oku dönünce kilit bırakılır.
type Okuyucu struct{ ix *Indeks }

// Oku, okuma kilidi altında fn'i çalıştırır. Sorgu motoru tüm işini tek bir
// RLock içinde yapar; böylece gönderi listelerini kopyalamamız gerekmez.
func (ix *Indeks) Oku(fn func(Okuyucu)) {
	ix.kilit.RLock()
	defer ix.kilit.RUnlock()
	fn(Okuyucu{ix})
}

// BelgeSayisi tombstone'lar dahil toplam doküman yuvası sayısını döner.
// Doküman kimlikleri [0, BelgeSayisi) aralığındadır.
func (o Okuyucu) BelgeSayisi() int { return len(o.ix.Belgeler) }

// CanliSayisi silinmemiş doküman sayısını döner.
func (o Okuyucu) CanliSayisi() int { return len(o.ix.Belgeler) - o.ix.SilinmisN }

// TerimSayisi sözlük büyüklüğünü döner (içerik + yol terimleri).
func (o Okuyucu) TerimSayisi() int {
	return len(o.ix.Gonderiler) + len(o.ix.AdGonderiler)
}

// SonTarama biten son taramanın zamanını döner. Sıfır dönerse ve doküman
// varsa, önceki tarama yarım kalmıştır.
func (o Okuyucu) SonTarama() time.Time {
	if o.ix.SonTarama == 0 {
		return time.Time{}
	}
	return time.Unix(0, o.ix.SonTarama)
}

// Belge kimliğe karşılık gelen dokümanı döner.
func (o Okuyucu) Belge(id uint32) Belge { return o.ix.Belgeler[id] }

// AdNorm dosya adının normalize edilmiş halini döner (alt dize araması için).
func (o Okuyucu) AdNorm(id uint32) string { return o.ix.adNorm[id] }

// YolNorm tam yolun normalize edilmiş halini döner (klasör filtresi için).
func (o Okuyucu) YolNorm(id uint32) string { return o.ix.yolNorm[id] }

// SilinmisMi dokümanın tombstone'lu olup olmadığını söyler.
func (o Okuyucu) SilinmisMi(id uint32) bool { return o.ix.silinmisMi(id) }

// Gonderiler terimi İÇERİĞİNDE geçiren dokümanların kimliklerini artan
// sırada döner. Dönen dilim indeksin içindedir; DEĞİŞTİRİLMEMELİDİR.
func (o Okuyucu) Gonderiler(terim string) []uint32 { return o.ix.Gonderiler[terim] }

// AdGonderiler terimi YOLUNDA geçiren dokümanların kimliklerini döner.
func (o Okuyucu) AdGonderiler(terim string) []uint32 { return o.ix.AdGonderiler[terim] }

// Df terimin içerik doküman frekansını döner.
func (o Okuyucu) Df(terim string) int { return len(o.ix.Gonderiler[terim]) }

// ToplamDf idf hesabı için içerik ve yol frekanslarının toplamını döner.
// Terim ikisinde de geçen dokümanlar iki kez sayılır; idf için bu yaklaşım
// yeterince iyi.
func (o Okuyucu) ToplamDf(terim string) int {
	return len(o.ix.Gonderiler[terim]) + len(o.ix.AdGonderiler[terim])
}

// Uzantilar canlı dokümanların uzantılarını sayılarıyla, çoktan aza sıralı
// döner. Arama formundaki uzantı filtresini besler.
func (o Okuyucu) Uzantilar() []UzantiSayi {
	sayac := make(map[string]int, 64)
	for id, b := range o.ix.Belgeler {
		if o.ix.silinmisMi(uint32(id)) {
			continue
		}
		u := b.Uzanti()
		if u == "" {
			u = "(uzantısız)"
		}
		sayac[u]++
	}
	cikti := make([]UzantiSayi, 0, len(sayac))
	for u, n := range sayac {
		cikti = append(cikti, UzantiSayi{Uzanti: u, Sayi: n})
	}
	sort.Slice(cikti, func(i, j int) bool {
		if cikti[i].Sayi != cikti[j].Sayi {
			return cikti[i].Sayi > cikti[j].Sayi
		}
		return cikti[i].Uzanti < cikti[j].Uzanti
	})
	return cikti
}

// UstKlasorler verilen köklerin bir seviye altındaki klasörleri döner.
// Arama formundaki klasör açılır listesini besler.
func (o Okuyucu) UstKlasorler(kokler []string) []string {
	gorulen := make(map[string]string) // anahtar -> gösterilecek
	for id, b := range o.ix.Belgeler {
		if o.ix.silinmisMi(uint32(id)) {
			continue
		}
		for _, kok := range kokler {
			kalan, altinda := AltindaMi(b.Yol, kok)
			if !altinda {
				continue
			}
			if i := strings.IndexRune(kalan, filepath.Separator); i > 0 {
				alt := filepath.Join(kok, kalan[:i])
				gorulen[Anahtar(alt)] = alt
			}
			break
		}
	}
	cikti := make([]string, 0, len(gorulen))
	for _, v := range gorulen {
		cikti = append(cikti, v)
	}
	sort.Strings(cikti)
	return cikti
}

// Anahtar bir yolun kimlik anahtarını üretir.
//
// Bu, arama normalizasyonundan (metin.Normalize) KASITLI olarak ayrıdır:
// Normalize ı/i/İ hepsini 'i'ye katlar, ki arama için doğrudur ama kimlik için
// yanlıştır — "Rapor_İş.pdf" ile "Rapor_is.pdf" farklı dosyalardır ve aynı
// anahtara düşerlerse biri sessizce indeksten kaybolur. Windows dosya
// sistemi büyük/küçük harf duyarsız olduğu için ToLower yeterli kimliktir.
func Anahtar(yol string) string {
	return strings.ToLower(filepath.Clean(yol))
}

// AltindaMi yolun kökün altında olup olmadığını söyler ve kökten sonraki
// kalan kısmı döner.
//
// Karşılaştırma orijinal baytlar üzerinde, ayırıcı sınırına saygılı ve
// büyük/küçük harf duyarsız yapılır. Sınır kontrolü olmadan "C:\Arsiv-gizli"
// yolu "C:\Arsiv" kökünün altında sayılırdı.
func AltindaMi(yol, kok string) (kalan string, tamam bool) {
	kok = strings.TrimRight(filepath.Clean(kok), string(filepath.Separator))
	if len(yol) < len(kok) {
		return "", false
	}
	if !strings.EqualFold(yol[:len(kok)], kok) {
		return "", false
	}
	if len(yol) == len(kok) {
		return "", true
	}
	if yol[len(kok)] != filepath.Separator {
		return "", false
	}
	return yol[len(kok)+1:], true
}

// ---------------------------------------------------------------- yazma

// EkleToplu bir parti girdiyi indekse yazar. Var olan bir yol yeniden
// eklenirse eski dokümanı tombstone'lar (yeniden adlandırma = sil + ekle;
// kimlik yoldur).
//
// Tarayıcı bunu 512'lik partilerle çağırır: yazma kilidi kısa tutulur, arama
// gecikmesi hissedilmez.
func (ix *Indeks) EkleToplu(girdiler []Girdi) {
	if len(girdiler) == 0 {
		return
	}
	ix.kilit.Lock()
	defer ix.kilit.Unlock()

	for _, g := range girdiler {
		ak := Anahtar(g.Yol)
		if eski, v := ix.YolID[ak]; v {
			ix.tombstone(eski)
		}
		id := uint32(len(ix.Belgeler))
		ix.Belgeler = append(ix.Belgeler, Belge{
			Yol:   g.Yol,
			Boyut: g.Boyut,
			Zaman: g.Zaman,
			Durum: g.Durum,
		})
		ix.adNorm = append(ix.adNorm, metin.Normalize(filepath.Base(g.Yol)))
		ix.yolNorm = append(ix.yolNorm, metin.Normalize(g.Yol))
		ix.YolID[ak] = id
		ix.bitBuyut(id)

		for _, t := range metin.TekilBelirtecler(g.Yol, 0) {
			ix.gonderiEkle(ix.AdGonderiler, t, id)
		}
		for _, t := range g.Belirtecler {
			ix.gonderiEkle(ix.Gonderiler, t, id)
		}
	}
}

// gonderiEkle terimin gönderi listesine kimliği ekler. Kimlikler artan
// sırada üretildiği için listeler doğal olarak sıralı kalır.
func (ix *Indeks) gonderiEkle(harita map[string][]uint32, terim string, id uint32) {
	g := harita[terim]
	if n := len(g); n > 0 && g[n-1] == id {
		return // aynı doküman için tekrar
	}
	harita[terim] = append(g, id)
}

// SilYollar verilen yolları tombstone'lar ve silinen sayısını döner.
func (ix *Indeks) SilYollar(yollar []string) int {
	ix.kilit.Lock()
	defer ix.kilit.Unlock()
	n := 0
	for _, y := range yollar {
		if id, v := ix.YolID[Anahtar(y)]; v && !ix.silinmisMi(id) {
			ix.tombstone(id)
			n++
		}
	}
	return n
}

// GorulmeyenleriSil, verilen köklerin altındaki ama bu taramada görülmeyen
// dokümanları tombstone'lar. Silinen dosyaları indeksten düşürmenin yolu bu.
//
// gorulen kümesi Anahtar(yol) ile anahtarlanmış olmalıdır.
//
// Yalnızca taranan köklerin altına bakar: A kökünü taramak B kökünün
// dokümanlarını silmemeli.
func (ix *Indeks) GorulmeyenleriSil(gorulen map[string]struct{}, kokler []string) int {
	ix.kilit.Lock()
	defer ix.kilit.Unlock()

	n := 0
	for id := range ix.Belgeler {
		kid := uint32(id)
		if ix.silinmisMi(kid) {
			continue
		}
		yol := ix.Belgeler[id].Yol
		altinda := false
		for _, k := range kokler {
			if _, v := AltindaMi(yol, k); v {
				altinda = true
				break
			}
		}
		if !altinda {
			continue
		}
		if _, v := gorulen[Anahtar(yol)]; !v {
			ix.tombstone(kid)
			n++
		}
	}
	return n
}

// KoklerDisindakileriSil, verilen köklerin HİÇBİRİNİN altında olmayan canlı
// dokümanları tombstone'lar ve sayısını döner.
//
// Bu, kullanıcı taranacak dizini değiştirdiğinde gerekir. GorulmeyenleriSil
// yalnızca taranan köklerin ALTINDAKİ dosyalara bakar (çok köklü kurulumları
// korumak için); bu yüzden ayarlardan çıkarılan bir kökün dokümanları
// kendiliğinden düşmez. Düşmezlerse arama sonuçlarında görünüp açılamayan
// hayalet kayıtlara dönüşürler: GuvenliYol o kökü artık tanımaz.
//
// Güvenlik freni: kokler boşsa hiçbir şey silinmez. Bozuk bir ayar dosyası
// yüzünden tüm indeksin sessizce silinmesini istemiyoruz.
func (ix *Indeks) KoklerDisindakileriSil(kokler []string) int {
	if len(kokler) == 0 {
		return 0
	}
	ix.kilit.Lock()
	defer ix.kilit.Unlock()

	n := 0
	for id := range ix.Belgeler {
		kid := uint32(id)
		if ix.silinmisMi(kid) {
			continue
		}
		yol := ix.Belgeler[id].Yol
		altinda := false
		for _, k := range kokler {
			if _, v := AltindaMi(yol, k); v {
				altinda = true
				break
			}
		}
		if !altinda {
			ix.tombstone(kid)
			n++
		}
	}
	return n
}

// Degismemis dosyanın indekste aynı boyut ve değişim zamanıyla bulunduğunu
// söyler. Artımlı taramanın kalbi: doğruysa çıkarımı tamamen atlıyoruz.
func (ix *Indeks) Degismemis(yol string, boyut, zaman int64) bool {
	ix.kilit.RLock()
	defer ix.kilit.RUnlock()
	id, v := ix.YolID[Anahtar(yol)]
	if !v || ix.silinmisMi(id) {
		return false
	}
	b := ix.Belgeler[id]
	return b.Boyut == boyut && b.Zaman == zaman
}

// TaramaBitti biten taramanın zamanını damgalar.
func (ix *Indeks) TaramaBitti(t time.Time) {
	ix.kilit.Lock()
	defer ix.kilit.Unlock()
	ix.SonTarama = t.UnixNano()
}

// Temizle indeksi tamamen boşaltır (tam yeniden tarama öncesi).
func (ix *Indeks) Temizle() {
	ix.kilit.Lock()
	defer ix.kilit.Unlock()
	ix.Belgeler = nil
	ix.adNorm = nil
	ix.yolNorm = nil
	ix.YolID = make(map[string]uint32)
	ix.Gonderiler = make(map[string][]uint32)
	ix.AdGonderiler = make(map[string][]uint32)
	ix.Silinmis = nil
	ix.SilinmisN = 0
	ix.SonTarama = 0
}

// TombstoneOrani ölü doküman oranını döner. %20'yi geçince Sikistir çağrılır.
func (ix *Indeks) TombstoneOrani() float64 {
	ix.kilit.RLock()
	defer ix.kilit.RUnlock()
	if len(ix.Belgeler) == 0 {
		return 0
	}
	return float64(ix.SilinmisN) / float64(len(ix.Belgeler))
}

// Sikistir ölü dokümanları ve onlara ait gönderileri kalıcı olarak atar,
// doküman kimliklerini yeniden numaralandırır.
//
// Gönderi listelerini süzüp yeniden eşleyerek çalışır; ileri indekse
// (doküman -> terim) ihtiyaç duymaz.
func (ix *Indeks) Sikistir() {
	ix.kilit.Lock()
	defer ix.kilit.Unlock()

	if ix.SilinmisN == 0 {
		return
	}

	// Eski kimlik -> yeni kimlik eşlemesi. Ölüler için geçersiz işaret.
	const olu = ^uint32(0)
	esle := make([]uint32, len(ix.Belgeler))
	yeniBelgeler := make([]Belge, 0, len(ix.Belgeler)-ix.SilinmisN)
	yeniAd := make([]string, 0, cap(yeniBelgeler))
	yeniYol := make([]string, 0, cap(yeniBelgeler))

	for id := range ix.Belgeler {
		if ix.silinmisMi(uint32(id)) {
			esle[id] = olu
			continue
		}
		esle[id] = uint32(len(yeniBelgeler))
		yeniBelgeler = append(yeniBelgeler, ix.Belgeler[id])
		yeniAd = append(yeniAd, ix.adNorm[id])
		yeniYol = append(yeniYol, ix.yolNorm[id])
	}

	for _, harita := range []map[string][]uint32{ix.Gonderiler, ix.AdGonderiler} {
		for terim, g := range harita {
			yeni := g[:0] // yerinde süzme; arka diziyi yeniden kullanıyoruz
			for _, id := range g {
				if esle[id] != olu {
					yeni = append(yeni, esle[id])
				}
			}
			if len(yeni) == 0 {
				delete(harita, terim)
				continue
			}
			harita[terim] = yeni
		}
	}

	ix.Belgeler = yeniBelgeler
	ix.adNorm = yeniAd
	ix.yolNorm = yeniYol
	ix.YolID = make(map[string]uint32, len(yeniBelgeler))
	for id, b := range yeniBelgeler {
		ix.YolID[Anahtar(b.Yol)] = uint32(id)
	}
	ix.Silinmis = nil
	ix.SilinmisN = 0
}

// ---------------------------------------------------------------- tombstone

func (ix *Indeks) silinmisMi(id uint32) bool {
	i, bit := id/64, id%64
	if int(i) >= len(ix.Silinmis) {
		return false
	}
	return ix.Silinmis[i]&(1<<bit) != 0
}

func (ix *Indeks) tombstone(id uint32) {
	if ix.silinmisMi(id) {
		return
	}
	ix.bitBuyut(id)
	ix.Silinmis[id/64] |= 1 << (id % 64)
	ix.SilinmisN++
	// YolID'den kaldırmıyoruz: aynı yol yeniden eklenirse üzerine yazılır.
	// Kaldırsaydık Degismemis kontrolü yanlış negatif verirdi.
}

func (ix *Indeks) bitBuyut(id uint32) {
	gerekli := int(id/64) + 1
	for len(ix.Silinmis) < gerekli {
		ix.Silinmis = append(ix.Silinmis, 0)
	}
}

// ---------------------------------------------------------------- kalıcılık

// Hazirla türetilmiş alanları (normalize ad ve yol önbellekleri) yeniden
// üretir. Yükleme sonrası çağrılmalıdır.
func (ix *Indeks) Hazirla() {
	ix.kilit.Lock()
	defer ix.kilit.Unlock()
	ix.hazirla()
}

func (ix *Indeks) hazirla() {
	if ix.YolID == nil {
		ix.YolID = make(map[string]uint32)
	}
	if ix.Gonderiler == nil {
		ix.Gonderiler = make(map[string][]uint32)
	}
	if ix.AdGonderiler == nil {
		ix.AdGonderiler = make(map[string][]uint32)
	}
	ix.adNorm = make([]string, len(ix.Belgeler))
	ix.yolNorm = make([]string, len(ix.Belgeler))
	for i, b := range ix.Belgeler {
		ix.adNorm[i] = metin.Normalize(filepath.Base(b.Yol))
		ix.yolNorm[i] = metin.Normalize(b.Yol)
	}
}

// Kaydet indeksi atomik olarak diske yazar.
func (ix *Indeks) Kaydet(veriDizini string) error {
	ix.kilit.RLock()
	defer ix.kilit.RUnlock()
	return atomik.Yaz(filepath.Join(veriDizini, DosyaAdi), func(w io.Writer) error {
		return gob.NewEncoder(w).Encode(ix)
	})
}

// Yukle indeksi diskten okur. Dosya yoksa boş bir indeks döner (hata değil).
//
// Sürüm uyuşmazlığında veya bozuk dosyada indeks atılır ve boş indeks döner;
// çağıran kullanıcıya yeniden tarama önerir.
func Yukle(veriDizini string) (*Indeks, error) {
	yol := filepath.Join(veriDizini, DosyaAdi)
	f, err := os.Open(yol)
	if err != nil {
		if os.IsNotExist(err) {
			return Yeni(), nil
		}
		return Yeni(), fmt.Errorf("indeks açılamadı: %w", err)
	}
	// Dosyayı DERHAL kapatıyoruz: Windows'ta os.Rename hedefi açık tutan bir
	// okuyucu varsa başarısız olur ve sonraki Kaydet patlardı.
	ix := &Indeks{}
	hata := gob.NewDecoder(f).Decode(ix)
	f.Close()

	if hata != nil {
		return Yeni(), fmt.Errorf("indeks bozuk, yeniden tarama gerekli: %w", hata)
	}
	if ix.Surum != SurumNo {
		return Yeni(), fmt.Errorf(
			"indeks sürümü %d, bu uygulama %d bekliyor; yeniden tarama gerekli",
			ix.Surum, SurumNo)
	}
	ix.hazirla()
	return ix, nil
}
