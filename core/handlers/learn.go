package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/vishnubalachandran596/portfolio/core/storage"
)

// CourseLearn serves a paid course's gated content to a verified
// purchaser (or a logged-in admin previewing it). The content is a full,
// self-contained interactive HTML document — it has its own <html>/<head>,
// CSS, and JS (including an in-browser code runner for exercises), so it's
// written directly to the response rather than passed through the site's
// own html/template layout, which would nest two <html> documents.
//
// The bytes come from a private Supabase Storage object. This handler
// opens that object only after courseAccess passes. The response is a
// shell: the lesson markup is not in the page source. The shell decodes
// it and writes the document, which carries the viewer's identity tiled
// across the whole viewport. The browser never receives a Storage URL.
//
// On protection: nothing served to a browser can be made 100% impossible
// to copy — screenshots and devtools still exist, and someone who decodes
// the payload can recover the lesson. What this does is keep the content
// off any public URL, leave the lesson tags out of view-source, send
// no-store caching, discourage right-click, stamp copied text with the
// viewer's identity throughout, and tile that identity over every
// screenful so a leak is traceable.
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

	if a.CourseStorage == nil || course.ContentObject == "" {
		a.serverError(w, "load course content", fmt.Errorf("no stored lesson for %q", slug))
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
		a.serverError(w, "read course content", err)
		return
	}
	if len(raw) > maxCourseUploadBytes {
		a.serverError(w, "read course content", fmt.Errorf("lesson %q exceeds %d bytes", slug, maxCourseUploadBytes))
		return
	}
	page, err := storage.PrepareLesson(raw, watermark)
	if err != nil {
		a.serverError(w, "prepare course content", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("X-Frame-Options", "DENY")
	if _, err := w.Write(page); err != nil {
		log.Printf("write course content %s: %v", slug, err)
	}
}
