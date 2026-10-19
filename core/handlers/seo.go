package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// indexNowKey authorizes this site's IndexNow submissions — proven by
// serving it back at /{key}.txt, per the protocol. IndexNow (supported by
// Bing, Yandex, and other engines that don't require a Search-Console-style
// account) lets a site push "this URL changed" directly instead of waiting
// for the next crawl, which is otherwise the only lever a site this size
// has over when a change actually gets re-indexed.
const indexNowKey = "4e5f66a954c278a99dc0547f78f9fd9f"

func (a *App) IndexNowKeyFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, indexNowKey)
}

// submitIndexNow pings the IndexNow API with a batch of changed URLs. Fire
// and forget with a short timeout — this is a nice-to-have nudge for
// faster re-crawling, never something a request should block or fail on.
func submitIndexNow(host string, paths []string) {
	urls := make([]string, len(paths))
	for i, p := range paths {
		urls[i] = "https://" + host + p
	}
	body, err := json.Marshal(map[string]any{
		"host":        host,
		"key":         indexNowKey,
		"keyLocation": "https://" + host + "/" + indexNowKey + ".txt",
		"urlList":     urls,
	})
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post("https://api.indexnow.org/indexnow", "application/json; charset=utf-8", bytes.NewReader(body))
	if err != nil {
		log.Printf("indexnow submit: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("indexnow submit: unexpected status %d", resp.StatusCode)
	}
}

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
	if sub == "home" {
		return scheme + "://www." + a.Cfg.Domain + path
	}
	return scheme + "://" + sub + "." + a.Cfg.Domain + path
}

// canonicalURLForRequest builds the canonical URL for a page. Listing
// filters never get their own canonical identity: "tag" (a topic filter),
// "sort" (a reorder of the same items) and "q" (site-internal search, which
// is also sent noindex, see robotsMetaForRequest) all canonicalise to the
// bare listing path, so filtered views collapse into the page they are a
// variant of. Only "page" beyond the first is kept: each page of a
// paginated list holds a genuinely different set of items, and Google's
// guidance is a self-referential canonical per page rather than collapsing
// page 2 onto page 1.
//
// History: an earlier version (Oct 2026) kept "tag" in the canonical so
// /courses?tag=java was self-canonical. The SEO audit of 19 Oct 2026
// reversed that: those pages are near-duplicates of the listing they
// filter, linked from every course card, and indexing them dilutes the
// listing. seo_canonical_test.go records the current rule.
func canonicalURLForRequest(base string, r *http.Request) string {
	u := base + r.URL.Path
	if page, _ := strconv.Atoi(r.URL.Query().Get("page")); page > 1 {
		u += "?page=" + strconv.Itoa(page)
	}
	return u
}

// sitemapStaticPaths are the static, indexable pages. /ai-assistant (a thin
// app shell) and /resume.pdf are deliberately absent: neither is a landing
// page worth indexing. A page whose page_seo row says noindex is dropped
// as well, so the sitemap never advertises a URL that asks not to be indexed.
var sitemapStaticPaths = []string{
	"/", "/about", "/courses", "/software", "/consultancy", "/articles",
	"/contact", "/privacy", "/terms", "/refund", "/cookies",
}

// staticLastmod returns the real "last edited" time of a static page: the
// newest of its page_seo edit and the live content the page is built from
// (courses for /courses, the profile for /about, and so on). The zero time
// means "unknown", and the caller then omits <lastmod> altogether — a
// made-up date teaches crawlers to ignore the field.
func staticLastmod(path string, d repo.SitemapDates, page *models.PageSEO) time.Time {
	var t time.Time
	bump := func(c time.Time) {
		if c.After(t) {
			t = c
		}
	}
	if page != nil {
		bump(page.UpdatedAt)
	}
	switch path {
	case "/":
		bump(d.ProfileUpdated)
		bump(d.LatestCourse)
		bump(d.LatestArticle)
	case "/about", "/consultancy", "/contact":
		bump(d.ProfileUpdated)
	case "/courses":
		bump(d.LatestCourse)
	case "/software":
		bump(d.LatestSoftware)
	case "/articles":
		bump(d.LatestArticle)
	}
	return t
}

type sitemapEntry struct {
	loc     string
	lastmod time.Time
}

// renderSitemap writes the sitemap XML. Priority is omitted on purpose
// (Google ignores it); lastmod is written only when known.
func renderSitemap(entries []sitemapEntry) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, e := range entries {
		b.WriteString("  <url><loc>" + html.EscapeString(e.loc) + "</loc>")
		if !e.lastmod.IsZero() {
			b.WriteString("<lastmod>" + e.lastmod.Format("2006-01-02") + "</lastmod>")
		}
		b.WriteString("</url>\n")
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func (a *App) Sitemap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	courses, err := a.Courses.ListPublished(ctx)
	if err != nil {
		a.serverError(w, "load courses for sitemap", err)
		return
	}
	software, err := a.Software.ListPublished(ctx)
	if err != nil {
		a.serverError(w, "load software for sitemap", err)
		return
	}
	articlesResult, err := a.Articles.ListPublishedFiltered(ctx, repo.ArticleListOptions{PageSize: 10000})
	if err != nil {
		a.serverError(w, "load articles for sitemap", err)
		return
	}
	dates, err := a.SEO.SitemapDates(ctx)
	if err != nil {
		a.serverError(w, "load sitemap dates", err)
		return
	}
	// Overrides are optional: a failed lookup only costs the noindex
	// filter and page_seo lastmod, never the sitemap.
	overrides, _ := a.SEO.ListPages(ctx)

	var entries []sitemapEntry
	for _, path := range sitemapStaticPaths {
		var page *models.PageSEO
		if p, ok := overrides[path]; ok {
			if p.Noindex {
				continue
			}
			page = &p
		}
		entries = append(entries, sitemapEntry{a.pageURL(r, path), staticLastmod(path, dates, page)})
	}
	for _, c := range courses {
		entries = append(entries, sitemapEntry{a.pageURL(r, "/courses/"+c.Slug), c.UpdatedAt})
	}
	for _, art := range articlesResult.Articles {
		lastmod, ok := dates.Articles[art.Slug]
		if !ok {
			lastmod = art.CreatedAt
		}
		entries = append(entries, sitemapEntry{a.pageURL(r, "/articles/"+art.Slug), lastmod})
	}
	for _, sw := range software {
		entries = append(entries, sitemapEntry{a.pageURL(r, "/software/"+sw.Slug), sw.UpdatedAt})
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write([]byte(renderSitemap(entries)))
}

func (a *App) Robots(w http.ResponseWriter, r *http.Request) {
	base := a.pageURL(r, "")
	if a.Cfg.SiteURL != "" {
		base = strings.TrimSuffix(a.Cfg.SiteURL, "/")
	} else if a.Cfg.Domain != "" {
		base = "https://www." + a.Cfg.Domain
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nDisallow: /admin/\n\nSitemap: %s/sitemap.xml\n", base)
}

// LLMsTxt serves a plain-text summary of the site for AI assistants and
// answer engines to read directly — the emerging (informal, unstandardized)
// llms.txt convention. Built from the same live profile/course data as the
// rest of the site rather than a static file, so it can't silently drift
// out of date.
func (a *App) LLMsTxt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile, err := a.Profile.Get(ctx)
	if err != nil {
		a.serverError(w, "load profile for llms.txt", err)
		return
	}
	courses, err := a.Courses.ListPublished(ctx)
	if err != nil {
		a.serverError(w, "load courses for llms.txt", err)
		return
	}

	base := a.pageURL(r, "")

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", profile.Name)
	fmt.Fprintf(&b, "> %s\n\n", profile.HeroSubtitle)
	if profile.Summary != "" {
		fmt.Fprintf(&b, "%s\n\n", profile.Summary)
	}

	b.WriteString("## Pages\n\n")
	fmt.Fprintf(&b, "- [Home](%s/): Overview, current work, and primary ways to engage\n", base)
	fmt.Fprintf(&b, "- [About](%s/about): Background, skills, and experience\n", base)
	fmt.Fprintf(&b, "- [Resume PDF](%s/resume.pdf): Downloadable resume generated from the live profile\n", base)
	fmt.Fprintf(&b, "- [Consultancy](%s/consultancy): Architecture reviews, fractional advisory retainers, and a free first call\n", base)
	fmt.Fprintf(&b, "- [Courses](%s/courses): Practical courses on Java, architecture, system design, and distributed systems\n", base)
	fmt.Fprintf(&b, "- [Software](%s/software): Downloadable apps and tools, sold directly\n", base)
	fmt.Fprintf(&b, "- [Articles](%s/articles): Original writing on software architecture, distributed systems, and engineering careers\n", base)
	fmt.Fprintf(&b, "- [Contact](%s/contact): Get in touch\n", base)
	if a.Nvidia.Enabled() {
		fmt.Fprintf(&b, "- [AI Assistant](%s/ai-assistant): Ask questions about his background, courses, consultancy, and articles, answered with retrieval-augmented generation\n", base)
	}
	fmt.Fprintf(&b, "- [Privacy Policy](%s/privacy): How data is collected, used, and deleted\n", base)
	fmt.Fprintf(&b, "- [Terms of Service](%s/terms): Terms governing use of this site\n", base)
	fmt.Fprintf(&b, "- [Refund Policy](%s/refund): Refund terms for paid courses\n", base)
	fmt.Fprintf(&b, "- [Cookie Policy](%s/cookies): How cookies are used on this site\n\n", base)

	if len(courses) > 0 {
		b.WriteString("## Courses\n\n")
		for _, c := range courses {
			fmt.Fprintf(&b, "- [%s](%s/courses/%s): %s\n", c.Title, base, c.Slug, c.Summary)
		}
		b.WriteString("\n")
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(b.String()))
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
	{"Articles", "/articles", "Original writing on architecture, AI, and engineering careers."},
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
