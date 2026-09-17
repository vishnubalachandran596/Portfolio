// Package handlers wires the repositories, renderer, and auth into HTTP
// handlers for both the public site and the admin portal.
package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/core/payments"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

type App struct {
	Cfg *config.Config
	DB  *sql.DB

	View *view.Renderer
	Auth *middleware.Auth

	Profile      *repo.ProfileRepo
	Skills       *repo.SkillGroupRepo
	Experience   *repo.ExperienceRepo
	Education    *repo.EducationRepo
	Features     *repo.HomeFeatureRepo
	Courses      *repo.CourseRepo
	Bookings     *repo.BookingRepo
	Availability *repo.AvailabilityRepo
	Admins       *repo.AdminRepo
	Purchases    *repo.PurchaseRepo

	Razorpay *payments.Razorpay
	Timezone *time.Location
}

// BaseData carries the fields every page template needs. Page-specific
// data structs embed it so templates can reach both the shared chrome and
// the page's own data through one dot.
type BaseData struct {
	SiteName        string
	Title           string
	MetaDescription string
	Active          string
	SiteURL         string
	CanonicalURL    string
	OGImageURL      string
}

func New(cfg *config.Config, db *sql.DB, renderer *view.Renderer) *App {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.UTC
	}
	return &App{
		Cfg:          cfg,
		DB:           db,
		View:         renderer,
		Auth:         middleware.NewAuth(cfg.JWTSecret, cfg.SessionTTL, cfg.IsProduction()),
		Profile:      repo.NewProfileRepo(db),
		Skills:       repo.NewSkillGroupRepo(db),
		Experience:   repo.NewExperienceRepo(db),
		Education:    repo.NewEducationRepo(db),
		Features:     repo.NewHomeFeatureRepo(db),
		Courses:      repo.NewCourseRepo(db),
		Bookings:     repo.NewBookingRepo(db),
		Availability: repo.NewAvailabilityRepo(db),
		Admins:       repo.NewAdminRepo(db),
		Purchases:    repo.NewPurchaseRepo(db),
		Razorpay:     payments.NewRazorpay(cfg.RazorpayKeyID, cfg.RazorpayKeySecret),
		Timezone:     loc,
	}
}

func (a *App) base(r *http.Request, title, metaDescription, active string) BaseData {
	base := a.baseURL(r)
	return BaseData{
		SiteName:        a.Cfg.SiteName,
		Title:           title,
		MetaDescription: metaDescription,
		Active:          active,
		SiteURL:         a.Cfg.SiteURL,
		CanonicalURL:    base + r.URL.Path,
		OGImageURL:      base + "/static/img/og-image.png",
	}
}
