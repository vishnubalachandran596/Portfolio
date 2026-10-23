package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
)

func TestTokenHashStable(t *testing.T) {
	h := authutil.TokenHash("abc")
	if len(h) != 64 || h == "abc" || h != authutil.TokenHash("abc") || h == authutil.TokenHash("abd") {
		t.Fatal("TokenHash must be a stable 64-char digest")
	}
}

func TestRedirectAccessTokenIgnoresOtherRequests(t *testing.T) {
	a := &App{}
	called := 0
	h := a.redirectAccessToken(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called++ }))
	for _, c := range []struct{ method, target string }{
		{"GET", "/courses/x/learn"},
		{"POST", "/courses/x/learn?token=abc"},
		{"GET", "/articles/x?token=abc"},
	} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(c.method, c.target, nil))
	}
	if called != 3 {
		t.Fatalf("expected passthrough for 3 requests, got %d", called)
	}
}
