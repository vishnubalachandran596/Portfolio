package handlers

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/storage"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestPreviewFromLesson(t *testing.T) {
	two := []byte(`<html><body><h2>Intro</h2><p>a</p><h2>Deep dive</h2><p>b</p></body></html>`)
	p := previewFromLesson(two)
	if !p.Ready || p.Status() != PreviewReady {
		t.Fatalf("two sections should be previewable: %+v", p)
	}
	if len(p.Outline) != 2 || len(p.PreviewSections) != 1 || p.PreviewSections[0] != "Intro" {
		t.Fatalf("outline %v preview %v", p.Outline, p.PreviewSections)
	}

	one := []byte(`<html><body><h2>Only</h2><p>a</p></body></html>`)
	p = previewFromLesson(one)
	if p.Ready || p.Status() != PreviewNone || len(p.Outline) != 1 || len(p.PreviewSections) != 0 {
		t.Fatalf("one section must not be previewable: %+v", p)
	}

	marker := []byte(`<h2>A</h2><p>x</p><h2>B</h2><p>y</p>` + storage.PreviewEndMarker + `<h2>C</h2>`)
	p = previewFromLesson(marker)
	if !p.Ready || len(p.PreviewSections) != 2 {
		t.Fatalf("marker preview: %+v", p)
	}
}

func TestNeedsPreviewBuild(t *testing.T) {
	cases := []struct {
		c    models.Course
		want bool
	}{
		{models.Course{ContentObject: "courses/x.html"}, true},
		{models.Course{ContentObject: "courses/x.html", Outline: "A\nB"}, true},
		{models.Course{ContentObject: "courses/x.html", Outline: "A", PreviewReady: true}, false},
		{models.Course{Outline: "", PreviewReady: false}, false},
	}
	for i, tc := range cases {
		if got := needsPreviewBuild(tc.c); got != tc.want {
			t.Errorf("case %d: got %v want %v", i, got, tc.want)
		}
	}
}

func TestPreviewTemplatesRender(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.Render(rec, 200, "admin_courses_list", AdminCoursesListData{
		BaseData: BaseData{SiteName: "Site", Title: "Courses — Admin"},
		Courses: []models.Course{
			{ID: 3, Title: "PyTorch", ContentObject: "courses/p.html", PreviewReady: true},
			{ID: 4, Title: "Draft"},
		},
		PreviewResults: []PreviewRebuildResult{
			{CourseID: 3, Title: "PyTorch", Status: PreviewReady, Sections: []string{"Tensors"}},
			{CourseID: 5, Title: "Gone", Status: PreviewMissingLesson},
		},
	})
	body := rec.Body.String()
	for _, want := range []string{`action="/admin/courses/rebuild-previews"`, `action="/admin/courses/3/rebuild-preview"`, "PyTorch: ready (Tensors)", "Gone: missing lesson"} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %q", want)
		}
	}
	if strings.Contains(body, "<!-- render error:") || strings.Contains(body, `/admin/courses/4/rebuild-preview`) {
		t.Fatal("list render problem")
	}

	for status, want := range map[string]string{PreviewReady: "Preview ready", PreviewNone: "No previewable section", PreviewMissingLesson: "Missing lesson"} {
		rec = httptest.NewRecorder()
		r.Render(rec, 200, "admin_course_form", AdminCourseFormData{
			BaseData:      BaseData{SiteName: "Site", Title: "Edit"},
			Course:        models.Course{ID: 3, Title: "T", ContentObject: "courses/t.html"},
			Preview:       CoursePreview{Ready: status == PreviewReady, PreviewSections: []string{"Intro section"}},
			PreviewStatus: status,
		})
		b := rec.Body.String()
		if strings.Contains(b, "<!-- render error:") || !strings.Contains(b, want) {
			t.Errorf("form status %q missing %q", status, want)
		}
		if status == PreviewReady && !strings.Contains(b, "Intro section") {
			t.Error("form does not list preview sections")
		}
	}
}

func previewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testDB(t)
	if _, err := db.Exec(`SELECT preview_checked_at FROM courses LIMIT 1`); err != nil {
		t.Skip("migration 090 not applied: ", err)
	}
	return db
}

// TestBackfillCoursePreview covers the lazy path end to end against a
// local content directory: a course with an outline but preview_ready =
// false is repaired, and a lesson with nothing to preview is not re-read
// until the retry window passes.
func TestBackfillCoursePreview(t *testing.T) {
	db := previewTestDB(t)
	ctx := context.Background()
	st := storage.New(&config.Config{LocalContentDir: t.TempDir()})
	if st == nil {
		t.Fatal("no local storage")
	}
	a := &App{Courses: repo.NewCourseRepo(db), CourseStorage: st}

	mk := func(slug, lesson string) models.Course {
		obj := "courses/" + slug + ".html"
		if err := st.Put(ctx, obj, "text/html", []byte(lesson)); err != nil {
			t.Fatal(err)
		}
		var id int64
		if err := db.QueryRowContext(ctx, `INSERT INTO courses (slug, title, summary, description, level, duration, content_object, outline, preview_ready)
			VALUES ($1,$1,'s','d','Beginner','1h',$2,'Intro',FALSE) RETURNING id`, slug, obj).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Exec(`DELETE FROM courses WHERE id = $1`, id) })
		c, err := a.Courses.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	good := mk("w6test-preview-good", `<h2>One</h2><p>a</p><h2>Two</h2><p>b</p>`)
	if !needsPreviewBuild(good) {
		t.Fatal("outline without preview_ready must need a build")
	}
	outline, ready, ok := a.backfillCoursePreview(ctx, good)
	if !ok || !ready || outline != "One\nTwo" {
		t.Fatalf("backfill: %q %v %v", outline, ready, ok)
	}
	got, _ := a.Courses.GetByID(ctx, good.ID)
	if !got.PreviewReady || got.Outline != "One\nTwo" {
		t.Fatalf("not persisted: %+v", got)
	}
	// Claimed just now: a second view inside the window does nothing.
	if _, _, ok := a.backfillCoursePreview(ctx, good); ok {
		t.Fatal("second attempt inside the retry window must be skipped")
	}

	// A one-section lesson stays not-ready and is throttled.
	one := mk("w6test-preview-one", `<h2>Only</h2><p>a</p>`)
	if _, ready, ok := a.backfillCoursePreview(ctx, one); !ok || ready {
		t.Fatalf("one-section lesson: ready=%v ok=%v", ready, ok)
	}
	if claimed, err := a.Courses.ClaimPreviewBuild(ctx, one.ID, previewRetryAfter); err != nil || claimed {
		t.Fatalf("throttle: claimed=%v err=%v", claimed, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE courses SET preview_checked_at = now() - interval '7 hours' WHERE id = $1`, one.ID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := a.Courses.ClaimPreviewBuild(ctx, one.ID, 6*time.Hour); err != nil || !claimed {
		t.Fatalf("claim after window: claimed=%v err=%v", claimed, err)
	}

	// Rebuild report: ready / no previewable section / missing lesson.
	missing := models.Course{ID: good.ID, Title: "Missing", ContentObject: "courses/nope.html"}
	res := a.rebuildPreviews(ctx, []models.Course{good, one, missing})
	want := []string{PreviewReady, PreviewNone, PreviewMissingLesson}
	for i, w := range want {
		if res[i].Status != w {
			t.Errorf("result %d: %q want %q", i, res[i].Status, w)
		}
	}
}
