package storage

import (
	"strings"
	"testing"
)

const previewLesson = `<html><head><style>p{color:red}</style></head><body>
<h2>1. Intro</h2><p>free text</p>
<!-- vb:preview-end -->
<h2>2. Deep dive</h2><p>paid text</p>
<script>var lab=1;</script>
</body></html>`

func TestNormalizeLessonKeepsPreviewMarker(t *testing.T) {
	out := string(NormalizeLesson([]byte(previewLesson)))
	if !strings.Contains(out, PreviewEndMarker) {
		t.Fatalf("marker lost by NormalizeLesson:\n%s", out)
	}
	again := string(NormalizeLesson([]byte(out)))
	if strings.Count(again, PreviewEndMarker) != 1 {
		t.Fatal("marker duplicated or lost on second pass")
	}
	if p, ok := LessonPreview([]byte(out)); !ok || strings.Contains(string(p), "paid text") {
		t.Fatalf("preview after normalize leaked or missing: %v", ok)
	}
}

func TestLessonPreviewMarker(t *testing.T) {
	p, ok := LessonPreview([]byte(previewLesson))
	if !ok {
		t.Fatal("expected preview")
	}
	s := string(p)
	if !strings.Contains(s, "free text") || strings.Contains(s, "paid text") || strings.Contains(s, "Deep dive") {
		t.Fatalf("wrong cut: %s", s)
	}
	if !strings.Contains(s, "var lab=1;") || !strings.HasSuffix(s, "</body></html>") {
		t.Fatalf("tail script or closing missing: %s", s)
	}
}

func TestLessonPreviewFirstSectionWithoutMarker(t *testing.T) {
	raw := `<body><h2>One</h2><p>first</p><script>var x="<h2>";</script><h2>Two</h2><p>second</p></body>`
	p, ok := LessonPreview([]byte(raw))
	if !ok || !strings.Contains(string(p), "first") || strings.Contains(string(p), "second") {
		t.Fatalf("first-section cut wrong: %v %s", ok, p)
	}
}

func TestLessonPreviewNoSecondHeading(t *testing.T) {
	if _, ok := LessonPreview([]byte(`<body><h2>Only</h2><p>all of it</p></body>`)); ok {
		t.Fatal("a one-section lesson must not have a preview")
	}
	if HasPreview([]byte(`<body><p>nothing</p></body>`)) {
		t.Fatal("no headings, no preview")
	}
}
