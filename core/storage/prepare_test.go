package storage

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPrepareLessonHidesMarkupAndDropsUploadedStyles(t *testing.T) {
	raw := []byte(`<!DOCTYPE html><html><head><title>Java</title><style>:root{--x:1}body{color:red}</style></head><body><p>Secret lesson body about heap internals</p><script>window.__lab=1</script></body></html>`)
	got, err := PrepareLesson(raw, "buyer@example.com", "/static/css/style.css?v=test")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Payload, "Secret lesson body") {
		t.Fatal("lesson text is visible in the payload")
	}
	decoded := decodePayload(t, got.Payload)
	if !strings.Contains(decoded, "Secret lesson body about heap internals") {
		t.Fatal("decoded lesson lost the body")
	}
	if strings.Contains(decoded, "color:red") {
		t.Fatal("uploaded styles must be stripped so the site typography wins")
	}
	if !strings.Contains(decoded, `href="/static/css/style.css?v=test"`) {
		t.Fatal("iframe document must load the site stylesheet")
	}
	if !strings.Contains(decoded, "window.__lab=1") {
		t.Fatal("lesson scripts must stay so labs still run")
	}
	if !strings.Contains(decoded, `<canvas id="__access_watermark"`) {
		t.Fatal("watermark must ship inside the iframe document")
	}
	if !strings.Contains(decoded, "buyer@example.com") {
		t.Fatal("copy identity missing from the iframe document")
	}
}

func TestWatermarkScriptSpansDiagonal(t *testing.T) {
	got := WatermarkScript(WatermarkLabel, "buyer@example.com")
	if !strings.Contains(got, `"vishnubalachandran.com"`) {
		t.Fatal("visible watermark is not the site domain")
	}
	if !strings.Contains(got, "Math.atan2") || !strings.Contains(got, "Math.hypot") {
		t.Fatal("watermark is not drawn corner-to-corner along the host diagonal")
	}
	if !strings.Contains(got, "html-renderer") {
		t.Fatal("watermark is not restricted to the renderer pane")
	}
	if strings.Count(got, "ctx.fillText(label") > 3 {
		t.Fatalf("watermark draws more than 3 lines")
	}
	if !strings.Contains(got, `<canvas id="__access_watermark"`) {
		t.Fatal("watermark is not canvas-rendered")
	}
	if !strings.Contains(got, "pointer-events:none") {
		t.Fatal("watermark would block the lesson")
	}
	if strings.Contains(got, "mix-blend-mode") {
		t.Fatal("difference blend mode inverts lesson text and makes it unreadable")
	}
	if !strings.Contains(got, "opacity:.16") {
		t.Fatal("watermark opacity is not light enough to keep text usable")
	}
	if !strings.Contains(got, "prefers-color-scheme: dark") {
		t.Fatal("watermark color does not adapt to dark theme")
	}
	if !strings.Contains(got, "buyer@example.com") {
		t.Fatal("copy handler missing identity")
	}
}

func TestPrepareLessonEscapesIdentity(t *testing.T) {
	got := WatermarkScript(WatermarkLabel, `a<b>@evil.com`)
	if strings.Contains(got, "<b>") {
		t.Fatal("identity was injected as HTML")
	}
	if !strings.Contains(got, `a\u003cb\u003e@evil.com`) {
		t.Fatal("identity was not escaped")
	}
}

func decodePayload(t *testing.T, payload string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
