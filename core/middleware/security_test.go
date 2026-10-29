package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersNonceNotUnsafeInline(t *testing.T) {
	var seen string
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = Nonce(r) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	var script string
	for _, d := range strings.Split(csp, ";") {
		if d = strings.TrimSpace(d); strings.HasPrefix(d, "script-src ") {
			script = d
		}
	}
	if seen == "" || !strings.Contains(script, "'nonce-"+seen+"'") {
		t.Fatalf("nonce %q missing from script-src: %s", seen, script)
	}
	if strings.Contains(script, "unsafe-inline") || strings.Contains(script, "unsafe-eval") {
		t.Fatalf("script-src must not allow unsafe-inline/eval: %s", script)
	}
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest("GET", "/", nil))
	if rec2.Header().Get("Content-Security-Policy") == csp {
		t.Error("nonce must change per request")
	}
}
