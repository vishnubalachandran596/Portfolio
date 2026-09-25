package storage

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPrepareLessonHidesMarkupAndTilesWatermark(t *testing.T) {
	raw := []byte(`<!DOCTYPE html><html><head><title>Java</title><style>:root{--x:1}body{color:red}</style></head><body><p>Secret lesson body about heap internals</p></body></html>`)
	got, err := PrepareLesson(raw, "buyer@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Payload, "Secret lesson body") {
		t.Fatal("lesson text is visible in the payload")
	}
	if !strings.Contains(got.Styles, ".lesson-embed") {
		t.Fatal("lesson CSS was not scoped to the embed")
	}
	if strings.Contains(got.Styles, ":root") || strings.Contains(got.Styles, "body{") {
		t.Fatal("unscoped html/body/:root rules would restyle the site chrome")
	}
	decoded := decodePayload(t, got.Payload)
	if !strings.Contains(decoded, "Secret lesson body about heap internals") {
		t.Fatal("decoded lesson lost the body")
	}
	if !strings.Contains(decoded, `"Licensed to buyer@example.com"`) {
		t.Fatal("watermark identity missing from the canvas script")
	}
	if !strings.Contains(decoded, `<canvas id="__access_watermark"`) {
		t.Fatal("watermark is not canvas-rendered (a DOM text node would be trivially removable)")
	}
	if !strings.Contains(decoded, "fillText") {
		t.Fatal("watermark canvas never draws the label")
	}
	if !strings.Contains(decoded, "MutationObserver") {
		t.Fatal("watermark has no self-healing against node deletion")
	}
	if !strings.Contains(decoded, "inset:0") {
		t.Fatal("watermark is not fixed across the viewport")
	}
	if !strings.Contains(decoded, "pointer-events:none") {
		t.Fatal("watermark would block the lesson")
	}
	if strings.Contains(decoded, "mix-blend-mode") {
		t.Fatal("difference blend mode inverts lesson text and makes it unreadable")
	}
	if !strings.Contains(decoded, "Licensed to buyer@example.com") {
		t.Fatal("copy handler missing identity")
	}
}

func TestPrepareLessonEscapesIdentity(t *testing.T) {
	raw := []byte("<html><body><p>x</p></body></html>")
	got, err := PrepareLesson(raw, `a<b>@evil.com`)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodePayload(t, got.Payload)
	if strings.Contains(decoded, "<b>") {
		t.Fatal("identity was injected as HTML")
	}
	// encoding/json HTML-escapes '<'/'>' by default (\u003c/\u003e), which
	// is what keeps this safe inside the <script> tag; canvas fillText
	// never parses its argument as markup either way.
	if !strings.Contains(decoded, `a\u003cb\u003e@evil.com`) {
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
