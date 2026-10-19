package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLegacyTarget(t *testing.T) {
	cases := []struct {
		path   string
		target string
		status int
		ok     bool
	}{
		{"/about/", "/about", http.StatusPermanentRedirect, true},
		{"/courses/java-fundamentals/", "/courses/java-fundamentals", http.StatusPermanentRedirect, true},
		{"/blog", "/articles", http.StatusMovedPermanently, true},
		{"/blog/", "/articles", http.StatusMovedPermanently, true},
		{"/blog/some-post", "/articles/some-post", http.StatusMovedPermanently, true},
		{"/course/java-fundamentals", "/courses/java-fundamentals", http.StatusMovedPermanently, true},
		{"/book", "/booking", http.StatusMovedPermanently, true},
		{"/", "", 0, false},
		{"/about", "", 0, false},
		{"/courses/x", "", 0, false},
		{"/blogger", "", 0, false},
		{"/booking", "", 0, false},
		{"/admin/", "", 0, false},
		{"/static/css/style.css", "", 0, false},
	}
	for _, c := range cases {
		target, status, ok := legacyTarget(c.path)
		if target != c.target || status != c.status || ok != c.ok {
			t.Errorf("legacyTarget(%q) = (%q, %d, %v), want (%q, %d, %v)", c.path, target, status, ok, c.target, c.status, c.ok)
		}
	}
}

func TestLegacyRedirectsKeepsQueryAndSkipsPost(t *testing.T) {
	called := false
	h := LegacyRedirects(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/blog/?tag=Kafka", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/articles?tag=Kafka" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if called {
		t.Fatal("redirect must not reach the next handler")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/contact/", nil))
	if !called {
		t.Fatal("POST must pass through untouched")
	}
}
