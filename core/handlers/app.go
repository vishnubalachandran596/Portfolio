// Package handlers wires the repositories, renderer, and auth into HTTP
// handlers for both the public site and the admin portal.
package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/geo"
	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/core/notify"
	"github.com/vishnubalachandran596/portfolio/core/oauth"
	"github.com/vishnubalachandran596/portfolio/core/payments"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/view"
	"github.com/vishnubalachandran596/portfolio/web"
)

type App struct {
	Cfg *config.Config
	DB  *sql.DB

	View *view.Renderer
	Auth *middleware.Auth

	Profile       *repo.ProfileRepo
	Skills        *repo.SkillGroupRepo
	Experience    *repo.ExperienceRepo
	Education     *repo.EducationRepo
	Features      *repo.HomeFeatureRepo
	Courses       *repo.CourseRepo
	Bookings      *repo.BookingRepo
	Availability  *repo.AvailabilityRepo
	Admins        *repo.AdminRepo
	Purchases     *repo.PurchaseRepo
	Users         *repo.UserRepo
	Articles      *repo.ArticleRepo
	PromoCodes    *repo.PromoCodeRepo
	QuizQuestions *repo.QuizQuestionRepo
	QuizAttempts  *repo.QuizAttemptRepo
	AdminMessages *repo.AdminMessageRepo
	CourseUpdates *repo.CourseUpdateRepo
	Testimonials  *repo.TestimonialRepo
	AuditRequests *repo.AuditRequestRepo

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
	IsUser              bool
	PreviewingAsVisitor bool
	UserName            string
	LinkedInURL         string
	Notifications       []NotificationItem
	UnreadNotifCount    int
	AssetVersion        string
}

// NotificationItem is one entry in the header's notification menu —
// aggregated at request time from a couple of concrete sources (a
// pending booking-reschedule notice, an unread admin message) rather
// than a dedicated notifications table.
type NotificationItem struct {
	Text   string
	URL    string
	Unread bool
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
		Cfg:           cfg,
		DB:            db,
		View:          renderer,
		Auth:          middleware.NewAuth(cfg.JWTSecret, cfg.SessionTTL, cfg.IsProduction()),
		Profile:       repo.NewProfileRepo(db),
		Skills:        repo.NewSkillGroupRepo(db),
		Experience:    repo.NewExperienceRepo(db),
		Education:     repo.NewEducationRepo(db),
		Features:      repo.NewHomeFeatureRepo(db),
		Courses:       repo.NewCourseRepo(db),
		Bookings:      repo.NewBookingRepo(db),
		Availability:  repo.NewAvailabilityRepo(db),
		Admins:        repo.NewAdminRepo(db),
		Purchases:     repo.NewPurchaseRepo(db),
		Users:         repo.NewUserRepo(db),
		Articles:      repo.NewArticleRepo(db),
		PromoCodes:    repo.NewPromoCodeRepo(db),
		QuizQuestions: repo.NewQuizQuestionRepo(db),
		QuizAttempts:  repo.NewQuizAttemptRepo(db),
		AdminMessages: repo.NewAdminMessageRepo(db),
		CourseUpdates: repo.NewCourseUpdateRepo(db),
		Testimonials:  repo.NewTestimonialRepo(db),
		AuditRequests: repo.NewAuditRequestRepo(db),
		Razorpay:      payments.NewRazorpay(cfg.RazorpayKeyID, cfg.RazorpayKeySecret),
		Telegram:      notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID),
		Google:        oauth.NewGoogle(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL),
		Timezone:      loc,
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
	ctx := r.Context()

	// Every page render pays for these, so they run concurrently rather
	// than as sequential DB round-trips: the profile fetch (just for the
	// footer's LinkedIn link) happens on every single page view, and a
	// logged-in visitor also pays for their name lookup and notifications.
	var wg sync.WaitGroup
	var userName, linkedIn string
	var notifications []NotificationItem
	if isUser {
		if uid, ok := a.Auth.UserID(r); ok {
			wg.Add(2)
			go func() {
				defer wg.Done()
				if u, err := a.Users.GetByID(ctx, uid); err == nil {
					userName = u.Name
				}
			}()
			go func() {
				defer wg.Done()
				notifications = a.userNotifications(ctx, uid)
			}()
		}
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if profile, err := a.Profile.Get(ctx); err == nil {
			linkedIn = profile.LinkedInURL
		}
	}()
	wg.Wait()

	unreadCount := 0
	for _, n := range notifications {
		if n.Unread {
			unreadCount++
		}
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
		IsUser:              isUser,
		PreviewingAsVisitor: previewing && a.Auth.IsAuthenticated(r),
		UserName:            userName,
		LinkedInURL:         linkedIn,
		Notifications:       notifications,
		UnreadNotifCount:    unreadCount,
		AssetVersion:        web.AssetVersion,
	}
}

// userNotifications aggregates a visitor's pending notices — a booking
// reschedule they haven't acknowledged, and any unread message from the
// admin — into one list for the header's notification menu. Errors are
// swallowed (an empty notification list is a fine degrade for a page
// that isn't about bookings or messages in the first place).
func (a *App) userNotifications(ctx context.Context, userID int64) []NotificationItem {
	var items []NotificationItem

	if bookings, err := a.Bookings.ListByUserID(ctx, userID); err == nil {
		for _, b := range bookings {
			if !b.RescheduleAcknowledged && b.PreviousSlotStart != nil {
				items = append(items, NotificationItem{
					Text:   "Your booking was rescheduled to " + b.SlotStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM"),
					URL:    "/account#bookings",
					Unread: true,
				})
			}
		}
	}

	if messages, err := a.AdminMessages.ListByUserID(ctx, userID); err == nil {
		for _, m := range messages {
			if !m.IsRead {
				items = append(items, NotificationItem{
					Text:   "New message: " + truncate(m.Body, 60),
					URL:    "/account#messages",
					Unread: true,
				})
			}
		}
	}

	return items
}

// withContactSignature appends a fixed "reach out to me" line pointing at
// the admin's own contact email, if one is on file. Used by every message
// sent to a visitor's account — a course-update notice or a one-off admin
// message alike — so they all close the same way.
func (a *App) withContactSignature(ctx context.Context, body string) string {
	if profile, err := a.Profile.Get(ctx); err == nil && profile.Email != "" {
		body += "\n\nFeel free to reach out to me at " + profile.Email
	}
	return body
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
