# Arşiv İndeks

Arşivlenmiş dosyaları adına, içeriğine, uzantısına, tarihine ve bulunduğu
klasöre göre aramak için yerel (yalnızca `127.0.0.1`) bir web uygulaması. Tek
`.exe`, veriler yalnızca bu bilgisayarda. Arama/tarama mantığının ayrıntıları
için **[REHBER.md](REHBER.md)**'ye bakın — bu dosya yalnızca "nerede ne var,
nasıl güncellenir" operasyonel özetidir.

Bu makinede oturum açılışında otomatik başlayacak şekilde kurulu
(`scripts\kur.ps1` ile, bkz. [REHBER.md §11](REHBER.md#11-otomatik-başlatma-windows)).

---

## Çalışan kurulum — önemli yollar

| Ne | Nerede |
|---|---|
| Kurulu (otomatik başlayan) exe | `%LOCALAPPDATA%\Programs\ArsivIndeks\arsiv-indeks.exe` |
| Bu depodaki geliştirme exe'si | `arsiv-indeks.exe` (depo kökü — **kurulu kopyayla aynı dosya değil**) |
| Ayarlar (`ayarlar.json`) + indeks (`indeks.gob`) | `%APPDATA%\ArsivIndeks\` |
| Log dosyası | `%APPDATA%\ArsivIndeks\arsiv-indeks.log` (2 MB'ı geçince `.eski` uzantılı yedeğe devreder) |
| Görev Zamanlayıcı görevi | `ArsivIndeks` (Görev Zamanlayıcı Kitaplığı kökünde) |
| Başlat menüsü kısayolu | `Arsiv Indeks.url` → `http://127.0.0.1:8080` açar |
| Kurulum / kaldırma script'leri | `scripts\kur.ps1`, `scripts\kaldir.ps1` |

**Neden iki ayrı exe var?** Depodeki `arsiv-indeks.exe` geliştirme sırasında
`go build` ile üretilen sıradan (konsollu) bir binary — git'e işlenmiş, ondan
dokunulmuyor. `scripts\kur.ps1` bunun yanına, `-H=windowsgui` bayrağıyla
**konsolsuz** ayrı bir kopya derleyip `%LOCALAPPDATA%\Programs\ArsivIndeks\`
altına kurar. Görevi hep bu ikinci kopyayı çalıştırır; böylece `go build`
geliştirme sırasında çalışan bir binary'yi kilitlemeye çalışmaz.

## Yeni bir build aldığında (kod değişti, kurulumu güncelle)

Kod değişikliği yaptıktan sonra çalışan kurulumu güncellemek için:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\kur.ps1
```

Bu script sırasıyla: çalışan süreci durdurur → `-H=windowsgui` ile yeniden
derler → `%LOCALAPPDATA%\Programs\ArsivIndeks\` üzerine yazar → Görev
Zamanlayıcı görevini yeniden kaydeder → görevi başlatır. **Ayarlara ve
indekse dokunmaz** (`%APPDATA%\ArsivIndeks` sabit kalır), yani her güncellemede
yeniden tarama gerekmez.

Yalnızca kod değişikliğini denemek, kurulumu güncellemeden test etmek
istiyorsanız normal geliştirme döngüsünü kullanın (aşağıya bakın) —
`kur.ps1`'i yalnızca değişikliği kalıcı/otomatik-başlayan kuruluma yansıtmak
istediğinizde çalıştırın.

## Çalışıyor mu?

```powershell
Get-Process arsiv-indeks
Get-Content "$env:APPDATA\ArsivIndeks\arsiv-indeks.log" -Tail 20
```

Ya da tarayıcıdan `http://127.0.0.1:8080`.

## Durdurma / kaldırma

```powershell
powershell -ExecutionPolicy Bypass -File scripts\kaldir.ps1
```

Görevi, kurulu kopyayı ve Başlat menüsü kısayolunu siler. Veri dizinine
(`%APPDATA%\ArsivIndeks` — ayarlar, indeks, log) **dokunmaz**; kurulumu
tekrar çalıştırırsanız kaldığı yerden devam eder.

Tek seferlik durdurmak (görevi silmeden) için Görev Zamanlayıcı'da `ArsivIndeks`
görevine sağ tıklayıp **Bitir**'i kullanın; bir sonraki oturum açılışında
yeniden başlar.

## Geliştirme

```bash
go run . -gelistirme      # şablonlar/statik dosyalar diskten okunur, kaydet-yenile
go build -o arsiv-indeks.exe .
go test ./...
```

Kod düzeni, arama/tarama tasarımı, güvenlik modeli ve Windows'a özgü tuzaklar
için **[REHBER.md](REHBER.md)**.
