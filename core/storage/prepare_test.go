package storage

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPrepareLessonHidesMarkupAndTilesWatermark(t *testing.T) {
	raw := []byte("<!DOCTYPE html><html><head><title>Java</title></head><body><p>Secret lesson body about heap internals</p></body></html>")
	shell, err := PrepareLesson(raw, "buyer@example.com")
	if err != nil {
		t.Fatal(err)
	}
	page := string(shell)
	if strings.Contains(page, "Secret lesson body") {
		t.Fatal("lesson text is visible in the response")
	}
	if strings.Contains(page, "<p>") {
		t.Fatal("lesson tags are visible in the response")
	}
	decoded := decodeShell(t, page)
	if !strings.Contains(decoded, "Secret lesson body about heap internals") {
		t.Fatal("decoded lesson lost the body")
	}
	if strings.Count(decoded, "Licensed to buyer@example.com") < 64 {
		t.Fatalf("watermark tiles = %d", strings.Count(decoded, "Licensed to buyer@example.com"))
	}
	if !strings.Contains(decoded, "inset:-25%") {
		t.Fatal("watermark is not fixed across the viewport")
	}
	if !strings.Contains(decoded, "pointer-events:none") {
		t.Fatal("watermark would block the lesson")
	}
	if !strings.Contains(decoded, `licensed to buyer@example.com`) {
		t.Fatal("copy handler missing identity")
	}
}

func TestPrepareLessonEscapesIdentity(t *testing.T) {
	raw := []byte("<html><body><p>x</p></body></html>")
	shell, err := PrepareLesson(raw, `a<b>@evil.com`)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeShell(t, string(shell))
	if strings.Contains(decoded, "<b>") {
		t.Fatal("identity was injected as HTML")
	}
	if !strings.Contains(decoded, "a&lt;b&gt;@evil.com") {
		t.Fatal("identity was not escaped")
	}
}

func decodeShell(t *testing.T, page string) string {
	t.Helper()
	const start = `var b = "`
	i := strings.Index(page, start)
	if i < 0 {
		t.Fatal("no payload")
	}
	rest := page[i+len(start):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatal("unterminated payload")
	}
	raw, err := base64.StdEncoding.DecodeString(rest[:j])
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
