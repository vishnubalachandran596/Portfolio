package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type PromoCodeRepo struct{ db *sql.DB }

func NewPromoCodeRepo(db *sql.DB) *PromoCodeRepo { return &PromoCodeRepo{db: db} }

// ErrDuplicateCode is returned when a promo code's text collides with an
// existing one (the UNIQUE constraint on promo_codes.code).
var ErrDuplicateCode = errors.New("promo code already exists")

const promoCodeColumns = `id, code, description, product, discount_percent, max_redemptions, max_redemptions_per_user, redemption_count, starts_at, expires_at, is_active, is_public, allowed_emails, created_at`

func scanPromoCode(row interface{ Scan(...any) error }) (models.PromoCode, error) {
	var p models.PromoCode
	var allowedEmails string
	err := row.Scan(&p.ID, &p.Code, &p.Description, &p.Product, &p.DiscountPercent,
		&p.MaxRedemptions, &p.MaxRedemptionsPerUser, &p.RedemptionCount, &p.StartsAt, &p.ExpiresAt,
		&p.IsActive, &p.IsPublic, &allowedEmails, &p.CreatedAt)
	p.AllowedEmails = splitEmails(allowedEmails)
	return p, err
}

// splitEmails parses the comma-separated allowed_emails column into a
// normalized (trimmed, lowercased, empty entries dropped) slice.
func splitEmails(raw string) []string {
	var out []string
	for _, e := range strings.Split(raw, ",") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// joinEmails is the inverse of splitEmails, for writing the column.
func joinEmails(emails []string) string {
	return strings.Join(emails, ",")
}

func (r *PromoCodeRepo) courseIDs(ctx context.Context, promoID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT course_id FROM promo_code_courses WHERE promo_code_id = $1 ORDER BY course_id`, promoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *PromoCodeRepo) ListAll(ctx context.Context) ([]models.PromoCode, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+promoCodeColumns+` FROM promo_codes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.PromoCode
	for rows.Next() {
		p, err := scanPromoCode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		ids, err := r.courseIDs(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].CourseIDs = ids
	}
	return out, nil
}

// ListActivePublic returns every promo code currently redeemable
// (active, started, not expired, not exhausted) and marked public — the
// source for the sitewide scrolling promo banner. A private, user-scoped
// code never appears here even if somehow marked public, since saving a
// code with AllowedEmails set always forces IsPublic false.
func (r *PromoCodeRepo) ListActivePublic(ctx context.Context) ([]models.PromoCode, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+promoCodeColumns+` FROM promo_codes
		WHERE is_active AND is_public AND now() >= starts_at AND now() < expires_at
			AND (max_redemptions = 0 OR redemption_count < max_redemptions)
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.PromoCode
	for rows.Next() {
		p, err := scanPromoCode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PromoCodeRepo) GetByID(ctx context.Context, id int64) (models.PromoCode, error) {
	p, err := scanPromoCode(r.db.QueryRowContext(ctx, `SELECT `+promoCodeColumns+` FROM promo_codes WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.PromoCode{}, ErrNotFound
	}
	if err != nil {
		return models.PromoCode{}, err
	}
	ids, err := r.courseIDs(ctx, id)
	if err != nil {
		return models.PromoCode{}, err
	}
	p.CourseIDs = ids
	return p, nil
}

// GetByCode looks up a promo code case-insensitively (codes are stored
// normalized to uppercase), for redemption at checkout.
func (r *PromoCodeRepo) GetByCode(ctx context.Context, code string) (models.PromoCode, error) {
	p, err := scanPromoCode(r.db.QueryRowContext(ctx,
		`SELECT `+promoCodeColumns+` FROM promo_codes WHERE code = $1`, strings.ToUpper(strings.TrimSpace(code))))
	if errors.Is(err, sql.ErrNoRows) {
		return models.PromoCode{}, ErrNotFound
	}
	if err != nil {
		return models.PromoCode{}, err
	}
	ids, err := r.courseIDs(ctx, p.ID)
	if err != nil {
		return models.PromoCode{}, err
	}
	p.CourseIDs = ids
	return p, nil
}

func (r *PromoCodeRepo) Create(ctx context.Context, p models.PromoCode) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO promo_codes (code, description, product, discount_percent, max_redemptions, max_redemptions_per_user, starts_at, expires_at, is_active, is_public, allowed_emails)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		strings.ToUpper(strings.TrimSpace(p.Code)), p.Description, p.Product, p.DiscountPercent,
		p.MaxRedemptions, p.MaxRedemptionsPerUser, p.StartsAt, p.ExpiresAt, p.IsActive, p.IsPublic, joinEmails(p.AllowedEmails),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrDuplicateCode
		}
		return 0, err
	}
	if err := insertPromoCourses(ctx, tx, id, p.CourseIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (r *PromoCodeRepo) Update(ctx context.Context, p models.PromoCode) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE promo_codes SET code=$1, description=$2, product=$3, discount_percent=$4,
			max_redemptions=$5, max_redemptions_per_user=$6, starts_at=$7, expires_at=$8, is_active=$9,
			is_public=$10, allowed_emails=$11
		WHERE id=$12`,
		strings.ToUpper(strings.TrimSpace(p.Code)), p.Description, p.Product, p.DiscountPercent,
		p.MaxRedemptions, p.MaxRedemptionsPerUser, p.StartsAt, p.ExpiresAt, p.IsActive,
		p.IsPublic, joinEmails(p.AllowedEmails), p.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateCode
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM promo_code_courses WHERE promo_code_id = $1`, p.ID); err != nil {
		return err
	}
	if err := insertPromoCourses(ctx, tx, p.ID, p.CourseIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func insertPromoCourses(ctx context.Context, tx *sql.Tx, promoID int64, courseIDs []int64) error {
	for _, cid := range courseIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO promo_code_courses (promo_code_id, course_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			promoID, cid); err != nil {
			return err
		}
	}
	return nil
}

func (r *PromoCodeRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM promo_codes WHERE id = $1`, id)
	return err
}

// IncrementRedemption atomically bumps a promo code's redemption count.
// Called once a payment using it is actually verified, so an abandoned
// checkout never counts against the cap.
func (r *PromoCodeRepo) IncrementRedemption(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE promo_codes SET redemption_count = redemption_count + 1 WHERE id = $1`, id)
	return err
}
