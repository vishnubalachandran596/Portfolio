package storage

import (
	"encoding/base64"
	"encoding/json"
)

// PreparedLesson is the gated lesson body (styles stripped) as a
// base64 payload so view-source of the outer page does not contain
// the lesson tags. The learn page injects it into the site-styled
// scroll pane and runs any leftover scripts so labs still work.
type PreparedLesson struct {
	Payload string
}

// WatermarkLabel is the visible mark on gated lessons and article pages.
const WatermarkLabel = "vishnubalachandran.com"

// PrepareLesson extracts the document body, drops uploaded <style>
// blocks so the site typography wins, and base64-encodes the fragment.
// identity is unused here — the page-level WatermarkScript stamps
// copies with it — but kept so callers do not fork a second signature.
func PrepareLesson(raw []byte, identity string) (PreparedLesson, error) {
	_ = identity
	body := StripStyleTags(ArticleHTML(raw))
	return PreparedLesson{
		Payload: base64.StdEncoding.EncodeToString([]byte(body)),
	}, nil
}

// WatermarkScript returns a self-contained <canvas>/<script> block that
// draws label at most three times along the viewport diagonal, from one
// corner to the opposite. Font size scales with the diagonal; canvas
// opacity stays low so the page underneath remains readable. A tiled
// grid with mix-blend-mode:difference used to invert real text.
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
<canvas id="__access_watermark" aria-hidden="true" style="position:fixed;inset:0;z-index:2147483647;pointer-events:none;width:100%;height:100%;opacity:.16;"></canvas>
<style>
@media print{ #__access_watermark{ opacity:.24; } }
</style>
<script>
(function () {
  var label = ` + string(labelJSON) + `;
  var copyId = ` + string(copyJSON) + `;
  var CSS = 'position:fixed;inset:0;z-index:2147483647;pointer-events:none;width:100%;height:100%;opacity:.16;';

  function canvasEl() { return document.getElementById('__access_watermark'); }

  function draw() {
    var c = canvasEl();
    if (!c) return;
    var dpr = window.devicePixelRatio || 1;
    var w = window.innerWidth, h = window.innerHeight;
    c.width = Math.round(w * dpr);
    c.height = Math.round(h * dpr);
    var ctx = c.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    var diag = Math.hypot(w, h);
    var fontSize = Math.max(15, Math.min(24, diag / 48));
    var theme = document.documentElement.getAttribute('data-theme');
    var dark = theme === 'dark' || (theme !== 'light' && window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
    ctx.save();
    ctx.translate(w / 2, h / 2);
    ctx.rotate(-Math.atan2(h, w));
    ctx.fillStyle = dark ? '#d6d3d1' : '#78716c';
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

  var mo = new MutationObserver(function () {
    if (!canvasEl()) {
      var el = document.createElement('canvas');
      el.id = '__access_watermark';
      el.setAttribute('aria-hidden', 'true');
      el.setAttribute('style', CSS);
      document.body.appendChild(el);
      draw();
    }
  });
  mo.observe(document.body, { childList: true });

  window.addEventListener('resize', schedule);
  draw();

  document.addEventListener('contextmenu', function (e) { e.preventDefault(); });
  document.addEventListener('copy', function (e) {
    try {
      var mark = ' — ' + label;
      if (copyId) mark += ' / ' + copyId;
      mark += ' —';
      var text = window.getSelection().toString() || '';
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
