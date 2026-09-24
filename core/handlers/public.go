package handlers

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/resume"
)

type HomeData struct {
	BaseData
	Profile models.Profile
	Skills  []models.SkillGroup
	// Companies is every employer's name, for the "Previously at" strip —
	// company-name credibility a visitor can register before deciding
	// whether to click through to About at all.
	Companies []string
	Articles  []models.Article
	JSONLD    template.JS
}

func (a *App) Home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var (
		profile  models.Profile
		skills   []models.SkillGroup
		exp      []models.Experience
		articles []models.Article
	)
	// The chrome's lookups start here, before the errgroup, so they go out
	// in the same wave as the page's own rather than as a second wave
	// after it — and it reads the footer's LinkedIn link off the profile
	// row loaded just below instead of fetching that row again.
	pf := a.startBaseWithProfile(r, &profile)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { profile, err = a.Profile.Get(gctx); return })
	g.Go(func() (err error) { skills, err = a.Skills.List(gctx); return })
	g.Go(func() (err error) { exp, err = a.Experience.List(gctx); return })
	g.Go(func() (err error) { articles, err = a.Articles.ListPublished(gctx, 3); return })
	if err := g.Wait(); err != nil {
		a.serverError(w, "load home page data", err)
		return
	}

	// The "Previously at" names come off the same already-loaded
	// experience rows, so the home page still pays for exactly one extra
	// DB round-trip and it runs concurrently with the other three above.
	companies := make([]string, 0, len(exp))
	for _, e := range exp {
		if e.Company != "" {
			companies = append(companies, e.Company)
		}
	}

	// A concise, keyword-first <title> for search results — the full
	// multi-title Profile.Title is what shows in the schema.org jobTitle
	// and on the About page, but a search-result title tag gets
	// truncated past ~60 characters, so lead with the two terms
	// recruiters/clients actually search for.
	title := profile.Name + " — Technical Consultant & Java Technical Expert"
	base := a.baseFrom(r, pf, title, profile.HeroSubtitle, "home")
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
		BaseData:  base,
		Profile:   profile,
		Skills:    skills,
		Companies: companies,
		Articles:  articles,
		JSONLD:    personAndWebsiteJSONLD(profile, base.CanonicalURL, imageURL, sameAs, skills),
	})
}

type AboutData struct {
	BaseData
	Profile      models.Profile
	Skills       []models.SkillGroup
	Experience   []models.Experience
	Education    []models.Education
	Testimonials []models.Testimonial
	// Both are empty until Vishnu enters real entries in the admin console;
	// the template renders nothing at all for an empty one rather than a
	// bare heading. See migration 046.
	Certifications []models.Certification
	Publications   []models.Publication
	// Resumes is every role-specific PDF, in sort order. The default
	// resume is not included. Empty means About shows only the direct
	// download, which serves the default file.
	Resumes []models.RoleResume
}

func (a *App) About(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var (
		profile      models.Profile
		skills       []models.SkillGroup
		exp          []models.Experience
		edu          []models.Education
		testimonials []models.Testimonial
		certs        []models.Certification
		pubs         []models.Publication
		resumes      []models.RoleResume
	)
	// This is the widest fan-out in the app, and the page that suffered
	// most from the chrome's lookups being a second wave behind it: the
	// prefetch starts here so all of it is one wave, and it shares the
	// profile row rather than fetching it twice.
	pf := a.startBaseWithProfile(r, &profile)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { profile, err = a.Profile.Get(gctx); return })
	g.Go(func() (err error) { skills, err = a.Skills.List(gctx); return })
	g.Go(func() (err error) { exp, err = a.Experience.List(gctx); return })
	g.Go(func() (err error) { edu, err = a.Education.List(gctx); return })
	g.Go(func() (err error) { testimonials, err = a.Testimonials.ListPublished(gctx); return })
	g.Go(func() (err error) { certs, err = a.Certifications.ListPublished(gctx); return })
	g.Go(func() (err error) { pubs, err = a.Publications.ListPublished(gctx); return })
	g.Go(func() (err error) { resumes, err = a.Resumes.List(gctx); return })
	if err := g.Wait(); err != nil {
		a.serverError(w, "load about page data", err)
		return
	}

	description := "Vishnu Balachandran's full background: skills, work history, and education — 8+ years architecting banking and fintech platforms, available for technical consulting."
	a.View.Render(w, http.StatusOK, "about", AboutData{
		BaseData:       a.baseFrom(r, pf, profile.Name+" — Technical Consultant, Java Expert & Software Architect", description, "about"),
		Profile:        profile,
		Skills:         skills,
		Experience:     exp,
		Education:      edu,
		Testimonials:   testimonials,
		Certifications: certs,
		Publications:   pubs,
		Resumes:        roleSpecificResumes(resumes),
	})
}

// roleKeyPattern matches the slug stored as role_resumes.role_key.
var roleKeyPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const defaultResumeFilename = "Resume_Vishnu_balachandran.pdf"

// resumeFileName is the download title. The default file is
// Resume_Vishnu_balachandran.pdf. A role-specific file is
// Resume-Vishnu-Balachandran-<role name>.pdf, with the role's own
// letter case kept and spaces turned into hyphens.
func resumeFileName(row models.RoleResume) string {
	if row.RoleKey == "" || row.RoleKey == models.DefaultResumeKey {
		return defaultResumeFilename
	}
	token := filenameToken(roleNameFromLabel(row.Label))
	if token == "" {
		token = row.RoleKey
	}
	return "Resume-Vishnu-Balachandran-" + token + ".pdf"
}

func roleNameFromLabel(label string) string {
	if i := strings.Index(label, " · "); i >= 0 {
		return strings.TrimSpace(label[:i])
	}
	return strings.TrimSpace(label)
}

func filenameToken(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
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

func roleSpecificResumes(list []models.RoleResume) []models.RoleResume {
	var out []models.RoleResume
	for _, row := range list {
		if row.RoleKey == models.DefaultResumeKey || row.ObjectPath == "" {
			continue
		}
		out = append(out, row)
	}
	return out
}

// Resume serves the uploaded default PDF. A role query serves that role's
// file only when one has been stored. Every other request serves the
// default. With no default file stored, it still generates a PDF from the
// live profile, skills, experience, and education.
func (a *App) Resume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if role != "" && role != models.DefaultResumeKey && roleKeyPattern.MatchString(role) {
		row, err := a.Resumes.Get(ctx, role)
		if err != nil && err != repo.ErrNotFound {
			a.serverError(w, "load resume", err)
			return
		}
		if err == nil && row.ObjectPath != "" {
			a.writeStoredResume(w, r, row)
			return
		}
	}

	row, err := a.Resumes.Get(ctx, models.DefaultResumeKey)
	if err != nil && err != repo.ErrNotFound {
		a.serverError(w, "load resume", err)
		return
	}
	if err == nil && row.ObjectPath != "" {
		a.writeStoredResume(w, r, row)
		return
	}

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
	w.Header().Set("Content-Disposition", `inline; filename="`+defaultResumeFilename+`"`)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(pdfBytes)
}

func (a *App) writeStoredResume(w http.ResponseWriter, r *http.Request, row models.RoleResume) {
	if a.CourseStorage == nil {
		a.serverError(w, "resume storage", fmt.Errorf("storage is not configured"))
		return
	}
	rc, err := a.CourseStorage.Open(r.Context(), row.ObjectPath)
	if err != nil {
		a.serverError(w, "open resume", err)
		return
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxCourseUploadBytes+1))
	if err != nil {
		a.serverError(w, "read resume", err)
		return
	}
	if len(raw) > maxCourseUploadBytes {
		a.serverError(w, "read resume", fmt.Errorf("resume exceeds %d bytes", maxCourseUploadBytes))
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+resumeFileName(row)+`"`)
	w.Header().Set("Cache-Control", "private, max-age=0")
	w.Write(raw)
}

type CoursesListData struct {
	BaseData
	Courses []models.Course
	// The same courses again, bucketed onto the three rungs of the
	// suggested learning path (models.Course.Track) so the list page can
	// offer a route through the catalogue rather than only a flat grid.
	// Grouping here rather than in the template keeps the ordering logic
	// testable and leaves the copy where the rest of the site's copy is.
	TrackStart []models.Course
	TrackBuild []models.Course
	TrackDeep  []models.Course
}

func (a *App) CoursesList(w http.ResponseWriter, r *http.Request) {
	pf := a.startBase(r)
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
	data := CoursesListData{
		BaseData: a.baseFrom(r, pf, "Java, Architecture & System Design Courses — "+a.Cfg.SiteName, "Practical, production-grade courses on Java, technical architecture, system design, and distributed systems from a working technical consultant.", "courses"),
		Courses:  courses,
	}
	for _, c := range courses {
		switch c.Track() {
		case models.CourseTrackStart:
			data.TrackStart = append(data.TrackStart, c)
		case models.CourseTrackDeep:
			data.TrackDeep = append(data.TrackDeep, c)
		default:
			data.TrackBuild = append(data.TrackBuild, c)
		}
	}
	a.View.Render(w, http.StatusOK, "courses_list", data)
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
	// QuizPassPercent comes straight from models.QuizPassPercent — the
	// same constant the scoring in quiz.go compares against — so the
	// threshold advertised on the detail page can't drift from the one
	// actually enforced.
	QuizPassPercent int
}

func (a *App) CourseDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := r.PathValue("slug")

	// Wave one: everything that only needs the request — the course row,
	// the chrome, and the signed-in visitor's email for the checkout form.
	pf := a.startBase(r)
	var (
		course    models.Course
		userEmail string
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { course, err = a.Courses.GetBySlug(gctx, slug); return })
	if uid, ok := a.Auth.UserID(r); ok {
		g.Go(func() error {
			// Best-effort: a missing email only costs the checkout form a
			// prefill, so it must not fail the page.
			if u, err := a.Users.GetByID(gctx, uid); err == nil {
				userEmail = u.Email
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
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

	// Wave two: the two lookups that genuinely need the course row. The
	// access check stays on this goroutine because it sets the access
	// cookie, and only one goroutine may touch the response headers.
	var quizCount int
	g2, g2ctx := errgroup.WithContext(ctx)
	g2.Go(func() (err error) { quizCount, err = a.QuizQuestions.CountByCourse(g2ctx, course.ID); return })
	hasAccess := false
	if course.IsPaid {
		hasAccess, _ = a.courseAccess(w, r, course)
	}
	if err := g2.Wait(); err != nil {
		a.serverError(w, "load quiz count", err)
		return
	}

	base := a.baseFrom(r, pf, course.Title+" — "+a.Cfg.SiteName, course.Summary, "courses")

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
		UserEmail:          userEmail,
		HasQuiz:            quizCount > 0,
		QuizPassPercent:    models.QuizPassPercent,
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
