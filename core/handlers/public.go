package handlers

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
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
	// Software is every published software item, for the Home page's
	// downloadable-software teaser card -- same reuse-not-reimplement
	// pattern as Articles above (a short teaser list, full catalog on its
	// own page).
	Software []models.Software
	// Highlights feed the hero's stat strip (Admin → Home highlights,
	// migration 078). When none are published, Home falls back to the
	// profile's years of experience plus counts it already knows, so the
	// strip never shows an invented figure.
	Highlights  []models.ProfileHighlight
	CourseCount int
	Subscribe   SubscribeFormData
	JSONLD      template.JS
}

// absoluteImageURL is the Person schema's photo URL: the app's own
// /media/profile-photo route (never a Storage URL directly -- see
// core/storage/client.go), absolute because structured data requires it.
// Empty until a photo has actually been uploaded via /admin/profile.
func absoluteImageURL(siteURL, photoObject string) string {
	if photoObject == "" {
		return ""
	}
	return siteURL + "/media/profile-photo"
}

func (a *App) Home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var (
		profile    models.Profile
		skills     []models.SkillGroup
		exp        []models.Experience
		articles   []models.Article
		software   []models.Software
		highlights []models.ProfileHighlight
		courses    []models.Course
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
	g.Go(func() (err error) { software, err = a.Software.ListPublished(gctx); return })
	g.Go(func() (err error) { highlights, err = a.Highlights.ListPublished(gctx); return })
	g.Go(func() (err error) { courses, err = a.Courses.ListPublished(gctx); return })
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
	imageURL := absoluteImageURL(base.SiteURL, profile.PhotoObject)

	a.View.Render(w, http.StatusOK, "home", HomeData{
		BaseData:    base,
		Profile:     profile,
		Skills:      skills,
		Companies:   companies,
		Articles:    articles,
		Software:    software,
		Highlights:  homeHighlights(highlights, profile, len(companies), len(courses)),
		CourseCount: len(courses),
		Subscribe:   SubscribeFormData{Source: "home", Subscribed: r.URL.Query().Get("subscribed") == "1"},
		JSONLD:      personAndWebsiteJSONLD(profile, base.CanonicalURL, imageURL, sameAs, skills),
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
	// CaseStudies is the same published list Consultancy renders — one
	// canonical source of proof-of-work for both pages. Empty until
	// Vishnu adds a real one; see migration 060.
	CaseStudies []models.CaseStudy
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
		caseStudies  []models.CaseStudy
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
	g.Go(func() (err error) { caseStudies, err = a.CaseStudies.ListPublished(gctx); return })
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
		CaseStudies:    caseStudies,
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
	// HasCourses gates the "Where to start" guided-path section, which
	// always reflects the full catalog regardless of the filters below --
	// a tag/search filter narrows "Every course," not the recommended path.
	HasCourses bool
	TrackStart []models.Course
	TrackBuild []models.Course
	TrackDeep  []models.Course
	// Courses is the current page of the filtered/sorted "Every course"
	// list -- same list-style UX as /articles (search, tag nav, sort,
	// pagination), not the full catalog.
	Courses    []models.Course
	Tags       []models.CourseTag
	Query      string
	ActiveTag  string
	Sort       string
	Page       int
	TotalPages int
	Total      int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
	Subscribe  SubscribeFormData
}

// coursesPageSize is large enough that the whole catalog renders on one
// page: /courses is a single indexable hub, not a paginated archive.
const coursesPageSize = 100

func (a *App) CoursesList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	previewAll := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)

	// Every course fits on one page (coursesPageSize), so ?page= has no
	// meaning here: redirect it permanently to the same view without it
	// rather than serve a duplicate of the listing.
	if _, has := q["page"]; has {
		q.Del("page")
		target := r.URL.Path
		if enc := q.Encode(); enc != "" {
			target += "?" + enc
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
		return
	}
	page := 1
	sort := q.Get("sort")
	if sort != "reviews_desc" && sort != "newest" {
		sort = "recommended"
	}
	opt := repo.CourseListOptions{
		Query:              strings.TrimSpace(q.Get("q")),
		Tag:                strings.TrimSpace(q.Get("tag")),
		Sort:               sort,
		Page:               page,
		PageSize:           coursesPageSize,
		IncludeUnpublished: previewAll,
	}

	// Three independent lookups -- the filtered/paginated page, the full
	// catalog that backs the always-complete "Where to start" tracks, and
	// the distinct tag list -- go out concurrently rather than as a serial
	// chain (CLAUDE.md §9). The chrome's own prefetch joins the same wave.
	var (
		result repo.CourseListResult
		allSrc []models.Course
		tags   []models.CourseTag
	)
	pf := a.startBase(r)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { result, err = a.Courses.ListPublishedFiltered(gctx, opt); return })
	g.Go(func() (err error) {
		if previewAll {
			allSrc, err = a.Courses.ListAll(gctx)
		} else {
			allSrc, err = a.Courses.ListPublished(gctx)
		}
		return
	})
	g.Go(func() (err error) { tags, err = a.Courses.ListAllTags(gctx); return })
	if err := g.Wait(); err != nil {
		a.serverError(w, "load courses page data", err)
		return
	}

	totalPages := (result.Total + coursesPageSize - 1) / coursesPageSize
	if totalPages < 1 {
		totalPages = 1
	}

	// A search-results view (?q=) is site-internal search, not content
	// worth its own index entry -- see the identical note in
	// ArticlesPage/articles.go.
	if opt.Query != "" {
		w.Header().Set("X-Robots-Tag", "noindex, follow")
	}

	data := CoursesListData{
		BaseData:   a.baseFrom(r, pf, catalogListTitle("Java, Architecture & System Design Courses — "+a.Cfg.SiteName, opt.Tag, page), "Practical, production-grade courses on Java, technical architecture, system design, and distributed systems from a working technical consultant.", "courses"),
		HasCourses: len(allSrc) > 0,
		Courses:    result.Courses,
		Tags:       tags,
		Query:      opt.Query,
		ActiveTag:  opt.Tag,
		Sort:       sort,
		Page:       page,
		TotalPages: totalPages,
		Total:      result.Total,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
		PrevPage:   page - 1,
		NextPage:   page + 1,
		Subscribe:  SubscribeFormData{Source: "courses", Subscribed: q.Get("subscribed") == "1"},
	}
	data.BreadcrumbJSONLD = breadcrumbJSONLD(a.trail(r, crumb{"Courses", a.pageURL(r, "/courses")}))
	for _, c := range allSrc {
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
	Course           models.Course
	JSONLD           template.JS
	PaymentsEnabled  bool
	RazorpayKeyID    string
	PurchaseRequired bool
	HasAccess        bool
	// ReceiptURL is empty unless HasAccess is backed by a real purchase
	// (not admin preview) — the course page's "Download Order Receipt"
	// button only renders when it's set.
	ReceiptURL         string
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
	// ReviewCount is the only review data ever shown publicly — never the
	// rating value or comment text (that's admin-only, see
	// AdminCourseReviewsListData).
	ReviewCount int
	// CanReview mirrors "has a real purchase" (same condition as
	// ReceiptURL being set) — admin preview never counts as a buyer.
	CanReview     bool
	MyReview      *models.CourseReview
	ReviewSaved   bool
	ReviewInvalid bool
}

func (a *App) CourseDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	seoWait := a.seoFieldsAsync(ctx, repo.SEOCourse, slug)

	// Wave one: everything that only needs the request — the course row,
	// the chrome, and the signed-in visitor's email for the checkout form.
	pf := a.startBase(r)
	var (
		course    models.Course
		userEmail string
	)
	uid, loggedIn := a.Auth.UserID(r)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { course, err = a.Courses.GetBySlug(gctx, slug); return })
	if loggedIn {
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
	var quizCount, reviewCount int
	var myReview *models.CourseReview
	var backfilledOutline string
	var backfilledPreview, backfilled bool
	g2, g2ctx := errgroup.WithContext(ctx)
	g2.Go(func() (err error) { quizCount, err = a.QuizQuestions.CountByCourse(g2ctx, course.ID); return })
	g2.Go(func() (err error) { reviewCount, err = a.CourseReviews.CountByCourse(g2ctx, course.ID); return })
	if needsPreviewBuild(course) && a.CourseStorage != nil {
		// Lazy backfill of "What's inside" (migration 079) and the free
		// preview flag (086) for a lesson whose derived columns are
		// missing: rebuild them once per previewRetryAfter (migration
		// 091) so a lesson with nothing to preview is not re-read on
		// every view. Best-effort: a Storage hiccup costs this one
		// view its outline, never the page.
		c := course
		g2.Go(func() error {
			backfilledOutline, backfilledPreview, backfilled = a.backfillCoursePreview(g2ctx, c)
			return nil
		})
	}
	if loggedIn {
		g2.Go(func() error {
			// Best-effort: a buyer's own review prefilling the form is a
			// nice-to-have, not something that should fail the page.
			if rev, err := a.CourseReviews.GetByUserAndCourse(g2ctx, course.ID, uid); err == nil {
				myReview = &rev
			}
			return nil
		})
	}
	hasAccess := false
	receiptURL := ""
	if course.IsPaid {
		var purchaseID int64
		hasAccess, _, purchaseID = a.courseAccess(w, r, course)
		if purchaseID != 0 {
			receiptURL = "/account/purchases/" + strconv.FormatInt(purchaseID, 10) + "/receipt.pdf"
		}
	}
	if err := g2.Wait(); err != nil {
		a.serverError(w, "load quiz count", err)
		return
	}
	if backfilled {
		course.Outline = backfilledOutline
		course.PreviewReady = backfilledPreview
	}

	seo := seoWait()
	description := seo.Description(course.Summary, course.Description)
	base := a.baseFrom(r, pf, seo.Title(course.Title+" — "+a.Cfg.SiteName), description, "courses")
	base.OGImageURL = ogImageFor(a.baseURL(r), seo, course.ImageURL)
	base.BreadcrumbJSONLD = breadcrumbJSONLD(a.trail(r,
		crumb{"Courses", a.pageURL(r, "/courses")}, crumb{course.Title, base.CanonicalURL}))

	minCents := course.FixedPriceCents()

	a.View.Render(w, http.StatusOK, "course_detail", CourseDetailData{
		BaseData:           base,
		Course:             course,
		JSONLD:             courseJSONLD(course, description, base.CanonicalURL, base.OGImageURL, a.Cfg.SiteName),
		PaymentsEnabled:    a.Razorpay.Enabled(),
		RazorpayKeyID:      a.Razorpay.KeyID,
		PurchaseRequired:   r.URL.Query().Get("purchase") == "required",
		HasAccess:          hasAccess,
		ReceiptURL:         receiptURL,
		ResolvedMinCents:   minCents,
		ResolvedMinWhole:   minCents / 100,
		ResolvedMinDisplay: models.FormatMoney(minCents, models.CurrencyINR),
		UserEmail:          userEmail,
		HasQuiz:            quizCount > 0,
		QuizPassPercent:    models.QuizPassPercent,
		ReviewCount:        reviewCount,
		CanReview:          receiptURL != "",
		MyReview:           myReview,
		ReviewSaved:        r.URL.Query().Get("reviewed") == "1",
		ReviewInvalid:      r.URL.Query().Get("review_error") == "1",
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
	if a.redirectRenamed(w, r) {
		return
	}
	base := a.base(r, "Page not found — "+a.Cfg.SiteName, "", "")
	base.NoCanonical = true // a 404 must not declare itself canonical
	a.View.Render(w, http.StatusNotFound, "not_found", base)
}

func (a *App) serverError(w http.ResponseWriter, action string, err error) {
	log.Printf("%s: %v", action, err)
	http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
}

// homeHighlights returns the admin-entered highlights, or — when none are
// published — a fallback built only from facts the database already
// holds, so the hero's stat strip is never empty and never invented.
func homeHighlights(published []models.ProfileHighlight, p models.Profile, companies, courses int) []models.ProfileHighlight {
	if len(published) > 0 {
		return published
	}
	var out []models.ProfileHighlight
	if p.YearsExperience > 0 {
		out = append(out, models.ProfileHighlight{Value: strconv.Itoa(p.YearsExperience) + "+", Label: "years building production systems"})
	}
	if companies > 0 {
		out = append(out, models.ProfileHighlight{Value: strconv.Itoa(companies), Label: "companies shipped for"})
	}
	if courses > 0 {
		out = append(out, models.ProfileHighlight{Value: strconv.Itoa(courses), Label: "self-paced courses"})
	}
	// The free scoping call is a standing offer (see Consultancy), not a
	// metric, so it is a safe fourth tile that keeps the 2 × 2 grid whole.
	if len(out) == 3 {
		out = append(out, models.ProfileHighlight{Value: "Free", Label: "first call to scope your problem"})
	}
	return out
}

// redirectRenamed answers a would-be 404 with a permanent redirect when
// the path was moved by a course slug rename (slug_redirects, migration
// 086). Only GET and HEAD are redirected, so a stale POST still fails
// loudly. It runs only on the 404 path, so live routes never query it.
func (a *App) redirectRenamed(w http.ResponseWriter, r *http.Request) bool {
	if a.SlugRedirects == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return false
	}
	to, err := a.SlugRedirects.Lookup(r.Context(), r.URL.Path)
	if err != nil || to == "" {
		return false
	}
	if r.URL.RawQuery != "" {
		to += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, to, http.StatusMovedPermanently)
	return true
}
