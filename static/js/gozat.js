// Sunucu taraflı dizin gezgini.
//
// Tarayıcının dosya seçme kutusu gerçek dosya sistemi yolu vermez, bu yüzden
// klasörleri sunucudan listeliyoruz. Metin girişi yetkili kalır: yol
// yapıştırmak hızlı yol, gözatma keşif yoludur.
(function () {
  "use strict";

  var kok = document.getElementById("gozat");
  if (!kok) return;

  var yolGiris = document.getElementById("gozat-yol");
  var gitBtn = document.getElementById("gozat-git");
  var ustBtn = document.getElementById("gozat-ust");
  var suruculer = document.getElementById("gozat-suruculer");
  var listeEl = document.getElementById("gozat-listesi");
  var hataEl = document.getElementById("gozat-hata");
  var secBtn = document.getElementById("gozat-sec");
  var dizinlerEl = document.getElementById("dizinler");

  var mevcutYol = "";
  var ustYol = "";

  function temizle(el) {
    while (el.firstChild) el.removeChild(el.firstChild);
  }

  function git(yol) {
    fetch("/api/gozat?yol=" + encodeURIComponent(yol || ""), {
      headers: { "Accept": "application/json" }
    })
      .then(function (c) { return c.json(); })
      .then(bas)
      .catch(function () { hataEl.textContent = "Sunucuya ulaşılamadı."; });
  }

  function bas(c) {
    mevcutYol = c.yol || "";
    ustYol = c.ust || "";
    yolGiris.value = mevcutYol;
    hataEl.textContent = c.hata || "";
    secBtn.disabled = !c.okunabilir || !mevcutYol;
    ustBtn.disabled = !ustYol;

    temizle(suruculer);
    (c.suruculer || []).forEach(function (s) {
      var b = document.createElement("button");
      b.type = "button";
      b.textContent = s;
      b.addEventListener("click", function () { git(s + "\\"); });
      suruculer.appendChild(b);
    });

    temizle(listeEl);
    (c.klasorler || []).forEach(function (k) {
      var li = document.createElement("li");
      var b = document.createElement("button");
      b.type = "button";
      b.textContent = k.ad;
      b.addEventListener("click", function () { git(k.yol); });
      li.appendChild(b);
      listeEl.appendChild(li);
    });
  }

  gitBtn.addEventListener("click", function () { git(yolGiris.value); });
  ustBtn.addEventListener("click", function () { if (ustYol) git(ustYol); });

  yolGiris.addEventListener("keydown", function (e) {
    if (e.key === "Enter") {
      e.preventDefault();
      git(yolGiris.value);
    }
  });

  secBtn.addEventListener("click", function () {
    if (!mevcutYol) return;
    var mevcut = dizinlerEl.value.split("\n").map(function (s) {
      return s.trim();
    }).filter(Boolean);
    var zatenVar = mevcut.some(function (s) {
      return s.toLowerCase() === mevcutYol.toLowerCase();
    });
    if (!zatenVar) mevcut.push(mevcutYol);
    dizinlerEl.value = mevcut.join("\n");
    hataEl.textContent = "";
  });

  // Sürücü listesiyle başla.
  git("");
})();
