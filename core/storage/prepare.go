package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// PreparedLesson is the gated lesson ready to sit inside the site chrome.
// Styles are the lesson's own CSS, rewritten so html/body/:root rules
// apply only inside .lesson-embed and cannot restyle the header or
// footer. Payload is a base64 document fragment: the lesson body plus
// the watermark script. The learn page decodes it into #lesson-root, so
// view-source shows the shell, not the lesson tags.
type PreparedLesson struct {
	Styles  string
	Payload string
}

// PrepareLesson returns the pieces for a gated lesson page. The lesson
// markup is not in the HTTP body as HTML: it is inside Payload. The
// decoded fragment carries a single light diagonal watermark, and copied
// text is stamped with the same identity throughout.
func PrepareLesson(raw []byte, identity string) (PreparedLesson, error) {
	styles := ScopeCSS(ExtractStyles(raw), ".lesson-embed")
	body := stripStyleTags(ArticleHTML(raw))
	if body == "" {
		body = "<div></div>"
	}
	var marked bytes.Buffer
	if err := WriteWatermarked(&marked, strings.NewReader(body), WatermarkScript("Licensed to "+identity)); err != nil {
		return PreparedLesson{}, err
	}
	return PreparedLesson{
		Styles:  styles,
		Payload: base64.StdEncoding.EncodeToString(marked.Bytes()),
	}, nil
}

// WatermarkScript returns a self-contained <canvas>/<script> block that
// draws label repeated along one rotated diagonal band across the
// viewport — a single visible line, not a grid of tiles covering the
// whole page (that made dense paragraphs unreadable: every tile's
// mix-blend-mode:difference inverted the real text underneath it).
//
// The text is canvas-rendered pixels, not a DOM node holding the string:
// there is no "<span>Licensed to ...</span>" for a one-line
// document.querySelector(...).remove() to find and delete, and devtools'
// "inspect element → delete node" only removes the empty <canvas> shell —
// a MutationObserver notices it's gone and re-appends + redraws it
// immediately. None of this makes the watermark impossible to strip (a
// visitor who disables JavaScript entirely, or screenshots the page,
// still gets a clean copy) — see the realism note in README.md — but it
// raises the bar past the two one-line tricks that used to defeat it.
//
// label is embedded via encoding/json, which HTML-escapes '<', '>', and
// '&' by default — safe both inside the <script> tag (no "</script"
// break-out) and as a canvas fillText argument (which never parses HTML
// anyway, so injection isn't possible here regardless of escaping).
func WatermarkScript(label string) string {
	labelJSON, _ := json.Marshal(label)
	return `
<canvas id="__access_watermark" aria-hidden="true" style="position:fixed;inset:0;z-index:2147483647;pointer-events:none;width:100%;height:100%;opacity:.2;"></canvas>
<style>
@media print{ #__access_watermark{ opacity:.28; } }
</style>
<script>
(function () {
  var label = ` + string(labelJSON) + `;
  var CSS = 'position:fixed;inset:0;z-index:2147483647;pointer-events:none;width:100%;height:100%;opacity:.2;';

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
    ctx.save();
    ctx.translate(w / 2, h / 2);
    ctx.rotate(-28 * Math.PI / 180);
    ctx.fillStyle = '#a8a29e';
    ctx.font = "600 20px system-ui, -apple-system, sans-serif";
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    var gap = 420;
    var span = (Math.abs(w) + Math.abs(h));
    for (var x = -span; x <= span; x += gap) {
      ctx.fillText(label, x, 0);
    }
    ctx.restore();
  }

  var pending = null;
  function schedule() {
    if (pending) return;
    pending = requestAnimationFrame(function () { pending = null; draw(); });
  }

  // Self-healing: if the <canvas> node is deleted (devtools "delete
  // node", or a scraper's one-line .remove()), put it straight back.
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
      var mark = ' — ' + label + ' —';
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
