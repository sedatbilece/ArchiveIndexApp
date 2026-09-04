// Arama sayfası: satır eylemleri ve içerik parçacıkları.
(function () {
  "use strict";

  // ------------------------------------------------ satır eylemleri
  //
  // "Klasörde göster" düz bir HTML form POST'u olarak da çalışır (JS
  // kapalıysa sunucu geri yönlendirir), ama o zaman sayfa yenilenir ve
  // kaydırma konumu kaybolur. Burada araya girip fetch ile gönderiyoruz:
  // Explorer açılır, kullanıcı sonuç listesinde kalır.
  document.querySelectorAll("form.satir-form").forEach(function (form) {
    form.addEventListener("submit", function (olay) {
      olay.preventDefault();

      var buton = form.querySelector("button");
      var eskiMetin = buton ? buton.textContent : "";
      if (buton) {
        buton.disabled = true;
        buton.textContent = "Açılıyor…";
      }

      fetch(form.action, {
        method: "POST",
        headers: {
          // Bu başlık sunucuya "JSON dön, yönlendirme yapma" diyor.
          "Accept": "application/json",
          "Content-Type": "application/x-www-form-urlencoded"
        },
        body: new URLSearchParams(new FormData(form)).toString()
      })
        .then(function (c) {
          if (!buton) return;
          buton.textContent = c.ok ? "Açıldı ✓" : "Açılamadı";
          if (!c.ok) buton.classList.add("hata-metni");
        })
        .catch(function () {
          if (buton) {
            buton.textContent = "Açılamadı";
            buton.classList.add("hata-metni");
          }
        })
        .then(function () {
          // Kısa bir süre sonra butonu eski haline döndür.
          setTimeout(function () {
            if (!buton) return;
            buton.disabled = false;
            buton.textContent = eskiMetin;
            buton.classList.remove("hata-metni");
          }, 1600);
        });
    });
  });

  // ------------------------------------------------ içerik parçacıkları
  //
  // Neden ayrı istek: 300 sayfalık bir PDF'ten metin çıkarmak 1-5 saniye
  // sürer. 20 sonucu sunucuda satır içi çıkarsaydık sayfa 10 saniyede
  // açılırdı. Sonuçlar hemen basılıyor, parçacıklar burada dolduruluyor.

  var liste = document.querySelector(".sonuclar");
  if (!liste) return;

  var sorgu = liste.dataset.sorgu || "";
  if (!sorgu.trim()) return;

  var kutular = Array.prototype.slice.call(
    document.querySelectorAll(".parcacik[data-yol]"));
  if (!kutular.length) return;

  // Parçaları DOM düğümü olarak kuruyoruz, innerHTML ile DEĞİL.
  // Sunucu {Metin, Vurgulu} listesi döndürüyor; metni createTextNode ile
  // basmak XSS'i yapısal olarak imkânsız kılıyor.
  function bas(kutu, parcalar) {
    if (!parcalar || !parcalar.length) return;
    var frag = document.createDocumentFragment();
    parcalar.forEach(function (p) {
      var dugum = document.createTextNode(p.Metin);
      if (p.Vurgulu) {
        var m = document.createElement("mark");
        m.appendChild(dugum);
        frag.appendChild(m);
      } else {
        frag.appendChild(dugum);
      }
    });
    kutu.appendChild(frag);
  }

  // Sırayla, birer birer: sunucu tarafında da 4'lük bir havuz sınırı var,
  // istemciden 20 paralel istek atmanın faydası yok.
  var sira = 0;
  function sonraki() {
    if (sira >= kutular.length) return;
    var kutu = kutular[sira++];
    var adres = "/api/parcacik?yol=" + encodeURIComponent(kutu.dataset.yol) +
      "&q=" + encodeURIComponent(sorgu);

    fetch(adres, { headers: { "Accept": "application/json" } })
      .then(function (c) { return c.ok ? c.json() : null; })
      .then(function (v) { if (v) bas(kutu, v.parcalar); })
      .catch(function () { /* parçacık gelmezse satır yine kullanılabilir */ })
      .then(sonraki);
  }
  sonraki();
})();
