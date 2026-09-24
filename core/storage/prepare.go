package storage

import (
	"bytes"
	"encoding/base64"
	"html"
	"strings"
)

// PrepareLesson returns the HTTP body for a gated lesson. The lesson
// markup is not in that body as HTML: it is inside a base64 payload the
// shell decodes and writes into the document. View-source therefore shows
// the shell, not the course. The decoded document carries a watermark
// tiled across the whole viewport, and copied text is stamped with the
// same identity throughout.
func PrepareLesson(raw []byte, identity string) ([]byte, error) {
	var marked bytes.Buffer
	if err := WriteWatermarked(&marked, bytes.NewReader(raw), watermarkBlock(identity)); err != nil {
		return nil, err
	}
	return lessonShell(marked.Bytes()), nil
}

func watermarkBlock(identity string) string {
	safe := html.EscapeString(identity)
	var tiles strings.Builder
	const n = 64
	for i := 0; i < n; i++ {
		tiles.WriteString("<span>Licensed to ")
		tiles.WriteString(safe)
		tiles.WriteString("</span>")
	}
	return `
<div id="__access_watermark" aria-hidden="true">` + tiles.String() + `</div>
<style>
#__access_watermark{
  position:fixed;
  inset:-25%;
  z-index:2147483647;
  pointer-events:none;
  user-select:none;
  display:flex;
  flex-wrap:wrap;
  align-content:space-around;
  justify-content:space-around;
  gap:2.5rem 1.75rem;
  overflow:hidden;
  transform:rotate(-28deg);
  opacity:.5;
  mix-blend-mode:difference;
  color:#fff;
  font:700 1.15rem/1.3 system-ui,sans-serif;
}
#__access_watermark span{white-space:nowrap;padding:.35rem .5rem}
@media print{
  #__access_watermark{
    opacity:.6;
    -webkit-print-color-adjust:exact;
    print-color-adjust:exact;
  }
}
</style>
<script>
document.addEventListener('contextmenu', function (e) { e.preventDefault(); });
document.addEventListener('copy', function (e) {
  try {
    var mark = " — licensed to ` + safe + ` —";
    var text = window.getSelection().toString() || "";
    var lines = text.split("\n");
    var out = [];
    for (var i = 0; i < lines.length; i++) {
      out.push(lines[i]);
      if (lines[i] !== "" && (i + 1) % 3 === 0) out.push(mark);
    }
    out.push(mark);
    e.clipboardData.setData('text/plain', out.join("\n"));
    e.preventDefault();
  } catch (err) {}
});
</script>`
}

func lessonShell(lesson []byte) []byte {
	payload := base64.StdEncoding.EncodeToString(lesson)
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="robots" content="noindex, nofollow">
<title>Lesson</title>
</head>
<body>
<script>
(function () {
  var b = "`)
	b.WriteString(payload)
	b.WriteString(`";
  var bin = atob(b);
  var bytes = new Uint8Array(bin.length);
  for (var i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  var html = new TextDecoder("utf-8").decode(bytes);
  document.open();
  document.write(html);
  document.close();
})();
</script>
</body>
</html>`)
	return []byte(b.String())
}
