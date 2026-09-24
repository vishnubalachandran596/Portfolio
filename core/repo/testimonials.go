package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type TestimonialRepo struct{ db *sql.DB }

func NewTestimonialRepo(db *sql.DB) *TestimonialRepo { return &TestimonialRepo{db: db} }

const testimonialColumns = `id, user_id, author_name, author_title, body, is_published, sort_order, created_at`

func scanTestimonial(row interface{ Scan(...any) error }) (models.Testimonial, error) {
	var t models.Testimonial
	var userID sql.NullInt64
	err := row.Scan(&t.ID, &userID, &t.AuthorName, &t.AuthorTitle, &t.Body, &t.IsPublished, &t.SortOrder, &t.CreatedAt)
	t.UserID = userID.Int64
	return t, err
}

// ListPublished returns every published testimonial, in display order —
// the source for the About page's testimonials section.
func (r *TestimonialRepo) ListPublished(ctx context.Context) ([]models.Testimonial, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+testimonialColumns+` FROM testimonials WHERE is_published = TRUE ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Testimonial
	for rows.Next() {
		t, err := scanTestimonial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TestimonialRepo) ListAll(ctx context.Context) ([]models.Testimonial, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+testimonialColumns+` FROM testimonials ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Testimonial
	for rows.Next() {
		t, err := scanTestimonial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TestimonialRepo) GetByID(ctx context.Context, id int64) (models.Testimonial, error) {
	t, err := scanTestimonial(r.db.QueryRowContext(ctx, `SELECT `+testimonialColumns+` FROM testimonials WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Testimonial{}, ErrNotFound
	}
	return t, err
}

// GetByUserID returns the testimonial a given visitor submitted themselves,
// if any — used so /account shows their submission's status instead of
// letting them submit a second one.
func (r *TestimonialRepo) GetByUserID(ctx context.Context, userID int64) (models.Testimonial, error) {
	t, err := scanTestimonial(r.db.QueryRowContext(ctx, `SELECT `+testimonialColumns+` FROM testimonials WHERE user_id = $1`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Testimonial{}, ErrNotFound
	}
	return t, err
}

func (r *TestimonialRepo) Create(ctx context.Context, t models.Testimonial) (int64, error) {
	var id int64
	var userID any
	if t.UserID != 0 {
		userID = t.UserID
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO testimonials (user_id, author_name, author_title, body, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		userID, t.AuthorName, t.AuthorTitle, t.Body, t.IsPublished, t.SortOrder,
	).Scan(&id)
	return id, err
}

// CreateFromUser records a testimonial submitted by a logged-in visitor
// themselves. It always starts unpublished — visible only after an admin
// approves it — regardless of anything the visitor could submit.
func (r *TestimonialRepo) CreateFromUser(ctx context.Context, userID int64, authorName, authorTitle, body string) (int64, error) {
	return r.Create(ctx, models.Testimonial{
		UserID:      userID,
		AuthorName:  authorName,
		AuthorTitle: authorTitle,
		Body:        body,
		IsPublished: false,
	})
}

func (r *TestimonialRepo) Update(ctx context.Context, t models.Testimonial) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE testimonials SET author_name=$1, author_title=$2, body=$3, is_published=$4, sort_order=$5 WHERE id=$6`,
		t.AuthorName, t.AuthorTitle, t.Body, t.IsPublished, t.SortOrder, t.ID,
	)
	return err
}

// Approve publishes a pending testimonial — the one-click admin moderation
// action, without needing to open the full edit form.
func (r *TestimonialRepo) Approve(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE testimonials SET is_published = TRUE WHERE id = $1`, id)
	return err
}

func (r *TestimonialRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM testimonials WHERE id = $1`, id)
	return err
}
