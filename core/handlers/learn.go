package handlers

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/content"
)

// CourseLearn serves a paid course's gated content to a verified
// purchaser (or a logged-in admin previewing it). The content is a full,
// self-contained interactive HTML document — it has its own <html>/<head>,
// CSS, and JS (including an in-browser code runner for exercises), so it's
// written directly to the response rather than passed through the site's
// own html/template layout, which would nest two <html> documents.
//
// On protection: nothing served to a browser can be made 100% impossible
// to copy — view-source, screenshots, and browser devtools always exist.
// What this does is keep the content off any public/guessable URL (it's
// never in /static or the sitemap, and this handler requires a verified
// purchase or admin session), send it with no-store caching, add a basic
// right-click/text-selection deterrent, and — the actually-effective
// measure — visibly watermark every page with the viewer's identity, so a
// leaked copy is traceable back to whoever shared it.
func (a *App) CourseLearn(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	if err != nil || !course.IsPaid || !course.IsPublished {
		a.NotFound(w, r)
		return
	}

	ok, watermark := a.courseAccess(w, r, course)
	if !ok {
		http.Redirect(w, r, "/courses/"+slug+"?purchase=required", http.StatusSeeOther)
		return
	}

	raw, found := content.Get(course.ContentKey)
	if !found {
		a.serverError(w, "load course content", fmt.Errorf("no content registered for key %q", course.ContentKey))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("X-Frame-Options", "DENY")

	injection := watermarkScript(watermark)
	if idx := strings.LastIndex(raw, "</body>"); idx != -1 {
		w.Write([]byte(raw[:idx]))
		w.Write([]byte(injection))
		w.Write([]byte(raw[idx:]))
	} else {
		w.Write([]byte(raw))
		w.Write([]byte(injection))
	}
}

func watermarkScript(identity string) string {
	safe := html.EscapeString(identity)
	return `
<div id="__access_watermark" style="position:fixed;bottom:10px;right:14px;z-index:99999;font:11px -apple-system,sans-serif;color:rgba(255,255,255,.28);pointer-events:none;user-select:none;">Licensed to ` + safe + `</div>
<style>#__access_watermark{mix-blend-mode:difference}</style>
<script>
document.addEventListener('contextmenu', function (e) { e.preventDefault(); });
document.addEventListener('copy', function (e) {
  try {
    var note = "\n\n— from a purchased copy, licensed to ` + safe + ` —";
    e.clipboardData.setData('text/plain', (window.getSelection().toString() || '') + note);
    e.preventDefault();
  } catch (err) {}
});
</script>`
}
