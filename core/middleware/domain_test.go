package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDomainRouterWWWIsNeverRedirected(t *testing.T) {
	h := DomainRouter("vishnubalachandran.com")(okHandler())
	for _, path := range []string{"/", "/about", "/articles"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://www.vishnubalachandran.com"+path, nil)
		req.Host = "www.vishnubalachandran.com"
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("www%s status = %d, want 200", path, rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "" {
			t.Errorf("www%s Location = %q, want none", path, loc)
		}
	}
}

func TestDomainRouterApexAndHome301ToWWW(t *testing.T) {
	h := DomainRouter("vishnubalachandran.com")(okHandler())
	cases := []struct {
		host, path, wantLoc string
	}{
		{"vishnubalachandran.com", "/", "https://www.vishnubalachandran.com/"},
		{"vishnubalachandran.com", "/about", "https://www.vishnubalachandran.com/about"},
		{"home.vishnubalachandran.com", "/", "https://www.vishnubalachandran.com/"},
		{"home.vishnubalachandran.com", "/articles", "https://www.vishnubalachandran.com/articles"},
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

func TestDomainRouterPageGroupSubdomainStillServed(t *testing.T) {
	h := DomainRouter("vishnubalachandran.com")(okHandler())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://about.vishnubalachandran.com/about", nil)
	req.Host = "about.vishnubalachandran.com"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("about. /about status = %d, want 200", rr.Code)
	}
}

func TestDomainRouterWrongSubdomain301ToOwnerOrWWW(t *testing.T) {
	h := DomainRouter("vishnubalachandran.com")(okHandler())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://about.vishnubalachandran.com/", nil)
	req.Host = "about.vishnubalachandran.com"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("about. / status = %d, want 301", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "https://www.vishnubalachandran.com/" {
		t.Fatalf("about. / Location = %q, want www homepage", loc)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://about.vishnubalachandran.com/courses", nil)
	req.Host = "about.vishnubalachandran.com"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("about. /courses status = %d, want 301", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "https://courses.vishnubalachandran.com/courses" {
		t.Fatalf("about. /courses Location = %q", loc)
	}
}

func TestDomainRouterEmptyIsPassthrough(t *testing.T) {
	h := DomainRouter("")(okHandler())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/", nil)
	req.Host = "127.0.0.1:8080"
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty DOMAIN status = %d, want 200", rr.Code)
	}
}

func TestHostForHomeIsWWW(t *testing.T) {
	if got := hostFor("home", "example.com"); got != "www.example.com" {
		t.Fatalf("hostFor(home) = %q, want www.example.com", got)
	}
	if got := hostFor("about", "example.com"); got != "about.example.com" {
		t.Fatalf("hostFor(about) = %q, want about.example.com", got)
	}
}
