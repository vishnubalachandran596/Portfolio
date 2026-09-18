package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// baseURL returns the canonical absolute origin used for canonical tags,
// Open Graph URLs, and the sitemap. When SITE_URL is configured, every
// page uses that single fixed host regardless of which host actually
// served the request (apex, www, or home.) — otherwise the same content
// would advertise a different "canonical" URL depending on how a visitor
// or crawler happened to arrive, splitting ranking signals across
// multiple hosts instead of consolidating them onto one. Falls back to
// the request's own host (dynamic) only when SITE_URL isn't set, e.g.
// local dev or a preview deployment before a domain is configured.
func (a *App) baseURL(r *http.Request) string {
	if a.Cfg.SiteURL != "" {
		return strings.TrimSuffix(a.Cfg.SiteURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// pageURL builds the absolute URL for an arbitrary path — unlike
// baseURL(r), which only knows the current request's own host, this picks
// the right subdomain for *that path* when a custom domain is configured,
// for building sitemap entries that span multiple subdomains from one
// request.
func (a *App) pageURL(r *http.Request, path string) string {
	if a.Cfg.Domain == "" {
		return a.baseURL(r) + path
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	sub := middleware.SubdomainForPath(path)
	return scheme + "://" + sub + "." + a.Cfg.Domain + path
}

func (a *App) Sitemap(w http.ResponseWriter, r *http.Request) {
	courses, err := a.Courses.ListPublished(r.Context())
	if err != nil {
		a.serverError(w, "load courses for sitemap", err)
		return
	}
	articlesResult, err := a.Articles.ListPublishedFiltered(r.Context(), repo.ArticleListOptions{PageSize: 10000})
	if err != nil {
		a.serverError(w, "load articles for sitemap", err)
		return
	}
	articles := articlesResult.Articles

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")

	staticPaths := []struct {
		path     string
		priority string
	}{
		{"/", "1.0"},
		{"/about", "0.9"},
		{"/courses", "0.9"},
		{"/consultancy", "0.8"},
		{"/articles", "0.6"},
		{"/contact", "0.5"},
		{"/privacy", "0.2"},
		{"/terms", "0.2"},
		{"/refund", "0.2"},
		{"/cookies", "0.2"},
	}
	for _, p := range staticPaths {
		fmt.Fprintf(&b, "  <url><loc>%s</loc><priority>%s</priority></url>\n", a.pageURL(r, p.path), p.priority)
	}
	for _, c := range courses {
		fmt.Fprintf(&b, "  <url><loc>%s</loc><lastmod>%s</lastmod><priority>0.7</priority></url>\n",
			a.pageURL(r, "/courses/"+c.Slug), c.UpdatedAt.Format("2006-01-02"))
	}
	for _, art := range articles {
		fmt.Fprintf(&b, "  <url><loc>%s</loc><lastmod>%s</lastmod><priority>0.6</priority></url>\n",
			a.pageURL(r, "/articles/"+art.Slug), art.CreatedAt.Format("2006-01-02"))
	}
	b.WriteString(`</urlset>`)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write([]byte(b.String()))
}

func (a *App) Robots(w http.ResponseWriter, r *http.Request) {
	base := a.pageURL(r, "")
	if a.Cfg.Domain != "" {
		base = "https://home." + a.Cfg.Domain
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /admin/\n\nSitemap: %s/sitemap.xml\n", base)
}

type SearchData struct {
	BaseData
	Query   string
	Courses []CourseHit
	Pages   []PageHit
}

type CourseHit struct {
	Title   string
	Slug    string
	Summary string
}

type PageHit struct {
	Title string
	Path  string
	Desc  string
}

var staticSearchIndex = []PageHit{
	{"Home", "/", "Overview, hero introduction, and site highlights."},
	{"About Me", "/about", "Full bio, skills, professional experience, and education."},
	{"Courses", "/courses", "Browse all published courses."},
	{"Consultancy", "/consultancy", "Book a paid 1:1 consultancy session."},
	{"Articles", "/articles", "Curated reading on architecture, AI, and engineering careers."},
	{"Contact Me", "/contact", "Get in touch."},
	{"Privacy Policy", "/privacy", "How data is collected, used, and deleted."},
	{"Terms of Service", "/terms", "Terms governing use of this site."},
	{"Refund Policy", "/refund", "Refund terms for paid courses and consultancy."},
	{"Cookie Policy", "/cookies", "How cookies are used on this site."},
}

// Search provides a simple full-site search across courses and static
// pages by substring match — no external search index required.
func (a *App) Search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	var courseHits []CourseHit
	var pageHits []PageHit

	if query != "" {
		lower := strings.ToLower(query)

		courses, err := a.Courses.ListPublished(r.Context())
		if err != nil {
			a.serverError(w, "load courses for search", err)
			return
		}
		for _, c := range courses {
			if strings.Contains(strings.ToLower(c.Title), lower) ||
				strings.Contains(strings.ToLower(c.Summary), lower) ||
				strings.Contains(strings.ToLower(c.Description), lower) {
				courseHits = append(courseHits, CourseHit{Title: c.Title, Slug: c.Slug, Summary: c.Summary})
			}
		}

		for _, p := range staticSearchIndex {
			if strings.Contains(strings.ToLower(p.Title), lower) || strings.Contains(strings.ToLower(p.Desc), lower) {
				p.Path = a.pageURL(r, p.Path)
				pageHits = append(pageHits, p)
			}
		}
	}

	a.View.Render(w, http.StatusOK, "search", SearchData{
		BaseData: a.base(r, "Search — "+a.Cfg.SiteName, "Search the site.", "search"),
		Query:    query,
		Courses:  courseHits,
		Pages:    pageHits,
	})
}
