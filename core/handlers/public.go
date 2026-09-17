package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/scheduler"
)

type HomeData struct {
	BaseData
	Profile  models.Profile
	Features []models.HomeFeature
	Courses  []models.Course
	JSONLD   template.JS
}

func (a *App) Home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile, err := a.Profile.Get(ctx)
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	features, err := a.Features.List(ctx)
	if err != nil {
		a.serverError(w, "load features", err)
		return
	}
	courses, err := a.Courses.ListPublished(ctx)
	if err != nil {
		a.serverError(w, "load courses", err)
		return
	}
	if len(courses) > 3 {
		courses = courses[:3]
	}

	base := a.base(r, profile.Name+" — "+profile.Title, profile.HeroSubtitle, "home")
	var sameAs []string
	if profile.LinkedInURL != "" {
		sameAs = append(sameAs, profile.LinkedInURL)
	}
	if profile.GitHubURL != "" {
		sameAs = append(sameAs, profile.GitHubURL)
	}

	a.View.Render(w, http.StatusOK, "home", HomeData{
		BaseData: base,
		Profile:  profile,
		Features: features,
		Courses:  courses,
		JSONLD:   personAndWebsiteJSONLD(profile.Name, profile.Title, base.CanonicalURL, profile.Location, sameAs),
	})
}

type AboutData struct {
	BaseData
	Profile    models.Profile
	Skills     []models.SkillGroup
	Experience []models.Experience
	Education  []models.Education
}

func (a *App) About(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile, err := a.Profile.Get(ctx)
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	skills, err := a.Skills.List(ctx)
	if err != nil {
		a.serverError(w, "load skills", err)
		return
	}
	exp, err := a.Experience.List(ctx)
	if err != nil {
		a.serverError(w, "load experience", err)
		return
	}
	edu, err := a.Education.List(ctx)
	if err != nil {
		a.serverError(w, "load education", err)
		return
	}

	a.View.Render(w, http.StatusOK, "about", AboutData{
		BaseData:   a.base(r, "About — "+profile.Name, profile.Summary, "about"),
		Profile:    profile,
		Skills:     skills,
		Experience: exp,
		Education:  edu,
	})
}

type CoursesListData struct {
	BaseData
	Courses []models.Course
}

func (a *App) CoursesList(w http.ResponseWriter, r *http.Request) {
	courses, err := a.Courses.ListPublished(r.Context())
	if err != nil {
		a.serverError(w, "load courses", err)
		return
	}
	a.View.Render(w, http.StatusOK, "courses_list", CoursesListData{
		BaseData: a.base(r, "Courses — "+a.Cfg.SiteName, "Practical courses on architecture, backend engineering, and technical leadership.", "courses"),
		Courses:  courses,
	})
}

type CourseDetailData struct {
	BaseData
	Course           models.Course
	JSONLD           template.JS
	PaymentsEnabled  bool
	RazorpayKeyID    string
	PurchaseRequired bool
	HasAccess        bool
}

func (a *App) CourseDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	if err != nil {
		if err == repo.ErrNotFound || !course.IsPublished {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load course", err)
		return
	}
	if !course.IsPublished {
		a.NotFound(w, r)
		return
	}

	hasAccess := false
	if course.IsPaid {
		hasAccess, _ = a.courseAccess(w, r, course)
	}

	base := a.base(r, course.Title+" — "+a.Cfg.SiteName, course.Summary, "courses")
	a.View.Render(w, http.StatusOK, "course_detail", CourseDetailData{
		BaseData:         base,
		Course:           course,
		JSONLD:           courseJSONLD(course.Title, course.Summary, base.CanonicalURL, a.Cfg.SiteName),
		PaymentsEnabled:  a.Razorpay.Enabled(),
		RazorpayKeyID:    a.Razorpay.KeyID,
		PurchaseRequired: r.URL.Query().Get("purchase") == "required",
		HasAccess:        hasAccess,
	})
}

type BookingData struct {
	BaseData
	Days    []scheduler.DaySlots
	Success bool
	Error   string
}

func (a *App) BookingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rules, err := a.Availability.ListActive(ctx)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}

	from := time.Now().In(a.Timezone)
	to := from.AddDate(0, 0, 15)
	taken, err := a.Bookings.BookedSlots(ctx, from, to)
	if err != nil {
		a.serverError(w, "load bookings", err)
		return
	}

	days := scheduler.ComputeDays(rules, taken, a.Timezone, 14)

	a.View.Render(w, http.StatusOK, "booking", BookingData{
		BaseData: a.base(r, "Book Time — "+a.Cfg.SiteName, "Book a 1:1 session directly on my calendar.", "booking"),
		Days:     days,
		Success:  r.URL.Query().Get("booked") == "1",
		Error:    r.URL.Query().Get("error"),
	})
}

func (a *App) BookingSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+form+submission", http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	message := strings.TrimSpace(r.FormValue("message"))
	slotRaw := strings.TrimSpace(r.FormValue("slot"))

	if name == "" || email == "" || slotRaw == "" || !strings.Contains(email, "@") {
		http.Redirect(w, r, "/booking?error=Please+fill+in+your+name%2C+a+valid+email%2C+and+pick+a+time", http.StatusSeeOther)
		return
	}

	slotStart, err := time.Parse(time.RFC3339, slotRaw)
	if err != nil {
		http.Redirect(w, r, "/booking?error=That+time+slot+is+no+longer+valid%2C+please+pick+another", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	rules, err := a.Availability.ListActive(ctx)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	if !a.slotIsOffered(rules, slotStart) {
		http.Redirect(w, r, "/booking?error=That+time+slot+is+not+available%2C+please+pick+another", http.StatusSeeOther)
		return
	}
	if slotStart.Before(time.Now()) {
		http.Redirect(w, r, "/booking?error=That+time+has+already+passed%2C+please+pick+another", http.StatusSeeOther)
		return
	}

	slotMinutes := 30
	for _, rr := range rules {
		if rr.Weekday == slotStart.In(a.Timezone).Weekday() {
			slotMinutes = rr.SlotMinutes
			break
		}
	}

	_, err = a.Bookings.Create(ctx, models.Booking{
		Name:      name,
		Email:     email,
		Message:   message,
		SlotStart: slotStart,
		SlotEnd:   slotStart.Add(time.Duration(slotMinutes) * time.Minute),
	})
	if err != nil {
		if err == repo.ErrSlotTaken {
			http.Redirect(w, r, "/booking?error=That+time+was+just+booked+by+someone+else%2C+please+pick+another", http.StatusSeeOther)
			return
		}
		a.serverError(w, "create booking", err)
		return
	}

	http.Redirect(w, r, "/booking?booked=1", http.StatusSeeOther)
}

// slotIsOffered re-derives the schedule server-side so a booking can never
// be created for a time the visitor didn't actually see offered.
func (a *App) slotIsOffered(rules []models.AvailabilityRule, slotStart time.Time) bool {
	local := slotStart.In(a.Timezone)
	for _, rule := range rules {
		if rule.Weekday != local.Weekday() {
			continue
		}
		start, err1 := time.ParseInLocation("15:04", rule.StartTime, a.Timezone)
		end, err2 := time.ParseInLocation("15:04", rule.EndTime, a.Timezone)
		if err1 != nil || err2 != nil {
			continue
		}
		dayStart := time.Date(local.Year(), local.Month(), local.Day(), start.Hour(), start.Minute(), 0, 0, a.Timezone)
		dayEnd := time.Date(local.Year(), local.Month(), local.Day(), end.Hour(), end.Minute(), 0, 0, a.Timezone)
		if local.Before(dayStart) || !local.Before(dayEnd) {
			continue
		}
		step := time.Duration(rule.SlotMinutes) * time.Minute
		if step <= 0 {
			continue
		}
		offset := local.Sub(dayStart)
		if offset%step == 0 {
			return true
		}
	}
	return false
}

func (a *App) NotFound(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusNotFound, "not_found", a.base(r, "Page not found — "+a.Cfg.SiteName, "", ""))
}

func (a *App) serverError(w http.ResponseWriter, action string, err error) {
	log.Printf("%s: %v", action, err)
	http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
}
