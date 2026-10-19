package handlers

import (
	"net/http"
	"net/url"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/storage"
)

// CoursePreviewData is the public free-trial page of a course.
type CoursePreviewData struct {
	BaseData
	Course    models.Course
	LessonURL string // public, iframe-only route serving the cut lesson
}

// publicPreviewCourse loads the course for the preview routes: it must be
// published (an admin may see unpublished), have stored content and a
// preview. ok is false when the response was already written.
func (a *App) publicPreviewCourse(w http.ResponseWriter, r *http.Request) (models.Course, bool) {
	course, err := a.Courses.GetBySlug(r.Context(), r.PathValue("slug"))
	isAdmin := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)
	if err != nil || course.ContentObject == "" || (!course.IsPublished && !isAdmin) {
		a.NotFound(w, r)
		return course, false
	}
	return course, true
}

// CoursePreview serves GET /courses/{slug}/preview: the public trial
// page. It shows the lesson up to its <!-- vb:preview-end --> marker (or
// the first section when there is none) in the same sandboxed iframe the
// gated reader uses, with an enrol call to action. The canonical URL is
// the sales page, so the preview supports it instead of competing with
// it. A course without a preview sends the visitor to the sales page.
func (a *App) CoursePreview(w http.ResponseWriter, r *http.Request) {
	course, ok := a.publicPreviewCourse(w, r)
	if !ok {
		return
	}
	if !course.PreviewReady {
		http.Redirect(w, r, "/courses/"+url.PathEscape(course.Slug), http.StatusSeeOther)
		return
	}
	base := a.base(r, "Free preview: "+course.Title+" — "+a.Cfg.SiteName, course.Summary, "courses")
	base.CanonicalURL = a.pageURL(r, "/courses/"+url.PathEscape(course.Slug))
	a.View.Render(w, http.StatusOK, "course_preview", CoursePreviewData{
		BaseData:  base,
		Course:    course,
		LessonURL: "/courses/" + url.PathEscape(course.Slug) + "/preview/lesson",
	})
}

// CoursePreviewLessonDocument serves the cut lesson document for the
// preview iframe. Anyone may load it, but only as an iframe, and only the
// preview part ever leaves the server.
func (a *App) CoursePreviewLessonDocument(w http.ResponseWriter, r *http.Request) {
	course, ok := a.publicPreviewCourse(w, r)
	if !ok {
		return
	}
	a.writeLessonDocumentCut(w, r, course, "Free preview", func(raw []byte) ([]byte, bool) {
		return storage.LessonPreview(raw)
	})
}
