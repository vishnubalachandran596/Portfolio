package middleware

import (
	"net/http"
	"strings"
)

// appPathPrefixes are the routes that belong on the application subdomain
// (app.<domain>) rather than the marketing apex domain — the "separate the
// marketing site from the app" split.
var appPathPrefixes = []string{"/booking", "/admin"}

// DomainRouter enforces the marketing-vs-app domain split: yourdomain.com
// serves the public/marketing pages (home, about, courses, legal) and
// app.yourdomain.com serves the booking flow and admin portal. A request
// for an app path on the apex domain (or a marketing path on the app
// subdomain) is redirected to its correct home. When domain is empty
// (local dev, or a raw *.vercel.app preview URL before a custom domain is
// attached) this is a no-op passthrough.
func DomainRouter(domain string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if domain == "" {
			return next
		}
		appHost := "app." + domain
		wwwHost := "www." + domain

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := stripPort(r.Host)

			switch host {
			case wwwHost:
				redirect(w, r, domain, r.URL.Path)
				return
			case appHost:
				if !hasAnyPrefix(r.URL.Path, appPathPrefixes) {
					redirect(w, r, domain, r.URL.Path)
					return
				}
			case domain:
				if hasAnyPrefix(r.URL.Path, appPathPrefixes) {
					redirect(w, r, appHost, r.URL.Path)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
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
		if path == p || strings.HasPrefix(path, p+"/") {
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
