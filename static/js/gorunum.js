// Tüm sayfalar: tema / yoğunluk menüsü ve "Yenilikler" penceresi.
(function () {
  "use strict";

  var kok = document.documentElement;
  // Gizli pencerede veya engellenmiş depolamada localStorage hata fırlatır; tercih o zaman yalnızca bu sayfada geçerli olur.
  function oku(anahtar) {
    try { return localStorage.getItem(anahtar); } catch (e) { return null; }
  }
  function yaz(anahtar, deger) {
    try {
      if (deger === null) { localStorage.removeItem(anahtar); } else { localStorage.setItem(anahtar, deger); }
    } catch (e) { /* yoksay */ }
  }

  // ------------------------------------------------ tema ve yoğunluk
  var ayarlar = {
    tema: { varsayilan: "sistem", uygula: function (d) {
      if (d === "sistem") { delete kok.dataset.tema; } else { kok.dataset.tema = d; }
    } },
    yogunluk: { varsayilan: "rahat", uygula: function (d) {
      if (d === "rahat") { delete kok.dataset.yogunluk; } else { kok.dataset.yogunluk = d; }
    } }
  };

  Object.keys(ayarlar).forEach(function (ad) {
    var ayar = ayarlar[ad];
    var mevcut = oku(ad) || ayar.varsayilan;
    document.querySelectorAll(".gorunum-panel input[name=" + ad + "]").forEach(function (kutu) {
      kutu.checked = kutu.value === mevcut;
      kutu.addEventListener("change", function () {
        ayar.uygula(kutu.value);
        yaz(ad, kutu.value === ayar.varsayilan ? null : kutu.value);
      });
    });
  });

  var menu = document.querySelector(".gorunum-menu");
  if (menu) {
    document.addEventListener("click", function (olay) {
      if (menu.open && !menu.contains(olay.target)) menu.open = false;
    });
    document.addEventListener("keydown", function (olay) {
      if (olay.key === "Escape" && menu.open) menu.open = false;
    });
  }

  // ------------------------------------------------ yenilikler
  //
  // Notlar sürüm sırasına göre (en yeni başta) yenilikler.json'da. En yeni
  // notun sürümü daha önce görülmemişse pencere bir kez kendiliğinden açılır.
  var pencere = document.getElementById("yenilikler");
  var icerik = document.getElementById("yenilikler-icerik");
  var surumButonu = document.getElementById("surumButonu");
  if (!pencere || !icerik || typeof pencere.showModal !== "function") return;

  var betik = document.currentScript;
  var surumParam = betik && betik.src.indexOf("?") !== -1 ? betik.src.slice(betik.src.indexOf("?")) : "";
  var notlar = null;

  function notlariGetir() {
    if (notlar) return Promise.resolve(notlar);
    return fetch("/static/yenilikler.json" + surumParam, { headers: { "Accept": "application/json" } })
      .then(function (c) { return c.ok ? c.json() : []; })
      .then(function (v) { notlar = Array.isArray(v) ? v : []; return notlar; })
      .catch(function () { return []; });
  }

  function eleman(etiket, sinif, metin) {
    var e = document.createElement(etiket);
    if (sinif) e.className = sinif;
    if (metin) e.textContent = metin;
    return e;
  }

  // Metin createTextNode/textContent ile basılıyor, innerHTML ile değil.
  function bas(liste) {
    icerik.textContent = "";
    if (!liste.length) {
      icerik.appendChild(eleman("p", "ipucu", "Henüz sürüm notu yok."));
      return;
    }
    liste.forEach(function (n) {
      var bolum = eleman("section", "yenilik");
      var baslik = eleman("h3", "", n.surum || "");
      if (n.tarih) baslik.appendChild(eleman("span", "yenilik-tarih", " · " + n.tarih));
      bolum.appendChild(baslik);
      var ul = eleman("ul");
      (n.maddeler || []).forEach(function (m) { ul.appendChild(eleman("li", "", m)); });
      bolum.appendChild(ul);
      icerik.appendChild(bolum);
    });
  }

  function goster() {
    notlariGetir().then(function (liste) {
      bas(liste);
      if (!pencere.open) pencere.showModal();
      if (liste.length) yaz("yenilikler-gorulen", liste[0].surum);
    });
  }

  if (surumButonu) surumButonu.addEventListener("click", goster);

  // Pencerenin dışına (arka plana) tıklayınca kapat.
  pencere.addEventListener("click", function (olay) {
    if (olay.target === pencere) pencere.close();
  });

  notlariGetir().then(function (liste) {
    if (liste.length && oku("yenilikler-gorulen") !== liste[0].surum) goster();
  });
})();
