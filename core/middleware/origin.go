package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// SameOrigin rejects state-changing requests that a browser reports as
// coming from another site. It is the CSRF defence for every form and fetch
// POST on the site, on top of SameSite=Lax cookies. Requests with no Origin
// header (server-to-server calls such as the token-guarded reindex route) are
// allowed through, because browsers always send Origin on cross-site POSTs.
func SameOrigin(allowedHosts ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, h := range allowedHosts {
		if h != "" {
			allowed[strings.ToLower(h)] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
				u, err := url.Parse(origin)
				host := ""
				if err == nil {
					host = strings.ToLower(u.Host)
				}
				if host != strings.ToLower(r.Host) && !allowed[host] {
					http.Error(w, "Cross-site request blocked", http.StatusForbidden)
					return
				}
			} else if origin == "null" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "Cross-site request blocked", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
