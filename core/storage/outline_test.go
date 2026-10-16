package storage

import (
	"reflect"
	"testing"
)

func TestLessonOutline(t *testing.T) {
	raw := []byte(`<html><head><style>h2{color:red}</style></head><body>
<h1>Course</h1>
<h2 id="a">1. <code>HashMap</code> &amp; friends</h2><p>x</p>
<h3>sub</h3>
<script>var s = "<h2>not a heading</h2>";</script>
<h2>Concurrency
  basics</h2>
<h2>Concurrency basics</h2>
</body></html>`)
	got := LessonOutline(raw)
	want := []string{"HashMap & friends", "Concurrency basics"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LessonOutline = %#v, want %#v", got, want)
	}
}

func TestLessonOutlineFallsBackToH3(t *testing.T) {
	got := LessonOutline([]byte(`<h3>One</h3><p>x</p><h3>Two</h3>`))
	if !reflect.DeepEqual(got, []string{"One", "Two"}) {
		t.Fatalf("got %#v", got)
	}
	if got := LessonOutline([]byte(`<p>no headings</p>`)); len(got) != 0 {
		t.Fatalf("expected empty outline, got %#v", got)
	}
}

func TestLessonOutlineStripsNumbering(t *testing.T) {
	got := LessonOutline([]byte(`<h2>1. Intro</h2><h2>Part 2 — Design</h2><h2>3) Ops</h2><h2>2024 roadmap</h2>`))
	want := []string{"Intro", "Design", "Ops", "2024 roadmap"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
