package models

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSEOFieldsTitleFallback(t *testing.T) {
	pattern := "Kafka Course — Site"
	if got := (SEOFields{}).Title(pattern); got != pattern {
		t.Errorf("empty seo_title: got %q, want the title pattern", got)
	}
	if got := (SEOFields{SEOTitle: "   "}).Title(pattern); got != pattern {
		t.Errorf("blank seo_title: got %q, want the title pattern", got)
	}
	if got := (SEOFields{SEOTitle: " Learn Kafka "}).Title(pattern); got != "Learn Kafka" {
		t.Errorf("explicit seo_title: got %q", got)
	}
}

func TestSEOFieldsDescriptionFallbackChain(t *testing.T) {
	long := strings.Repeat("Distributed systems in practice. ", 20)

	// 1. explicit meta_description wins over everything.
	f := SEOFields{MetaDescription: "Explicit meta."}
	if got := f.Description("A summary.", long); got != "Explicit meta." {
		t.Errorf("explicit: got %q", got)
	}
	// 2. otherwise the summary, flattened to plain text.
	if got := (SEOFields{}).Description("A **bold** and *italic* summary.", long); got != "A bold and italic summary." {
		t.Errorf("summary: got %q", got)
	}
	// 3. otherwise the description, truncated to the limit.
	got := (SEOFields{}).Description("", long)
	if n := utf8.RuneCountInString(got); n > SEODescriptionMax || n < SEODescriptionMax/2 {
		t.Errorf("truncated description has %d characters, want <= %d", n, SEODescriptionMax)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated description should end with an ellipsis: %q", got)
	}
	// 4. nothing at all stays empty rather than inventing text.
	if got := (SEOFields{}).Description("", ""); got != "" {
		t.Errorf("empty chain: got %q", got)
	}
}

func TestTruncateSEO(t *testing.T) {
	if got := TruncateSEO("short", 160); got != "short" {
		t.Errorf("short string changed: %q", got)
	}
	got := TruncateSEO("alpha beta gamma delta epsilon zeta eta theta", 20)
	if utf8.RuneCountInString(got) > 20 || strings.Contains(got, "thet") {
		t.Errorf("not truncated at a word boundary within 20: %q", got)
	}
	if got := TruncateSEO("ünïcödé ünïcödé ünïcödé ünïcödé", 12); utf8.RuneCountInString(got) > 12 {
		t.Errorf("rune-aware truncation failed: %q", got)
	}
}

func TestSEOPlainText(t *testing.T) {
	in := "  Hello <b>world</b>,\n  **bold**   and *it* "
	if got := SEOPlainText(in); got != "Hello world , bold and it" {
		t.Errorf("got %q", got)
	}
}

func TestSEOFieldsImageFallback(t *testing.T) {
	if got := (SEOFields{OGImage: "/img/a.png"}).Image("/cover.png"); got != "/img/a.png" {
		t.Errorf("og_image should win, got %q", got)
	}
	if got := (SEOFields{}).Image(" /cover.png "); got != "/cover.png" {
		t.Errorf("cover fallback, got %q", got)
	}
	if got := (SEOFields{}).Image(""); got != "" {
		t.Errorf("no image anywhere should be empty (caller applies site default), got %q", got)
	}
}

func TestIsStaticSEOPath(t *testing.T) {
	for _, p := range []string{"/", "/about", "/consultancy", "/terms"} {
		if !IsStaticSEOPath(p) {
			t.Errorf("%s should be a static SEO page", p)
		}
	}
	for _, p := range []string{"/login", "/courses/x", "/admin", "/account"} {
		if IsStaticSEOPath(p) {
			t.Errorf("%s must not be a static SEO page", p)
		}
	}
}
