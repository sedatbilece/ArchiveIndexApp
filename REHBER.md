# Arşiv İndeks — Rehber

Arşivlenmiş dosyaları **adına**, **içindeki veriye**, **uzantısına**,
**tarihine** ve **bulunduğu klasöre** göre bulmak için yerel bir web
uygulaması. Tek bir `.exe`, tek harici bağımlılık, veriler yalnızca bu
bilgisayarda.

---

## 1. Çalıştırma

```bash
go run . -gelistirme
```

Sonra tarayıcıdan **http://127.0.0.1:8080**

Tek dosya olarak derlemek için:

```bash
go build -o arsiv-indeks.exe .
```

Derlenmiş `.exe` her dizinden çalışır — şablonlar ve CSS/JS binary'nin içine
gömülüdür.

### Bayraklar

| Bayrak | Varsayılan | Ne yapar |
|---|---|---|
| `-adres` | `127.0.0.1:8080` | Dinlenecek adres. **`:8080` yazmayın**, uygulama ağa açılır. |
| `-veri` | `%APPDATA%\ArsivIndeks` | `ayarlar.json` ve `indeks.gob` buraya yazılır. |
| `-gelistirme` | kapalı | Şablonları ve `static/` dosyalarını diskten okur; kaydet-yenile ile çalışır. Proje kökünden çalıştırılmalıdır. |

`-gelistirme` olmadan çalıştırdığınızda şablon değişiklikleri görünmez —
gömülü kopya kullanılır, yeniden derlemek gerekir.

---

## 2. İlk kullanım

1. **Ayarlar** sayfasını açın.
2. Arşiv klasörünü seçin: yolu yazın/yapıştırın ya da gözat listesinden
   tıklayarak inip **"Bu klasörü listeye ekle"** deyin. Birden çok kök
   ekleyebilirsiniz (her satıra bir tane).
3. **"Ayarları kaydet"**.
4. **"Taramayı başlat"**. İlerleme çubuğu ve "şu an: …" satırı canlı akar;
   sayfayı yenilemek veya kapatmak taramayı durdurmaz.
5. **Arama** sayfasına geçin.

---

## 3. Arama nasıl çalışır

### Kapsam

- **Ad ve içerik** (varsayılan) — üç kaynağın birleşimi: içerik indeksi,
  dosya yolu indeksi, dosya adında alt dize taraması.
- **Sadece dosya adı** — alt dize de çalışır: `apor` yazınca `rapor.pdf`
  bulunur.
- **Sadece içerik** — dosya adı eşleşmeleri gelmez. İçerik ve yol
  belirteçleri indekste ayrı haritalarda tutulduğu için bu ayrım gerçektir.

### Türkçe harf katlaması

`ı i I İ` hepsi `i`'ye, `ç→c ğ→g ö→o ş→s ü→u` katlanır. Yani şunların hepsi
aynı sonucu verir:

```
santiye · Şantiye · ŞANTIYE · santıye
```

Hangi harfi yazdığınızı hatırlamak zorunda değilsiniz. Aynı katlama hem
indeksleme hem sorgu anında uygulanır — bu ikisi ayrışırsa arama sessizce
sonuç vermez, o yüzden tek bir yerde (`internal/metin/katla.go`) durur.

### Filtreler

Uzantı (indeksteki sayılarıyla), klasör, tarih aralığı, sıralama. Hepsi URL
sorgu parametresinde yaşar; sonucu **yer imine ekleyebilir**, bağlantı olarak
paylaşabilir, geri tuşunu kullanabilirsiniz.

### Skorlama

`Σ idf(terim) × alan çarpanı`, çarpanlar: **ad ×3, klasör/yol ×2, içerik ×1**.
Arşivde dosya adı içerikten çok daha güçlü bir sinyaldir. Eşitlikte yeni
dosya öne gelir.

---

## 4. Hangi dosyaların içeriği okunur

| Tür | Nasıl | Bağımlılık |
|---|---|---|
| `.txt .md .csv .log .json .xml .yaml .sql .htm` + kod dosyaları | doğrudan | — |
| `.docx .xlsx .pptx` | `archive/zip` + `encoding/xml` | — |
| `.pdf` | `github.com/ledongthuc/pdf` | tek harici bağımlılık |

### Bilinçli sınırlar

Bunlar **yalnızca adıyla** indekslenir ve arayüzde nedeni etiketle gösterilir:

- **Taranmış PDF** → *"taranmış PDF — içerik aranamaz"*. Metin katmanı yok;
  OCR kapsam dışı.
- **Eski Office biçimleri** (`.doc .xls .ppt`) → *"eski Office biçimi"*.
  Bunlar OLE2 bileşik ikili dosyadır; stdlib desteği yok, gerçek
  BIFF/WordDocument ayrıştırması çok haftalık bir iş. Bayt kazımak ise
  indeksi çöp belirteçlerle zehirler. Bu dosyaları `.docx`'e çevirirseniz
  içerikleri aranabilir olur.
- **Bulut (OneDrive) yer tutucuları** → *"bulutta (indirilmedi)"*. Okumak
  dosyayı sessizce indirir; 100.000 dosyalık arşivde bu gigabaytlarca
  indirme demektir.
- **Boyut sınırını aşanlar**, **kilitli dosyalar**, **ikili dosyalar**
  (uzantısı `.txt` olsa bile).

Tarama bitince Ayarlar sayfasındaki özet kaç dosyanın yalnızca adıyla
indekslendiğini söyler — neyin aranabilir **olmadığını** bilmek önemlidir.

---

## 5. Tarama

### Artımlı tarama

**"Taramayı başlat"** artımlıdır: boyutu ve değişim zamanı aynı kalan
dosyaların içeriği yeniden okunmaz. Değişmemiş bir arşivin yeniden taranması
dakikalar yerine saniyeler sürer. Günlük kullanımda bunu kullanın.

**"Sıfırdan tara"** indeksi siler ve her şeyi yeniden okur. Ayarları
(özellikle içerik uzantılarını) değiştirdiğinizde gerekir.

Silinen dosyalar taramada görülmedikleri için indeksten düşürülür. Yalnızca
taranan köklerin altına bakılır — A kökünü taramak B kökünün dosyalarını
silmez.

### İki geçiş

Birinci geçiş dosyaları sayar (`Stat` yok, dosya açma yok; 100.000 dosyada
~1-3 saniye), ikinci geçiş asıl taramayı yapar. Karşılığında belirsiz bir
çubuk değil gerçek bir yüzde görürsünüz.

### Taranacak dizini değiştirirsem indekse ne olur?

Ayarları kaydettiğiniz anda, **artık taranmayan dizinlerdeki dokümanlar
indeksten düşürülür** ve kaydetme onayında kaç dosyanın düştüğü yazar.
Tarama yapmanız gerekmez.

Bu temizlik önemlidir: yapılmazsa o dosyalar arama sonuçlarında görünmeye
devam eder ama **açılamazlar** — `GuvenliYol` artık o kökü tanımadığı için
"İndir" ve "Klasörde göster" 403 döner. Yani hayalet kayıtlar oluşur.

Çok kök kullanıyorsanız yalnızca **çıkardığınız** kökün dokümanları düşer,
kalanlar korunur.

Aynı temizlik uygulama açılışında da yapılır; böylece `ayarlar.json`
dosyasını uygulama kapalıyken elle düzenleyip bir dizini çıkarmanız da
sorun olmaz.

Bir dizini geri eklerseniz dosyaları yeniden taranır — indeks her zaman
yeniden üretilebilir, kalıcı bir kayıp yaşanmaz.

### Yarım kalan tarama

İptal ettiğinizde veya uygulama kapandığında indekste doküman olur ama bitiş
damgası olmaz; Ayarlar sayfası **"önceki tarama yarım kaldı"** uyarısı
gösterir. İndeks her 20.000 dosyada bir diske yazılır, yani çöken bir tarama
her şeyi kaybetmez.

---

## 6. Kod düzeni

```
main.go            embed, şablonlar, PageData, biçimleme, rotalar, bayraklar
sayfalar.go        aramaHandler, ayarlarHandler, ayarlarKaydetHandler
tarama_api.go      taramaBasla / taramaIptal / taramaDurum (JSON)
dosya_api.go       dosyaIndir, klasordeGoster, parcacik + metin önbelleği
gozat_api.go       sunucu taraflı dizin gezgini
guvenlik.go        GuvenliYol, yerelKontrol, CSRF jetonu
explorer_windows.go / explorer_diger.go

internal/metin/    katla.go (Türkçe katlama) · belirtec.go (tokenizer)
                   kodlama.go (BOM/UTF-16/cp1254) · ikili.go
internal/cikarim/  cikarici.go (arayüz + GuvenliCikar) · ooxml.go · pdf.go
internal/indeks/   Indeks, ters indeks, tombstone, gob, sıkıştırma
internal/sorgu/    sorgu.go (ayrıştır/çalıştır/skorla) · vurgu.go
internal/tarayici/ tarayici.go (iş hattı) · is.go (durum makinesi)
                   ilerleme.go · ozellik_windows.go / ozellik_diger.go
internal/ayarlar/  Ayarlar, JSON yükle/kaydet, doğrulama
internal/atomik/   yarım dosya bırakmayan yazma (.tmp + Sync + Rename)

templates/         layout.html + arama.html + ayarlar.html
static/            style.css + js/arama.js · js/tarama.js · js/gozat.js
```

İş mantığı `internal/` altında ve HTTP'den bağımsızdır; `go test ./internal/...`
sunucu ayağa kalkmadan çalışır.

---

## 7. Tasarımdaki üç önemli karar

### İndeks neden `map[string][]uint32`?

Okunması ve ayıklanması kolay. **~30.000 dosyaya kadar rahat.** 100.000
dosyada ~1 GB RSS'e çıkar. Dışarıya yalnızca `indeks.Okuyucu` arayüzü açık
olduğu için, temsili paketlenmiş bir arena'ya (terimler bitişik `[]byte`,
gönderiler delta+varint) çevirmek yalnızca `internal/indeks` paketini
etkiler. Bu geçiş ~165 MB'a indirir; gerçek arşivi tarayıp Görev
Yöneticisi'nden RSS'e bakarak karar verin.

### Eşzamanlılık: RWMutex + partili yazma

İndeks kendi `RWMutex`'i ile korunur; tarayıcı 512'lik partiler halinde yazar,
yani yazma kilidi taramanın toplam süresinin binde birinden azını tutar.
Aramalar tarama sürerken kesintisiz çalışır.

"Değişmez anlık görüntü + `atomic.Pointer` takası" alternatifi değerlendirildi
ve reddedildi: artımlı tarama ile uyumsuz. Değişmemiş bir dosyanın
gönderilerini yeni bir indekse taşımak için doküman başına terim listesi
(ileri indeks) saklamak gerekirdi ve bu belleği ikiye katlardı.

### Parçacıklar neden ayrı istekle geliyor?

Çıkarılan metni hiç saklamıyoruz (100.000 × 50 KB = 5 GB, absürt). Onun
yerine görüntülenen sayfanın dosyaları talep üzerine yeniden okunuyor. Ama
bunu arama handler'ında satır içi yapsaydık: 300 sayfalık bir PDF saf Go'da
1-5 saniye sürer, 20 tanesi 10 saniyelik bir sayfa demekti. Sonuçlar hemen
basılıyor, parçacıklar `/api/parcacik` üzerinden satır satır dolduruluyor
(4'lük havuz, 3 sn zaman aşımı, 300 kayıtlık önbellek).

---

## 8. Güvenlik

Yerel bir araç olsa da `/dosya/klasorde-goster` bir **süreç çalıştırıyor**,
o yüzden katmanlı savunma var:

- Sunucu yalnızca `127.0.0.1` üzerinde dinler.
- `yerelKontrol` middleware: `Host` localhost/127.0.0.1 olmalı (DNS
  rebinding'i kapatır) **ve** `Sec-Fetch-Site` same-origin/none olmalı.
- Durum değiştiren rotalar yalnızca POST + süreç başına rastgele CSRF jetonu.
- `GuvenliYol` her dosya erişimini doğrular: mutlak yol, NUL yok, alternatif
  veri akışı yok, symlink çözümlemesi, **ayırıcı sınırına saygılı** ve
  büyük/küçük harf duyarsız önek karşılaştırması. Bu sınır kontrolü olmadan
  `C:\Arsiv-gizli`, `C:\Arsiv` kökünün altında sayılırdı.

### XSS

Dosya içeriğine veya adına **asla** `template.HTML` uygulanmaz. Vurgular
yapısal döner:

```go
type Parca struct { Metin string; Vurgulu bool }
```

```html
{{range .Parcalar}}{{if .Vurgulu}}<mark>{{.Metin}}</mark>{{else}}{{.Metin}}{{end}}{{end}}
```

`html/template` her `.Metin`'i kaçırır, `<mark>` etiketleri yazar
kontrolündedir. İstemci tarafında da parçacıklar `createTextNode` ile
basılır, `innerHTML` ile değil. Cazip görünen
`strings.ReplaceAll(s, q, "<mark>"+q+"</mark>")` yolu, içinde `<script>`
geçen **herhangi** bir dosyanın tetikleyeceği depolanmış XSS deliğidir.

---

## 9. Testler

```bash
go test ./...
```

```bash
go test ./internal/cikarim/ -v
```

Notlar:

- Office fixture'ları **kod içinde üretilir** (`archive/zip` ile
  `bytes.Buffer`'a). Depoda ikili blob yok ve test aynı zamanda biçimi
  belgeliyor.
- `main_test.go` uçtan uca çalışır: `httptest` ile ayar kaydeder, gerçek bir
  tarama koşturur, sonra arama sonuçlarını doğrular.
- `-race` bu makinede çalışmaz (CGO ve gcc gerekir, `CGO_ENABLED=0`).
  Eşzamanlılık elle gözden geçirildi: indeks RWMutex, sayaçlar `sync/atomic`,
  iş durumu mutex, `gorulen` haritası tek yazıcı + `wg.Wait()` sonrası okuma.

---

## 10. Windows'a özgü tuzaklar (hepsi ele alındı)

1. **OneDrive yer tutucuları** — `RECALL_ON_OPEN` / `RECALL_ON_DATA_ACCESS` /
   `OFFLINE` öznitelikleri kontrol edilir. Buradaki en önemli kontrol bu.
2. **Uzun yollar (>260 karakter)** — kökler `filepath.Abs` + `Clean` ile
   mutlaklaştırılır; Go'nun `os` katmanı `\\?\` düzeltmesini yalnızca
   mutlak+temiz yollara uygular. Göreli kök 260 karakterden sonra sessizce
   başarısız olurdu.
3. **Junction döngüleri** — `FILE_ATTRIBUTE_REPARSE_POINT` doğrudan okunur;
   `WalkDir`'in symlink davranışı dizin junction'larında sürüm sürüm
   tutarsızdır.
4. **İzin reddi** — `WalkDir`'in hata argümanı ele alınır: klasörse alt ağaç
   atlanır, tüm tarama sonlanmaz.
5. **Explorer'ın argüman ayrıştırması** — `/select,` ve yol tek argv token'ı
   olmalı; ham komut satırı `SysProcAttr.CmdLine` ile kurulur.
   `cmd /c start` asla kullanılmaz (argüman enjeksiyonu).
6. **Ağ sürücüsü** — 100.000 `Stat` dakikalar sürebilir; tarama her an iptal
   edilebilir.

---

## 11. Otomatik başlatma (Windows)

Oturum açıldığında kendiliğinden başlayıp arka planda (konsolsuz) çalışmasını
istiyorsanız:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\kur.ps1
```

Bu script:

1. Çalışan `arsiv-indeks.exe`'yi durdurur ve eski görevi kaldırır.
2. `-ldflags "-H=windowsgui"` ile **konsolsuz** bir kopya derler — normal
   `go build` çıktısından farklıdır, çift tıklayınca ya da görevle
   başlatıldığında pencere hiç açılmaz.
3. Bu kopyayı `%LOCALAPPDATA%\Programs\ArsivIndeks\` altına kurar (depodaki
   `.exe`'ye dokunmaz; `go build` çalışan bir binary'yi kilitleyeceği için
   geliştirme kopyasıyla kurulum kopyası bilerek ayrılmıştır).
4. `ArsivIndeks` adında bir Görev Zamanlayıcı görevi kaydeder: oturum
   açılışında başlar, çökerse 1 dakika arayla 3 kez kendini yeniden başlatır,
   3 günlük varsayılan çalışma süresi sınırı kaldırılmıştır.
5. Başlat menüsüne `http://127.0.0.1:8080` açan bir kısayol bırakır —
   konsol görünmediği için erişimin tek "kapısı" bu.

Kaldırmak için:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\kaldir.ps1
```

Görevi, kurulum kopyasını ve kısayolu siler; **veri dizinine
(`%APPDATA%\ArsivIndeks` — ayarlar, indeks, log) dokunmaz**.

### Konsolsuz çalışırken günlük (log)

`-H=windowsgui` derlemesinde konsol yok, yani her zamanki `log.Printf`
çıktısı hiçbir yere gitmez. Bunun yerine loglar her zaman
`%APPDATA%\ArsivIndeks\arsiv-indeks.log` dosyasına da yazılır (2 MB'ı
geçince `arsiv-indeks.log.eski`'ye devrederek). "Çalışıyor mu?" sorusunun
cevabı burada:

```powershell
Get-Process arsiv-indeks
Get-Content "$env:APPDATA\ArsivIndeks\arsiv-indeks.log" -Tail 20
```

### Neden Windows Service değil

Sunucu yalnızca `127.0.0.1`'de dinliyor ve veri dizini kullanıcıya özel
(`%APPDATA%`). SYSTEM hesabıyla koşan bir Windows Service bu dizini System
profiline çözer — ayarlar ve indeks yanlış yere yazılırdı. Görev
Zamanlayıcı + oturum tetikleyicisi, kullanıcı bağlamında koşarak bunu
bedavaya çözer; ayrıca NSSM/WinSW gibi üçüncü parti bir bağımlılık
gerektirmez.

### Portu veya kurulumu değiştirmek

`scripts\kur.ps1` başındaki `$Adres` değişkenini değiştirip scripti tekrar
çalıştırmak yeterli — mevcut görevi ve kopyayı üzerine yazar.

---

## 12. Kapsam dışı

OCR (taranmış PDF içeriği) · eski OLE2 Office içerikleri · Türkçe gövdeleme
(stemming) · gerçek zamanlı dosya izleme · konum indeksi ve terim frekansı
(indeksi 2-3× büyütür, ad ağırlıklı bir arşivde karşılığı yok).
