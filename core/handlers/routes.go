package handlers

import (
	"net/http"

	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/web"
)

// Routes builds the full HTTP mux for both the public site and the admin
// portal, wrapped with request logging.
func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.FileServerFS(web.StaticFS))

	// Public site.
	mux.HandleFunc("GET /{$}", a.Home)
	mux.HandleFunc("GET /about", a.About)
	mux.HandleFunc("GET /courses", a.CoursesList)
	mux.HandleFunc("GET /courses/{slug}", a.CourseDetail)
	mux.HandleFunc("GET /booking", a.BookingPage)
	mux.HandleFunc("POST /booking", a.BookingSubmit)
	mux.HandleFunc("GET /search", a.Search)

	// SEO.
	mux.HandleFunc("GET /sitemap.xml", a.Sitemap)
	mux.HandleFunc("GET /robots.txt", a.Robots)

	// Legal / compliance.
	mux.HandleFunc("GET /privacy", a.LegalPage("privacy", "Privacy Policy"))
	mux.HandleFunc("GET /terms", a.LegalPage("terms", "Terms of Service"))
	mux.HandleFunc("GET /refund", a.LegalPage("refund", "Refund Policy"))
	mux.HandleFunc("GET /cookies", a.LegalPage("cookies", "Cookie Policy"))

	// Admin auth (unauthenticated).
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /admin/login", a.AdminLoginPage)
	adminMux.HandleFunc("POST /admin/login", a.AdminLoginSubmit)
	adminMux.HandleFunc("POST /admin/logout", a.AdminLogout)

	// Admin portal (protected).
	protected := http.NewServeMux()
	protected.HandleFunc("GET /admin/{$}", a.AdminDashboard)
	protected.HandleFunc("GET /admin/courses", a.AdminCoursesList)
	protected.HandleFunc("GET /admin/courses/new", a.AdminCourseNewPage)
	protected.HandleFunc("POST /admin/courses/new", a.AdminCourseSave)
	protected.HandleFunc("GET /admin/courses/{id}/edit", a.AdminCourseEditPage)
	protected.HandleFunc("POST /admin/courses/{id}/edit", a.AdminCourseSave)
	protected.HandleFunc("POST /admin/courses/{id}/delete", a.AdminCourseDelete)
	protected.HandleFunc("GET /admin/bookings", a.AdminBookingsList)
	protected.HandleFunc("POST /admin/bookings/{id}/status", a.AdminBookingStatus)
	protected.HandleFunc("GET /admin/availability", a.AdminAvailabilityList)
	protected.HandleFunc("POST /admin/availability/new", a.AdminAvailabilityCreate)
	protected.HandleFunc("POST /admin/availability/{id}/delete", a.AdminAvailabilityDelete)
	protected.HandleFunc("POST /admin/availability/{id}/toggle", a.AdminAvailabilityToggle)
	protected.HandleFunc("GET /admin/profile", a.AdminProfilePage)
	protected.HandleFunc("POST /admin/profile", a.AdminProfileSave)
	adminMux.Handle("/admin/", a.Auth.RequireAdmin(protected))

	mux.Handle("/admin/", noindex(adminMux))

	mux.HandleFunc("/", a.NotFound)

	return middleware.Logging(middleware.DomainRouter(a.Cfg.Domain)(mux))
}

// noindex tags every response under a handler as non-indexable — used for
// the admin portal, which should never appear in search results.
func noindex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		next.ServeHTTP(w, r)
	})
}
