package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// AdminCourseReviewsListData backs /admin/courses/{courseID}/reviews — the
// only place a review's rating value and comment text are ever readable.
// The public course page only ever shows the count (CourseDetailData's
// ReviewCount).
type AdminCourseReviewsListData struct {
	BaseData
	Course  models.Course
	Reviews []models.CourseReview
}

func (a *App) AdminCourseReviewsList(w http.ResponseWriter, r *http.Request) {
	courseID, err := strconv.ParseInt(r.PathValue("courseID"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	course, err := a.Courses.GetByID(r.Context(), courseID)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load course", err)
		return
	}
	reviews, err := a.CourseReviews.ListByCourse(r.Context(), courseID)
	if err != nil {
		a.serverError(w, "load course reviews", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_course_reviews_list", AdminCourseReviewsListData{
		BaseData: a.base(r, "Reviews — "+course.Title+" — Admin", "", "admin"),
		Course:   course,
		Reviews:  reviews,
	})
}

// CourseReviewSubmit records (or replaces) a verified buyer's 1-5 star
// rating and optional comment for a course. The content is never shown
// publicly — only the course's total review count is (see
// CourseDetailData.ReviewCount) — so there's no moderation step here: a
// buyer's own review is theirs to set.
func (a *App) CourseReviewSubmit(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	uid, ok := a.Auth.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login?next=/courses/"+slug, http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	course, err := a.Courses.GetBySlug(ctx, slug)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	bought, err := a.Purchases.HasPaidPurchase(ctx, uid, course.ID)
	if err != nil {
		a.serverError(w, "check purchase for review", err)
		return
	}
	if !bought {
		http.Redirect(w, r, "/courses/"+slug, http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	rating, _ := strconv.Atoi(r.FormValue("rating"))
	if rating < 1 || rating > 5 {
		http.Redirect(w, r, "/courses/"+slug+"?review_error=1", http.StatusSeeOther)
		return
	}
	comment := strings.TrimSpace(r.FormValue("comment"))

	if err := a.CourseReviews.Upsert(ctx, course.ID, uid, rating, comment); err != nil {
		a.serverError(w, "save course review", err)
		return
	}
	http.Redirect(w, r, "/courses/"+slug+"?reviewed=1", http.StatusSeeOther)
}
