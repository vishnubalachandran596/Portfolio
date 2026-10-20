package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type SoftwareRepo struct{ db *sql.DB }

func NewSoftwareRepo(db *sql.DB) *SoftwareRepo { return &SoftwareRepo{db: db} }

const softwareColumns = `id, slug, title, version, summary, description, rag_context, download_link, price_cents, license_terms, post_purchase_instructions, is_published, sort_order, licence_product, COALESCE(microsoft_store_url,''), created_at, updated_at`

func scanSoftware(row interface{ Scan(...any) error }) (models.Software, error) {
	var s models.Software
	err := row.Scan(&s.ID, &s.Slug, &s.Title, &s.Version, &s.Summary, &s.Description, &s.RagContext, &s.DownloadLink,
		&s.PriceCents, &s.LicenseTerms, &s.PostPurchaseInstructions, &s.IsPublished, &s.SortOrder, &s.LicenceProduct, &s.MicrosoftStoreURL, &s.CreatedAt, &s.UpdatedAt)
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
		INSERT INTO software (slug, title, version, summary, description, rag_context, download_link, price_cents, license_terms, post_purchase_instructions, is_published, sort_order, licence_product, microsoft_store_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,'')) RETURNING id`,
		s.Slug, s.Title, s.Version, s.Summary, s.Description, s.RagContext, s.DownloadLink, s.PriceCents,
		s.LicenseTerms, s.PostPurchaseInstructions, s.IsPublished, s.SortOrder, s.LicenceProduct, s.MicrosoftStoreURL,
	).Scan(&id)
	return id, err
}

func (r *SoftwareRepo) Update(ctx context.Context, s models.Software) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE software SET slug=$1, title=$2, version=$3, summary=$4, description=$5, rag_context=$6, download_link=$7,
			price_cents=$8, license_terms=$9, post_purchase_instructions=$10, is_published=$11, sort_order=$12, licence_product=$13, microsoft_store_url=NULLIF($14,''), updated_at=now()
		WHERE id=$15`,
		s.Slug, s.Title, s.Version, s.Summary, s.Description, s.RagContext, s.DownloadLink, s.PriceCents,
		s.LicenseTerms, s.PostPurchaseInstructions, s.IsPublished, s.SortOrder, s.LicenceProduct, s.MicrosoftStoreURL, s.ID,
	)
	return err
}

func (r *SoftwareRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM software WHERE id = $1`, id)
	return err
}
