package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/storage"
	"github.com/vishnubalachandran596/portfolio/web"
)

type CourseLearnData struct {
	BaseData
	Course    models.Course
	HasQuiz   bool
	LessonURL string // gated route the sandboxed iframe loads
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

// CourseLearn serves a paid course's gated reader to a verified
// purchaser (or a logged-in admin previewing it). The page is the site
// chrome; the lesson itself loads into a 75vw × 75dvh iframe from the
// separate gated route CourseLessonDocument. That iframe is sandboxed
// (allow-scripts, no allow-same-origin), so it has a unique origin and
// the parent cannot read the lesson DOM. The browser never receives a
// Storage URL, and the lesson tags are not in the outer page source.
//
// On protection: nothing served to a browser can be made 100% impossible
// to copy — screenshots and the network panel still exist. What this
// does is keep the content off any public URL (the lesson route needs the
// buyer's session and answers only to an iframe load), leave the lesson
// tags out of the outer page source and out of the parent DOM, send
// no-store caching, discourage right-click, stamp copied text with the
// viewer's identity, and mark only the renderer pane.
func (a *App) CourseLearn(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	isAdmin := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)
	if err != nil || !course.IsPaid || (!course.IsPublished && !isAdmin) {
		a.NotFound(w, r)
		return
	}

	ok, _, _ := a.courseAccess(w, r, course)
	if !ok {
		http.Redirect(w, r, "/courses/"+slug+"?purchase=required", http.StatusSeeOther)
		return
	}

	a.writeCourseReader(w, r, course, "/courses/"+url.PathEscape(slug)+"/learn/lesson")
}

// lessonCSP is the policy of the lesson document itself, set on the lesson
// route in place of the site-wide one. The document is its own sandbox
// (`sandbox allow-scripts`, so a unique origin: no cookies, no storage, no
// access to the parent) and cannot make any network request, open a frame
// or submit a form. Inside that box the interactive labs in the lessons
// compile the learner's own code with new Function, which needs
// 'unsafe-eval'. It is granted here and nowhere else: the site-wide CSP
// still has no 'unsafe-eval'.
const lessonCSP = "sandbox allow-scripts; " +
	"default-src 'none'; " +
	"script-src 'unsafe-inline' 'unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"font-src 'self' data:; " +
	"img-src 'self' data: https:; " +
	"media-src 'self' data: https:; " +
	"connect-src 'none'; frame-src 'none'; object-src 'none'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

// CourseLessonDocument serves the prepared lesson document for the reader's
// iframe. It applies the same access rules as CourseLearn.
func (a *App) CourseLessonDocument(w http.ResponseWriter, r *http.Request) {
	course, err := a.Courses.GetBySlug(r.Context(), r.PathValue("slug"))
	isAdmin := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)
	if err != nil || !course.IsPaid || (!course.IsPublished && !isAdmin) {
		a.NotFound(w, r)
		return
	}
	ok, watermark, _ := a.courseAccess(w, r, course)
	if !ok {
		http.Error(w, "purchase required", http.StatusForbidden)
		return
	}
	a.writeLessonDocument(w, r, course, watermark)
}

// writeCourseReader renders the reader page around the lesson iframe.
func (a *App) writeCourseReader(w http.ResponseWriter, r *http.Request, course models.Course, lessonURL string) {
	if a.CourseStorage == nil || course.ContentObject == "" {
		a.serverError(w, "load course content", fmt.Errorf("no stored lesson for %q", course.Slug))
		return
	}
	quizCount, err := a.QuizQuestions.CountByCourse(r.Context(), course.ID)
	if err != nil {
		a.serverError(w, "load course content", err)
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
		BaseData:    base,
		Course:      course,
		HasQuiz:     quizCount > 0,
		LessonURL:   lessonURL,
		Compilers:   compilers,
		DefaultLang: defaultLang,
	})
}

// writeLessonDocument reads the stored lesson, prepares it (site CSS,
// runtime, watermark) and writes it as the iframe document under lessonCSP.
// An iframe load is answered; opening the URL as a page, or from
// view-source (the browser says Sec-Fetch-Dest: document), gets a 403. A
// browser too old to send Sec-Fetch-Dest is let through rather than locked
// out of a course it paid for.
func (a *App) writeLessonDocument(w http.ResponseWriter, r *http.Request, course models.Course, watermark string) {
	if a.CourseStorage == nil || course.ContentObject == "" {
		a.serverError(w, "load course content", fmt.Errorf("no stored lesson for %q", course.Slug))
		return
	}
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" && dest != "iframe" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	body, err := a.CourseStorage.Open(r.Context(), course.ContentObject)
	if err != nil {
		a.serverError(w, "load course content", err)
		return
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, maxCourseUploadBytes+1))
	if err != nil {
		a.serverError(w, "load course content", err)
		return
	}
	if len(raw) > maxCourseUploadBytes {
		a.serverError(w, "load course content", fmt.Errorf("lesson %q exceeds %d bytes", course.Slug, maxCourseUploadBytes))
		return
	}
	prepared, err := storage.PrepareLesson(raw, watermark, "/static/css/style.css?v="+web.AssetVersion)
	if err != nil {
		a.serverError(w, "prepare course content", err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store, private")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Content-Security-Policy", lessonCSP)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, prepared.Document)
}
