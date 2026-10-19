package handlers

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/certificate"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// quizIdentity resolves who is taking a course's quiz. An authenticated
// admin always qualifies (using their own admin account's name/email,
// mirroring the full course access they already get elsewhere) even
// without a separate visitor account; otherwise a logged-in visitor's
// account is used. Name/email always come from the account, never from
// client input, so a quiz attempt and certificate can't be forged under
// someone else's name.
func (a *App) quizIdentity(r *http.Request) (userID *int64, isAdmin bool, name, email string, ok bool) {
	ctx := r.Context()
	if adminID, has := a.Auth.AdminID(r); has && !a.isPreviewingAsVisitor(r) {
		if admin, err := a.Admins.GetByID(ctx, adminID); err == nil {
			return nil, true, admin.Name, admin.Email, true
		}
	}
	if uid, has := a.Auth.UserID(r); has {
		if user, err := a.Users.GetByID(ctx, uid); err == nil {
			id := uid
			return &id, false, user.Name, user.Email, true
		}
	}
	return nil, false, "", "", false
}

// quizCourseAndAccess loads the course for the quiz/certificate routes and
// applies the same visibility + payment-access rules as the course detail
// and learn pages: unpublished courses are admin-only, and only someone
// with real access to the (always-paid) course content may reach its quiz.
func (a *App) quizCourseAndAccess(w http.ResponseWriter, r *http.Request) (course models.Course, ok bool) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	isAdminReq := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)
	if err != nil || !course.IsPaid || (!course.IsPublished && !isAdminReq) {
		a.NotFound(w, r)
		return models.Course{}, false
	}
	hasAccess, _, _ := a.courseAccess(w, r, course)
	if !hasAccess {
		http.Redirect(w, r, "/courses/"+slug+"?purchase=required", http.StatusSeeOther)
		return models.Course{}, false
	}
	return course, true
}

type QuizPageData struct {
	BaseData
	Course    models.Course
	Questions []models.QuizQuestion
}

func (a *App) QuizPage(w http.ResponseWriter, r *http.Request) {
	course, ok := a.quizCourseAndAccess(w, r)
	if !ok {
		return
	}
	questions, err := a.QuizQuestions.ListByCourse(r.Context(), course.ID)
	if err != nil {
		a.serverError(w, "load quiz", err)
		return
	}
	if len(questions) == 0 {
		a.NotFound(w, r)
		return
	}
	a.View.Render(w, http.StatusOK, "quiz", QuizPageData{
		BaseData:  a.base(r, "Final Quiz — "+course.Title+" — "+a.Cfg.SiteName, "", "courses"),
		Course:    course,
		Questions: questions,
	})
}

type QuizResultData struct {
	BaseData
	Course  models.Course
	Attempt models.QuizAttempt
	// PaymentsEnabled gates the optional contribution box.
	PaymentsEnabled bool
}

func (a *App) QuizSubmit(w http.ResponseWriter, r *http.Request) {
	course, ok := a.quizCourseAndAccess(w, r)
	if !ok {
		return
	}
	userID, isAdmin, name, email, hasIdentity := a.quizIdentity(r)
	if !hasIdentity {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	questions, err := a.QuizQuestions.ListByCourse(r.Context(), course.ID)
	if err != nil {
		a.serverError(w, "load quiz", err)
		return
	}
	if len(questions) == 0 {
		a.NotFound(w, r)
		return
	}

	correct := 0
	for _, q := range questions {
		if strings.EqualFold(r.FormValue("q"+strconv.FormatInt(q.ID, 10)), q.CorrectOption) {
			correct++
		}
	}
	total := len(questions)
	scorePercent := correct * 100 / total
	passed := scorePercent >= models.QuizPassPercent

	attempt := models.QuizAttempt{
		CourseID:       course.ID,
		UserID:         userID,
		IsAdmin:        isAdmin,
		Name:           name,
		Email:          email,
		TotalQuestions: total,
		CorrectAnswers: correct,
		ScorePercent:   scorePercent,
		Passed:         passed,
	}
	if passed {
		code, err := generateCertificateCode()
		if err != nil {
			a.serverError(w, "generate certificate code", err)
			return
		}
		attempt.CertificateCode = code
	}
	if _, err := a.QuizAttempts.Create(r.Context(), attempt); err != nil {
		a.serverError(w, "record quiz attempt", err)
		return
	}

	a.View.Render(w, http.StatusOK, "quiz_result", QuizResultData{
		BaseData: a.base(r, "Quiz Results — "+course.Title+" — "+a.Cfg.SiteName, "", "courses"),
		Course:   course,
		Attempt:  attempt,

		PaymentsEnabled: a.Razorpay.Enabled() && !isAdmin,
	})
}

// CertificateDownload streams a freshly-generated PDF for the caller's own
// best passing attempt on this course. Nothing is cached server-side — the
// PDF is cheap to render and this guarantees it always reflects the actual
// recorded attempt.
func (a *App) CertificateDownload(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	_, _, name, email, hasIdentity := a.quizIdentity(r)
	if !hasIdentity {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
		return
	}

	attempt, err := a.QuizAttempts.BestPassingAttempt(r.Context(), course.ID, email)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load quiz attempt", err)
		return
	}

	issuer := a.Cfg.SiteName
	if profile, err := a.Profile.Get(r.Context()); err == nil && profile.Name != "" {
		issuer = profile.Name
	}

	pdfBytes, err := certificate.Generate(certificate.Data{
		RecipientName: certificate.RecipientName(name, email),
		CourseTitle:   course.Title,
		IssuedAt:      attempt.CreatedAt,
		Code:          attempt.CertificateCode,
		SiteName:      a.Cfg.SiteName,
		IssuerName:    issuer,
		ScorePercent:  attempt.ScorePercent,
		PassPercent:   models.QuizPassPercent,
		VerifyURL:     strings.TrimRight(a.Cfg.SiteURL, "/") + "/certificate/verify/" + attempt.CertificateCode,
	})
	if err != nil {
		a.serverError(w, "generate certificate", err)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="certificate-`+course.Slug+`.pdf"`)
	w.Write(pdfBytes)
}

// CertificateVerifyData drives the public certificate verification page.
type CertificateVerifyData struct {
	BaseData
	Valid  bool
	Name   string
	Course string
	Date   string
	Score  int
	Pass   int
	Code   string
}

// CertificateVerify is the public page a certificate's printed code and URL
// point to. It confirms a certificate exists (a recorded, passed, non-admin
// quiz attempt) and shows only the printed facts: name, course, date and
// score. It is noindex and the response is identical in shape for unknown
// codes, so it reveals nothing beyond what the certificate itself shows.
func (a *App) CertificateVerify(w http.ResponseWriter, r *http.Request) {
	code := certificate.NormalizeCode(r.PathValue("code"))
	d := CertificateVerifyData{Code: certificate.FormatCode(code), Pass: models.QuizPassPercent}
	status := http.StatusNotFound
	if code != "" {
		attempt, err := a.QuizAttempts.GetByCertificateCode(r.Context(), code)
		if err != nil && err != repo.ErrNotFound {
			a.serverError(w, "verify certificate", err)
			return
		}
		// Admin attempts are testing only and never verify as real.
		if err == nil && attempt.Passed && !attempt.IsAdmin {
			if course, cerr := a.Courses.GetByID(r.Context(), attempt.CourseID); cerr == nil {
				d.Valid = true
				d.Name = certificate.RecipientName(attempt.Name, attempt.Email)
				d.Course = certificate.SanitizeText(course.Title)
				d.Date = certificate.DateText(attempt.CreatedAt)
				d.Score = attempt.ScorePercent
				status = http.StatusOK
			}
		}
	}
	d.BaseData = a.base(r, "Verify certificate - "+a.Cfg.SiteName, "Check that a certificate was genuinely issued.", "")
	d.NoIndex = true
	w.Header().Set("Cache-Control", "no-store")
	a.View.Render(w, status, "certificate_verify", d)
}

func generateCertificateCode() (string, error) {
	code, err := authutil.RandomHex(8)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(code), nil
}
