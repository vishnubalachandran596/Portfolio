package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/internal/authutil"
	"github.com/vishnubalachandran596/portfolio/internal/models"
	"github.com/vishnubalachandran596/portfolio/internal/repo"
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
		BaseData: a.base("Admin Login", "", ""),
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

type AdminDashboardData struct {
	BaseData
	CourseCount      int
	PublishedCourses int
	UpcomingBookings []models.Booking
	PendingBookings  int
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

	bookings, err := a.Bookings.ListUpcoming(ctx)
	if err != nil {
		a.serverError(w, "load bookings", err)
		return
	}
	pending := 0
	for _, b := range bookings {
		if b.Status == models.BookingStatusPending {
			pending++
		}
	}
	if len(bookings) > 10 {
		bookings = bookings[:10]
	}

	a.View.Render(w, http.StatusOK, "admin_dashboard", AdminDashboardData{
		BaseData:         a.base("Admin Dashboard", "", "admin"),
		CourseCount:      len(courses),
		PublishedCourses: published,
		UpcomingBookings: bookings,
		PendingBookings:  pending,
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
		BaseData: a.base("Courses — Admin", "", "admin"),
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
		BaseData: a.base("New Course — Admin", "", "admin"),
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
		BaseData: a.base("Edit Course — Admin", "", "admin"),
		Course:   course,
	})
}

func (a *App) AdminCourseSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
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

	price, _ := strconv.Atoi(r.FormValue("price_cents"))
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))

	course := models.Course{
		ID:          id,
		Slug:        slugify(r.FormValue("slug")),
		Title:       strings.TrimSpace(r.FormValue("title")),
		Summary:     strings.TrimSpace(r.FormValue("summary")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Level:       strings.TrimSpace(r.FormValue("level")),
		Duration:    strings.TrimSpace(r.FormValue("duration")),
		PriceCents:  price,
		Currency:    orDefault(r.FormValue("currency"), "USD"),
		ImageURL:    strings.TrimSpace(r.FormValue("image_url")),
		IsPublished: r.FormValue("is_published") == "on",
		SortOrder:   sortOrder,
	}
	if course.Slug == "" {
		course.Slug = slugify(course.Title)
	}
	if course.Title == "" || course.Slug == "" {
		formData := AdminCourseFormData{
			BaseData: a.base("Course — Admin", "", "admin"),
			Course:   course,
			IsNew:    id == 0,
			Error:    "Title is required.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
		return
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.Courses.Create(ctx, course)
	} else {
		err = a.Courses.Update(ctx, course)
	}
	if err != nil {
		log.Printf("save course: %v", err)
		formData := AdminCourseFormData{
			BaseData: a.base("Course — Admin", "", "admin"),
			Course:   course,
			IsNew:    id == 0,
			Error:    "Could not save course — the slug may already be in use.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_course_form", formData)
		return
	}

	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
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
		BaseData: a.base("Bookings — Admin", "", "admin"),
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
	case models.BookingStatusConfirmed, models.BookingStatusCancelled, models.BookingStatusPending:
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
		BaseData: a.base("Availability — Admin", "", "admin"),
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
		BaseData: a.base("Profile — Admin", "", "admin"),
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
