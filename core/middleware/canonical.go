package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// CanonicalHost 301-redirects the bare apex and the old home. landing
// host to the host in SITE_URL (www). The canonical host itself is never
// redirected — a www URL that 3xxs is what Google reports as "Page with
// redirect" and refuses to index.
//
// Preview hosts (*.vercel.app) and local dev are left alone. When SITE_URL
// is empty, or does not prefer www, this is a no-op passthrough.
func CanonicalHost(siteURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		canonical, apex := parseCanonicalHost(siteURL)
		if canonical == "" || !wwwPreferred(canonical, apex) {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := strings.ToLower(stripPort(r.Host))
			if host == canonical {
				next.ServeHTTP(w, r)
				return
			}
			if host == apex || host == "home."+apex {
				redirectPermanent(w, r, canonical, r.URL.Path)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseCanonicalHost(siteURL string) (canonical, apex string) {
	siteURL = strings.TrimSpace(siteURL)
	if siteURL == "" {
		return "", ""
	}
	u, err := url.Parse(siteURL)
	if err != nil || u.Host == "" {
		return "", ""
	}
	canonical = strings.ToLower(stripPort(u.Host))
	apex = strings.TrimPrefix(canonical, "www.")
	return canonical, apex
}

func wwwPreferred(canonical, apex string) bool {
	return canonical != "" && apex != "" && canonical == "www."+apex
}
