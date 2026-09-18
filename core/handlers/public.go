package handlers

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/resume"
)

type HomeData struct {
	BaseData
	Profile  models.Profile
	Skills   []models.SkillGroup
	Articles []models.Article
	JSONLD   template.JS
}

func (a *App) Home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var (
		profile  models.Profile
		skills   []models.SkillGroup
		articles []models.Article
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { profile, err = a.Profile.Get(gctx); return })
	g.Go(func() (err error) { skills, err = a.Skills.List(gctx); return })
	g.Go(func() (err error) { articles, err = a.Articles.ListPublished(gctx, 3); return })
	if err := g.Wait(); err != nil {
		a.serverError(w, "load home page data", err)
		return
	}

	// A concise, keyword-first <title> for search results — the full
	// multi-title Profile.Title is what shows in the schema.org jobTitle
	// and on the About page, but a search-result title tag gets
	// truncated past ~60 characters, so lead with the two terms
	// recruiters/clients actually search for.
	title := profile.Name + " — Technical Consultant & Java Technical Expert"
	base := a.base(r, title, profile.HeroSubtitle, "home")
	var sameAs []string
	if profile.LinkedInURL != "" {
		sameAs = append(sameAs, profile.LinkedInURL)
	}
	if profile.GitHubURL != "" {
		sameAs = append(sameAs, profile.GitHubURL)
	}
	imageURL := ""
	if profile.PhotoURL != "" {
		imageURL = base.SiteURL + profile.PhotoURL
	}

	a.View.Render(w, http.StatusOK, "home", HomeData{
		BaseData: base,
		Profile:  profile,
		Skills:   skills,
		Articles: articles,
		JSONLD:   personAndWebsiteJSONLD(profile, base.CanonicalURL, imageURL, sameAs),
	})
}

type AboutData struct {
	BaseData
	Profile      models.Profile
	Skills       []models.SkillGroup
	Experience   []models.Experience
	Education    []models.Education
	Testimonials []models.Testimonial
}

func (a *App) About(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var (
		profile      models.Profile
		skills       []models.SkillGroup
		exp          []models.Experience
		edu          []models.Education
		testimonials []models.Testimonial
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { profile, err = a.Profile.Get(gctx); return })
	g.Go(func() (err error) { skills, err = a.Skills.List(gctx); return })
	g.Go(func() (err error) { exp, err = a.Experience.List(gctx); return })
	g.Go(func() (err error) { edu, err = a.Education.List(gctx); return })
	g.Go(func() (err error) { testimonials, err = a.Testimonials.ListPublished(gctx); return })
	if err := g.Wait(); err != nil {
		a.serverError(w, "load about page data", err)
		return
	}

	description := "Vishnu Balachandran's full background: skills, work history, and education — 8+ years architecting banking and fintech platforms, available for technical consulting."
	a.View.Render(w, http.StatusOK, "about", AboutData{
		BaseData:     a.base(r, profile.Name+" — Technical Consultant, Java Expert & Software Architect", description, "about"),
		Profile:      profile,
		Skills:       skills,
		Experience:   exp,
		Education:    edu,
		Testimonials: testimonials,
	})
}

// Resume renders a PDF resume generated in-process from the same profile,
// skills, experience, and education data shown on the About page — always
// in sync with the live site, no separate file to keep updated by hand.
func (a *App) Resume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var (
		profile models.Profile
		skills  []models.SkillGroup
		exp     []models.Experience
		edu     []models.Education
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { profile, err = a.Profile.Get(gctx); return })
	g.Go(func() (err error) { skills, err = a.Skills.List(gctx); return })
	g.Go(func() (err error) { exp, err = a.Experience.List(gctx); return })
	g.Go(func() (err error) { edu, err = a.Education.List(gctx); return })
	if err := g.Wait(); err != nil {
		a.serverError(w, "load resume data", err)
		return
	}

	pdfBytes, err := resume.Generate(resume.Data{
		Profile:    profile,
		Skills:     skills,
		Experience: exp,
		Education:  edu,
	})
	if err != nil {
		a.serverError(w, "generate resume", err)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="vishnu-balachandran-resume.pdf"`)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(pdfBytes)
}

type CoursesListData struct {
	BaseData
	Courses []models.Course
}

func (a *App) CoursesList(w http.ResponseWriter, r *http.Request) {
	var courses []models.Course
	var err error
	if a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r) {
		// Admin sees every course, including unpublished drafts, so a new
		// or in-progress course can be previewed before it goes live.
		courses, err = a.Courses.ListAll(r.Context())
	} else {
		courses, err = a.Courses.ListPublished(r.Context())
	}
	if err != nil {
		a.serverError(w, "load courses", err)
		return
	}
	a.View.Render(w, http.StatusOK, "courses_list", CoursesListData{
		BaseData: a.base(r, "Java, Architecture & System Design Courses — "+a.Cfg.SiteName, "Practical, production-grade courses on Java, technical architecture, system design, and distributed systems from a working technical consultant.", "courses"),
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
	UserEmail          string
	HasQuiz            bool
}

func (a *App) CourseDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load course", err)
		return
	}
	isAdmin := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)
	if !course.IsPublished && !isAdmin {
		a.NotFound(w, r)
		return
	}

	hasAccess := false
	if course.IsPaid {
		hasAccess, _ = a.courseAccess(w, r, course)
	}

	base := a.base(r, course.Title+" — "+a.Cfg.SiteName, course.Summary, "courses")

	currency, minCents := course.RegionalPrice(base.Country)

	userEmail := ""
	if uid, ok := a.Auth.UserID(r); ok {
		if u, err := a.Users.GetByID(r.Context(), uid); err == nil {
			userEmail = u.Email
		}
	}

	quizCount, err := a.QuizQuestions.CountByCourse(r.Context(), course.ID)
	if err != nil {
		a.serverError(w, "load quiz count", err)
		return
	}

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
		UserEmail:          userEmail,
		HasQuiz:            quizCount > 0,
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
