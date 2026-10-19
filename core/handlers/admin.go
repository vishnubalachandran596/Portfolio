package handlers

import (
	"context"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/storage"
)

type AdminLoginData struct {
	BaseData
	Error string
}

func (a *App) AdminLoginPage(w http.ResponseWriter, r *http.Request) {
	if a.Auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_login", AdminLoginData{
		BaseData: a.base(r, "Admin Login", "", ""),
		Error:    r.URL.Query().Get("error"),
	})
}

func (a *App) AdminLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/login?error=Invalid+form+submission", http.StatusSeeOther)
		return
	}
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	password := r.FormValue("password")

	// The password is the ADMIN_BOOTSTRAP_PASSWORD environment secret and
	// nothing stored: it is checked first, in constant time, then the email
	// has to be on the admin list. One generic error covers both.
	ek, ik := authutil.HashKey(email), authutil.HashKey(authutil.ClientIP(r))
	if until, locked := a.throttled(r, throttleAdmin, ek, ik); locked {
		http.Redirect(w, r, "/admin/login?error="+urlQuery(lockedMessage(until)), http.StatusSeeOther)
		return
	}
	passwordOK := authutil.SecretEquals(password, a.Cfg.AdminPassword)
	admin, err := a.Admins.GetByEmail(r.Context(), email)
	if !passwordOK || err != nil {
		a.recordFailure(r, throttleAdmin, loginMaxFails, ek, ik)
		http.Redirect(w, r, "/admin/login?error=Invalid+email+or+password", http.StatusSeeOther)
		return
	}

	a.clearFailures(r, throttleAdmin, ek, ik)
	a.Auth.IssueSession(w, admin.ID)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (a *App) AdminLogout(w http.ResponseWriter, r *http.Request) {
	a.Auth.ClearSession(w)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// AdminPreviewAsVisitor sets a cookie that makes the site behave as if
// the admin were a normal visitor (no Admin Console tab, no automatic
// gated-content access) without touching the actual admin session, so
// they can check what a real visitor sees and come straight back.
func (a *App) AdminPreviewAsVisitor(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     previewVisitorCookie,
		Value:    "1",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.Cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ExitVisitorPreview clears the preview cookie, restoring the admin's
// normal view. Doesn't require re-authentication since the real admin
// session cookie was never touched.
func (a *App) ExitVisitorPreview(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: previewVisitorCookie, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0),
	})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

type AdminDashboardData struct {
	BaseData
	CourseCount              int
	PublishedCourses         int
	ArticleCount             int
	PublishedArticles        int
	UpcomingBookings         []models.Booking
	PendingBookings          int
	ConfirmedBookings        int
	TotalPurchases           int
	PaidPurchases            int
	ArticleThumbs            int
	PaidArticleContributions int
}

func (a *App) AdminDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	courses, err := a.Courses.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load courses", err)
		return
	}
	published := 0
	for _, c := range courses {
		if c.IsPublished {
			published++
		}
	}

	allBookings, err := a.Bookings.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load bookings", err)
		return
	}
	pending := 0
	confirmed := 0
	var upcoming []models.Booking
	for _, b := range allBookings {
		switch b.Status {
		case models.BookingStatusPendingPayment:
			pending++
		case models.BookingStatusConfirmed:
			confirmed++
			if len(upcoming) < 10 {
				upcoming = append(upcoming, b)
			}
		}
	}

	purchases, err := a.Purchases.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load purchases", err)
		return
	}
	paidPurchases := 0
	for _, p := range purchases {
		if p.Status == models.PurchaseStatusPaid {
			paidPurchases++
		}
	}

	articles, err := a.Articles.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load articles", err)
		return
	}
	publishedArticles := 0
	articleThumbs := 0
	for _, art := range articles {
		if art.IsPublished {
			publishedArticles++
		}
		articleThumbs += art.ThumbsUpCount
	}

	paidArticleContributions, err := a.ArticleContributions.CountPaid(ctx)
	if err != nil {
		a.serverError(w, "count article contributions", err)
		return
	}

	a.View.Render(w, http.StatusOK, "admin_dashboard", AdminDashboardData{
		BaseData:                 a.base(r, "Admin Dashboard", "", "admin"),
		CourseCount:              len(courses),
		PublishedCourses:         published,
		ArticleCount:             len(articles),
		PublishedArticles:        publishedArticles,
		ArticleThumbs:            articleThumbs,
		PaidArticleContributions: paidArticleContributions,
		UpcomingBookings:         upcoming,
		PendingBookings:          pending,
		ConfirmedBookings:        confirmed,
		TotalPurchases:           len(purchases),
		PaidPurchases:            paidPurchases,
	})
}

// --- Courses -----------------------------------------------------------

type AdminCoursesListData struct {
	BaseData
	Courses []models.Course
}

func (a *App) AdminCoursesList(w http.ResponseWriter, r *http.Request) {
	courses, err := a.Courses.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load courses", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_courses_list", AdminCoursesListData{
		BaseData: a.base(r, "Courses — Admin", "", "admin"),
		Courses:  courses,
	})
}

type AdminCourseFormData struct {
	BaseData
	Course models.Course
	IsNew  bool
	Error  string
	SEO    models.SEOFields
}

func (a *App) AdminCourseNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_course_form", AdminCourseFormData{
		BaseData: a.base(r, "New Course — Admin", "", "admin"),
		IsNew:    true,
	})
}

func (a *App) AdminCourseEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	course, err := a.Courses.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load course", err)
		return
	}
	seo, _ := a.SEO.GetFields(r.Context(), repo.SEOCourse, id)
	a.View.Render(w, http.StatusOK, "admin_course_form", AdminCourseFormData{
		BaseData: a.base(r, "Edit Course — Admin", "", "admin"),
		Course:   course,
		SEO:      seo,
	})
}

const maxCourseUploadBytes = 8 << 20 // 8MB — comfortably covers even the largest course content file

func (a *App) AdminCourseSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxCourseUploadBytes); err != nil {
		http.Error(w, "invalid form (file may be too large)", http.StatusBadRequest)
		return
	}

	var id int64
	if idStr := r.PathValue("id"); idStr != "" {
		var err error
		id, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			a.NotFound(w, r)
			return
		}
	}
	isEdit := id != 0

	price, _ := strconv.Atoi(r.FormValue("price_cents"))
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	minPriceCents, _ := strconv.Atoi(r.FormValue("min_price_cents"))
	isPaid := r.FormValue("is_paid") == "on"

	course := models.Course{
		ID:            id,
		Slug:          slugify(r.FormValue("slug")),
		Title:         strings.TrimSpace(r.FormValue("title")),
		Summary:       strings.TrimSpace(r.FormValue("summary")),
		Description:   strings.TrimSpace(r.FormValue("description")),
		Level:         strings.TrimSpace(r.FormValue("level")),
		Duration:      strings.TrimSpace(r.FormValue("duration")),
		PriceCents:    price,
		ImageURL:      strings.TrimSpace(r.FormValue("image_url")),
		IsPublished:   r.FormValue("is_published") == "on",
		SortOrder:     sortOrder,
		IsPaid:        isPaid,
		MinPriceCents: minPriceCents,
		RagContext:    strings.TrimSpace(r.FormValue("rag_context")),
		Tags:          parseTags(r.FormValue("tags")),
	}
	if course.Slug == "" {
		course.Slug = slugify(course.Title)
	}
	if course.Title == "" || course.Slug == "" {
		formData := AdminCourseFormData{
			BaseData: a.base(r, "Course — Admin", "", "admin"),
			Course:   course,
			SEO:      seoFieldsFromForm(r),
			IsNew:    id == 0,
			Error:    "Title is required.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
		return
	}

	// An uploaded file is stored in the private Storage bucket, not in
	// Postgres and not in the repo. Read it before the row save so a
	// missing Storage config fails before a course row is created.
	var uploaded []byte
	if file, header, ferr := r.FormFile("content_file"); ferr == nil {
		defer file.Close()
		if !strings.HasSuffix(strings.ToLower(header.Filename), ".html") && !strings.HasSuffix(strings.ToLower(header.Filename), ".htm") {
			formData := AdminCourseFormData{
				BaseData: a.base(r, "Course — Admin", "", "admin"),
				Course:   course,
				SEO:      seoFieldsFromForm(r),
				IsNew:    id == 0,
				Error:    "Course content must be an .html file.",
			}
			a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
			return
		}
		b, rerr := io.ReadAll(io.LimitReader(file, maxCourseUploadBytes+1))
		if rerr != nil {
			a.serverError(w, "read uploaded course content", rerr)
			return
		}
		if len(b) > maxCourseUploadBytes {
			formData := AdminCourseFormData{
				BaseData: a.base(r, "Course — Admin", "", "admin"),
				Course:   course,
				SEO:      seoFieldsFromForm(r),
				IsNew:    id == 0,
				Error:    "Course content must be 8MB or smaller.",
			}
			a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
			return
		}
		uploaded = b
		if a.CourseStorage == nil {
			formData := AdminCourseFormData{
				BaseData: a.base(r, "Course — Admin", "", "admin"),
				Course:   course,
				SEO:      seoFieldsFromForm(r),
				IsNew:    id == 0,
				Error:    "Course storage is not configured on this server.",
			}
			a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
			return
		}
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		id, err = a.Courses.Create(ctx, course)
	} else {
		err = a.Courses.Update(ctx, course)
	}
	if err != nil {
		log.Printf("save course: %v", err)
		formData := AdminCourseFormData{
			BaseData: a.base(r, "Course — Admin", "", "admin"),
			Course:   course,
			SEO:      seoFieldsFromForm(r),
			IsNew:    id == 0,
			Error:    "Could not save course — the slug may already be in use.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
		return
	}
	a.saveSEOFields(ctx, repo.SEOCourse, id, r)
	if len(uploaded) > 0 {
		// Every lesson is stored in the site's content conventions
		// (docs/DESIGN.md "Content updates").
		uploaded = storage.NormalizeLesson(uploaded)
		var previous string
		if isEdit {
			if existing, gerr := a.Courses.GetByID(ctx, id); gerr == nil {
				previous = existing.ContentObject
			}
		}
		objectPath, perr := storage.CourseObject(course.Slug)
		if perr != nil {
			a.serverError(w, "course object path", perr)
			return
		}
		if err := a.CourseStorage.Put(ctx, objectPath, "text/html", uploaded); err != nil {
			a.serverError(w, "store course content", err)
			return
		}
		if err := a.Courses.UpdateContentObject(ctx, id, objectPath); err != nil {
			a.serverError(w, "save course content path", err)
			return
		}
		// The outline is derived from the bytes just stored, so it can
		// never describe a different file than the one on Storage.
		if err := a.Courses.UpdateOutline(ctx, id, strings.Join(storage.LessonOutline(uploaded), "\n")); err != nil {
			log.Printf("save course outline %d: %v", id, err)
		}
		if previous != "" && previous != objectPath {
			if err := a.CourseStorage.Delete(ctx, previous); err != nil {
				log.Printf("delete previous course object %s: %v", previous, err)
			}
		}
	}

	if isEdit {
		if note := strings.TrimSpace(r.FormValue("update_note")); note != "" {
			// One row is the whole notification — a database trigger fans
			// it out to every paid buyer as an admin_messages row (see
			// migration 026_course_updates.sql).
			if err := a.CourseUpdates.Create(ctx, id, note); err != nil {
				log.Printf("record course update for course %d: %v", id, err)
			}
		}
	}

	a.markChatbotReindexPending(r.Context())
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

func (a *App) AdminCourseDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if existing, gerr := a.Courses.GetByID(r.Context(), id); gerr == nil && existing.ContentObject != "" && a.CourseStorage != nil {
		if err := a.CourseStorage.Delete(r.Context(), existing.ContentObject); err != nil {
			log.Printf("delete course object %s: %v", existing.ContentObject, err)
		}
	}
	if err := a.Courses.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete course", err)
		return
	}
	a.markChatbotReindexPending(r.Context())
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

// --- Bookings ------------------------------------------------------------

type AdminBookingsData struct {
	BaseData
	Bookings []models.Booking
}

func (a *App) AdminBookingsList(w http.ResponseWriter, r *http.Request) {
	bookings, err := a.Bookings.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load bookings", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_bookings", AdminBookingsData{
		BaseData: a.base(r, "Bookings — Admin", "", "admin"),
		Bookings: bookings,
	})
}

func (a *App) AdminBookingStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	status := r.FormValue("status")
	switch status {
	case models.BookingStatusConfirmed, models.BookingStatusCancelled, models.BookingStatusPendingPayment:
	default:
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := a.Bookings.UpdateStatus(r.Context(), id, status); err != nil {
		a.serverError(w, "update booking", err)
		return
	}
	http.Redirect(w, r, "/admin/bookings", http.StatusSeeOther)
}

// --- Purchases -------------------------------------------------------------

type AdminPurchasesData struct {
	BaseData
	Purchases []models.Purchase
	Courses   map[int64]models.Course
}

func (a *App) AdminPurchasesList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	purchases, err := a.Purchases.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load purchases", err)
		return
	}
	courses, err := a.Courses.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load courses", err)
		return
	}
	byID := make(map[int64]models.Course, len(courses))
	for _, c := range courses {
		byID[c.ID] = c
	}

	a.View.Render(w, http.StatusOK, "admin_purchases", AdminPurchasesData{
		BaseData:  a.base(r, "Purchases — Admin", "", "admin"),
		Purchases: purchases,
		Courses:   byID,
	})
}

// --- Availability ---------------------------------------------------------

type AdminAvailabilityData struct {
	BaseData
	Rules []models.AvailabilityRule
}

func (a *App) AdminAvailabilityList(w http.ResponseWriter, r *http.Request) {
	rules, err := a.Availability.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_availability", AdminAvailabilityData{
		BaseData: a.base(r, "Availability — Admin", "", "admin"),
		Rules:    rules,
	})
}

func (a *App) AdminAvailabilityCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	weekday, _ := strconv.Atoi(r.FormValue("weekday"))
	slotMinutes, _ := strconv.Atoi(r.FormValue("slot_minutes"))
	if slotMinutes <= 0 {
		slotMinutes = 30
	}
	_, err := a.Availability.Create(r.Context(), models.AvailabilityRule{
		Weekday:     time.Weekday(weekday),
		StartTime:   r.FormValue("start_time"),
		EndTime:     r.FormValue("end_time"),
		SlotMinutes: slotMinutes,
		Active:      true,
	})
	if err != nil {
		a.serverError(w, "create availability rule", err)
		return
	}
	http.Redirect(w, r, "/admin/availability", http.StatusSeeOther)
}

func (a *App) AdminAvailabilityDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Availability.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete availability rule", err)
		return
	}
	http.Redirect(w, r, "/admin/availability", http.StatusSeeOther)
}

func (a *App) AdminAvailabilityToggle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	active := r.FormValue("active") == "on"
	if err := a.Availability.SetActive(r.Context(), id, active); err != nil {
		a.serverError(w, "update availability rule", err)
		return
	}
	http.Redirect(w, r, "/admin/availability", http.StatusSeeOther)
}

// --- Profile ---------------------------------------------------------------

type AdminProfileData struct {
	BaseData
	Profile          models.Profile
	HasDefaultResume bool
	Saved            bool
	Error            string
}

func (a *App) AdminProfilePage(w http.ResponseWriter, r *http.Request) {
	profile, err := a.Profile.Get(r.Context())
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	hasDefault, err := a.hasDefaultResume(r.Context())
	if err != nil {
		a.serverError(w, "load resume", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_profile", AdminProfileData{
		BaseData:         a.base(r, "Profile — Admin", "", "admin"),
		Profile:          profile,
		HasDefaultResume: hasDefault,
		Saved:            r.URL.Query().Get("saved") == "1",
	})
}

func (a *App) AdminProfileSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxCourseUploadBytes); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	// The photo/logo file inputs are optional on every save (most saves
	// just edit a text field) -- load the current row first so an
	// untouched image isn't wiped out by Update's full-row upsert.
	current, err := a.Profile.Get(ctx)
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	years, _ := strconv.Atoi(r.FormValue("years_experience"))
	profile := models.Profile{
		Name:            strings.TrimSpace(r.FormValue("name")),
		Title:           strings.TrimSpace(r.FormValue("title")),
		Tagline:         strings.TrimSpace(r.FormValue("tagline")),
		HeroSubtitle:    strings.TrimSpace(r.FormValue("hero_subtitle")),
		Summary:         r.FormValue("summary"),
		RagContext:      strings.TrimSpace(r.FormValue("rag_context")),
		Location:        strings.TrimSpace(r.FormValue("location")),
		Email:           strings.TrimSpace(r.FormValue("email")),
		LinkedInURL:     strings.TrimSpace(r.FormValue("linkedin_url")),
		GitHubURL:       strings.TrimSpace(r.FormValue("github_url")),
		ResumeURL:       strings.TrimSpace(r.FormValue("resume_url")),
		PhotoObject:     current.PhotoObject,
		LogoObject:      current.LogoObject,
		YearsExperience: years,
	}
	pdf, pdfErr := readResumeUpload(r)
	photo, photoExt, photoErr := readImageUpload(r, "photo_file")
	logo, logoExt, logoErr := readImageUpload(r, "logo_file")
	errMsg := firstNonEmpty(pdfErr, photoErr, logoErr)
	if errMsg != "" || ((len(pdf) > 0 || len(photo) > 0 || len(logo) > 0) && a.CourseStorage == nil) {
		if errMsg == "" {
			errMsg = "Storage is not configured, so the upload cannot be saved."
		}
		hasDefault, _ := a.hasDefaultResume(ctx)
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_profile", AdminProfileData{
			BaseData:         a.base(r, "Profile — Admin", "", "admin"),
			Profile:          profile,
			HasDefaultResume: hasDefault,
			Error:            errMsg,
		})
		return
	}
	if len(photo) > 0 {
		path, err := a.replaceStoredImage(ctx, storage.ProfilePhotoObject, current.PhotoObject, photo, photoExt)
		if err != nil {
			a.serverError(w, "save profile photo", err)
			return
		}
		profile.PhotoObject = path
	}
	if len(logo) > 0 {
		path, err := a.replaceStoredImage(ctx, storage.LogoObject, current.LogoObject, logo, logoExt)
		if err != nil {
			a.serverError(w, "save logo", err)
			return
		}
		profile.LogoObject = path
	}
	if err := a.Profile.Update(ctx, profile); err != nil {
		a.serverError(w, "save profile", err)
		return
	}
	if len(pdf) > 0 {
		if err := a.replaceDefaultResume(ctx, pdf); err != nil {
			a.serverError(w, "save resume", err)
			return
		}
	}
	a.markChatbotReindexPending(ctx)
	http.Redirect(w, r, "/admin/profile?saved=1", http.StatusSeeOther)
}

func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if p != "" {
			return p
		}
	}
	return ""
}

func (a *App) hasDefaultResume(ctx context.Context) (bool, error) {
	row, err := a.Resumes.Get(ctx, models.DefaultResumeKey)
	if err == repo.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.ObjectPath != "", nil
}

func (a *App) replaceDefaultResume(ctx context.Context, pdf []byte) error {
	path, err := storage.ResumeObject(models.DefaultResumeKey)
	if err != nil {
		return err
	}
	if err := a.CourseStorage.Put(ctx, path, "application/pdf", pdf); err != nil {
		return err
	}
	return a.Resumes.Upsert(ctx, models.RoleResume{
		RoleKey:    models.DefaultResumeKey,
		Label:      "Resume",
		ObjectPath: path,
		SortOrder:  0,
	})
}

// --- helpers ---------------------------------------------------------------

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
