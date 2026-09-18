package handlers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
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

	admin, err := a.Admins.GetByEmail(r.Context(), email)
	if err != nil || !authutil.CheckPassword(admin.PasswordHash, password) {
		http.Redirect(w, r, "/admin/login?error=Invalid+email+or+password", http.StatusSeeOther)
		return
	}

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
	CourseCount       int
	PublishedCourses  int
	ArticleCount      int
	PublishedArticles int
	UpcomingBookings  []models.Booking
	PendingBookings   int
	ConfirmedBookings int
	TotalPurchases    int
	PaidPurchases     int
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
	for _, art := range articles {
		if art.IsPublished {
			publishedArticles++
		}
	}

	a.View.Render(w, http.StatusOK, "admin_dashboard", AdminDashboardData{
		BaseData:          a.base(r, "Admin Dashboard", "", "admin"),
		CourseCount:       len(courses),
		PublishedCourses:  published,
		ArticleCount:      len(articles),
		PublishedArticles: publishedArticles,
		UpcomingBookings:  upcoming,
		PendingBookings:   pending,
		ConfirmedBookings: confirmed,
		TotalPurchases:    len(purchases),
		PaidPurchases:     paidPurchases,
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
	a.View.Render(w, http.StatusOK, "admin_course_form", AdminCourseFormData{
		BaseData: a.base(r, "Edit Course — Admin", "", "admin"),
		Course:   course,
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
	minPriceCentsINR, _ := strconv.Atoi(r.FormValue("min_price_cents_inr"))
	isPaid := r.FormValue("is_paid") == "on"

	course := models.Course{
		ID:               id,
		Slug:             slugify(r.FormValue("slug")),
		Title:            strings.TrimSpace(r.FormValue("title")),
		Summary:          strings.TrimSpace(r.FormValue("summary")),
		Description:      strings.TrimSpace(r.FormValue("description")),
		Level:            strings.TrimSpace(r.FormValue("level")),
		Duration:         strings.TrimSpace(r.FormValue("duration")),
		PriceCents:       price,
		Currency:         orDefault(r.FormValue("currency"), "USD"),
		ImageURL:         strings.TrimSpace(r.FormValue("image_url")),
		IsPublished:      r.FormValue("is_published") == "on",
		SortOrder:        sortOrder,
		IsPaid:           isPaid,
		MinPriceCents:    minPriceCents,
		MinPriceCentsINR: minPriceCentsINR,
	}
	if course.Slug == "" {
		course.Slug = slugify(course.Title)
	}
	if course.Title == "" || course.Slug == "" {
		formData := AdminCourseFormData{
			BaseData: a.base(r, "Course — Admin", "", "admin"),
			Course:   course,
			IsNew:    id == 0,
			Error:    "Title is required.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
		return
	}

	// An uploaded content file, if present, is how a course's full
	// interactive HTML gets added without a code deploy — read it now so
	// a brand-new course can be created with its content in one save.
	var uploadedHTML string
	if file, header, ferr := r.FormFile("content_file"); ferr == nil {
		defer file.Close()
		if !strings.HasSuffix(strings.ToLower(header.Filename), ".html") && !strings.HasSuffix(strings.ToLower(header.Filename), ".htm") {
			formData := AdminCourseFormData{
				BaseData: a.base(r, "Course — Admin", "", "admin"),
				Course:   course,
				IsNew:    id == 0,
				Error:    "Course content must be an .html file.",
			}
			a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
			return
		}
		b, rerr := io.ReadAll(io.LimitReader(file, maxCourseUploadBytes))
		if rerr != nil {
			a.serverError(w, "read uploaded course content", rerr)
			return
		}
		uploadedHTML = string(b)
		course.ContentHTML = uploadedHTML
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
			IsNew:    id == 0,
			Error:    "Could not save course — the slug may already be in use.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
		return
	}
	if uploadedHTML != "" {
		if err := a.Courses.UpdateContentHTML(ctx, id, uploadedHTML); err != nil {
			a.serverError(w, "save uploaded course content", err)
			return
		}
	}

	if isEdit {
		if note := strings.TrimSpace(r.FormValue("update_note")); note != "" {
			a.notifyCourseBuyers(ctx, id, course.Title, note)
		}
	}

	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

// notifyCourseBuyers sends every paid buyer of a course a message noting
// what changed, reusing the same admin-message mechanism (and its fixed
// contact-email signature) as a one-off message sent from a user's detail
// page. A buyer sees it the same way: in the notification bell and under
// /account#messages.
func (a *App) notifyCourseBuyers(ctx context.Context, courseID int64, courseTitle, note string) {
	userIDs, err := a.Purchases.ListPaidUserIDsByCourseID(ctx, courseID)
	if err != nil {
		log.Printf("notify course buyers: load buyers for course %d: %v", courseID, err)
		return
	}
	body := a.withContactSignature(ctx, fmt.Sprintf("%q was updated: %s", courseTitle, note))
	for _, uid := range userIDs {
		if _, err := a.AdminMessages.Create(ctx, uid, body); err != nil {
			log.Printf("notify course buyers: message user %d: %v", uid, err)
		}
	}
}

func (a *App) AdminCourseDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Courses.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete course", err)
		return
	}
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
	Profile models.Profile
	Saved   bool
}

func (a *App) AdminProfilePage(w http.ResponseWriter, r *http.Request) {
	profile, err := a.Profile.Get(r.Context())
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_profile", AdminProfileData{
		BaseData: a.base(r, "Profile — Admin", "", "admin"),
		Profile:  profile,
		Saved:    r.URL.Query().Get("saved") == "1",
	})
}

func (a *App) AdminProfileSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	years, _ := strconv.Atoi(r.FormValue("years_experience"))
	profile := models.Profile{
		Name:            strings.TrimSpace(r.FormValue("name")),
		Title:           strings.TrimSpace(r.FormValue("title")),
		Tagline:         strings.TrimSpace(r.FormValue("tagline")),
		HeroSubtitle:    strings.TrimSpace(r.FormValue("hero_subtitle")),
		Summary:         r.FormValue("summary"),
		Location:        strings.TrimSpace(r.FormValue("location")),
		Email:           strings.TrimSpace(r.FormValue("email")),
		LinkedInURL:     strings.TrimSpace(r.FormValue("linkedin_url")),
		GitHubURL:       strings.TrimSpace(r.FormValue("github_url")),
		ResumeURL:       strings.TrimSpace(r.FormValue("resume_url")),
		PhotoURL:        strings.TrimSpace(r.FormValue("photo_url")),
		YearsExperience: years,
	}
	if err := a.Profile.Update(r.Context(), profile); err != nil {
		a.serverError(w, "save profile", err)
		return
	}
	http.Redirect(w, r, "/admin/profile?saved=1", http.StatusSeeOther)
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
