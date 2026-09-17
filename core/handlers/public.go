package handlers

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

type HomeData struct {
	BaseData
	Profile  models.Profile
	Articles []models.Article
	JSONLD   template.JS
}

func (a *App) Home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profile, err := a.Profile.Get(ctx)
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	articles, err := a.Articles.ListPublished(ctx)
	if err != nil {
		a.serverError(w, "load articles", err)
		return
	}
	if len(articles) > 3 {
		articles = articles[:3]
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
		Articles: articles,
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
	Course             models.Course
	JSONLD             template.JS
	PaymentsEnabled    bool
	RazorpayKeyID      string
	PurchaseRequired   bool
	HasAccess          bool
	ResolvedCurrency   string
	ResolvedMinCents   int
	ResolvedMinWhole   int
	ResolvedMinDisplay string
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

	currency, minCents := course.RegionalPrice(base.Country)

	a.View.Render(w, http.StatusOK, "course_detail", CourseDetailData{
		BaseData:           base,
		Course:             course,
		JSONLD:             courseJSONLD(course.Title, course.Summary, base.CanonicalURL, a.Cfg.SiteName),
		PaymentsEnabled:    a.Razorpay.Enabled(),
		RazorpayKeyID:      a.Razorpay.KeyID,
		PurchaseRequired:   r.URL.Query().Get("purchase") == "required",
		HasAccess:          hasAccess,
		ResolvedCurrency:   currency,
		ResolvedMinCents:   minCents,
		ResolvedMinWhole:   minCents / 100,
		ResolvedMinDisplay: models.FormatMoney(minCents, currency),
	})
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
