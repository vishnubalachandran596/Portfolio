package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// Certifications and publications are two more small About-page aggregates,
// kept in one file the way profile.go keeps skills, experience, and
// education together — they are the same shape of thing (a short, ordered,
// admin-curated list rendered in one section) and splitting them into two
// near-identical files would only spread the same pattern thinner.
//
// Both ListPublished methods return a nil slice when there are no rows,
// which is what lets the About page drop the whole section rather than
// render an empty heading. See core/view/templates/pages/about.html.

type CertificationRepo struct{ db *sql.DB }

func NewCertificationRepo(db *sql.DB) *CertificationRepo { return &CertificationRepo{db: db} }

const certificationColumns = `id, name, issuer, issued_on, expires_on, credential_url, is_published, sort_order, created_at`

func scanCertification(row interface{ Scan(...any) error }) (models.Certification, error) {
	var c models.Certification
	var issued, expires sql.NullTime
	err := row.Scan(&c.ID, &c.Name, &c.Issuer, &issued, &expires, &c.CredentialURL,
		&c.IsPublished, &c.SortOrder, &c.CreatedAt)
	if issued.Valid {
		c.IssuedOn = &issued.Time
	}
	if expires.Valid {
		c.ExpiresOn = &expires.Time
	}
	return c, err
}

func (r *CertificationRepo) listWhere(ctx context.Context, where string) ([]models.Certification, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+certificationColumns+` FROM certifications `+where+` ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Certification
	for rows.Next() {
		c, err := scanCertification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListPublished feeds the public About page.
func (r *CertificationRepo) ListPublished(ctx context.Context) ([]models.Certification, error) {
	return r.listWhere(ctx, `WHERE is_published = TRUE`)
}

// ListAll feeds the admin console, which needs to see hidden rows too.
func (r *CertificationRepo) ListAll(ctx context.Context) ([]models.Certification, error) {
	return r.listWhere(ctx, ``)
}

func (r *CertificationRepo) GetByID(ctx context.Context, id int64) (models.Certification, error) {
	c, err := scanCertification(r.db.QueryRowContext(ctx,
		`SELECT `+certificationColumns+` FROM certifications WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Certification{}, ErrNotFound
	}
	return c, err
}

func (r *CertificationRepo) Create(ctx context.Context, c models.Certification) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO certifications (name, issuer, issued_on, expires_on, credential_url, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		c.Name, c.Issuer, c.IssuedOn, c.ExpiresOn, c.CredentialURL, c.IsPublished, c.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *CertificationRepo) Update(ctx context.Context, c models.Certification) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE certifications SET name=$1, issuer=$2, issued_on=$3, expires_on=$4,
		credential_url=$5, is_published=$6, sort_order=$7 WHERE id=$8`,
		c.Name, c.Issuer, c.IssuedOn, c.ExpiresOn, c.CredentialURL, c.IsPublished, c.SortOrder, c.ID,
	)
	return err
}

func (r *CertificationRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM certifications WHERE id = $1`, id)
	return err
}

type PublicationRepo struct{ db *sql.DB }

func NewPublicationRepo(db *sql.DB) *PublicationRepo { return &PublicationRepo{db: db} }

const publicationColumns = `id, title, kind, venue, url, published_on, summary, is_published, sort_order, created_at`

func scanPublication(row interface{ Scan(...any) error }) (models.Publication, error) {
	var p models.Publication
	var published sql.NullTime
	err := row.Scan(&p.ID, &p.Title, &p.Kind, &p.Venue, &p.URL, &published, &p.Summary,
		&p.IsPublished, &p.SortOrder, &p.CreatedAt)
	if published.Valid {
		p.PublishedOn = &published.Time
	}
	return p, err
}

func (r *PublicationRepo) listWhere(ctx context.Context, where string) ([]models.Publication, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+publicationColumns+` FROM publications `+where+` ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Publication
	for rows.Next() {
		p, err := scanPublication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListPublished feeds the public About page.
func (r *PublicationRepo) ListPublished(ctx context.Context) ([]models.Publication, error) {
	return r.listWhere(ctx, `WHERE is_published = TRUE`)
}

// ListAll feeds the admin console, which needs to see hidden rows too.
func (r *PublicationRepo) ListAll(ctx context.Context) ([]models.Publication, error) {
	return r.listWhere(ctx, ``)
}

func (r *PublicationRepo) GetByID(ctx context.Context, id int64) (models.Publication, error) {
	p, err := scanPublication(r.db.QueryRowContext(ctx,
		`SELECT `+publicationColumns+` FROM publications WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Publication{}, ErrNotFound
	}
	return p, err
}

func (r *PublicationRepo) Create(ctx context.Context, p models.Publication) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO publications (title, kind, venue, url, published_on, summary, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		p.Title, p.Kind, p.Venue, p.URL, p.PublishedOn, p.Summary, p.IsPublished, p.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *PublicationRepo) Update(ctx context.Context, p models.Publication) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE publications SET title=$1, kind=$2, venue=$3, url=$4, published_on=$5,
		summary=$6, is_published=$7, sort_order=$8 WHERE id=$9`,
		p.Title, p.Kind, p.Venue, p.URL, p.PublishedOn, p.Summary, p.IsPublished, p.SortOrder, p.ID,
	)
	return err
}

func (r *PublicationRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM publications WHERE id = $1`, id)
	return err
}
