package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/web"
)

// Routes builds the full HTTP mux for both the public site and the admin
// portal, wrapped with request logging.
func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", staticCache(http.FileServerFS(web.StaticFS)))
	// Some browsers and bots still probe the root path directly regardless
	// of the <link rel="icon"> tags in <head>.
	mux.Handle("GET /favicon.ico", staticCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/static/img/favicon.ico"
		http.FileServerFS(web.StaticFS).ServeHTTP(w, r2)
	})))

	// Public site.
	mux.HandleFunc("GET /{$}", a.Home)
	mux.HandleFunc("GET /about", a.About)
	mux.HandleFunc("GET /resume.pdf", a.Resume)
	mux.HandleFunc("GET /media/profile-photo", a.ProfilePhoto)
	mux.HandleFunc("GET /media/logo", a.Logo)
	mux.HandleFunc("GET /courses", a.CoursesList)
	mux.HandleFunc("GET /courses/{slug}", a.CourseDetail)
	mux.HandleFunc("GET /courses/{slug}/preview", a.CoursePreview)
	mux.Handle("GET /courses/{slug}/preview/lesson", noindex(http.HandlerFunc(a.CoursePreviewLessonDocument)))
	mux.HandleFunc("POST /courses/{slug}/checkout/order", a.CheckoutCreateOrder)
	mux.HandleFunc("POST /courses/{slug}/checkout/verify", a.CheckoutVerify)
	mux.HandleFunc("POST /courses/{slug}/checkout/quote", a.CheckoutQuote)
	mux.HandleFunc("POST /courses/{slug}/contribute/order", a.CourseContributeOrder)
	mux.HandleFunc("POST /courses/{slug}/contribute/verify", a.CourseContributeVerify)
	mux.Handle("GET /courses/{slug}/enrolled", noindex(http.HandlerFunc(a.CoursePurchaseSuccess)))
	mux.HandleFunc("GET /software", a.SoftwareList)
	mux.HandleFunc("GET /software/{slug}", a.SoftwareDetail)
	mux.HandleFunc("POST /software/{slug}/checkout/order", a.SoftwareCheckoutOrder)
	mux.HandleFunc("POST /software/{slug}/checkout/verify", a.SoftwareCheckoutVerify)
	mux.Handle("GET /courses/{slug}/learn", noindex(http.HandlerFunc(a.CourseLearn)))
	mux.Handle("GET /courses/{slug}/learn/lesson", noindex(http.HandlerFunc(a.CourseLessonDocument)))
	mux.Handle("GET /courses/{slug}/learn/compiler", noindex(http.HandlerFunc(a.CourseCompilerEmbed)))
	mux.Handle("GET /courses/{slug}/quiz", noindex(http.HandlerFunc(a.QuizPage)))
	mux.Handle("POST /courses/{slug}/quiz", noindex(http.HandlerFunc(a.QuizSubmit)))
	mux.Handle("POST /courses/{slug}/quiz/start", noindex(http.HandlerFunc(a.QuizStart)))
	mux.HandleFunc("POST /courses/{slug}/review", a.CourseReviewSubmit)
	mux.Handle("GET /certificate/verify/{code}", noindex(http.HandlerFunc(a.CertificateVerify)))
	mux.Handle("GET /courses/{slug}/certificate", noindex(http.HandlerFunc(a.CertificateDownload)))
	mux.HandleFunc("GET /consultancy", a.ConsultancyPage)
	// Public booking: no login. /booking is indexable; the private manage
	// links are noindex, uncached and send no Referer (booking_manage.go).
	mux.HandleFunc("GET /booking", a.BookingPage)
	mux.HandleFunc("POST /booking/confirm", a.BookingConfirm)
	mux.Handle("GET /booking/manage/{token}", manageGuard(http.HandlerFunc(a.BookingManagePage)))
	mux.Handle("POST /booking/manage/{token}/cancel", manageGuard(http.HandlerFunc(a.BookingManageCancel)))
	mux.Handle("GET /booking/manage/{token}/reschedule", manageGuard(http.HandlerFunc(a.BookingManageReschedulePage)))
	mux.Handle("POST /booking/manage/{token}/reschedule", manageGuard(http.HandlerFunc(a.BookingManageRescheduleSubmit)))
	mux.HandleFunc("GET /articles", a.ArticlesPage)
	mux.HandleFunc("GET /articles/{slug}", a.ArticleDetailPage)
	mux.HandleFunc("POST /articles/{slug}/thumbs-up", a.ArticleThumbsUp)
	mux.HandleFunc("POST /articles/{slug}/checkout/order", a.ArticleContributeOrder)
	mux.HandleFunc("POST /articles/{slug}/checkout/verify", a.ArticleContributeVerify)
	mux.HandleFunc("GET /contact", a.ContactPage)
	mux.HandleFunc("POST /contact", a.ContactSubmit)
	mux.HandleFunc("POST /subscribe", a.NewsletterSubscribe)
	mux.HandleFunc("GET /ai-assistant", a.AIAssistantPage)
	mux.Handle("POST /chat", noindex(http.HandlerFunc(a.Chat)))
	mux.Handle("POST /api/chatbot/reindex", noindex(http.HandlerFunc(a.APIChatbotReindex)))
	mux.Handle("GET /internal/chatbot/cron-reindex", noindex(http.HandlerFunc(a.CronChatbotReindex)))
	mux.Handle("GET /search", noindex(http.HandlerFunc(a.Search)))
	mux.Handle("GET /account", noindex(a.Auth.RequireUser(http.HandlerFunc(a.AccountPage))))
	mux.Handle("GET /account/bookings/{id}/receipt.pdf", a.Auth.RequireUser(http.HandlerFunc(a.AccountBookingReceipt)))
	mux.Handle("GET /account/purchases/{id}/receipt.pdf", a.Auth.RequireUser(http.HandlerFunc(a.AccountPurchaseReceipt)))
	mux.Handle("GET /account/claim", noindex(a.Auth.RequireUser(http.HandlerFunc(a.AccountClaimPage))))
	mux.Handle("POST /account/claim", noindex(a.Auth.RequireUser(http.HandlerFunc(a.AccountClaimSubmit))))
	mux.Handle("GET /account/licences", a.Auth.RequireUser(http.HandlerFunc(a.AccountLicences)))
	mux.Handle("POST /account/licences/{id}/issue", a.Auth.RequireUser(http.HandlerFunc(a.AccountLicenceIssue)))
	mux.Handle("GET /account/licences/{id}/download", a.Auth.RequireUser(http.HandlerFunc(a.AccountLicenceDownload)))
	mux.Handle("GET /account/software/{id}/receipt.pdf", a.Auth.RequireUser(http.HandlerFunc(a.AccountSoftwareReceipt)))
	mux.Handle("GET /account/articles/{id}/receipt.pdf", a.Auth.RequireUser(http.HandlerFunc(a.AccountArticleReceipt)))
	mux.Handle("POST /account/testimonials", a.Auth.RequireUser(http.HandlerFunc(a.AccountTestimonialSubmit)))
	mux.Handle("POST /account/bookings/{id}/acknowledge", a.Auth.RequireUser(http.HandlerFunc(a.AccountAcknowledgeReschedule)))
	mux.Handle("GET /account/bookings/{id}/reschedule", noindex(a.Auth.RequireUser(http.HandlerFunc(a.AccountReschedulePage))))
	mux.Handle("POST /account/bookings/{id}/reschedule", a.Auth.RequireUser(http.HandlerFunc(a.AccountRescheduleSubmit)))

	// Visitor auth.
	mux.Handle("GET /exit-preview", noindex(http.HandlerFunc(a.ExitVisitorPreview)))
	mux.Handle("GET /login", noindex(http.HandlerFunc(a.LoginPage)))
	mux.Handle("POST /login", noindex(http.HandlerFunc(a.PasswordLoginSubmit)))
	mux.Handle("GET /signup", noindex(http.HandlerFunc(a.SignupPage)))
	mux.Handle("POST /signup", noindex(http.HandlerFunc(a.SignupSubmit)))
	mux.Handle("GET /forgot", noindex(http.HandlerFunc(a.ForgotPage)))
	mux.Handle("POST /forgot", noindex(http.HandlerFunc(a.ForgotQuestion)))
	mux.Handle("POST /forgot/reset", noindex(http.HandlerFunc(a.ForgotReset)))
	mux.Handle("GET /account/security", noindex(a.Auth.RequireUser(http.HandlerFunc(a.AccountSecurityPage))))
	mux.Handle("POST /account/security", noindex(a.Auth.RequireUser(http.HandlerFunc(a.AccountSecuritySubmit))))
	mux.Handle("GET /account/delete", noindex(a.Auth.RequireUser(http.HandlerFunc(a.AccountDeletePage))))
	mux.Handle("POST /account/delete", noindex(a.Auth.RequireUser(http.HandlerFunc(a.AccountDeleteSubmit))))
	mux.Handle("GET /login/google", noindex(http.HandlerFunc(a.GoogleLoginStart)))
	mux.Handle("GET /login/google/callback", noindex(http.HandlerFunc(a.GoogleLoginCallback)))
	mux.HandleFunc("POST /logout", a.UserLogout)

	// SEO.
	mux.HandleFunc("GET /sitemap.xml", a.Sitemap)
	mux.HandleFunc("GET /robots.txt", a.Robots)
	mux.HandleFunc("GET /llms.txt", a.LLMsTxt)
	mux.HandleFunc("GET /"+indexNowKey+".txt", a.IndexNowKeyFile)

	// Legal / compliance.
	mux.HandleFunc("GET /privacy", a.LegalPage("privacy", "Privacy Policy"))
	mux.HandleFunc("GET /temple-accounts/privacy", a.LegalPage("temple-accounts-privacy", "Temple Accounts Privacy Policy"))
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
	protected.HandleFunc("GET /admin/courses/bulk-upload", a.AdminCourseBulkPage)
	protected.HandleFunc("POST /admin/courses/bulk-upload", a.AdminCourseBulkUpload)
	protected.HandleFunc("POST /admin/courses/rebuild-previews", a.AdminCoursesRebuildPreviews)
	protected.HandleFunc("POST /admin/courses/{id}/rebuild-preview", a.AdminCoursesRebuildPreviews)
	protected.HandleFunc("GET /admin/courses/new", a.AdminCourseNewPage)
	protected.HandleFunc("POST /admin/courses/new", a.AdminCourseSave)
	protected.HandleFunc("GET /admin/courses/{id}/preview", a.AdminCoursePreview)
	protected.HandleFunc("GET /admin/courses/{id}/preview/lesson", a.AdminCourseLessonDocument)
	protected.HandleFunc("GET /admin/courses/{id}/edit", a.AdminCourseEditPage)
	protected.HandleFunc("POST /admin/courses/{id}/edit", a.AdminCourseSave)
	protected.HandleFunc("POST /admin/courses/{id}/delete", a.AdminCourseDelete)
	protected.HandleFunc("GET /admin/software", a.AdminSoftwareList)
	protected.HandleFunc("GET /admin/software/new", a.AdminSoftwareNewPage)
	protected.HandleFunc("POST /admin/software/new", a.AdminSoftwareSave)
	protected.HandleFunc("GET /admin/software/{id}/edit", a.AdminSoftwareEditPage)
	protected.HandleFunc("POST /admin/software/{id}/edit", a.AdminSoftwareSave)
	protected.HandleFunc("POST /admin/software/{id}/delete", a.AdminSoftwareDelete)
	protected.HandleFunc("GET /admin/licences", a.AdminLicences)
	protected.HandleFunc("POST /admin/licences/add", a.AdminLicenceAdd)
	protected.HandleFunc("GET /admin/licences/keygen", a.AdminLicenceKeygenPage)
	protected.HandleFunc("POST /admin/licences/keygen", a.AdminLicenceKeygen)
	protected.HandleFunc("POST /admin/licences/{id}/reissue", a.AdminLicenceReissue)
	protected.HandleFunc("POST /admin/licences/{id}/revoke", a.AdminLicenceRevoke)
	protected.HandleFunc("GET /admin/licences/{id}/download", a.AdminLicenceDownload)
	protected.HandleFunc("GET /admin/courses/{courseID}/quiz", a.AdminQuizList)
	protected.HandleFunc("POST /admin/courses/{courseID}/quiz/import", a.AdminQuizImport)
	protected.HandleFunc("POST /admin/courses/{courseID}/quiz/assign-sets", a.AdminQuizAssignSets)
	protected.HandleFunc("GET /admin/courses/{courseID}/quiz/new", a.AdminQuizNewPage)
	protected.HandleFunc("POST /admin/courses/{courseID}/quiz/new", a.AdminQuizSave)
	protected.HandleFunc("GET /admin/courses/{courseID}/quiz/{id}/edit", a.AdminQuizEditPage)
	protected.HandleFunc("POST /admin/courses/{courseID}/quiz/{id}/edit", a.AdminQuizSave)
	protected.HandleFunc("POST /admin/courses/{courseID}/quiz/{id}/delete", a.AdminQuizDelete)
	protected.HandleFunc("GET /admin/courses/{courseID}/reviews", a.AdminCourseReviewsList)
	protected.HandleFunc("GET /admin/bookings", a.AdminBookingsList)
	protected.HandleFunc("POST /admin/bookings/{id}/status", a.AdminBookingStatus)
	protected.HandleFunc("GET /admin/bookings/{id}/reschedule", a.AdminBookingReschedulePage)
	protected.HandleFunc("POST /admin/bookings/{id}/reschedule", a.AdminBookingRescheduleSubmit)
	protected.HandleFunc("GET /admin/purchases", a.AdminPurchasesList)
	protected.HandleFunc("GET /admin/payments", a.AdminPaymentsList)
	protected.HandleFunc("GET /admin/contact-messages", a.AdminContactMessagesList)
	protected.HandleFunc("POST /admin/contact-messages/{id}/read", a.AdminContactMessageMarkRead)
	protected.HandleFunc("GET /admin/users", a.AdminUsersList)
	protected.HandleFunc("GET /admin/users/{id}", a.AdminUserDetail)
	protected.HandleFunc("POST /admin/users/{id}/promote", a.AdminUserPromote)
	protected.HandleFunc("POST /admin/users/{id}/demote", a.AdminUserDemote)
	protected.HandleFunc("POST /admin/users/{id}/messages", a.AdminUserMessageSend)
	protected.HandleFunc("POST /admin/users/{id}/quiz-grants", a.AdminUserQuizGrant)
	protected.HandleFunc("GET /admin/skills", a.AdminSkillsList)
	protected.HandleFunc("GET /admin/skills/new", a.AdminSkillNewPage)
	protected.HandleFunc("POST /admin/skills/new", a.AdminSkillSave)
	protected.HandleFunc("GET /admin/skills/{id}/edit", a.AdminSkillEditPage)
	protected.HandleFunc("POST /admin/skills/{id}/edit", a.AdminSkillSave)
	protected.HandleFunc("POST /admin/skills/{id}/delete", a.AdminSkillDelete)
	protected.HandleFunc("GET /admin/testimonials", a.AdminTestimonialsList)
	protected.HandleFunc("GET /admin/testimonials/new", a.AdminTestimonialNewPage)
	protected.HandleFunc("POST /admin/testimonials/new", a.AdminTestimonialSave)
	protected.HandleFunc("GET /admin/testimonials/{id}/edit", a.AdminTestimonialEditPage)
	protected.HandleFunc("POST /admin/testimonials/{id}/edit", a.AdminTestimonialSave)
	protected.HandleFunc("POST /admin/testimonials/{id}/approve", a.AdminTestimonialApprove)
	protected.HandleFunc("POST /admin/testimonials/{id}/delete", a.AdminTestimonialDelete)

	protected.HandleFunc("GET /admin/highlights", a.AdminHighlightsList)
	protected.HandleFunc("GET /admin/highlights/new", a.AdminHighlightNewPage)
	protected.HandleFunc("POST /admin/highlights/new", a.AdminHighlightSave)
	protected.HandleFunc("GET /admin/highlights/{id}/edit", a.AdminHighlightEditPage)
	protected.HandleFunc("POST /admin/highlights/{id}/edit", a.AdminHighlightSave)
	protected.HandleFunc("POST /admin/highlights/{id}/delete", a.AdminHighlightDelete)
	protected.HandleFunc("GET /admin/certifications", a.AdminCertificationsList)
	protected.HandleFunc("GET /admin/certifications/new", a.AdminCertificationNewPage)
	protected.HandleFunc("POST /admin/certifications/new", a.AdminCertificationSave)
	protected.HandleFunc("GET /admin/certifications/{id}/edit", a.AdminCertificationEditPage)
	protected.HandleFunc("POST /admin/certifications/{id}/edit", a.AdminCertificationSave)
	protected.HandleFunc("POST /admin/certifications/{id}/delete", a.AdminCertificationDelete)

	protected.HandleFunc("GET /admin/publications", a.AdminPublicationsList)
	protected.HandleFunc("GET /admin/publications/new", a.AdminPublicationNewPage)
	protected.HandleFunc("POST /admin/publications/new", a.AdminPublicationSave)
	protected.HandleFunc("GET /admin/publications/{id}/edit", a.AdminPublicationEditPage)
	protected.HandleFunc("POST /admin/publications/{id}/edit", a.AdminPublicationSave)
	protected.HandleFunc("POST /admin/publications/{id}/delete", a.AdminPublicationDelete)

	protected.HandleFunc("GET /admin/case-studies", a.AdminCaseStudiesList)
	protected.HandleFunc("GET /admin/case-studies/new", a.AdminCaseStudyNewPage)
	protected.HandleFunc("POST /admin/case-studies/new", a.AdminCaseStudySave)
	protected.HandleFunc("GET /admin/case-studies/{id}/edit", a.AdminCaseStudyEditPage)
	protected.HandleFunc("POST /admin/case-studies/{id}/edit", a.AdminCaseStudySave)
	protected.HandleFunc("POST /admin/case-studies/{id}/delete", a.AdminCaseStudyDelete)

	protected.HandleFunc("GET /admin/subscribers", a.AdminSubscribersList)
	protected.HandleFunc("POST /admin/subscribers/{id}/delete", a.AdminSubscriberDelete)
	protected.HandleFunc("GET /admin/experience", a.AdminExperienceList)
	protected.HandleFunc("GET /admin/experience/new", a.AdminExperienceNewPage)
	protected.HandleFunc("POST /admin/experience/new", a.AdminExperienceSave)
	protected.HandleFunc("GET /admin/resumes/{roleKey}", a.AdminResumePreview)
	protected.HandleFunc("GET /admin/experience/{id}/edit", a.AdminExperienceEditPage)
	protected.HandleFunc("POST /admin/experience/{id}/edit", a.AdminExperienceSave)
	protected.HandleFunc("POST /admin/experience/{id}/delete", a.AdminExperienceDelete)
	protected.HandleFunc("GET /admin/promo-codes", a.AdminPromoList)
	protected.HandleFunc("GET /admin/promo-codes/new", a.AdminPromoNewPage)
	protected.HandleFunc("POST /admin/promo-codes/new", a.AdminPromoSave)
	protected.HandleFunc("GET /admin/promo-codes/{id}/edit", a.AdminPromoEditPage)
	protected.HandleFunc("POST /admin/promo-codes/{id}/edit", a.AdminPromoSave)
	protected.HandleFunc("POST /admin/promo-codes/{id}/delete", a.AdminPromoDelete)
	protected.HandleFunc("GET /admin/articles", a.AdminArticlesList)
	protected.HandleFunc("GET /admin/articles/new", a.AdminArticleNewPage)
	protected.HandleFunc("POST /admin/articles/new", a.AdminArticleSave)
	protected.HandleFunc("GET /admin/articles/{id}/preview", a.AdminArticlePreview)
	protected.HandleFunc("GET /admin/articles/{id}/edit", a.AdminArticleEditPage)
	protected.HandleFunc("POST /admin/articles/{id}/edit", a.AdminArticleSave)
	protected.HandleFunc("POST /admin/articles/{id}/delete", a.AdminArticleDelete)
	protected.HandleFunc("GET /admin/availability", a.AdminAvailabilityList)
	protected.HandleFunc("POST /admin/availability/new", a.AdminAvailabilityCreate)
	protected.HandleFunc("POST /admin/availability/{id}/delete", a.AdminAvailabilityDelete)
	protected.HandleFunc("POST /admin/availability/{id}/toggle", a.AdminAvailabilityToggle)
	protected.HandleFunc("GET /admin/chatbot", a.AdminChatbotPage)
	protected.HandleFunc("POST /admin/chatbot/reindex", a.AdminChatbotReindex)
	protected.HandleFunc("GET /admin/profile", a.AdminProfilePage)
	protected.HandleFunc("POST /admin/profile", a.AdminProfileSave)
	protected.HandleFunc("GET /admin/preview-visitor", a.AdminPreviewAsVisitor)
	protected.HandleFunc("GET /admin/seo", a.AdminSEOPage)
	protected.HandleFunc("POST /admin/seo", a.AdminSEOSave)
	adminMux.Handle("/admin/", a.Auth.RequireAdmin(protected))

	mux.Handle("/admin/", noindex(adminMux))

	mux.HandleFunc("/", a.NotFound)

	siteHost := ""
	if u, err := url.Parse(a.Cfg.SiteURL); err == nil {
		siteHost = u.Host
	}
	return middleware.Logging(middleware.SecurityHeaders(
		middleware.CanonicalHost(a.Cfg.SiteURL)(
			middleware.DomainRouter(a.Cfg.Domain)(
				middleware.SameOrigin(siteHost)(middleware.LegacyRedirects(a.Auth.Attach(a.redirectAccessToken(mux))))))))
}

// noindex tags every response under a handler as non-indexable — used for
// admin, auth, account, search, and gated flows that should never appear in
// search results.
func noindex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		next.ServeHTTP(w, markNoindex(r))
	})
}

// staticCache marks every static asset cacheable forever. This is only
// safe because every CSS/JS URL is requested with a "?v=" query string
// derived from the file's own content (web.AssetVersion) — the exact URL
// only ever repeats when the bytes behind it are identical, so there's no
// way for a visitor to get stuck on a stale file after a deploy.
func staticCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		// Fonts are fetched in CORS mode, and the sandboxed lesson iframe
		// has an opaque ("null") origin — without this the lesson text
		// fell back to system fonts. Public files, so "*" is safe.
		if strings.HasPrefix(r.URL.Path, "/static/fonts/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		next.ServeHTTP(w, r)
	})
}
