package handlers

import (
	"net/http/httptest"
	"testing"
)

// TestCanonicalURLForRequest guards against the regression that caused
// Search Console's "Duplicate, Google chose different canonical than
// user" report (Oct 2026): /courses?tag=java and /articles?tag=Kafka
// both declared canonical straight to their bare list page despite
// showing a different set of items. tag and page>1 must stay in the
// canonical; sort and q must not, for the reasons in
// canonicalURLForRequest's own doc comment.
func TestCanonicalURLForRequest(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"/courses", "https://example.com/courses"},
		{"/courses?tag=Java", "https://example.com/courses?tag=Java"},
		{"/courses?page=2", "https://example.com/courses?page=2"},
		{"/courses?tag=Java&page=2", "https://example.com/courses?tag=Java&page=2"},
		{"/courses?sort=newest", "https://example.com/courses"},
		{"/courses?q=kafka", "https://example.com/courses"},
		{"/courses?page=1", "https://example.com/courses"},
		{"/articles?tag=Kafka+Streams", "https://example.com/articles?tag=Kafka+Streams"},
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
