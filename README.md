# Arşiv İndeks

Arşiv klasörlerindeki dosyaları **adına, içeriğine, uzantısına, tarihine ve
klasörüne** göre saniyeler içinde bulan, Windows için yerel bir arama aracı.

Klasörleri bir kez tarar, dosya adlarını ve okunabilir içeriği (metin, PDF,
Office) yerel bir indekste tutar; aramalar diske gitmeden milisaniyeler içinde
döner. Tek `.exe`, yalnızca `127.0.0.1`'i dinler, hiçbir veri bilgisayardan
çıkmaz.

![Arama sonuçları](docs/arama-sonuclar.png)

## Özellikler

- **İçerik araması** — `.txt`, kod dosyaları, `.docx/.xlsx/.pptx` ve `.pdf`
  içinde tam metin arama; dosya adını hatırlamak gerekmez.
- **Türkçe duyarlı** — `ı/i/İ`, `ç/ş/ğ/ö/ü` katlanır: `santiye` = `Şantiye` = `ŞANTIYE`.
- **Filtreler** — uzantı, klasör, tarih aralığı, sıralama; her arama URL'e
  yansır, yer imine eklenebilir.
- **Artımlı tarama** — değişmeyen dosyalar yeniden okunmaz; tarama sürerken
  arama kesintisiz çalışır.
- **Sonuçtan dosyaya** — eşleşmeler vurgulanır, dosya indirilebilir veya
  Gezgin'de gösterilebilir.
- **Gizlilik ve güvenlik** — yalnızca yerel erişim, CSRF korumalı uç noktalar,
  dosya içeriğinden XSS'e karşı yapısal koruma.

## Kurulum

Gereksinimler: Windows 10/11, [Go 1.27+](https://go.dev/dl/).

```powershell
powershell -ExecutionPolicy Bypass -File scripts\kur.ps1
```

Script konsolsuz bir kopya derleyip `%LOCALAPPDATA%\Programs\ArsivIndeks\`
altına kurar ve oturum açılışında otomatik başlayan bir Görev Zamanlayıcı
görevi (`ArsivIndeks`) kaydeder. Uygulamaya Başlat menüsündeki **Arsiv
Indeks** kısayolundan veya <http://127.0.0.1:8080> adresinden erişilir.

Kod değiştiğinde kurulumu güncellemek için aynı script tekrar çalıştırılır;
ayarlar ve indeks korunur, yeniden tarama gerekmez.

## Kullanım

1. **Ayarlar** → taranacak klasör(ler)i ekleyin, kaydedin, **Taramayı başlat**.
2. **Arama** → kelime yazın; gerekirse uzantı, klasör veya tarihle daraltın.

| Ayarlar | Filtreler |
|---|---|
| ![Ayarlar sayfası](docs/ayarlar-indexleme.png) | ![Arama filtreleri](docs/arama-filtreler.png) |

## Geliştirme

```bash
go run . -gelistirme            # şablon/statik dosyalar diskten okunur
go build -o arsiv-indeks.exe .
go test ./...
```

Geliştirme binary'si (depo kökü) ile kurulu kopya bilerek ayrıdır — böylece
`go build` arka planda çalışan binary'yi kilitlemez. Şablonlar ve statik
dosyalar `go:embed` ile gömüldüğü için değişikliklerin kurulu sürümde
görünmesi `kur.ps1` ile yeniden kurulum gerektirir.

## Dosya konumları

| Ne | Nerede |
|---|---|
| Kurulu exe | `%LOCALAPPDATA%\Programs\ArsivIndeks\arsiv-indeks.exe` |
| Ayarlar, indeks, log | `%APPDATA%\ArsivIndeks\` (`ayarlar.json`, `indeks.gob`, `arsiv-indeks.log`) |
| Kurulum / kaldırma | `scripts\kur.ps1`, `scripts\kaldir.ps1` |

Çalışıp çalışmadığını kontrol etmek için:

```powershell
Get-Process arsiv-indeks
Get-Content "$env:APPDATA\ArsivIndeks\arsiv-indeks.log" -Tail 20
```

## Kaldırma

```powershell
powershell -ExecutionPolicy Bypass -File scripts\kaldir.ps1
```

Görevi, kurulu kopyayı ve kısayolu siler; `%APPDATA%\ArsivIndeks` altındaki
verilere dokunmaz.

## Belgeler

Ayrıntılar için **[REHBER.md](REHBER.md)**:

- [Arama nasıl çalışır](REHBER.md#3-arama-nasıl-çalışır) · [Hangi dosyaların içeriği okunur](REHBER.md#4-hangi-dosyaların-içeriği-okunur) · [Tarama](REHBER.md#5-tarama)
- [Kod düzeni](REHBER.md#6-kod-düzeni) · [Tasarım kararları](REHBER.md#7-tasarımdaki-üç-önemli-karar) · [Güvenlik](REHBER.md#8-güvenlik)
- [Windows'a özgü tuzaklar](REHBER.md#10-windowsa-özgü-tuzaklar-hepsi-ele-alındı) · [Otomatik başlatma](REHBER.md#11-otomatik-başlatma-windows) · [Kapsam dışı](REHBER.md#12-kapsam-dışı)
