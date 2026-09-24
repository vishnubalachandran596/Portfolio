package middleware

import (
	"net/http"
	"strings"
)

// pageSubdomains maps each path prefix to the subdomain it belongs on.
// Order matters: checked longest-prefix-first (via matchSubdomain) so
// "/courses" and "/admin" don't shadow more specific future prefixes.
// Anything not matched here (home, search, legal pages, static assets)
// lives on the canonical www host — not a home. subdomain. Google will
// not index a canonical URL that itself 3xxs.
var pageSubdomains = []struct {
	prefix    string
	subdomain string
}{
	{"/about", "about"},
	{"/courses", "courses"},
	{"/consultancy", "consultancy"},
	{"/booking", "consultancy"},
	{"/articles", "articles"},
	{"/contact", "contact"},
	{"/admin", "admin"},
}

// hostExemptPrefixes are served identically on every subdomain rather
// than redirected — static assets and the SEO files must resolve on
// whichever host actually requested them, since pages reference them
// with relative paths.
var hostExemptPrefixes = []string{"/static/", "/sitemap.xml", "/robots.txt"}

// DomainRouter enforces one subdomain per page group when DOMAIN is set:
// about./courses./consultancy./articles./contact./admin. each own their
// path prefix. The public canonical host is www.<domain> — it is never
// redirected, including for page-group paths (the live site is path-based
// on www). The bare apex and the old home. landing host 301 to www with
// the same path. Other page-group subdomains still 301 to the subdomain
// that owns the path.
//
// When domain is empty (local dev, or a raw *.vercel.app preview URL
// before a custom domain is attached) this is a no-op passthrough.
func DomainRouter(domain string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if domain == "" {
			return next
		}
		domain = strings.ToLower(domain)
		wwwHost := "www." + domain

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hasAnyPrefix(r.URL.Path, hostExemptPrefixes) {
				next.ServeHTTP(w, r)
				return
			}

			host := strings.ToLower(stripPort(r.Host))

			// Canonical public host: never redirect www to itself or to a
			// page-group subdomain. A 3xx here is the Search Console failure.
			if host == wwwHost {
				next.ServeHTTP(w, r)
				return
			}

			// Apex and the retired home. host both collapse onto www in one hop.
			if host == domain || host == "home."+domain {
				redirectPermanent(w, r, wwwHost, r.URL.Path)
				return
			}

			wantHost := hostFor(subdomainFor(r.URL.Path), domain)
			if host == wantHost {
				next.ServeHTTP(w, r)
				return
			}
			if isKnownSubdomainHost(host, domain) {
				redirectPermanent(w, r, wantHost, r.URL.Path)
				return
			}
			// Unrecognized host (e.g. a *.vercel.app preview URL still
			// pointed at this deployment): don't redirect, just serve.
			next.ServeHTTP(w, r)
		})
	}
}

// SubdomainForPath returns which page-group subdomain a given path
// belongs on (e.g. "/courses/foo" -> "courses"), so SEO/canonical URL
// generation can build the right absolute URL for any page, not just the
// one currently being served. Unmatched paths return "home", which
// callers should resolve to the www host (see hostFor / pageURL).
func SubdomainForPath(path string) string { return subdomainFor(path) }

func subdomainFor(path string) string {
	for _, p := range pageSubdomains {
		if path == p.prefix || strings.HasPrefix(path, p.prefix+"/") {
			return p.subdomain
		}
	}
	return "home"
}

// hostFor maps a page-group label onto the host that should serve it.
// "home" is www.<domain>, never home.<domain>.
func hostFor(sub, domain string) string {
	if sub == "home" {
		return "www." + domain
	}
	return sub + "." + domain
}

func isKnownSubdomainHost(host, domain string) bool {
	if host == "home."+domain || host == "www."+domain {
		return true
	}
	for _, p := range pageSubdomains {
		if host == p.subdomain+"."+domain {
			return true
		}
	}
	return false
}

func redirectPermanent(w http.ResponseWriter, r *http.Request, host, path string) {
	if path == "" {
		path = "/"
	}
	target := "https://" + host + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func hasAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func stripPort(host string) string {
	if i := strings.IndexByte(host, ':'); i != -1 {
		return host[:i]
	}
	return host
}
