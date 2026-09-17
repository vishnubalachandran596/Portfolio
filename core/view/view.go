// Package view renders the site's html/template pages from the embedded
// web/templates directory.
package view

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

//go:embed all:templates
var templateFS embed.FS

var funcs = template.FuncMap{
	"currentYear": func() int { return time.Now().Year() },
	"formatDate":  func(t time.Time) string { return t.Format("Jan 2, 2006") },
	"formatTime":  func(t time.Time) string { return t.Format("3:04 PM") },
	"formatDateTime": func(t time.Time) string {
		return t.Format("Mon, Jan 2 2006 · 3:04 PM")
	},
	"splitLines": func(s string) []string {
		var out []string
		for _, p := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n\n") {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	},
}

type Renderer struct {
	pages map[string]*template.Template
}

// New parses every page template in web/templates against the shared base
// layout and partials, failing fast if any template is broken.
func New() (*Renderer, error) {
	base := []string{
		"templates/layout/base.html",
		"templates/layout/header.html",
		"templates/layout/footer.html",
		"templates/layout/adminnav.html",
	}

	pageFiles := []string{
		"templates/pages/home.html",
		"templates/pages/about.html",
		"templates/pages/courses_list.html",
		"templates/pages/course_detail.html",
		"templates/pages/booking.html",
		"templates/pages/not_found.html",
		"templates/pages/admin_login.html",
		"templates/pages/admin_dashboard.html",
		"templates/pages/admin_courses_list.html",
		"templates/pages/admin_course_form.html",
		"templates/pages/admin_bookings.html",
		"templates/pages/admin_availability.html",
		"templates/pages/admin_profile.html",
	}

	r := &Renderer{pages: map[string]*template.Template{}}
	for _, page := range pageFiles {
		files := append(append([]string{}, base...), page)
		t, err := template.New("base.html").Funcs(funcs).ParseFS(templateFS, files...)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", page, err)
		}
		r.pages[page] = t
	}
	return r, nil
}

// Render writes the named page (e.g. "home") with the given data.
func (r *Renderer) Render(w http.ResponseWriter, status int, page string, data any) {
	t, ok := r.pages["templates/pages/"+page+".html"]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "base.html", data); err != nil {
		// Headers are already sent at this point; log-and-continue is the
		// best we can do for a mid-render template error.
		fmt.Fprintf(w, "\n<!-- render error: %v -->", err)
	}
}
