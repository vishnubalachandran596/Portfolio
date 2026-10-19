// Package view renders the site's html/template pages from the embedded
// web/templates directory.
package view

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
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
	// money reuses models.FormatMoney rather than re-implementing currency
	// formatting a second time — the two independent implementations that
	// existed here before disagreed on a real case ("$3000" vs "$3000.00"
	// for a whole-dollar amount), which is exactly the bug class a single
	// shared utility exists to prevent.
	"money": models.FormatMoney,
	"add1":  func(i int) int { return i + 1 },
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
	// articleBody renders a blog post's body as either the HTML the admin
	// wrote directly (detected by the content *starting* with a tag — the
	// admin is the only one who can ever write this, so it's trusted
	// content, same as every other admin-authored field rendered on the
	// site) or, for a plain-text body written the old way, one <p> per
	// blank-line-separated paragraph — so a single Body field keeps
	// accepting both without the admin needing to pick a mode up front.
	// Checking only the prefix (not "contains a < and a > anywhere")
	// matters: plain prose that happens to mention e.g. "List<String>"
	// mid-paragraph must still go through the escaping path below, not
	// get emitted raw.
	"articleBody": func(s string) template.HTML {
		trimmed := strings.TrimSpace(s)
		if strings.HasPrefix(trimmed, "<") {
			return template.HTML(trimmed)
		}
		var b strings.Builder
		for _, p := range strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n\n") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			b.WriteString("<p>")
			b.WriteString(template.HTMLEscapeString(p))
			b.WriteString("</p>")
		}
		return template.HTML(b.String())
	},
	"rich": rich,
}

var richBoldPattern = regexp.MustCompile(`\*\*(.+?)\*\*`)
var richItalicPattern = regexp.MustCompile(`\*(.+?)\*`)

// rich renders a small, deliberately limited subset of markdown --
// **bold** and *italic* only -- as styled HTML, for admin-authored
// free-text fields (profile tagline, skill achievements, experience
// bullets, testimonial quotes, and the like) that aren't full HTML
// documents the way an uploaded article/lesson body is.
//
// The input is HTML-escaped first, so only the two recognized marker
// patterns can ever turn into tags -- nothing else in the original
// text can introduce markup. That's what makes this safe to apply
// directly to any plain text field without a separate HTML sanitizer:
// there is no raw HTML path here at all, unlike articleBody above.
// Bold runs before italic so "**x**" is never also read as a pair of
// italic markers once its own asterisks are gone.
func rich(s string) template.HTML {
	escaped := template.HTMLEscapeString(s)
	escaped = richBoldPattern.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = richItalicPattern.ReplaceAllString(escaped, "<em>$1</em>")
	return template.HTML(escaped)
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
//
// publicLayout also carries shared component partials that more than one
// page renders — skills_grid.html is the first of these, so Home and
// About render one identical skills grid rather than two copies that can
// drift. A new shared component belongs in that list too.
func New() (*Renderer, error) {
	publicLayout := []string{
		"templates/layout/base.html",
		"templates/layout/header.html",
		"templates/layout/footer.html",
		"templates/layout/skills_grid.html",
		"templates/layout/subscribe_form.html",
		"templates/layout/promo_banner.html",
		"templates/layout/chatbot_widget.html",
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
		"templates/pages/software_list.html",
		"templates/pages/software_detail.html",
		"templates/pages/course_learn.html",
		"templates/pages/consultancy.html",
		"templates/pages/booking.html",
		"templates/pages/articles_list.html",
		"templates/pages/article_detail.html",
		"templates/pages/contact.html",
		"templates/pages/ai_assistant.html",
		"templates/pages/account.html",
		"templates/pages/account_reschedule.html",
		"templates/pages/quiz.html",
		"templates/pages/quiz_result.html",
		"templates/pages/login.html",
		"templates/pages/signup.html",
		"templates/pages/forgot.html",
		"templates/pages/account_security.html",
		"templates/pages/search.html",
		"templates/pages/legal.html",
		"templates/pages/not_found.html",
		"templates/pages/admin_login.html",
	}
	adminPages := []string{
		"templates/pages/admin_dashboard.html",
		"templates/pages/admin_courses_list.html",
		"templates/pages/admin_course_form.html",
		"templates/pages/admin_course_bulk.html",
		"templates/pages/admin_highlights_list.html",
		"templates/pages/admin_highlight_form.html",
		"templates/pages/admin_software_list.html",
		"templates/pages/admin_software_form.html",
		"templates/pages/admin_bookings.html",
		"templates/pages/admin_booking_reschedule.html",
		"templates/pages/admin_purchases.html",
		"templates/pages/admin_payments_list.html",
		"templates/pages/admin_contact_messages.html",
		"templates/pages/admin_availability.html",
		"templates/pages/admin_profile.html",
		"templates/pages/admin_articles_list.html",
		"templates/pages/admin_article_form.html",
		"templates/pages/admin_users_list.html",
		"templates/pages/admin_user_detail.html",
		"templates/pages/admin_skills_list.html",
		"templates/pages/admin_skill_form.html",
		"templates/pages/admin_experience_list.html",
		"templates/pages/admin_experience_form.html",
		"templates/pages/admin_testimonials_list.html",
		"templates/pages/admin_testimonial_form.html",
		"templates/pages/admin_certifications_list.html",
		"templates/pages/admin_certification_form.html",
		"templates/pages/admin_publications_list.html",
		"templates/pages/admin_publication_form.html",
		"templates/pages/admin_case_studies_list.html",
		"templates/pages/admin_case_study_form.html",
		"templates/pages/admin_subscribers_list.html",
		"templates/pages/admin_promo_list.html",
		"templates/pages/admin_promo_form.html",
		"templates/pages/admin_quiz_list.html",
		"templates/pages/admin_course_reviews_list.html",
		"templates/pages/admin_chatbot.html",
		"templates/pages/admin_quiz_form.html",
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
