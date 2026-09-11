# ArchiveIndexApp — Claude için notlar

## Kurulu sürümü güncelleme

Uygulama, depodaki dosyalardan bağımsız olarak `%LOCALAPPDATA%\Programs\ArsivIndeks`
altında ayrı bir kopya olarak, Görev Zamanlayıcı ile çalışıyor (bkz. REHBER.md §11).
`templates/` ve `static/` `go:embed` ile derleme anında binary'nin içine gömülüyor,
yani kaynak dosyalarda yapılan bir değişiklik derlenip yeniden kurulmadan görünmez.

Kullanıcı "güncelle", "güncelleme çıkar" gibi bir şey söylediğinde veya yapılan bir
kod/şablon/statik dosya değişikliğinin çalışan uygulamada görünmesi gerektiğinde:
`scripts\kur.ps1` çalıştırılarak yeniden derlenip kurulum kopyası güncellenmeli ve
görev yeniden başlatılmalı. Bu script çalışan süreci durdurup Görev Zamanlayıcı
görevini yeniden kaydettiği için sistem durumunu değiştiren bir adımdır; çalıştırmadan
önce kullanıcıya haber ver / onay al (bkz. [[feedback-sistem-degisikliklerinde-durdur]]
memory notu), ama "yeniden derleyip kur.ps1 çalıştırmak gerekiyor" tespitini kendin
yap — kullanıcının bunu her seferinde ayrıca hatırlatmasına gerek yok.

## Versiyon (version.txt)

Kök dizindeki `version.txt`, `v<büyük>.<küçük>.<yama>.<GGAAYY>` biçiminde tek
satırlık bir sürüm damgası tutar (ör. `v0.0.1.110926`). Güncellemesi
`scripts\versiyon-guncelle.ps1` ile yapılır: yama numarasını 1 artırır, tarihi
bugüne çeker. Bu script iki durumda çalıştırılır:

1. Kullanıcı "versiyon güncelle" derse, doğrudan bu script çalıştırılır.
2. `scripts\kur.ps1` her çalıştığında otomatik çağrılır (her kurulum yeni bir
   sürüm demektir) — bu script'e elle dokunmaya gerek yok. Çağrı, derleme
   adımından ÖNCE yapılır (`go:embed` version.txt'yi derleme anında binary'ye
   gömdüğü için — aksi halde header'da bir önceki kurulumun sürümü görünür).

## Commit

Bu depoda asla kendi başına `git commit` çalıştırma. Kullanıcı "commit mesajı"
dediğinde tek satırlık bir commit mesajı üret ve sadece sohbette göster —
commit'i sen atma, kullanıcı isterse kendisi kullanır.

## Yorum satırları

- Basit bir fonksiyon veya küçük bir değişiklik için yorum: **en fazla 1 satır**,
  amacı özetlesin.
- Büyük/karmaşık bir bölümün açıklaması gerekiyorsa: **en fazla 3 satır**.
- Kodun NE yaptığını değil (iyi isimlendirme zaten bunu anlatır), NEDEN öyle
  yapıldığını (gizli bir kısıt, ince bir invaryant, belirli bir hataya karşı
  workaround) açıklayan yorumlar yaz; aksi halde yorum yazma.
