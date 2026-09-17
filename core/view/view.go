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
	"formatDateOptional": func(t *time.Time) string {
		if t == nil {
			return "—"
		}
		return t.Format("Jan 2, 2006")
	},
	"formatTime": func(t time.Time) string { return t.Format("3:04 PM") },
	"formatDateTime": func(t time.Time) string {
		return t.Format("Mon, Jan 2 2006 · 3:04 PM")
	},
	"money": func(cents int, currency string) string {
		symbol := map[string]string{"EUR": "€", "GBP": "£", "INR": "₹"}[currency]
		if symbol == "" {
			symbol = "$"
		}
		return fmt.Sprintf("%s%d.%02d", symbol, cents/100, cents%100)
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

type renderEntry struct {
	tmpl *template.Template
	base string // the layout entrypoint to execute: "base.html" or "admin_base.html"
}

type Renderer struct {
	pages map[string]renderEntry
}

// New parses every page template in web/templates against its layout
// (the public site's chrome, or the admin console's own vertical-nav
// shell) and partials, failing fast if any template is broken.
func New() (*Renderer, error) {
	publicLayout := []string{
		"templates/layout/base.html",
		"templates/layout/header.html",
		"templates/layout/footer.html",
	}
	adminLayout := []string{
		"templates/layout/admin_base.html",
		"templates/layout/adminnav.html",
	}

	publicPages := []string{
		"templates/pages/home.html",
		"templates/pages/about.html",
		"templates/pages/courses_list.html",
		"templates/pages/course_detail.html",
		"templates/pages/consultancy.html",
		"templates/pages/booking.html",
		"templates/pages/articles_list.html",
		"templates/pages/contact.html",
		"templates/pages/login.html",
		"templates/pages/search.html",
		"templates/pages/legal.html",
		"templates/pages/not_found.html",
		"templates/pages/admin_login.html",
	}
	adminPages := []string{
		"templates/pages/admin_dashboard.html",
		"templates/pages/admin_courses_list.html",
		"templates/pages/admin_course_form.html",
		"templates/pages/admin_bookings.html",
		"templates/pages/admin_purchases.html",
		"templates/pages/admin_availability.html",
		"templates/pages/admin_profile.html",
		"templates/pages/admin_articles_list.html",
		"templates/pages/admin_article_form.html",
	}

	r := &Renderer{pages: map[string]renderEntry{}}
	for _, page := range publicPages {
		files := append(append([]string{}, publicLayout...), page)
		t, err := template.New("base.html").Funcs(funcs).ParseFS(templateFS, files...)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", page, err)
		}
		r.pages[page] = renderEntry{tmpl: t, base: "base.html"}
	}
	for _, page := range adminPages {
		files := append(append([]string{}, adminLayout...), page)
		t, err := template.New("admin_base.html").Funcs(funcs).ParseFS(templateFS, files...)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", page, err)
		}
		r.pages[page] = renderEntry{tmpl: t, base: "admin_base.html"}
	}
	return r, nil
}

// Render writes the named page (e.g. "home") with the given data.
func (r *Renderer) Render(w http.ResponseWriter, status int, page string, data any) {
	entry, ok := r.pages["templates/pages/"+page+".html"]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := entry.tmpl.ExecuteTemplate(w, entry.base, data); err != nil {
		// Headers are already sent at this point; log-and-continue is the
		// best we can do for a mid-render template error.
		fmt.Fprintf(w, "\n<!-- render error: %v -->", err)
	}
}
