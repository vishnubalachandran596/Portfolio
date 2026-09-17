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

	// Admin auth.
	mux.HandleFunc("GET /admin/login", a.AdminLoginPage)
	mux.HandleFunc("POST /admin/login", a.AdminLoginSubmit)
	mux.HandleFunc("POST /admin/logout", a.AdminLogout)

	// Admin portal (protected).
	admin := http.NewServeMux()
	admin.HandleFunc("GET /admin/{$}", a.AdminDashboard)
	admin.HandleFunc("GET /admin/courses", a.AdminCoursesList)
	admin.HandleFunc("GET /admin/courses/new", a.AdminCourseNewPage)
	admin.HandleFunc("POST /admin/courses/new", a.AdminCourseSave)
	admin.HandleFunc("GET /admin/courses/{id}/edit", a.AdminCourseEditPage)
	admin.HandleFunc("POST /admin/courses/{id}/edit", a.AdminCourseSave)
	admin.HandleFunc("POST /admin/courses/{id}/delete", a.AdminCourseDelete)
	admin.HandleFunc("GET /admin/bookings", a.AdminBookingsList)
	admin.HandleFunc("POST /admin/bookings/{id}/status", a.AdminBookingStatus)
	admin.HandleFunc("GET /admin/availability", a.AdminAvailabilityList)
	admin.HandleFunc("POST /admin/availability/new", a.AdminAvailabilityCreate)
	admin.HandleFunc("POST /admin/availability/{id}/delete", a.AdminAvailabilityDelete)
	admin.HandleFunc("POST /admin/availability/{id}/toggle", a.AdminAvailabilityToggle)
	admin.HandleFunc("GET /admin/profile", a.AdminProfilePage)
	admin.HandleFunc("POST /admin/profile", a.AdminProfileSave)
	mux.Handle("/admin/", a.Auth.RequireAdmin(admin))

	mux.HandleFunc("/", a.NotFound)

	return middleware.Logging(mux)
}
