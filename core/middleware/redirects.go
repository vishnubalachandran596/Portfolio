package middleware

import (
	"net/http"
	"strings"
)

// legacyPrefixes maps retired or commonly guessed URL prefixes onto the
// real ones. Each entry matches the bare prefix and anything beneath it.
var legacyPrefixes = []struct{ from, to string }{
	{"/blog", "/articles"},
	{"/course", "/courses"},
}

// legacyExact maps single retired paths onto their replacements.
var legacyExact = map[string]string{
	"/book": "/booking",
}

// LegacyRedirects tidies URL variants that would otherwise 404 or split
// ranking signals, answering with a redirect before routing runs:
//
//   - a trailing slash (/about/) redirects to the bare path with 308, which
//     keeps the method and query, so GET and HEAD only are redirected;
//   - /blog, /blog/{slug}, /course/{slug} and /book permanently redirect (301)
//     to /articles, /articles/{slug}, /courses/{slug} and /booking.
//
// /admin and /static are exempt: the admin console registers "/admin/"
// itself and static assets are fingerprinted by query string.
func LegacyRedirects(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		if target, status, ok := legacyTarget(r.URL.Path); ok {
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, status)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// legacyTarget reports where path should redirect to, if anywhere.
func legacyTarget(path string) (target string, status int, ok bool) {
	if path == "/admin" || strings.HasPrefix(path, "/admin/") || strings.HasPrefix(path, "/static/") {
		return "", 0, false
	}
	// Collapse a trailing slash first so "/blog/" and "/blog/x/" take the
	// same single hop as their slash-free forms.
	trimmed := path
	slash := false
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		trimmed = strings.TrimRight(path, "/")
		slash = true
	}
	if to, found := legacyExact[trimmed]; found {
		return to, http.StatusMovedPermanently, true
	}
	for _, p := range legacyPrefixes {
		if trimmed == p.from || strings.HasPrefix(trimmed, p.from+"/") {
			return p.to + strings.TrimPrefix(trimmed, p.from), http.StatusMovedPermanently, true
		}
	}
	if slash && trimmed != "" {
		return trimmed, http.StatusPermanentRedirect, true
	}
	return "", 0, false
}
