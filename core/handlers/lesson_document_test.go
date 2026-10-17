package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/storage"
)

// The lesson document is the one place 'unsafe-eval' is allowed (labs run
// the learner's code with new Function). It has to stay sandboxed, closed to
// the network, and answer only to an iframe load.
func TestLessonDocumentIsSandboxedAndIframeOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "courses"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "courses", "demo.html"), []byte(`<html><body><h2>Hello</h2><p>Lab body</p></body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: []byte("test-secret-at-least-32-bytes-long"), LocalContentDir: dir}
	a := &App{Cfg: cfg, CourseStorage: storage.New(cfg)}
	course := models.Course{Slug: "demo", ContentObject: "courses/demo.html"}

	// A page navigation, view-source or fetch is refused. (A browser too old
	// to send the header at all is allowed, so it isn't locked out.)
	for _, dest := range []string{"document", "empty", "frame"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/courses/demo/learn/lesson", nil)
		req.Header.Set("Sec-Fetch-Dest", dest)
		a.writeLessonDocument(rec, req, course, "buyer@example.com")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("Sec-Fetch-Dest %q: status %d, want 403", dest, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "Lab body") {
			t.Fatalf("Sec-Fetch-Dest %q leaked the lesson", dest)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/courses/demo/learn/lesson", nil)
	req.Header.Set("Sec-Fetch-Dest", "iframe")
	a.writeLessonDocument(rec, req, course, "buyer@example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"sandbox allow-scripts", "default-src 'none'", "connect-src 'none'", "frame-src 'none'", "form-action 'none'", "'unsafe-eval'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("lesson CSP lacks %q: %s", want, csp)
		}
	}
	if strings.Contains(csp, "allow-same-origin") {
		t.Error("allow-same-origin would give the lesson the site's origin")
	}
	if !strings.Contains(rec.Body.String(), "Lab body") {
		t.Error("lesson body missing")
	}
	if rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Error("the reader must be able to frame its own lesson")
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Error("lesson must not be cached")
	}
}

// The site-wide policy must never gain 'unsafe-eval'. Only string literals
// count, so the comment explaining why it is absent doesn't trip the check.
func TestSiteCSPHasNoUnsafeEval(t *testing.T) {
	src, err := os.ReadFile("../middleware/security.go")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`"[^"\n]*unsafe-eval[^"\n]*"`).Match(src) {
		t.Fatal("core/middleware/security.go grants unsafe-eval; only the lesson document may (lessonCSP in learn.go)")
	}
}
