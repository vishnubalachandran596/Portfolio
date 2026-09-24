package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// AdminResumePreview streams one stored resume PDF for a logged-in admin.
// roleKey is "default" for the main download, or the experience slug for
// a role file. A missing row is a 404 — this route does not generate a
// fallback PDF and does not substitute the default file.
func (a *App) AdminResumePreview(w http.ResponseWriter, r *http.Request) {
	roleKey := r.PathValue("roleKey")
	if !roleKeyPattern.MatchString(roleKey) {
		a.NotFound(w, r)
		return
	}
	row, err := a.Resumes.Get(r.Context(), roleKey)
	if err == repo.ErrNotFound || (err == nil && row.ObjectPath == "") {
		a.NotFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, "load resume", err)
		return
	}
	a.writeStoredResume(w, r, row)
}

// AdminArticlePreview renders an article the way the public page does,
// including a hidden draft and a post whose HTML lives only in Storage.
// The browser never receives a Storage URL.
func (a *App) AdminArticlePreview(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	article, err := a.Articles.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load article", err)
		return
	}
	if article.ContentObject == "" && strings.TrimSpace(article.Body) == "" {
		a.NotFound(w, r)
		return
	}
	if err := a.loadArticleHTML(r.Context(), &article); err != nil {
		a.serverError(w, "load article html", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store, private")
	base := a.base(r, article.Title+" — Preview", article.Summary, "articles")
	a.View.Render(w, http.StatusOK, "article_detail", ArticleDetailData{
		BaseData: base,
		Article:  article,
		JSONLD:   articleJSONLD(article, base.CanonicalURL, a.Cfg.SiteName),
	})
}

// AdminCoursePreview writes the stored lesson HTML for a logged-in admin.
// Visitors still receive the watermarked shell from CourseLearn; this
// route shows the uploaded file so the lesson can be checked before it
// is published. The browser never receives a Storage URL.
func (a *App) AdminCoursePreview(w http.ResponseWriter, r *http.Request) {
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
	if course.ContentObject == "" {
		a.NotFound(w, r)
		return
	}
	raw, err := a.readArticleObject(r.Context(), course.ContentObject)
	if err != nil {
		a.serverError(w, "load course html", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Write(raw)
}
