// Arama sayfası: yazarken arama, klavyeyle sonuç gezme, satır eylemleri ve
// içerik parçacıkları.
(function () {
  "use strict";

  var form = document.querySelector(".arama-form");
  var aramaKutusu = form && form.querySelector("input[name=q]");
  var alan = document.getElementById("sonuc-alani");

  // ------------------------------------------------ satır eylemleri
  //
  // "Aç" ve "Klasörde göster" düz bir HTML form POST'u olarak da çalışır (JS
  // kapalıysa sunucu geri yönlendirir), ama o zaman sayfa yenilenir ve
  // kaydırma konumu kaybolur. Burada araya girip fetch ile gönderiyoruz.
  // Dinleyici sonuç alanında: canlı arama satırları yeniden bassa da çalışır.
  function satirEylemi(form) {
    var buton = form.querySelector("button");
    if (!buton || buton.disabled) return;
    var eskiMetin = buton.textContent;
    buton.disabled = true;
    buton.textContent = "Açılıyor…";

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
        if (!c.ok) throw new Error();
        return c.json();
      })
      .then(function (v) {
        buton.textContent = v && v.klasorde ? "Güvenlik: klasörde gösterildi" : "Açıldı ✓";
      })
      .catch(function () {
        buton.textContent = "Açılamadı";
        buton.classList.add("hata-metni");
      })
      .then(function () {
        // Kısa bir süre sonra butonu eski haline döndür.
        setTimeout(function () {
          buton.disabled = false;
          buton.textContent = eskiMetin;
          buton.classList.remove("hata-metni");
        }, 1600);
      });
  }

  if (alan) {
    alan.addEventListener("submit", function (olay) {
      if (!olay.target.classList.contains("satir-form")) return;
      olay.preventDefault();
      satirEylemi(olay.target);
    });
  }

  // ------------------------------------------------ uzantı süzgeci
  //
  // Uzantı sayısı arttıkça pill listesinde aranan uzantıyı bulmak
  // zorlaşıyor; burada yazdıkça eşleşmeyenleri gizliyoruz. Zaten işaretli
  // (checked) kutular filtre yazılsa bile açık kalır, aksi halde bir
  // uzantı seçip sonra süzgece yazan kullanıcı seçimini görünmez halde
  // kaybettiğini sanır.
  var uzantiSuzgec = document.querySelector(".uzanti-suzgec");
  var uzantiEtiketleri = Array.prototype.slice.call(
    document.querySelectorAll(".uzanti-etiket"));

  if (uzantiSuzgec && uzantiEtiketleri.length) {
    uzantiSuzgec.addEventListener("input", function () {
      var terim = uzantiSuzgec.value.trim().toLowerCase();
      uzantiEtiketleri.forEach(function (etiket) {
        var uzanti = (etiket.dataset.uzanti || "").toLowerCase();
        var kutu = etiket.querySelector("input[type=checkbox]");
        var goster = !terim || uzanti.indexOf(terim) !== -1 ||
          (kutu && kutu.checked);
        etiket.style.display = goster ? "" : "none";
      });
    });
  }

  // ------------------------------------------------ yolu kopyala
  //
  // navigator.clipboard yalnızca güvenli bağlamda var (127.0.0.1 sayılır);
  // başka bir adresten açılırsa eski execCommand yoluna düşüyoruz.
  function panoyaYaz(metin) {
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(metin);
    }
    return new Promise(function (coz, reddet) {
      var alanT = document.createElement("textarea");
      alanT.value = metin;
      alanT.setAttribute("readonly", "");
      alanT.style.position = "fixed";
      alanT.style.opacity = "0";
      document.body.appendChild(alanT);
      alanT.select();
      var tamam = false;
      try { tamam = document.execCommand("copy"); } catch (e) { tamam = false; }
      document.body.removeChild(alanT);
      if (tamam) { coz(); } else { reddet(); }
    });
  }

  function kopyala(buton, yalnizAd) {
    var metin = yalnizAd ? buton.dataset.ad : buton.dataset.yol;
    if (!buton.dataset.eskiMetin) buton.dataset.eskiMetin = buton.textContent;
    panoyaYaz(metin)
      .then(function () {
        buton.textContent = yalnizAd ? "Ad kopyalandı ✓" : "Kopyalandı ✓";
        buton.classList.remove("hata-metni");
      }, function () {
        buton.textContent = "Kopyalanamadı";
        buton.classList.add("hata-metni");
      })
      .then(function () {
        clearTimeout(buton._zamanlayici);
        buton._zamanlayici = setTimeout(function () {
          buton.textContent = buton.dataset.eskiMetin;
          buton.classList.remove("hata-metni");
        }, 1600);
      });
  }

  if (alan) {
    alan.addEventListener("click", function (olay) {
      var buton = olay.target.closest(".kopyala-buton");
      if (buton) {
        kopyala(buton, olay.shiftKey);
        return;
      }
      // Satırın boş bir yerine tıklamak onu klavye seçimi yapar.
      var satir = olay.target.closest(".sonuc");
      if (satir && !olay.target.closest("a, button, form")) {
        sec(satirlar().indexOf(satir));
      }
    });
    alan.addEventListener("dblclick", function (olay) {
      var satir = olay.target.closest(".sonuc");
      if (!satir || olay.target.closest("a, button, form")) return;
      var f = satir.querySelector(".ac-form");
      if (f) satirEylemi(f);
    });
  }

  // ------------------------------------------------ filtreleri sıfırla
  //
  // Alanları tek tek temizleyip formu göndermek yerine dosdoğru parametresiz
  // /arama'ya yönlendiriyoruz: hem daha basit hem de URL'de boş değerli
  // (ör. klasor=) parametre kalma ihtimalini yapısal olarak ortadan kaldırıyor.
  var sifirlaButonu = document.getElementById("filtreleriSifirla");
  if (sifirlaButonu) {
    sifirlaButonu.addEventListener("click", function () {
      location.href = "/arama";
    });
  }

  // ------------------------------------------------ klavyeyle gezme
  var secili = -1;

  function satirlar() {
    return alan ? Array.prototype.slice.call(alan.querySelectorAll(".sonuc")) : [];
  }

  function sec(i) {
    var l = satirlar();
    l.forEach(function (s, j) { s.classList.toggle("secili", j === i); });
    secili = i >= 0 && i < l.length ? i : -1;
    if (secili >= 0) l[secili].scrollIntoView({ block: "nearest" });
  }

  function alanaYaziyorMu(hedef) {
    return hedef.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(hedef.tagName);
  }

  document.addEventListener("keydown", function (olay) {
    var hedef = olay.target;
    var kutuda = hedef === aramaKutusu;
    if (alanaYaziyorMu(hedef) && !kutuda) return;

    // "/" arama kutusuna odaklanır.
    if (olay.key === "/" && !kutuda && !olay.ctrlKey && !olay.altKey && !olay.metaKey) {
      if (!aramaKutusu) return;
      olay.preventDefault();
      aramaKutusu.focus();
      aramaKutusu.select();
      return;
    }

    var l = satirlar();
    if (olay.key === "ArrowDown" || olay.key === "ArrowUp") {
      if (!l.length || olay.altKey) return;
      olay.preventDefault();
      var yeni = secili + (olay.key === "ArrowDown" ? 1 : -1);
      if (yeni < 0) {
        sec(-1);
        if (aramaKutusu) aramaKutusu.focus();
        return;
      }
      sec(Math.min(yeni, l.length - 1));
      return;
    }

    if (secili < 0) return;
    var satir = l[secili];

    if (olay.key === "Enter") {
      // Seçim yoksa Enter formu gönderir; seçim varsa dosyayı açar.
      olay.preventDefault();
      var f = satir.querySelector(olay.ctrlKey || olay.metaKey ? ".klasor-form" : ".ac-form");
      if (f) satirEylemi(f);
      return;
    }

    if ((olay.key === "c" || olay.key === "C") && (olay.ctrlKey || olay.metaKey)) {
      // Kullanıcı bir metni seçmişse normal kopyalamayı bozma.
      if (kutuda && aramaKutusu.selectionStart !== aramaKutusu.selectionEnd) return;
      if (!kutuda && String(window.getSelection())) return;
      var b = satir.querySelector(".kopyala-buton");
      if (!b) return;
      olay.preventDefault();
      kopyala(b, olay.shiftKey);
      return;
    }

    if (olay.key === "Escape") {
      sec(-1);
    }
  });

  if (aramaKutusu) {
    aramaKutusu.addEventListener("keydown", function (olay) {
      if (olay.key !== "Escape" || secili >= 0) return;
      olay.preventDefault();
      if (aramaKutusu.value) {
        aramaKutusu.value = "";
        aramaKutusu.dispatchEvent(new Event("input"));
      } else {
        aramaKutusu.blur();
      }
    });
  }

  // ------------------------------------------------ yazarken arama
  //
  // Sorgu sunucuda milisaniyeler sürüyor; darboğaz Enter'a basmaktı. Yalnızca
  // sonuç bölümü (parca=sonuclar) istenir ve yerine basılır; adres çubuğu
  // replaceState ile eşlenir, yani yenileme ve yer imi aynı sonucu verir.
  var zamanlayici = null;
  var denetleyici = null;
  var istekNo = 0;

  function formAdresi() {
    var temiz = new URLSearchParams();
    new URLSearchParams(new FormData(form)).forEach(function (v, k) {
      if (v !== "") temiz.append(k, v);
    });
    return temiz.toString();
  }

  function canliAra() {
    var sorguDizesi = formAdresi();
    if (denetleyici) denetleyici.abort();
    denetleyici = typeof AbortController === "function" ? new AbortController() : null;
    var no = ++istekNo;
    var adres = "/arama?" + (sorguDizesi ? sorguDizesi + "&" : "") + "parca=sonuclar";

    fetch(adres, { signal: denetleyici ? denetleyici.signal : undefined })
      .then(function (c) { return c.ok ? c.text() : Promise.reject(); })
      .then(function (html) {
        if (no !== istekNo) return; // daha yeni bir istek yolda
        alan.innerHTML = html;
        history.replaceState(null, "", "/arama" + (sorguDizesi ? "?" + sorguDizesi : ""));
        sonuclariHazirla();
      })
      .catch(function () { /* iptal edilen ya da başarısız istek: eski sonuç kalır */ });
  }

  if (form && aramaKutusu && alan && window.fetch) {
    aramaKutusu.addEventListener("input", function () {
      clearTimeout(zamanlayici);
      zamanlayici = setTimeout(canliAra, 200);
    });
    form.querySelectorAll("input[name=kapsam]").forEach(function (r) {
      r.addEventListener("change", canliAra);
    });
  }

  // ------------------------------------------------ içerik parçacıkları
  //
  // Neden ayrı istek: 300 sayfalık bir PDF'ten metin çıkarmak 1-5 saniye
  // sürer. 20 sonucu sunucuda satır içi çıkarsaydık sayfa 10 saniyede
  // açılırdı. Sonuçlar hemen basılıyor, parçacıklar burada dolduruluyor.

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

  // nesil, canlı arama sonuçları yeniden bastığında eski kuyruğu durdurur.
  var nesil = 0;

  function parcaciklariDoldur() {
    var benimNesil = ++nesil;
    var liste = alan && alan.querySelector(".sonuclar");
    if (!liste) return;
    var sorgu = liste.dataset.sorgu || "";
    if (!sorgu.trim()) return;

    var kutular = Array.prototype.slice.call(
      liste.querySelectorAll(".parcacik[data-yol]"));

    // Sırayla, birer birer: sunucu tarafında da 4'lük bir havuz sınırı var,
    // istemciden 20 paralel istek atmanın faydası yok.
    var sira = 0;
    function sonraki() {
      if (benimNesil !== nesil || sira >= kutular.length) return;
      var kutu = kutular[sira++];
      var adres = "/api/parcacik?yol=" + encodeURIComponent(kutu.dataset.yol) +
        "&q=" + encodeURIComponent(sorgu);

      fetch(adres, { headers: { "Accept": "application/json" } })
        .then(function (c) { return c.ok ? c.json() : null; })
        .then(function (v) { if (v && benimNesil === nesil) bas(kutu, v.parcalar); })
        .catch(function () { /* parçacık gelmezse satır yine kullanılabilir */ })
        .then(sonraki);
    }
    sonraki();
  }

  // sonuclariHazirla ilk yüklemede ve her canlı aramadan sonra çalışır.
  function sonuclariHazirla() {
    secili = -1;
    if (!alan) return;
    // JS olmadan işlevsiz oldukları için gizli gelirler.
    alan.querySelectorAll(".kopyala-buton").forEach(function (b) { b.hidden = false; });
    parcaciklariDoldur();
  }

  sonuclariHazirla();
})();
