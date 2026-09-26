package storage

import (
	"encoding/base64"
	"encoding/json"
	"html"
	"strings"
)

// PreparedLesson is a complete iframe document (site CSS + stripped
// body + watermark) as a base64 payload. The learn page assigns it to
// srcdoc; view-source of the outer page does not contain the lesson
// tags, and the iframe is sandboxed without allow-same-origin so the
// parent cannot read the lesson DOM.
type PreparedLesson struct {
	Payload string
}

// WatermarkLabel is the visible mark on gated lessons and article pages.
const WatermarkLabel = "vishnubalachandran.com"

// PrepareLesson extracts the document body, drops uploaded <style>
// blocks so the site typography wins, wraps the fragment in a document
// that loads stylesheet, and injects the watermark. identity is stamped
// onto copied text inside the iframe.
func PrepareLesson(raw []byte, identity, stylesheet string) (PreparedLesson, error) {
	body := StripStyleTags(ArticleHTML(raw))
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html lang=\"en\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	if stylesheet != "" {
		b.WriteString(`<link rel="stylesheet" href="`)
		b.WriteString(html.EscapeString(stylesheet))
		b.WriteString(`">`)
	}
	b.WriteString(`<style>`)
	b.WriteString(`html,body{margin:0;min-height:100%;background:var(--color-surface,#fff);color:var(--color-text,#1c1917);}`)
	b.WriteString(`body{padding:1.25rem 1.5rem 2rem;}`)
	b.WriteString(`html{-webkit-user-select:none;user-select:none;}`)
	b.WriteString(`textarea,input,select,.lab-editor{-webkit-user-select:text;user-select:text;}`)
	b.WriteString(`</style></head><body class="html-content article-body">`)
	b.WriteString(body)
	b.WriteString(WatermarkScript(WatermarkLabel, identity))
	b.WriteString("</body></html>")
	return PreparedLesson{
		Payload: base64.StdEncoding.EncodeToString([]byte(b.String())),
	}, nil
}

// WatermarkScript returns a self-contained <canvas>/<script> block that
// draws label at most three times along the host diagonal, from one
// corner to the opposite. When #html-renderer is present it paints only
// inside that pane; otherwise (the lesson iframe) it covers the iframe
// viewport, which is the same pane. Font size scales with the diagonal.
//
// The text is canvas-rendered pixels, not a DOM node holding the string.
// A MutationObserver re-appends the canvas if it is deleted. None of this
// makes the watermark impossible to strip — see the realism note in
// README.md — but it raises the bar past a one-line .remove().
//
// label and copyIdentity are embedded via encoding/json, which
// HTML-escapes '<', '>', and '&' by default.
func WatermarkScript(label, copyIdentity string) string {
	labelJSON, _ := json.Marshal(label)
	copyJSON, _ := json.Marshal(copyIdentity)
	return `
<canvas id="__access_watermark" aria-hidden="true" style="position:absolute;inset:0;z-index:2147483647;pointer-events:none;width:100%;height:100%;opacity:.16;"></canvas>
<style>
@media print{ #__access_watermark{ opacity:.24; } }
#html-renderer, .html-renderer { position: relative; }
</style>
<script>
(function () {
  var label = ` + string(labelJSON) + `;
  var copyId = ` + string(copyJSON) + `;
  var PINNED = 'position:absolute;inset:0;z-index:2147483647;pointer-events:none;width:100%;height:100%;opacity:.16;';
  var FIXED = 'position:fixed;inset:0;z-index:2147483647;pointer-events:none;width:100%;height:100%;opacity:.16;';

  function hostEl() { return document.getElementById('html-renderer') || document.body; }
  function pinned() { return !!document.getElementById('html-renderer'); }
  function canvasEl() { return document.getElementById('__access_watermark'); }

  function mount(c) {
    var host = hostEl();
    if (c.parentNode !== host) host.appendChild(c);
    c.setAttribute('style', pinned() ? PINNED : FIXED);
  }

  function size() {
    if (pinned()) {
      var h = hostEl();
      return { w: h.clientWidth, h: h.clientHeight };
    }
    return { w: window.innerWidth, h: window.innerHeight };
  }

  function isDark() {
    var theme = document.documentElement.getAttribute('data-theme');
    if (theme === 'dark') return true;
    if (theme === 'light') return false;
    return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  }

  function draw() {
    var c = canvasEl();
    if (!c) return;
    mount(c);
    var dpr = window.devicePixelRatio || 1;
    var box = size();
    var w = box.w, h = box.h;
    if (w < 8 || h < 8) return;
    c.width = Math.round(w * dpr);
    c.height = Math.round(h * dpr);
    var ctx = c.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    var diag = Math.hypot(w, h);
    var fontSize = Math.max(15, Math.min(24, diag / 48));
    ctx.save();
    ctx.translate(w / 2, h / 2);
    ctx.rotate(-Math.atan2(h, w));
    ctx.fillStyle = isDark() ? '#d6d3d1' : '#78716c';
    ctx.font = '600 ' + fontSize + 'px system-ui, -apple-system, sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    var textW = ctx.measureText(label).width;
    if (textW * 3.5 > diag) {
      fontSize = Math.max(12, fontSize * (diag / (textW * 3.5)));
      ctx.font = '600 ' + fontSize + 'px system-ui, -apple-system, sans-serif';
      textW = ctx.measureText(label).width;
    }
    var inset = Math.min(textW / 2 + 16, diag / 2 - 8);
    var start = -diag / 2 + inset;
    var end = diag / 2 - inset;
    ctx.fillText(label, start, 0);
    ctx.fillText(label, 0, 0);
    ctx.fillText(label, end, 0);
    ctx.restore();
  }

  var pending = null;
  function schedule() {
    if (pending) return;
    pending = requestAnimationFrame(function () { pending = null; draw(); });
  }

  function insideHost(node) {
    var host = document.getElementById('html-renderer');
    if (!host) return true;
    return !!(node && host.contains(node));
  }

  var mo = new MutationObserver(function () {
    if (!canvasEl()) {
      var el = document.createElement('canvas');
      el.id = '__access_watermark';
      el.setAttribute('aria-hidden', 'true');
      mount(el);
      draw();
    }
  });
  mo.observe(document.documentElement, { childList: true, subtree: true });

  window.addEventListener('resize', schedule);
  window.addEventListener('message', function (e) {
    if (!e.data || e.data.type !== 'site-theme') return;
    if (e.data.theme) document.documentElement.setAttribute('data-theme', e.data.theme);
    else document.documentElement.removeAttribute('data-theme');
    schedule();
  });
  if (window.ResizeObserver) {
    new ResizeObserver(schedule).observe(hostEl());
  }
  mount(canvasEl());
  draw();

  document.addEventListener('contextmenu', function (e) {
    if (insideHost(e.target)) e.preventDefault();
  });
  document.addEventListener('copy', function (e) {
    try {
      var sel = window.getSelection();
      if (!insideHost(sel && sel.anchorNode)) return;
      var mark = ' — ' + label;
      if (copyId) mark += ' / ' + copyId;
      mark += ' —';
      var text = (sel && sel.toString()) || '';
      var lines = text.split('\n');
      var out = [];
      for (var i = 0; i < lines.length; i++) {
        out.push(lines[i]);
        if (lines[i] !== '' && (i + 1) % 3 === 0) out.push(mark);
      }
      out.push(mark);
      e.clipboardData.setData('text/plain', out.join('\n'));
      e.preventDefault();
    } catch (err) {}
  });
})();
</script>`
}
