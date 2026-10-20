package handlers

import (
	"context"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/storage"
)

// The preview builder: the one place that turns a stored lesson into the
// two derived columns on courses, outline and preview_ready. It composes
// storage.LessonOutline and storage.LessonPreview; the rules for what a
// preview is live there and nowhere else. Callers: the course page's lazy
// backfill, the admin "Rebuild previews" action, and (through
// previewFromLesson) the lesson upload paths.

// previewRetryAfter is how long a course whose preview could not be built
// is left alone before a page view tries again (migration 090).
const previewRetryAfter = 6 * time.Hour

// Preview build outcomes, shown verbatim in the admin results.
const (
	PreviewReady         = "ready"
	PreviewNone          = "no previewable section"
	PreviewMissingLesson = "missing lesson"
)

// CoursePreview is what a lesson yields: the full outline, whether it has
// a free preview, and the section titles that preview will show.
type CoursePreview struct {
	Outline         []string
	Ready           bool
	PreviewSections []string
}

// Status is the admin-facing outcome for a lesson that was read.
func (p CoursePreview) Status() string {
	if p.Ready {
		return PreviewReady
	}
	return PreviewNone
}

// previewFromLesson derives the outline and preview from lesson bytes.
// Pure: no I/O, so uploads that already hold the bytes use it too.
func previewFromLesson(raw []byte) CoursePreview {
	p := CoursePreview{Outline: storage.LessonOutline(raw)}
	if cut, ok := storage.LessonPreview(raw); ok {
		p.Ready = true
		p.PreviewSections = storage.LessonOutline(cut)
	}
	return p
}

// readCourseLesson reads a course's stored lesson. ok is false when the
// course has no lesson file or Storage cannot return it.
func (a *App) readCourseLesson(ctx context.Context, course models.Course) ([]byte, bool) {
	if a.CourseStorage == nil || course.ContentObject == "" {
		return nil, false
	}
	body, err := a.CourseStorage.Open(ctx, course.ContentObject)
	if err != nil {
		log.Printf("read lesson %s: %v", course.ContentObject, err)
		return nil, false
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, maxCourseUploadBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxCourseUploadBytes {
		return nil, false
	}
	return raw, true
}

// buildCoursePreview reads the course's stored lesson, computes outline
// and preview_ready and persists them. found is false (and nothing is
// written) when there is no readable lesson.
func (a *App) buildCoursePreview(ctx context.Context, course models.Course) (p CoursePreview, found bool, err error) {
	raw, ok := a.readCourseLesson(ctx, course)
	if !ok {
		return CoursePreview{}, false, nil
	}
	p = previewFromLesson(raw)
	err = a.Courses.UpdateOutline(ctx, course.ID, strings.Join(p.Outline, "\n"), p.Ready)
	return p, true, err
}

// backfillCoursePreview is the course page's lazy path: when the outline
// is empty or the preview is not ready, rebuild once per previewRetryAfter
// (ClaimPreviewBuild). ok reports that outline/ready are fresh values to
// show on this view. Best-effort: failures cost one view, never the page.
func (a *App) backfillCoursePreview(ctx context.Context, course models.Course) (outline string, ready bool, ok bool) {
	claimed, err := a.Courses.ClaimPreviewBuild(ctx, course.ID, previewRetryAfter)
	if err != nil || !claimed {
		if err != nil {
			log.Printf("preview claim %d: %v", course.ID, err)
		}
		return "", false, false
	}
	p, found, err := a.buildCoursePreview(ctx, course)
	if err != nil {
		log.Printf("preview backfill save %d: %v", course.ID, err)
	}
	if !found {
		return "", false, false
	}
	return strings.Join(p.Outline, "\n"), p.Ready, true
}

// needsPreviewBuild is the page-view condition for the lazy backfill.
func needsPreviewBuild(c models.Course) bool {
	return c.ContentObject != "" && (c.Outline == "" || !c.PreviewReady)
}

// PreviewRebuildResult is one row of the admin "Rebuild previews" report.
type PreviewRebuildResult struct {
	CourseID int64
	Title    string
	Status   string // PreviewReady, PreviewNone, PreviewMissingLesson or "save failed"
	Sections []string
}

// OK is true for a course that now has a free preview.
func (r PreviewRebuildResult) OK() bool { return r.Status == PreviewReady }

// rebuildPreviews runs the builder over the given courses (4 at a time, so
// the whole catalogue fits one request) and returns one result per course,
// in the input order.
func (a *App) rebuildPreviews(ctx context.Context, courses []models.Course) []PreviewRebuildResult {
	out := make([]PreviewRebuildResult, len(courses))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for i, c := range courses {
		g.Go(func() error {
			res := PreviewRebuildResult{CourseID: c.ID, Title: c.Title}
			p, found, err := a.buildCoursePreview(gctx, c)
			switch {
			case !found:
				res.Status = PreviewMissingLesson
			case err != nil:
				log.Printf("rebuild preview %d: %v", c.ID, err)
				res.Status = "save failed"
			default:
				res.Status, res.Sections = p.Status(), p.PreviewSections
			}
			out[i] = res
			return nil
		})
	}
	_ = g.Wait()
	return out
}

// AdminCoursesRebuildPreviews handles POST /admin/courses/rebuild-previews
// (every course) and POST /admin/courses/{id}/rebuild-preview (one). Both
// sit behind the admin guard and middleware.SameOrigin like every POST.
func (a *App) AdminCoursesRebuildPreviews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var courses []models.Course
	if raw := r.PathValue("id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			a.NotFound(w, r)
			return
		}
		c, err := a.Courses.GetByID(ctx, id)
		if err != nil {
			if err == repo.ErrNotFound {
				a.NotFound(w, r)
				return
			}
			a.serverError(w, "load course", err)
			return
		}
		courses = []models.Course{c}
	} else {
		var err error
		if courses, err = a.Courses.ListAll(ctx); err != nil {
			a.serverError(w, "load courses", err)
			return
		}
	}
	results := a.rebuildPreviews(ctx, courses)
	all, err := a.Courses.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load courses", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_courses_list", AdminCoursesListData{
		BaseData:       a.base(r, "Courses — Admin", "", "admin"),
		Courses:        all,
		PreviewResults: results,
	})
}
