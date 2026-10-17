package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

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
// It deliberately carries no embed key: keys never appear in page HTML.
// The drawer asks CourseCompilerEmbed for one when it opens.
type CompilerOption struct {
	Lang  string
	Label string
}

var compilerLabels = map[string]string{"java": "Java", "python": "Python", "go": "Go", "javascript": "JavaScript", "sql": "SQL", "kotlin": "Kotlin", "cpp": "C++", "bash": "Bash"}

func compilerOptions(keys map[string]string) []CompilerOption {
	var out []CompilerOption
	order := []string{"java", "python", "go", "javascript", "bash", "kotlin", "sql", "cpp"}
	seen := map[string]bool{}
	for _, lang := range order {
		if _, ok := keys[lang]; ok {
			out = append(out, CompilerOption{Lang: lang, Label: compilerLabels[lang]})
			seen[lang] = true
		}
	}
	for lang := range keys {
		if !seen[lang] {
			out = append(out, CompilerOption{Lang: lang, Label: lang})
		}
	}
	return out
}

// CourseCompilerEmbed hands the reader the JDoodle embed address for one
// language, only to someone who may read this course (a verified purchaser
// or a logged-in admin), only on a same-origin request, and never cached.
// This keeps embed keys out of page HTML and out of reach of anyone who
// hasn't bought the course. A buyer's own browser still loads the embed
// from jdoodle.com, so the address is visible to that buyer's developer
// tools — unavoidable for any third-party embed.
func (a *App) CourseCompilerEmbed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	// A custom header forces a CORS preflight, which this route never
	// answers, so another site's script can't call it with a buyer's cookies.
	if r.Header.Get("X-Requested-With") != "vb-compiler" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	course, err := a.Courses.GetBySlug(r.Context(), r.PathValue("slug"))
	isAdmin := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)
	if err != nil || !course.IsPaid || (!course.IsPublished && !isAdmin) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if ok, _, _ := a.courseAccess(w, r, course); !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "purchase required"})
		return
	}
	key, ok := a.Cfg.JDoodleEmbedKeys[r.URL.Query().Get("lang")]
	if !ok || key == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no compiler for that language"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"src": "https://www.jdoodle.com/embed/v1/" + url.PathEscape(key)})
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
