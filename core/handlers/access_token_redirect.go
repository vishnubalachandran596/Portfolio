package handlers

import (
	"net/http"
	"net/url"
	"strings"
)

// redirectAccessToken keeps bearer access tokens out of URLs. A legacy
// /courses/{slug}/... or /software/{slug} link carrying ?token= is turned
// into the access cookie and redirected to the same URL without the token,
// so the token is not left in browser history, server logs or Referer
// headers. An invalid token is just dropped. Only GET requests are touched.
func (a *App) redirectAccessToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		token := q.Get("token")
		if r.Method != http.MethodGet || token == "" {
			next.ServeHTTP(w, r)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 2 || (parts[0] != "courses" && parts[0] != "software") {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()
		switch parts[0] {
		case "courses":
			if course, err := a.Courses.GetBySlug(ctx, parts[1]); err == nil {
				if p, err := a.Purchases.GetPaidByAccessToken(ctx, token); err == nil && p.CourseID == course.ID {
					a.setAccessCookie(w, accessCookieName(course.ID), token)
				}
			}
		case "software":
			if sw, err := a.Software.GetBySlug(ctx, parts[1]); err == nil {
				if p, err := a.SoftwarePurchases.GetPaidByAccessToken(ctx, token); err == nil && p.SoftwareID == sw.ID {
					a.setAccessCookie(w, softwareAccessCookieName(sw.ID), token)
				}
			}
		}
		q.Del("token")
		clean := url.URL{Path: r.URL.Path, RawQuery: q.Encode()}
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, clean.String(), http.StatusSeeOther)
	})
}
