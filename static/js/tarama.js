// Tarama başlatma, iptal ve canlı ilerleme yoklaması.
(function () {
  "use strict";

  var alan = document.getElementById("tarama-alani");
  if (!alan) return;

  var jeton = alan.dataset.jeton;
  var kutu = document.getElementById("ilerleme-kutusu");
  var dolu = document.getElementById("ilerleme-dolu");
  var asama = document.getElementById("ilerleme-asama");
  var sayilar = document.getElementById("ilerleme-sayilar");
  var dosya = document.getElementById("ilerleme-dosya");
  var mesaj = document.getElementById("tarama-mesaj");
  var basla = document.getElementById("tarama-basla");
  var tam = document.getElementById("tarama-tam");
  var iptal = document.getElementById("tarama-iptal");

  var zamanlayici = null;

  function bicim(n) {
    return (n || 0).toLocaleString("tr-TR");
  }

  function mesajYaz(metin, tur) {
    mesaj.textContent = metin || "";
    mesaj.className = "tarama-mesaj" + (tur ? " " + tur : "");
  }

  var favicon = document.getElementById("favicon");
  var faviconNormal = favicon ? favicon.getAttribute("href") : "";
  function faviconAyarla(calisiyor) {
    if (!favicon) return;
    var hedef = calisiyor ? faviconNormal.replace("favicon.svg", "favicon-tarama.svg") : faviconNormal;
    if (favicon.getAttribute("href") !== hedef) favicon.setAttribute("href", hedef);
  }

  // durumBas ekranı günceller ve taramanın bitip bitmediğini döner.
  function durumBas(d) {
    var calisiyor = d.calisiyor;
    faviconAyarla(calisiyor);

    kutu.hidden = !calisiyor;
    iptal.hidden = !calisiyor;
    basla.disabled = calisiyor;
    tam.disabled = calisiyor;

    dolu.style.width = (d.yuzde || 0) + "%";
    asama.textContent = d.asama || "";

    var parcalar = [];
    if (d.toplam) {
      parcalar.push(bicim(d.islenen + d.atlanan) + " / " +
        bicim(d.toplam) + " dosya");
    } else if (d.islenen) {
      parcalar.push(bicim(d.islenen) + " dosya");
    }
    if (d.atlanan) parcalar.push(bicim(d.atlanan) + " atlandı");
    if (d.hatali) parcalar.push(bicim(d.hatali) + " hatalı");
    if (calisiyor && d.kalanTahminSaniye > 0) {
      parcalar.push("~" + bicim(d.kalanTahminSaniye) + " sn kaldı");
    }
    sayilar.textContent = parcalar.length ? " — " + parcalar.join(", ") : "";
    dosya.textContent = calisiyor && d.sonDosya ? "şu an: " + d.sonDosya : "";

    if (calisiyor) return false;

    if (d.durum === "tamamlandi") {
      mesajYaz("Tarama tamamlandı. Özet ve güncel sayılar için sayfayı yenileyin.",
        "basari");
    } else if (d.durum === "iptal") {
      mesajYaz("Tarama iptal edildi. İndeks yarım kaldı; yeniden tarayın.",
        "hata");
    } else if (d.durum === "hata") {
      mesajYaz("Tarama hatası: " + (d.hata || "bilinmeyen hata"), "hata");
    }
    return true;
  }

  function yokla() {
    fetch("/tarama/durum", { headers: { "Accept": "application/json" } })
      .then(function (c) { return c.json(); })
      .then(function (d) {
        if (durumBas(d) && zamanlayici) {
          clearInterval(zamanlayici);
          zamanlayici = null;
        }
      })
      .catch(function () { /* geçici hata; sonraki yoklamada düzelir */ });
  }

  function yoklamayaBasla() {
    if (zamanlayici) return;
    zamanlayici = setInterval(yokla, 1000);
    yokla();
  }

  function taramaIstek(adres, tamTarama) {
    var govde = new URLSearchParams();
    govde.set("jeton", jeton);
    if (tamTarama) govde.set("tamTarama", "1");

    mesajYaz("");
    fetch(adres, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: govde.toString()
    })
      .then(function (c) {
        return c.json().then(function (v) {
          return { kod: c.status, govde: v };
        });
      })
      .then(function (s) {
        // 409: zaten süren bir tarama var. Hata gösterip yoklamaya
        // geçiyoruz ki kullanıcı mevcut taramanın ilerlemesini görsün.
        if (s.kod === 409) {
          mesajYaz(s.govde.hata || "Tarama zaten sürüyor.", "hata");
          yoklamayaBasla();
          return;
        }
        if (s.kod >= 400) {
          mesajYaz(s.govde.hata || "İstek başarısız oldu.", "hata");
          return;
        }
        yoklamayaBasla();
      })
      .catch(function () { mesajYaz("Sunucuya ulaşılamadı.", "hata"); });
  }

  basla.addEventListener("click", function () {
    taramaIstek("/tarama/basla", false);
  });

  tam.addEventListener("click", function () {
    if (!confirm("Mevcut indeks silinip her şey yeniden okunacak. Devam edilsin mi?")) {
      return;
    }
    taramaIstek("/tarama/basla", true);
  });

  iptal.addEventListener("click", function () {
    taramaIstek("/tarama/iptal", false);
  });

  // Sayfa tarama ortasında yenilendiyse yoklamayı sürdür: durum sunucuda
  // yaşadığı için yenileme bedava.
  var d = alan.dataset.durum;
  if (d === "sayiliyor" || d === "taraniyor" || d === "kaydediliyor") {
    yoklamayaBasla();
  }
})();
