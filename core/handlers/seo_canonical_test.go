package handlers

import (
	"net/http/httptest"
	"testing"
)

// TestCanonicalURLForRequest pins the listing canonical rule from the SEO
// audit of 19 Oct 2026: tag, sort and q variants canonicalise to the bare
// listing (they are near-duplicates of it); only page > 1 keeps its own
// self-referential canonical, because each page lists different items.
func TestCanonicalURLForRequest(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"/courses", "https://example.com/courses"},
		{"/courses?tag=Java", "https://example.com/courses"},
		{"/courses?page=2", "https://example.com/courses?page=2"},
		{"/articles?tag=Java&page=2", "https://example.com/articles?page=2"},
		{"/courses?sort=newest", "https://example.com/courses"},
		{"/courses?q=kafka", "https://example.com/courses"},
		{"/courses?page=1", "https://example.com/courses"},
		{"/articles?tag=Kafka+Streams", "https://example.com/articles"},
		{"/courses/java-fundamentals?utm_source=x", "https://example.com/courses/java-fundamentals"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", c.url, nil)
		if got := canonicalURLForRequest("https://example.com", r); got != c.want {
			t.Errorf("canonicalURLForRequest(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestCatalogListTitle(t *testing.T) {
	cases := []struct {
		tag  string
		page int
		want string
	}{
		{"", 1, "Blog"},
		{"Kafka", 1, "Kafka — Blog"},
		{"", 3, "Blog (Page 3)"},
		{"Kafka", 2, "Kafka — Blog (Page 2)"},
	}
	for _, c := range cases {
		if got := catalogListTitle("Blog", c.tag, c.page); got != c.want {
			t.Errorf("catalogListTitle(%q, %d) = %q, want %q", c.tag, c.page, got, c.want)
		}
	}
}
