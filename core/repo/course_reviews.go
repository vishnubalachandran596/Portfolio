package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type CourseReviewRepo struct{ db *sql.DB }

func NewCourseReviewRepo(db *sql.DB) *CourseReviewRepo { return &CourseReviewRepo{db: db} }

const courseReviewColumns = `id, course_id, user_id, rating, comment, created_at, updated_at`

func scanCourseReview(row interface{ Scan(...any) error }) (models.CourseReview, error) {
	var r models.CourseReview
	err := row.Scan(&r.ID, &r.CourseID, &r.UserID, &r.Rating, &r.Comment, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

// CountByCourse returns how many reviews a course has — the only review
// data ever shown on the public course page (never the rating value or
// comment text).
func (r *CourseReviewRepo) CountByCourse(ctx context.Context, courseID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM course_reviews WHERE course_id = $1`, courseID).Scan(&count)
	return count, err
}

// GetByUserAndCourse returns the review a given account already left on a
// course, if any — so the review form can prefill/edit instead of adding
// a second row (course_reviews has a UNIQUE(course_id, user_id)).
func (r *CourseReviewRepo) GetByUserAndCourse(ctx context.Context, courseID, userID int64) (models.CourseReview, error) {
	rev, err := scanCourseReview(r.db.QueryRowContext(ctx,
		`SELECT `+courseReviewColumns+` FROM course_reviews WHERE course_id = $1 AND user_id = $2`, courseID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.CourseReview{}, ErrNotFound
	}
	return rev, err
}

// Upsert records a buyer's review, or replaces their existing one for the
// same course (one review per account per course).
func (r *CourseReviewRepo) Upsert(ctx context.Context, courseID, userID int64, rating int, comment string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO course_reviews (course_id, user_id, rating, comment)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (course_id, user_id) DO UPDATE SET rating = $3, comment = $4, updated_at = now()`,
		courseID, userID, rating, comment,
	)
	return err
}

// ListByCourse returns every review left on a course, newest first, with
// the reviewer's own email/name joined in — the admin-only view at
// /admin/courses/{id}/reviews. Never exposed to a public page.
func (r *CourseReviewRepo) ListByCourse(ctx context.Context, courseID int64) ([]models.CourseReview, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT cr.id, cr.course_id, cr.user_id, cr.rating, cr.comment, cr.created_at, cr.updated_at,
			u.email, u.name
		FROM course_reviews cr
		JOIN users u ON u.id = cr.user_id
		WHERE cr.course_id = $1
		ORDER BY cr.created_at DESC`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.CourseReview
	for rows.Next() {
		var rev models.CourseReview
		if err := rows.Scan(&rev.ID, &rev.CourseID, &rev.UserID, &rev.Rating, &rev.Comment, &rev.CreatedAt, &rev.UpdatedAt,
			&rev.ReviewerEmail, &rev.ReviewerName); err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}
