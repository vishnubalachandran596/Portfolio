package middleware

import (
	"net/http"
	"strings"
)

// pageSubdomains maps each path prefix to the subdomain it belongs on.
// Order matters: checked longest-prefix-first (via matchSubdomain) so
// "/courses" and "/admin" don't shadow more specific future prefixes.
// Anything not matched here (home, search, legal pages, static assets)
// lives on the "home" subdomain — the site's landing subdomain.
var pageSubdomains = []struct {
	prefix    string
	subdomain string
}{
	{"/about", "about"},
	{"/courses", "courses"},
	{"/consultancy", "consultancy"},
	{"/booking", "consultancy"},
	{"/contact", "contact"},
	{"/admin", "admin"},
}

// hostExemptPrefixes are served identically on every subdomain rather
// than redirected — static assets and the SEO files must resolve on
// whichever host actually requested them, since pages reference them
// with relative paths.
var hostExemptPrefixes = []string{"/static/", "/sitemap.xml", "/robots.txt"}

// DomainRouter enforces one subdomain per page group: home.<domain> is
// the landing subdomain (and catch-all for anything not otherwise
// mapped), and about./courses./booking./admin.<domain> each serve their
// own section. A request for a path on the wrong host — including the
// bare apex or a "www" host — is redirected to its correct subdomain.
// When domain is empty (local dev, or a raw *.vercel.app preview URL
// before a custom domain is attached) this is a no-op passthrough.
func DomainRouter(domain string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if domain == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hasAnyPrefix(r.URL.Path, hostExemptPrefixes) {
				next.ServeHTTP(w, r)
				return
			}

			host := stripPort(r.Host)
			want := subdomainFor(r.URL.Path)
			wantHost := want + "." + domain

			if host == wantHost {
				next.ServeHTTP(w, r)
				return
			}
			// Any other host for this path — the apex, "www", or a
			// different page's subdomain — redirects to the right one.
			if host == domain || host == "www."+domain || isKnownSubdomainHost(host, domain) {
				redirect(w, r, wantHost, r.URL.Path)
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
// one currently being served.
func SubdomainForPath(path string) string { return subdomainFor(path) }

func subdomainFor(path string) string {
	for _, p := range pageSubdomains {
		if path == p.prefix || strings.HasPrefix(path, p.prefix+"/") {
			return p.subdomain
		}
	}
	return "home"
}

func isKnownSubdomainHost(host, domain string) bool {
	if host == "home."+domain {
		return true
	}
	for _, p := range pageSubdomains {
		if host == p.subdomain+"."+domain {
			return true
		}
	}
	return false
}

func redirect(w http.ResponseWriter, r *http.Request, host, path string) {
	target := "https://" + host + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	// 307 preserves the request method (relevant for the rare cross-domain
	// POST); browsers still treat it as a normal redirect for GET.
	http.Redirect(w, r, target, http.StatusTemporaryRedirect)
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
