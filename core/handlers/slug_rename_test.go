package handlers

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/storage"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

// testDB opens DATABASE_URL and skips when there is no database with
// migration 086 applied, so the suite stays green on a bare checkout.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`SELECT 1 FROM slug_redirects LIMIT 1`); err != nil {
		t.Skip("migration 086 not applied: ", err)
	}
	return db
}

func TestRedirectRenamed(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	a := &App{SlugRedirects: repo.NewSlugRedirectRepo(db)}
	const from, to = "/courses/w5test-redirect-old", "/courses/w5test-redirect-new"
	if _, err := db.ExecContext(ctx, `INSERT INTO slug_redirects (old_path,new_path) VALUES ($1,$2) ON CONFLICT (old_path) DO UPDATE SET new_path=EXCLUDED.new_path`, from, to); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM slug_redirects WHERE old_path = $1`, from) })

	rec := httptest.NewRecorder()
	if !a.redirectRenamed(rec, httptest.NewRequest("GET", from+"?x=1", nil)) {
		t.Fatal("expected a redirect")
	}
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != to+"?x=1" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = httptest.NewRecorder()
	if a.redirectRenamed(rec, httptest.NewRequest("POST", from, nil)) {
		t.Fatal("POST must not be redirected")
	}
	rec = httptest.NewRecorder()
	if a.redirectRenamed(rec, httptest.NewRequest("GET", "/courses/w5test-unknown", nil)) {
		t.Fatal("unknown path must fall through to the 404")
	}
}

func TestRenameCourse(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	dir := t.TempDir()
	a := &App{
		Courses:       repo.NewCourseRepo(db),
		SlugRedirects: repo.NewSlugRedirectRepo(db),
		CourseStorage: storage.New(&config.Config{LocalContentDir: dir}),
	}
	const oldSlug, newSlug = "w5test-rename-old", "w5test-rename-new"
	cleanup := func() {
		db.Exec(`DELETE FROM courses WHERE slug IN ($1,$2)`, oldSlug, newSlug)
		db.Exec(`DELETE FROM slug_redirects WHERE old_path LIKE '/courses/w5test-rename-%'`)
	}
	cleanup()
	t.Cleanup(cleanup)

	id, err := a.Courses.Create(ctx, models.Course{Slug: oldSlug, Title: "W5 rename test", Level: "Beginner", Tags: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	oldObj, _ := storage.CourseObject(oldSlug)
	if err := a.CourseStorage.Put(ctx, oldObj, "text/html", []byte("<body><h2>One</h2>lesson</body>")); err != nil {
		t.Fatal(err)
	}
	if err := a.Courses.UpdateContentObject(ctx, id, oldObj); err != nil {
		t.Fatal(err)
	}
	if err := a.Courses.UpdateOutline(ctx, id, "One", true); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ { // second pass proves idempotence
		prev, err := a.Courses.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		next := prev
		next.Slug = newSlug
		if i == 1 {
			prev.Slug, prev.ContentObject = oldSlug, oldObj // replay the same request
		}
		report, err := a.renameCourse(ctx, prev, next)
		if err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
		if len(report) < 3 {
			t.Fatalf("report too short: %v", report)
		}
	}

	got, err := a.Courses.GetByID(ctx, id)
	if err != nil || got.Slug != newSlug || got.Outline != "" || got.PreviewReady {
		t.Fatalf("course after rename: %+v err=%v", got, err)
	}
	newObj, _ := storage.CourseObject(newSlug)
	if got.ContentObject != newObj {
		t.Fatalf("content_object = %q", got.ContentObject)
	}
	for _, name := range []string{oldObj, newObj} {
		rc, err := a.CourseStorage.Open(ctx, name)
		if err != nil {
			t.Fatalf("object %s missing: %v", name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if !strings.Contains(string(b), "lesson") {
			t.Fatalf("object %s wrong bytes", name)
		}
	}
	for _, suf := range []string{"", "/preview", "/learn", "/quiz", "/certificate"} {
		to, err := a.SlugRedirects.Lookup(ctx, "/courses/"+oldSlug+suf)
		if err != nil || to != "/courses/"+newSlug+suf {
			t.Fatalf("redirect %q -> %q err=%v", suf, to, err)
		}
	}

	// Renaming back must drop the now-circular rows and re-point chains.
	back := got
	back.Slug = oldSlug
	if _, err := a.renameCourse(ctx, got, back); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SlugRedirects.Lookup(ctx, "/courses/"+oldSlug); err != repo.ErrNotFound {
		t.Fatalf("old slug is live again, its redirect must be gone: %v", err)
	}
	if to, err := a.SlugRedirects.Lookup(ctx, "/courses/"+newSlug); err != nil || to != "/courses/"+oldSlug {
		t.Fatalf("reverse redirect: %q %v", to, err)
	}
}

func TestCoursePreviewTemplate(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.Render(rec, 200, "course_preview", CoursePreviewData{
		BaseData:  BaseData{SiteName: "S", Title: "Free preview", CanonicalURL: "https://x.test/courses/c"},
		Course:    models.Course{Slug: "c", Title: "C", Outline: "A\nB"},
		LessonURL: "/courses/c/preview/lesson",
	})
	body := rec.Body.String()
	if strings.Contains(body, "render error") || !strings.Contains(body, `src="/courses/c/preview/lesson"`) ||
		!strings.Contains(body, `href="https://x.test/courses/c"`) || !strings.Contains(body, "/courses/c#buy-card") {
		t.Fatalf("preview page wrong:\n%s", body)
	}
}
