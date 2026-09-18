package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type TestimonialRepo struct{ db *sql.DB }

func NewTestimonialRepo(db *sql.DB) *TestimonialRepo { return &TestimonialRepo{db: db} }

const testimonialColumns = `id, author_name, author_title, body, is_published, sort_order, created_at`

func scanTestimonial(row interface{ Scan(...any) error }) (models.Testimonial, error) {
	var t models.Testimonial
	err := row.Scan(&t.ID, &t.AuthorName, &t.AuthorTitle, &t.Body, &t.IsPublished, &t.SortOrder, &t.CreatedAt)
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

func (r *TestimonialRepo) Create(ctx context.Context, t models.Testimonial) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO testimonials (author_name, author_title, body, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		t.AuthorName, t.AuthorTitle, t.Body, t.IsPublished, t.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *TestimonialRepo) Update(ctx context.Context, t models.Testimonial) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE testimonials SET author_name=$1, author_title=$2, body=$3, is_published=$4, sort_order=$5 WHERE id=$6`,
		t.AuthorName, t.AuthorTitle, t.Body, t.IsPublished, t.SortOrder, t.ID,
	)
	return err
}

func (r *TestimonialRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM testimonials WHERE id = $1`, id)
	return err
}
