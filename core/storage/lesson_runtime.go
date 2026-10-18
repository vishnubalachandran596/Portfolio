package storage

// lessonRuntimeCSS and lessonRuntimeJS are injected into every lesson
// document by PrepareLesson. They give all lessons — whatever HTML they
// were authored with — a comfortable reading column, one-time section
// reveals, and a bridge to the course reader around the iframe
// (course_learn.html):
//
//   - lesson → reader  {type:"lesson-toc", items:[{id,text,level}]}
//   - lesson → reader  {type:"lesson-progress", pct, active}
//   - lesson → reader  {type:"lesson-run", code, lang}   ("Run in compiler")
//   - reader → lesson  {type:"lesson-goto", id}
//   - reader → lesson  {type:"reader-config", compiler}  (shows Run buttons)
//
// The iframe stays sandboxed without allow-same-origin, so the reader
// still cannot read the lesson DOM; it only ever sees what this script
// chooses to post. Messages from the reader are accepted only from the
// parent window.
const lessonRuntimeCSS = `
body.html-content{max-width:52rem;margin:0 auto;box-sizing:border-box}
.lr-reveal{opacity:0;transform:translateY(.75rem);transition:opacity 480ms cubic-bezier(.23,1,.32,1),transform 480ms cubic-bezier(.23,1,.32,1)}
.lr-reveal.lr-in{opacity:1;transform:none}
@media (prefers-reduced-motion:reduce){.lr-reveal{transform:none;transition:opacity 200ms ease}}
@media print{.lr-reveal{opacity:1!important;transform:none!important}}
pre{position:relative}
.lr-code-bar{position:absolute;top:.5rem;right:.5rem;display:flex;gap:.25rem}
.lr-code-btn{font:600 .75rem/1 var(--font-text,system-ui);min-height:1.875rem;padding:0 .55rem;border-radius:.375rem;border:1px solid var(--line-strong,#d6d1ca);background:var(--surface,#fff);color:var(--ink-2,#57534e);cursor:pointer}
.lr-code-btn:hover{color:var(--ink,#1c1917)}
.lr-code-btn.lr-run{color:var(--accent-strong,#854d0e);border-color:var(--accent,#a16207)}
.lr-code-btn:active{transform:scale(.95)}
h2,h3{scroll-margin-top:1rem}
pre{max-width:100%;overflow-x:auto}
`

const lessonRuntimeJS = `
(function () {
  "use strict";
  var parentWin = window.parent;
  function post(msg) { try { parentWin.postMessage(msg, "*"); } catch (e) {} }
  function slug(t) { return (t || "").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "").slice(0, 60) || "section"; }

  function init() {
    var body = document.body;
    var heads = Array.prototype.slice.call(body.querySelectorAll("h2, h3"));
    var hasH2 = heads.some(function (h) { return h.tagName === "H2"; });
    var used = {};
    var items = [];
    heads.forEach(function (h) {
      var text = (h.textContent || "").replace(/\s+/g, " ").trim();
      if (!text || h.closest("details:not([open]) > :not(summary)")) return;
      // Headings inside a lesson's own hidden tab panel can't be scrolled to.
      if (h.getClientRects().length === 0) return;
      // The lesson's own navigation label — mirrors models.IsNavigationHeading.
      if (/^(on this page|in this (lesson|course)|(table of )?contents|toc|navigation|jump to( a section)?):?$/i.test(text)) return;
      var level = h.tagName === "H2" ? 2 : (hasH2 ? 3 : 2);
      if (!h.id) {
        var base = slug(text), id = base, n = 2;
        while (used[id] || document.getElementById(id)) id = base + "-" + n++;
        h.id = id;
      }
      used[h.id] = true;
      items.push({ id: h.id, text: text.slice(0, 120), level: level, el: h });
    });
    post({ type: "lesson-toc", items: items.map(function (i) { return { id: i.id, text: i.text, level: i.level }; }) });

    // Progress + active section, throttled to one post per frame.
    var ticking = false;
    function report() {
      ticking = false;
      var doc = document.documentElement;
      var max = doc.scrollHeight - doc.clientHeight;
      var pct = max > 0 ? Math.min(1, doc.scrollTop / max) : 1;
      var active = "";
      for (var i = 0; i < items.length; i++) {
        if (items[i].el.getBoundingClientRect().top <= window.innerHeight * 0.3) active = items[i].id; else break;
      }
      if (!active && items.length) active = items[0].id;
      post({ type: "lesson-progress", pct: pct, active: active });
    }
    window.addEventListener("scroll", function () { if (!ticking) { ticking = true; requestAnimationFrame(report); } }, { passive: true });
    window.addEventListener("resize", report);
    report();

    window.addEventListener("message", function (e) {
      if (e.source !== parentWin || !e.data) return;
      if (e.data.type === "reader-config") {
        // A Run button shows only when the reader has a compiler for that
        // snippet's language (reader sends the list it has embed keys for).
        var langs = e.data.langs || [];
        Array.prototype.forEach.call(document.querySelectorAll(".lr-run"), function (b) {
          b.hidden = !e.data.compiler || langs.indexOf(b.getAttribute("data-lang")) < 0;
        });
      }
      if (e.data.type === "lesson-goto") {
        var el = document.getElementById(e.data.id);
        if (el) el.scrollIntoView({ behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth", block: "start" });
      }
    });

    // One-time reveals for top-level content blocks below the fold.
    if ("IntersectionObserver" in window) {
      var root = body.children.length === 1 && body.firstElementChild.children.length > 3 ? body.firstElementChild : body;
      var blocks = Array.prototype.filter.call(root.children, function (el) {
        return !/^(SCRIPT|STYLE|CANVAS|TEMPLATE)$/.test(el.tagName) && el.id !== "__access_watermark" && el.getBoundingClientRect().top > window.innerHeight;
      });
      var io = new IntersectionObserver(function (entries) {
        entries.forEach(function (en) { if (en.isIntersecting) { en.target.classList.add("lr-in"); io.unobserve(en.target); } });
      }, { rootMargin: "0px 0px -6% 0px" });
      // Inside sections, the units a reader scans — concept cards, labs,
      // walkthroughs, figures, tables, code — reveal individually (capped).
      // A top-level block that contains such units is not faded as a whole,
      // so a long section never sits invisible while its first card shows.
      var unitSel = ".concept-card, .lab-block, .walkthrough, figure, table, pre, .callout, .recap, .objectives, h2, h3";
      var inner = Array.prototype.slice.call(root.querySelectorAll(unitSel), 0, 600)
        .filter(function (el) { return el.parentElement !== root && el.getBoundingClientRect().top > window.innerHeight && !(el.parentElement && el.parentElement.closest(unitSel)); });
      blocks = blocks.filter(function (el) { return !el.querySelector(unitSel); });
      blocks.concat(inner).forEach(function (el) { el.classList.add("lr-reveal"); io.observe(el); });
    }

    // Code blocks: copy + run in the reader's compiler.
    Array.prototype.forEach.call(body.querySelectorAll("pre"), function (pre) {
      if (pre.querySelector(".lr-code-bar") || pre.closest(".lab-block, .lab-editor")) return;
      // An empty <pre> is a placeholder a lesson fills later (program output,
      // a result box), not a snippet to run.
      if (!(pre.textContent || "").trim()) return;
      var code = pre.querySelector("code") || pre;
      var cls = (code.className || "") + " " + (pre.className || "");
      var m = cls.match(/(?:language|lang)-([a-z0-9+#]+)/i);
      var lang = m ? m[1].toLowerCase() : "";
      var bar = document.createElement("div");
      bar.className = "lr-code-bar";
      // A snippet that needs a whole project (many files, a framework,
      // third-party libraries) is marked data-norun on its <pre>: it keeps
      // Copy but gets no Run button, since the one-file compiler cannot run it.
      var noRun = pre.hasAttribute("data-norun");
      var copy = document.createElement("button");
      copy.type = "button";
      copy.className = "lr-code-btn lr-copy";
      copy.textContent = "Copy";
      copy.title = "Copy this snippet";
      copy.addEventListener("click", function () {
        post({ type: "lesson-copy", code: (code.innerText || code.textContent) });
        copy.textContent = "Copied";
        setTimeout(function () { copy.textContent = "Copy"; }, 1500);
      });
      bar.appendChild(copy);
      var run = document.createElement("button");
      run.type = "button";
      run.className = "lr-code-btn lr-run";
      run.textContent = "Run";
      run.title = "Open this snippet in the compiler";
      run.hidden = true; // shown once the reader says a compiler exists for lang
      run.setAttribute("data-lang", lang === "js" || lang === "node" ? "javascript" : (lang === "py" || lang === "python3" ? "python" : (lang === "sh" || lang === "shell" || lang === "zsh" ? "bash" : lang)));
      run.addEventListener("click", function () { post({ type: "lesson-run", code: (code.innerText || code.textContent), lang: run.getAttribute("data-lang") }); });
      if (!noRun) bar.appendChild(run);
      pre.appendChild(bar);
    });
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init); else init();
})();
`
