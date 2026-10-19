package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// defaultOGImagePath is the site-wide Open Graph image, used whenever a
// page has no og_image of its own and no cover image to fall back on.
const defaultOGImagePath = "/static/img/og-image.png"

// absoluteURL makes ref absolute against base. An absolute http(s) URL is
// returned unchanged; a site-relative path is joined to base; anything
// else (a bare "img/x.png") is treated as site-relative.
func absoluteURL(base, ref string) string {
	ref = strings.TrimSpace(ref)
	switch {
	case ref == "":
		return ""
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return ref
	case strings.HasPrefix(ref, "/"):
		return strings.TrimSuffix(base, "/") + ref
	}
	return strings.TrimSuffix(base, "/") + "/" + ref
}

// ogImageFor resolves the Open Graph image for an item: its og_image, else
// its own cover image, else the site default — always absolute.
func ogImageFor(base string, f models.SEOFields, cover string) string {
	if img := f.Image(cover); img != "" {
		return absoluteURL(base, img)
	}
	return base + defaultOGImagePath
}

// noindexCtxKey marks a request that passed through the noindex
// middleware, so baseFrom can mirror the X-Robots-Tag header as a
// <meta name="robots"> tag in the HTML.
type noindexCtxKey struct{}

func markNoindex(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), noindexCtxKey{}, true))
}

// robotsMetaForRequest is the robots meta value for a request, "" when the
// page is indexable. Routes wrapped in noindex (account, login, signup,
// forgot, booking, search, gated course pages, admin) get "noindex,
// nofollow"; an internal-search view of a listing (?q=) gets "noindex,
// follow", matching the X-Robots-Tag header those handlers also send.
func robotsMetaForRequest(r *http.Request) string {
	if v, _ := r.Context().Value(noindexCtxKey{}).(bool); v {
		return "noindex, nofollow"
	}
	if (r.URL.Path == "/courses" || r.URL.Path == "/articles") && strings.TrimSpace(r.URL.Query().Get("q")) != "" {
		return "noindex, follow"
	}
	return ""
}

// pageSEOApplies reports whether the page_seo override for this request's
// path should be looked up and applied. Only the bare static page
// qualifies: a filtered or paginated listing (?tag=, ?q=, ?page=2) keeps
// the title pattern that names its filter or page number, and 404s are
// never overridden.
func pageSEOApplies(r *http.Request) bool {
	if !models.IsStaticSEOPath(r.URL.Path) {
		return false
	}
	q := r.URL.Query()
	if q.Get("tag") != "" || q.Get("q") != "" {
		return false
	}
	if page, _ := strconv.Atoi(q.Get("page")); page > 1 {
		return false
	}
	return true
}

// seoFieldsAsync starts the SEO-columns lookup for a content item
// concurrently with the handler's own queries and returns a function that
// waits for it. Best-effort by design: a failed lookup yields empty
// fields, so the page falls back to the title/summary chain instead of
// failing the render.
func (a *App) seoFieldsAsync(ctx context.Context, kind repo.SEOKind, slug string) func() models.SEOFields {
	ch := make(chan models.SEOFields, 1)
	go func() {
		f, _ := a.SEO.GetFieldsBySlug(ctx, kind, slug)
		ch <- f
	}()
	return func() models.SEOFields { return <-ch }
}

// trail builds an absolute breadcrumb trail starting at Home.
func (a *App) trail(r *http.Request, steps ...crumb) []crumb {
	out := []crumb{{Name: "Home", URL: a.pageURL(r, "/")}}
	return append(out, steps...)
}
