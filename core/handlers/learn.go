package handlers

import (
	"fmt"
	"html/template"
	"io"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/storage"
)

type CourseLearnData struct {
	BaseData
	Course        models.Course
	HasQuiz       bool
	LessonStyles  template.CSS
	LessonPayload string
}

// CourseLearn serves a paid course's gated content to a verified
// purchaser (or a logged-in admin previewing it). The lesson is a
// self-contained HTML document with its own CSS and JS (including an
// in-browser code runner). Those styles are scoped to .lesson-embed so
// they cannot restyle the site header or footer. The page itself is the
// site chrome; the lesson markup is a base64 payload the page decodes
// into #lesson-root. View-source therefore shows the shell, not the
// course. The decoded fragment carries the viewer's identity along one
// light diagonal. The browser never receives a Storage URL.
//
// On protection: nothing served to a browser can be made 100% impossible
// to copy — screenshots and devtools still exist, and someone who decodes
// the payload can recover the lesson. What this does is keep the content
// off any public URL, leave the lesson tags out of view-source, send
// no-store caching, discourage right-click, stamp copied text with the
// viewer's identity, and draw that identity once across the viewport so
// a leak is still traceable.
func (a *App) CourseLearn(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	isAdmin := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)
	if err != nil || !course.IsPaid || (!course.IsPublished && !isAdmin) {
		a.NotFound(w, r)
		return
	}

	ok, watermark := a.courseAccess(w, r, course)
	if !ok {
		http.Redirect(w, r, "/courses/"+slug+"?purchase=required", http.StatusSeeOther)
		return
	}

	a.writeCourseLesson(w, r, course, watermark)
}

func (a *App) writeCourseLesson(w http.ResponseWriter, r *http.Request, course models.Course, watermark string) {
	if a.CourseStorage == nil || course.ContentObject == "" {
		a.serverError(w, "load course content", fmt.Errorf("no stored lesson for %q", course.Slug))
		return
	}

	var (
		raw       []byte
		quizCount int
	)
	g, ctx := errgroup.WithContext(r.Context())
	g.Go(func() error {
		body, err := a.CourseStorage.Open(ctx, course.ContentObject)
		if err != nil {
			return err
		}
		defer body.Close()
		raw, err = io.ReadAll(io.LimitReader(body, maxCourseUploadBytes+1))
		if err != nil {
			return err
		}
		if len(raw) > maxCourseUploadBytes {
			return fmt.Errorf("lesson %q exceeds %d bytes", course.Slug, maxCourseUploadBytes)
		}
		return nil
	})
	g.Go(func() (err error) {
		quizCount, err = a.QuizQuestions.CountByCourse(ctx, course.ID)
		return
	})
	if err := g.Wait(); err != nil {
		a.serverError(w, "load course content", err)
		return
	}

	prepared, err := storage.PrepareLesson(raw, watermark)
	if err != nil {
		a.serverError(w, "prepare course content", err)
		return
	}

	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	a.View.Render(w, http.StatusOK, "course_learn", CourseLearnData{
		BaseData:      a.base(r, course.Title+" — Lesson", course.Summary, "courses"),
		Course:        course,
		HasQuiz:       quizCount > 0,
		LessonStyles:  template.CSS(prepared.Styles),
		LessonPayload: prepared.Payload,
	})
}
