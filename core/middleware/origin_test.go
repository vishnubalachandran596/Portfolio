package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	h := SameOrigin("www.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cases := []struct {
		method, origin, site string
		want                 int
	}{
		{"GET", "https://evil.com", "", 200},
		{"POST", "", "", 200},
		{"POST", "https://www.example.com", "", 200},
		{"POST", "https://evil.com", "", 403},
		{"POST", "null", "", 403},
		{"POST", "", "cross-site", 403},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, "https://www.example.com/x", nil)
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		if c.site != "" {
			req.Header.Set("Sec-Fetch-Site", c.site)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s origin=%q site=%q: got %d want %d", c.method, c.origin, c.site, rec.Code, c.want)
		}
	}
}
