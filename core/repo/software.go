package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type SoftwareRepo struct{ db *sql.DB }

func NewSoftwareRepo(db *sql.DB) *SoftwareRepo { return &SoftwareRepo{db: db} }

const softwareColumns = `id, slug, title, version, description, rag_context, download_link, price_cents, license_terms, post_purchase_instructions, is_published, sort_order, created_at, updated_at`

func scanSoftware(row interface{ Scan(...any) error }) (models.Software, error) {
	var s models.Software
	err := row.Scan(&s.ID, &s.Slug, &s.Title, &s.Version, &s.Description, &s.RagContext, &s.DownloadLink,
		&s.PriceCents, &s.LicenseTerms, &s.PostPurchaseInstructions, &s.IsPublished, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *SoftwareRepo) ListPublished(ctx context.Context) ([]models.Software, error) {
	return r.list(ctx, `SELECT `+softwareColumns+` FROM software WHERE is_published = TRUE ORDER BY sort_order, id`)
}

func (r *SoftwareRepo) ListAll(ctx context.Context) ([]models.Software, error) {
	return r.list(ctx, `SELECT `+softwareColumns+` FROM software ORDER BY sort_order, id`)
}

func (r *SoftwareRepo) list(ctx context.Context, query string) ([]models.Software, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Software
	for rows.Next() {
		s, err := scanSoftware(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SoftwareRepo) GetBySlug(ctx context.Context, slug string) (models.Software, error) {
	s, err := scanSoftware(r.db.QueryRowContext(ctx, `SELECT `+softwareColumns+` FROM software WHERE slug = $1`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Software{}, ErrNotFound
	}
	return s, err
}

func (r *SoftwareRepo) GetByID(ctx context.Context, id int64) (models.Software, error) {
	s, err := scanSoftware(r.db.QueryRowContext(ctx, `SELECT `+softwareColumns+` FROM software WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Software{}, ErrNotFound
	}
	return s, err
}

func (r *SoftwareRepo) Create(ctx context.Context, s models.Software) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO software (slug, title, version, description, rag_context, download_link, price_cents, license_terms, post_purchase_instructions, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		s.Slug, s.Title, s.Version, s.Description, s.RagContext, s.DownloadLink, s.PriceCents,
		s.LicenseTerms, s.PostPurchaseInstructions, s.IsPublished, s.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *SoftwareRepo) Update(ctx context.Context, s models.Software) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE software SET slug=$1, title=$2, version=$3, description=$4, rag_context=$5, download_link=$6,
			price_cents=$7, license_terms=$8, post_purchase_instructions=$9, is_published=$10, sort_order=$11, updated_at=now()
		WHERE id=$12`,
		s.Slug, s.Title, s.Version, s.Description, s.RagContext, s.DownloadLink, s.PriceCents,
		s.LicenseTerms, s.PostPurchaseInstructions, s.IsPublished, s.SortOrder, s.ID,
	)
	return err
}

func (r *SoftwareRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM software WHERE id = $1`, id)
	return err
}
