package handlers

import (
	"fmt"
	"io"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/storage"
	"github.com/vishnubalachandran596/portfolio/web"
)

type CourseLearnData struct {
	BaseData
	Course        models.Course
	HasQuiz       bool
	LessonPayload string
	// Compilers are the JDoodle embeds configured in JDOODLE_EMBED_KEYS,
	// in a stable order; DefaultLang is the course's own language when an
	// embed exists for it. Empty Compilers hides the drawer entirely.
	Compilers   []CompilerOption
	DefaultLang string
}

// CompilerOption is one language the reader's compiler drawer can open.
type CompilerOption struct {
	Lang  string
	Label string
	Key   string
}

var compilerLabels = map[string]string{"java": "Java", "python": "Python", "go": "Go", "javascript": "JavaScript", "sql": "SQL", "kotlin": "Kotlin", "cpp": "C++", "bash": "Bash"}

func compilerOptions(keys map[string]string) []CompilerOption {
	var out []CompilerOption
	order := []string{"java", "python", "go", "javascript", "bash", "kotlin", "sql", "cpp"}
	seen := map[string]bool{}
	for _, lang := range order {
		if k, ok := keys[lang]; ok {
			out = append(out, CompilerOption{Lang: lang, Label: compilerLabels[lang], Key: k})
			seen[lang] = true
		}
	}
	for lang, k := range keys {
		if !seen[lang] {
			out = append(out, CompilerOption{Lang: lang, Label: lang, Key: k})
		}
	}
	return out
}

// CourseLearn serves a paid course's gated content to a verified
// purchaser (or a logged-in admin previewing it). The lesson HTML is
// accepted as-is (any tags), but uploaded <style> is stripped so the
// site typography and layout win. The page is the site chrome; the
// lesson loads in a 75vw × 75dvh pane as a unique-origin iframe
// (sandbox allow-scripts, no allow-same-origin) so the parent cannot
// read the lesson DOM. The iframe document is a base64 payload —
// view-source of the outer page does not contain the lesson tags.
// The watermark is drawn only inside that pane. The browser never
// receives a Storage URL.
//
// On protection: nothing served to a browser can be made 100% impossible
// to copy — screenshots and decoding the payload still exist. What this
// does is keep the content off any public URL, leave the lesson tags
// out of the outer page source and out of the parent DOM (unique-origin
// iframe), send no-store caching, discourage right-click, stamp copied
// text with the viewer's identity, and mark only the renderer pane.
func (a *App) CourseLearn(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	isAdmin := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)
	if err != nil || !course.IsPaid || (!course.IsPublished && !isAdmin) {
		a.NotFound(w, r)
		return
	}

	ok, watermark, _ := a.courseAccess(w, r, course)
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

	prepared, err := storage.PrepareLesson(raw, watermark, "/static/css/style.css?v="+web.AssetVersion)
	if err != nil {
		a.serverError(w, "prepare course content", err)
		return
	}

	compilers := compilerOptions(a.Cfg.JDoodleEmbedKeys)
	defaultLang := ""
	if len(compilers) > 0 {
		defaultLang = compilers[0].Lang
		want := course.CodeLanguage()
		for _, c := range compilers {
			if c.Lang == want {
				defaultLang = want
			}
		}
	}

	base := a.base(r, course.Title+" — Lesson", course.Summary, "courses")
	base.Immersive = true
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	a.View.Render(w, http.StatusOK, "course_learn", CourseLearnData{
		BaseData:      base,
		Course:        course,
		HasQuiz:       quizCount > 0,
		LessonPayload: prepared.Payload,
		Compilers:     compilers,
		DefaultLang:   defaultLang,
	})
}
