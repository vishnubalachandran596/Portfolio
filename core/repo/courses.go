package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

var ErrNotFound = errors.New("not found")

type CourseRepo struct{ db *sql.DB }

func NewCourseRepo(db *sql.DB) *CourseRepo { return &CourseRepo{db: db} }

const courseColumns = `id, slug, title, summary, description, level, duration, price_cents, currency, image_url, is_published, sort_order, is_paid, min_price_cents, min_price_cents_inr, content_key, created_at, updated_at`

func scanCourse(row interface{ Scan(...any) error }) (models.Course, error) {
	var c models.Course
	err := row.Scan(&c.ID, &c.Slug, &c.Title, &c.Summary, &c.Description, &c.Level, &c.Duration,
		&c.PriceCents, &c.Currency, &c.ImageURL, &c.IsPublished, &c.SortOrder,
		&c.IsPaid, &c.MinPriceCents, &c.MinPriceCentsINR, &c.ContentKey, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *CourseRepo) ListPublished(ctx context.Context) ([]models.Course, error) {
	return r.list(ctx, `SELECT `+courseColumns+` FROM courses WHERE is_published = TRUE ORDER BY sort_order, id`)
}

func (r *CourseRepo) ListAll(ctx context.Context) ([]models.Course, error) {
	return r.list(ctx, `SELECT `+courseColumns+` FROM courses ORDER BY sort_order, id`)
}

func (r *CourseRepo) list(ctx context.Context, query string) ([]models.Course, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Course
	for rows.Next() {
		c, err := scanCourse(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CourseRepo) GetBySlug(ctx context.Context, slug string) (models.Course, error) {
	c, err := scanCourse(r.db.QueryRowContext(ctx, `SELECT `+courseColumns+` FROM courses WHERE slug = $1`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Course{}, ErrNotFound
	}
	return c, err
}

func (r *CourseRepo) GetByID(ctx context.Context, id int64) (models.Course, error) {
	c, err := scanCourse(r.db.QueryRowContext(ctx, `SELECT `+courseColumns+` FROM courses WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Course{}, ErrNotFound
	}
	return c, err
}

func (r *CourseRepo) Create(ctx context.Context, c models.Course) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO courses (slug, title, summary, description, level, duration, price_cents, currency, image_url, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		c.Slug, c.Title, c.Summary, c.Description, c.Level, c.Duration, c.PriceCents, c.Currency, c.ImageURL, c.IsPublished, c.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *CourseRepo) Update(ctx context.Context, c models.Course) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE courses SET slug=$1, title=$2, summary=$3, description=$4, level=$5, duration=$6,
			price_cents=$7, currency=$8, image_url=$9, is_published=$10, sort_order=$11, updated_at=now()
		WHERE id=$12`,
		c.Slug, c.Title, c.Summary, c.Description, c.Level, c.Duration, c.PriceCents, c.Currency, c.ImageURL, c.IsPublished, c.SortOrder, c.ID,
	)
	return err
}

func (r *CourseRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM courses WHERE id = $1`, id)
	return err
}
