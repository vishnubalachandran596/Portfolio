// Package handlers wires the repositories, renderer, and auth into HTTP
// handlers for both the public site and the admin portal.
package handlers

import (
	"context"
	"database/sql"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/ai"
	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/licence"
	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/notify"
	"github.com/vishnubalachandran596/portfolio/core/oauth"
	"github.com/vishnubalachandran596/portfolio/core/payments"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/storage"
	"github.com/vishnubalachandran596/portfolio/core/view"
	"github.com/vishnubalachandran596/portfolio/web"
)

type App struct {
	Cfg *config.Config
	DB  *sql.DB

	View *view.Renderer
	Auth *middleware.Auth

	Profile             *repo.ProfileRepo
	Skills              *repo.SkillGroupRepo
	Experience          *repo.ExperienceRepo
	Resumes             *repo.RoleResumeRepo
	Education           *repo.EducationRepo
	Features            *repo.HomeFeatureRepo
	Courses             *repo.CourseRepo
	SlugRedirects       *repo.SlugRedirectRepo
	Software            *repo.SoftwareRepo
	SoftwarePurchases   *repo.SoftwarePurchaseRepo
	Bookings            *repo.BookingRepo
	Availability        *repo.AvailabilityRepo
	Admins              *repo.AdminRepo
	Purchases           *repo.PurchaseRepo
	Users               *repo.UserRepo
	AuthThrottle        *repo.AuthThrottleRepo
	Licences            *repo.LicenceRepo
	SEO                 *repo.SEORepo
	Claims              *repo.ClaimRepo
	CourseContributions *repo.CourseContributionRepo
	// LicenceSigner is nil until LICENCE_SIGNING_KEY_B64 and LICENCE_KID are set.
	LicenceSigner        *licence.Signer
	Articles             *repo.ArticleRepo
	PromoCodes           *repo.PromoCodeRepo
	QuizQuestions        *repo.QuizQuestionRepo
	QuizAttempts         *repo.QuizAttemptRepo
	QuizGrants           *repo.QuizGrantRepo
	AdminMessages        *repo.AdminMessageRepo
	CourseUpdates        *repo.CourseUpdateRepo
	Testimonials         *repo.TestimonialRepo
	Certifications       *repo.CertificationRepo
	Highlights           *repo.HighlightRepo
	Publications         *repo.PublicationRepo
	CaseStudies          *repo.CaseStudyRepo
	AuditRequests        *repo.AuditRequestRepo
	ArticleVotes         *repo.ArticleVoteRepo
	ArticleContributions *repo.ArticleContributionRepo
	ContactMessages      *repo.ContactMessageRepo
	Subscribers          *repo.SubscriberRepo
	CourseReviews        *repo.CourseReviewRepo
	ChatbotChunks        *repo.ChatbotChunkRepo
	ChatbotAnswerCache   *repo.ChatbotAnswerCacheRepo
	ChatbotRateLimits    *repo.ChatbotRateLimitRepo
	CourseStorage        *storage.Client

	Razorpay *payments.Razorpay
	Telegram *notify.Telegram
	Google   *oauth.Google
	Nvidia   *ai.Client
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
	IsLoggedIn          bool
	IsAdmin             bool
	IsUser              bool
	PreviewingAsVisitor bool
	UserName            string
	LinkedInURL         string
	// HasLogo is true once a logo has been uploaded via /admin/profile.
	// header.html shows the image logo (GET /media/logo) only then;
	// otherwise it falls back to the text brand mark, same as today.
	HasLogo          bool
	Notifications    []NotificationItem
	UnreadNotifCount int
	AssetVersion     string
	// NoIndex adds a robots noindex meta tag (the noindex() route wrapper
	// also sets the X-Robots-Tag header).
	NoIndex bool
	// PromoCodes is every currently active, public promo code — the
	// source for the sitewide scrolling banner (promo_banner.html),
	// rendered just below the header on every page. Empty renders no
	// banner at all.
	PromoCodes []models.PromoCode
	// ChatbotEnabled gates the floating chat widget's markup/JS -- hidden
	// entirely until NVIDIA_API_KEY is configured (core/ai.Client.Enabled()).
	ChatbotEnabled bool
	// ChatbotFullPage is true only for AIAssistantPage's own render. It
	// switches chatbot_widget.html (templates/layout) between its two
	// shells -- a floating bubble everywhere else, or this page's entire
	// main content -- and tells base.html not to also render the floating
	// bubble on top of that inline chat. False (zero value) everywhere
	// else, so no other handler needs to set it.
	ChatbotFullPage bool
	// Immersive is set by the course reader: base.html marks the body
	// .is-immersive, which hides the promo bar, footer and floating
	// widgets so the lesson can fill the viewport under the header.
	Immersive bool
	// RobotsMeta, when set, renders <meta name="robots" content="…">.
	// Set by baseFrom for pages wrapped in the noindex middleware (the
	// header alone is invisible to anyone reading the HTML), for a
	// page_seo noindex override, and for internal-search listing views.
	RobotsMeta string
	// NoCanonical drops <link rel="canonical"> and og:url — set on 404s,
	// which must not declare themselves canonical.
	NoCanonical bool
	// BreadcrumbJSONLD is an optional BreadcrumbList block, emitted in
	// the head by base.html when a handler sets it.
	BreadcrumbJSONLD template.JS
}

// SubscribeFormData backs the shared subscribe_form partial embedded on
// Home, Courses, and Articles — Source is the fixed label NewsletterSubscribe
// switches on to pick a redirect target, and Subscribed drives the one-line
// "thanks" banner after a successful POST /subscribe back to this page.
type SubscribeFormData struct {
	Source     string
	Subscribed bool
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
	Software    string
	Consultancy string
	Booking     string
	Articles    string
	Contact     string
	AIAssistant string
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
	auth := middleware.NewAuth(cfg.JWTSecret, cfg.SessionTTL, cfg.IsProduction())
	users := repo.NewUserRepo(db)
	admins := repo.NewAdminRepo(db)
	auth.SetVerifiers(
		func(ctx context.Context, id int64) (int, bool) { return users.TokenVersion(ctx, id) },
		func(ctx context.Context, id int64) bool { _, err := admins.GetByID(ctx, id); return err == nil },
	)
	return &App{
		Cfg:                  cfg,
		DB:                   db,
		View:                 renderer,
		Auth:                 auth,
		AuthThrottle:         repo.NewAuthThrottleRepo(db),
		Licences:             repo.NewLicenceRepo(db),
		SEO:                  repo.NewSEORepo(db),
		Claims:               repo.NewClaimRepo(db),
		CourseContributions:  repo.NewCourseContributionRepo(db),
		Profile:              repo.NewProfileRepo(db),
		Skills:               repo.NewSkillGroupRepo(db),
		Experience:           repo.NewExperienceRepo(db),
		Resumes:              repo.NewRoleResumeRepo(db),
		Education:            repo.NewEducationRepo(db),
		Features:             repo.NewHomeFeatureRepo(db),
		Courses:              repo.NewCourseRepo(db),
		SlugRedirects:        repo.NewSlugRedirectRepo(db),
		Software:             repo.NewSoftwareRepo(db),
		SoftwarePurchases:    repo.NewSoftwarePurchaseRepo(db),
		Bookings:             repo.NewBookingRepo(db),
		Availability:         repo.NewAvailabilityRepo(db),
		Admins:               admins,
		Purchases:            repo.NewPurchaseRepo(db),
		Users:                users,
		Articles:             repo.NewArticleRepo(db),
		PromoCodes:           repo.NewPromoCodeRepo(db),
		QuizQuestions:        repo.NewQuizQuestionRepo(db),
		QuizAttempts:         repo.NewQuizAttemptRepo(db),
		QuizGrants:           repo.NewQuizGrantRepo(db),
		AdminMessages:        repo.NewAdminMessageRepo(db),
		CourseUpdates:        repo.NewCourseUpdateRepo(db),
		Testimonials:         repo.NewTestimonialRepo(db),
		Certifications:       repo.NewCertificationRepo(db),
		Highlights:           repo.NewHighlightRepo(db),
		Publications:         repo.NewPublicationRepo(db),
		CaseStudies:          repo.NewCaseStudyRepo(db),
		AuditRequests:        repo.NewAuditRequestRepo(db),
		ArticleVotes:         repo.NewArticleVoteRepo(db),
		ArticleContributions: repo.NewArticleContributionRepo(db),
		ContactMessages:      repo.NewContactMessageRepo(db),
		Subscribers:          repo.NewSubscriberRepo(db),
		CourseReviews:        repo.NewCourseReviewRepo(db),
		ChatbotChunks:        repo.NewChatbotChunkRepo(db),
		ChatbotAnswerCache:   repo.NewChatbotAnswerCacheRepo(db),
		ChatbotRateLimits:    repo.NewChatbotRateLimitRepo(db),
		CourseStorage:        storage.New(cfg),
		Razorpay:             payments.NewRazorpay(cfg.RazorpayKeyID, cfg.RazorpayKeySecret),
		Telegram:             notify.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID),
		Google:               oauth.NewGoogle(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL),
		Nvidia:               ai.New(cfg.NvidiaAPIKey),
		Timezone:             loc,
		LicenceSigner:        newLicenceSigner(cfg),
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

// basePrefetch is base()'s own DB work while it is still in flight. A
// handler starts it *before* its own concurrent lookups and finishes it
// with baseFrom afterwards, so the chrome's round trips ride in the same
// wave as the page's own instead of forming a second one behind them —
// see docs/ARCHITECTURE.md §9.
type basePrefetch struct {
	wg sync.WaitGroup

	previewing bool
	isAdmin    bool
	isUser     bool

	// profile is where the footer's LinkedIn link is read from. It points
	// either at ownProfile (fetched by startBase) or at a variable the
	// caller's own errgroup fills in (startBaseWithProfile) — which is
	// what keeps the profile row to one fetch per request on the pages
	// that render it. Read only after baseFrom has waited.
	profile    *models.Profile
	ownProfile models.Profile

	userName      string
	notifications []NotificationItem
	promoCodes    []models.PromoCode

	// pageSEO is the page_seo override for a static page, nil when the
	// path has none (or is not a static SEO page at all).
	pageSEO *models.PageSEO
}

// startBase begins the lookups every page's chrome needs, for a handler
// that doesn't load the profile row itself.
//
// These stay on their own sync.WaitGroup rather than joining the caller's
// errgroup on purpose: the contract here is "degrade quietly" — a missing
// LinkedIn link or an empty notification menu is a fine outcome for a page
// that isn't about either, whereas the page's own lookups must fail the
// render. Folding these into the errgroup would silently promote a
// swallowed error into a 500.
func (a *App) startBase(r *http.Request) *basePrefetch {
	pf := a.startBaseChrome(r)
	pf.profile = &pf.ownProfile
	pf.wg.Add(1)
	go func() {
		defer pf.wg.Done()
		if profile, err := a.Profile.Get(r.Context()); err == nil {
			pf.ownProfile = profile
		}
	}()
	return pf
}

// startBaseWithProfile is startBase for a handler that already loads the
// profile row in its own concurrent wave (Home, About): it reads the
// footer's LinkedIn link off that row instead of fetching the same
// single-row table a second time.
//
// profile must be filled in by the time baseFrom is called — i.e. the
// caller's g.Wait() has to have returned — which is exactly where every
// caller renders from.
func (a *App) startBaseWithProfile(r *http.Request, profile *models.Profile) *basePrefetch {
	pf := a.startBaseChrome(r)
	pf.profile = profile
	return pf
}

func (a *App) startBaseChrome(r *http.Request) *basePrefetch {
	previewing := a.isPreviewingAsVisitor(r)
	pf := &basePrefetch{
		previewing: previewing,
		isAdmin:    a.Auth.IsAuthenticated(r) && !previewing,
		isUser:     a.Auth.IsUserAuthenticated(r),
	}
	if pf.isUser {
		if uid, ok := a.Auth.UserID(r); ok {
			ctx := r.Context()
			pf.wg.Add(2)
			go func() {
				defer pf.wg.Done()
				if u, err := a.Users.GetByID(ctx, uid); err == nil {
					pf.userName = u.Name
				}
			}()
			go func() {
				defer pf.wg.Done()
				pf.notifications = a.userNotifications(ctx, uid)
			}()
		}
	}
	// Best-effort, same as the notifications/profile lookups above: a
	// missing banner on one request is a fine outcome, so this never
	// fails the render.
	ctx := r.Context()
	if pageSEOApplies(r) {
		pf.wg.Add(1)
		go func() {
			defer pf.wg.Done()
			if p, err := a.SEO.GetPage(ctx, r.URL.Path); err == nil {
				pf.pageSEO = &p
			}
		}()
	}
	pf.wg.Add(1)
	go func() {
		defer pf.wg.Done()
		if codes, err := a.PromoCodes.ListActivePublic(ctx); err == nil {
			pf.promoCodes = codes
		}
	}()
	return pf
}

// base is the one-shot form, for handlers with no concurrent wave of their
// own to join: admin pages, error pages, and form re-renders. Everything
// on the public hot path should start the prefetch alongside its own
// lookups and finish with baseFrom instead.
func (a *App) base(r *http.Request, title, metaDescription, active string) BaseData {
	return a.baseFrom(r, a.startBase(r), title, metaDescription, active)
}

// baseFrom completes a prefetch and assembles the shared chrome. Call it
// after the handler's own errgroup has returned; by then the prefetch has
// almost always finished too, so it contributes no round trip of its own.
func (a *App) baseFrom(r *http.Request, pf *basePrefetch, title, metaDescription, active string) BaseData {
	pf.wg.Wait()

	base := a.baseURL(r)
	linkedIn := ""
	hasLogo := false
	if pf.profile != nil {
		linkedIn = pf.profile.LinkedInURL
		hasLogo = pf.profile.LogoObject != ""
	}
	unreadCount := 0
	for _, n := range pf.notifications {
		if n.Unread {
			unreadCount++
		}
	}
	ogImage := base + defaultOGImagePath
	robots := robotsMetaForRequest(r)
	if p := pf.pageSEO; p != nil {
		title = p.Title(title)
		if d := strings.TrimSpace(p.MetaDescription); d != "" {
			metaDescription = d
		}
		if img := p.Image(""); img != "" {
			ogImage = absoluteURL(base, img)
		}
		if p.Noindex && robots == "" {
			robots = "noindex, follow"
		}
	}
	return BaseData{
		SiteName:        a.Cfg.SiteName,
		Title:           title,
		MetaDescription: metaDescription,
		Active:          active,
		SiteURL:         a.Cfg.SiteURL,
		CanonicalURL:    canonicalURLForRequest(base, r),
		OGImageURL:      ogImage,
		RobotsMeta:      robots,
		Nav: NavLinks{
			Home:        a.pageURL(r, "/"),
			About:       a.pageURL(r, "/about"),
			Courses:     a.pageURL(r, "/courses"),
			Software:    a.pageURL(r, "/software"),
			Consultancy: a.pageURL(r, "/consultancy"),
			Booking:     a.pageURL(r, "/booking"),
			Articles:    a.pageURL(r, "/articles"),
			Contact:     a.pageURL(r, "/contact"),
			AIAssistant: a.pageURL(r, "/ai-assistant"),
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
		IsLoggedIn:          pf.isUser || pf.isAdmin,
		IsAdmin:             pf.isAdmin,
		IsUser:              pf.isUser,
		PreviewingAsVisitor: pf.previewing && a.Auth.IsAuthenticated(r),
		UserName:            pf.userName,
		LinkedInURL:         linkedIn,
		HasLogo:             hasLogo,
		Notifications:       pf.notifications,
		UnreadNotifCount:    unreadCount,
		AssetVersion:        web.AssetVersion,
		PromoCodes:          pf.promoCodes,
		ChatbotEnabled:      a.Nvidia.Enabled(),
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
