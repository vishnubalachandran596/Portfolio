package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestAdminPreviewRequiresLogin(t *testing.T) {
	renderer, err := view.New()
	if err != nil {
		t.Fatalf("view.New: %v", err)
	}
	app := New(&config.Config{JWTSecret: []byte("test-secret-at-least-32-bytes-long")}, nil, renderer)
	h := app.Routes()
	for _, path := range []string{
		"/admin/resumes/default",
		"/admin/resumes/zafin-lead-product-engineer-i",
		"/admin/articles/4/preview",
		"/admin/courses/3/preview",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s: status %d", path, rec.Code)
		}
		if !strings.HasPrefix(rec.Header().Get("Location"), "/admin/login") {
			t.Fatalf("%s: location %q", path, rec.Header().Get("Location"))
		}
	}
}
