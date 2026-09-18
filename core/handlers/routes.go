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
	mux.HandleFunc("POST /courses/{slug}/checkout/order", a.CheckoutCreateOrder)
	mux.HandleFunc("POST /courses/{slug}/checkout/verify", a.CheckoutVerify)
	mux.Handle("GET /courses/{slug}/learn", noindex(http.HandlerFunc(a.CourseLearn)))
	mux.HandleFunc("GET /consultancy", a.ConsultancyPage)
	mux.Handle("GET /booking", a.Auth.RequireUser(http.HandlerFunc(a.BookingPage)))
	mux.HandleFunc("POST /booking/checkout/order", a.BookingCheckoutOrder)
	mux.HandleFunc("POST /booking/checkout/verify", a.BookingCheckoutVerify)
	mux.HandleFunc("GET /articles", a.ArticlesPage)
	mux.HandleFunc("GET /articles/{slug}", a.ArticleDetailPage)
	mux.HandleFunc("POST /articles/{slug}/thumbs-up", a.ArticleThumbsUp)
	mux.HandleFunc("GET /contact", a.ContactPage)
	mux.HandleFunc("GET /search", a.Search)
	mux.Handle("GET /account", noindex(a.Auth.RequireUser(http.HandlerFunc(a.AccountPage))))

	// Visitor auth.
	mux.HandleFunc("GET /exit-preview", a.ExitVisitorPreview)
	mux.HandleFunc("GET /login", a.LoginPage)
	mux.HandleFunc("GET /login/google", a.GoogleLoginStart)
	mux.HandleFunc("GET /login/google/callback", a.GoogleLoginCallback)
	mux.HandleFunc("POST /logout", a.UserLogout)

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
	protected.HandleFunc("GET /admin/purchases", a.AdminPurchasesList)
	protected.HandleFunc("GET /admin/articles", a.AdminArticlesList)
	protected.HandleFunc("GET /admin/articles/new", a.AdminArticleNewPage)
	protected.HandleFunc("POST /admin/articles/new", a.AdminArticleSave)
	protected.HandleFunc("GET /admin/articles/{id}/edit", a.AdminArticleEditPage)
	protected.HandleFunc("POST /admin/articles/{id}/edit", a.AdminArticleSave)
	protected.HandleFunc("POST /admin/articles/{id}/delete", a.AdminArticleDelete)
	protected.HandleFunc("GET /admin/availability", a.AdminAvailabilityList)
	protected.HandleFunc("POST /admin/availability/new", a.AdminAvailabilityCreate)
	protected.HandleFunc("POST /admin/availability/{id}/delete", a.AdminAvailabilityDelete)
	protected.HandleFunc("POST /admin/availability/{id}/toggle", a.AdminAvailabilityToggle)
	protected.HandleFunc("GET /admin/profile", a.AdminProfilePage)
	protected.HandleFunc("POST /admin/profile", a.AdminProfileSave)
	protected.HandleFunc("GET /admin/preview-visitor", a.AdminPreviewAsVisitor)
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
