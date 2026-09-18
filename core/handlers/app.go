// Package handlers wires the repositories, renderer, and auth into HTTP
// handlers for both the public site and the admin portal.
package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/geo"
	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/core/notify"
	"github.com/vishnubalachandran596/portfolio/core/oauth"
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
	Users        *repo.UserRepo
	Articles     *repo.ArticleRepo

	Razorpay *payments.Razorpay
	Telegram *notify.Telegram
	Google   *oauth.Google
	Timezone *time.Location
}

// BaseData carries the fields every page template needs. Page-specific
// data structs embed it so templates can reach both the shared chrome and
// the page's own data through one dot.
type BaseData struct {
	SiteName            string
	Title               string
	MetaDescription     string
	Active              string
	SiteURL             string
	CanonicalURL        string
	OGImageURL          string
	Nav                 NavLinks
	Country             string
	IsLoggedIn          bool
	IsAdmin             bool
	PreviewingAsVisitor bool
	UserName            string
	LinkedInURL         string
}

// NavLinks holds the absolute URL for each primary nav destination, so
// header/footer links jump straight to the right subdomain instead of
// bouncing through a redirect (e.g. clicking "About" from home.domain
// goes directly to about.domain, not home.domain/about -> redirect).
// Root-relative when no custom domain is configured.
type NavLinks struct {
	Home        string
	About       string
	Courses     string
	Consultancy string
	Booking     string
	Articles    string
	Contact     string
	Search      string
	Account     string
	Admin       string
	Login       string
	Logout      string
	Privacy     string
	Terms       string
	Refund      string
	Cookies     string
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
		Users:        repo.NewUserRepo(db),
		Articles:     repo.NewArticleRepo(db),
		Razorpay:     payments.NewRazorpay(cfg.RazorpayKeyID, cfg.RazorpayKeySecret),
		Telegram:     notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID),
		Google:       oauth.NewGoogle(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL),
		Timezone:     loc,
	}
}

// previewVisitorCookie, when present, makes an authenticated admin see the
// site exactly as a normal visitor would — no Admin Console tab, no
// automatic admin-preview access to gated course content — without
// actually clearing their admin session. See AdminPreviewAsVisitor /
// ExitVisitorPreview.
const previewVisitorCookie = "preview_visitor"

func (a *App) isPreviewingAsVisitor(r *http.Request) bool {
	c, err := r.Cookie(previewVisitorCookie)
	return err == nil && c.Value == "1"
}

func (a *App) base(r *http.Request, title, metaDescription, active string) BaseData {
	base := a.baseURL(r)
	previewing := a.isPreviewingAsVisitor(r)
	isAdmin := a.Auth.IsAuthenticated(r) && !previewing
	isUser := a.Auth.IsUserAuthenticated(r)
	userName := ""
	if isUser {
		if uid, ok := a.Auth.UserID(r); ok {
			if u, err := a.Users.GetByID(r.Context(), uid); err == nil {
				userName = u.Name
			}
		}
	}
	linkedIn := ""
	if profile, err := a.Profile.Get(r.Context()); err == nil {
		linkedIn = profile.LinkedInURL
	}
	return BaseData{
		SiteName:        a.Cfg.SiteName,
		Title:           title,
		MetaDescription: metaDescription,
		Active:          active,
		SiteURL:         a.Cfg.SiteURL,
		CanonicalURL:    base + r.URL.Path,
		OGImageURL:      base + "/static/img/og-image.png",
		Nav: NavLinks{
			Home:        a.pageURL(r, "/"),
			About:       a.pageURL(r, "/about"),
			Courses:     a.pageURL(r, "/courses"),
			Consultancy: a.pageURL(r, "/consultancy"),
			Booking:     a.pageURL(r, "/booking"),
			Articles:    a.pageURL(r, "/articles"),
			Contact:     a.pageURL(r, "/contact"),
			Search:      a.pageURL(r, "/search"),
			Account:     a.pageURL(r, "/account"),
			Admin:       a.pageURL(r, "/admin"),
			Login:       a.pageURL(r, "/login"),
			Logout:      a.pageURL(r, "/logout"),
			Privacy:     a.pageURL(r, "/privacy"),
			Terms:       a.pageURL(r, "/terms"),
			Refund:      a.pageURL(r, "/refund"),
			Cookies:     a.pageURL(r, "/cookies"),
		},
		Country:             geo.CountryFromRequest(r),
		IsLoggedIn:          isUser || isAdmin,
		IsAdmin:             isAdmin,
		PreviewingAsVisitor: previewing && a.Auth.IsAuthenticated(r),
		UserName:            userName,
		LinkedInURL:         linkedIn,
	}
}
