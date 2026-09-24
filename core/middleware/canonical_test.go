package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestCanonicalHostWWWIsNeverRedirected(t *testing.T) {
	h := CanonicalHost("https://www.vishnubalachandran.com")(okHandler())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://www.vishnubalachandran.com/", nil)
	req.Host = "www.vishnubalachandran.com"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("www homepage status = %d, want 200 (canonical must not 3xx)", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "" {
		t.Fatalf("www homepage Location = %q, want none", loc)
	}
}

func TestCanonicalHostApexAndHome301ToWWW(t *testing.T) {
	h := CanonicalHost("https://www.vishnubalachandran.com/")(okHandler())
	cases := []struct {
		host, path, wantLoc string
	}{
		{"vishnubalachandran.com", "/", "https://www.vishnubalachandran.com/"},
		{"vishnubalachandran.com", "/about", "https://www.vishnubalachandran.com/about"},
		{"vishnubalachandran.com", "/articles", "https://www.vishnubalachandran.com/articles"},
		{"home.vishnubalachandran.com", "/", "https://www.vishnubalachandran.com/"},
		{"home.vishnubalachandran.com", "/about", "https://www.vishnubalachandran.com/about"},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://"+tc.host+tc.path, nil)
		req.Host = tc.host
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusMovedPermanently {
			t.Errorf("%s%s status = %d, want 301", tc.host, tc.path, rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != tc.wantLoc {
			t.Errorf("%s%s Location = %q, want %q", tc.host, tc.path, loc, tc.wantLoc)
		}
	}
}

func TestCanonicalHostPreservesQuery(t *testing.T) {
	h := CanonicalHost("https://www.vishnubalachandran.com")(okHandler())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://vishnubalachandran.com/search?q=go", nil)
	req.Host = "vishnubalachandran.com"
	h.ServeHTTP(rr, req)
	want := "https://www.vishnubalachandran.com/search?q=go"
	if loc := rr.Header().Get("Location"); loc != want {
		t.Fatalf("Location = %q, want %q", loc, want)
	}
}

func TestCanonicalHostSkipsPreviewAndEmpty(t *testing.T) {
	pass := CanonicalHost("")(okHandler())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)
	req.Host = "localhost:8080"
	pass.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty SITE_URL status = %d, want 200", rr.Code)
	}

	h := CanonicalHost("https://www.vishnubalachandran.com")(okHandler())
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://portfolio-abc.vercel.app/", nil)
	req.Host = "portfolio-abc.vercel.app"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("preview host status = %d, want 200", rr.Code)
	}
}

func TestCanonicalHostDoesNotFlipWWWWhenSiteURLIsApex(t *testing.T) {
	h := CanonicalHost("https://vishnubalachandran.com")(okHandler())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://www.vishnubalachandran.com/", nil)
	req.Host = "www.vishnubalachandran.com"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("www status = %d when SITE_URL is apex; must not steal www", rr.Code)
	}
}
