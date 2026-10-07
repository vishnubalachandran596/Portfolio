package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type SoftwarePurchaseRepo struct{ db *sql.DB }

func NewSoftwarePurchaseRepo(db *sql.DB) *SoftwarePurchaseRepo { return &SoftwarePurchaseRepo{db: db} }

const softwarePurchaseColumns = `id, software_id, user_id, email, amount_cents, currency, status, razorpay_order_id, razorpay_payment_id, access_token, promo_code_id, promo_code, promo_text, discount_cents, created_at, paid_at`

func scanSoftwarePurchase(row interface{ Scan(...any) error }) (models.SoftwarePurchase, error) {
	var p models.SoftwarePurchase
	var userID sql.NullInt64
	var promoCodeID sql.NullInt64
	err := row.Scan(&p.ID, &p.SoftwareID, &userID, &p.Email, &p.AmountCents, &p.Currency, &p.Status,
		&p.RazorpayOrderID, &p.RazorpayPaymentID, &p.AccessToken, &promoCodeID, &p.PromoCode, &p.PromoText, &p.DiscountCents,
		&p.CreatedAt, &p.PaidAt)
	p.UserID = userID.Int64
	if promoCodeID.Valid {
		p.PromoCodeID = &promoCodeID.Int64
	}
	return p, err
}

// Create records a new Razorpay order in "created" state, before the
// buyer completes checkout. Mirrors PurchaseRepo.Create.
func (r *SoftwarePurchaseRepo) Create(ctx context.Context, p models.SoftwarePurchase) (int64, error) {
	var id int64
	var userID any
	if p.UserID != 0 {
		userID = p.UserID
	}
	var promoCodeID any
	if p.PromoCodeID != nil {
		promoCodeID = *p.PromoCodeID
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO software_purchases (software_id, user_id, email, amount_cents, currency, status, razorpay_order_id, access_token, promo_code_id, promo_code, promo_text, discount_cents)
		VALUES ($1,$2,$3,$4,$5,'created',$6,$7,$8,$9,$10,$11) RETURNING id`,
		p.SoftwareID, userID, p.Email, p.AmountCents, p.Currency, p.RazorpayOrderID, p.AccessToken,
		promoCodeID, p.PromoCode, p.PromoText, p.DiscountCents,
	).Scan(&id)
	return id, err
}

func (r *SoftwarePurchaseRepo) GetByRazorpayOrderID(ctx context.Context, orderID string) (models.SoftwarePurchase, error) {
	p, err := scanSoftwarePurchase(r.db.QueryRowContext(ctx, `SELECT `+softwarePurchaseColumns+` FROM software_purchases WHERE razorpay_order_id = $1`, orderID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.SoftwarePurchase{}, ErrNotFound
	}
	return p, err
}

// GetPaidByAccessToken looks up a purchase by its bearer access token,
// only returning it if payment has actually been verified.
func (r *SoftwarePurchaseRepo) GetPaidByAccessToken(ctx context.Context, token string) (models.SoftwarePurchase, error) {
	p, err := scanSoftwarePurchase(r.db.QueryRowContext(ctx,
		`SELECT `+softwarePurchaseColumns+` FROM software_purchases WHERE access_token = $1 AND status = 'paid'`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return models.SoftwarePurchase{}, ErrNotFound
	}
	return p, err
}

// MarkPaid transitions an order to "paid" after the Razorpay signature has
// been verified server-side. Scoped to status='created' so this can never
// re-process an order twice.
func (r *SoftwarePurchaseRepo) MarkPaid(ctx context.Context, orderID, paymentID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE software_purchases SET status = 'paid', razorpay_payment_id = $1, paid_at = now()
		WHERE razorpay_order_id = $2 AND status = 'created'`,
		paymentID, orderID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPaidByUserID returns every paid software purchase made by a given
// account, newest first — the source for the "My Software" section of
// /account.
func (r *SoftwarePurchaseRepo) ListPaidByUserID(ctx context.Context, userID int64) ([]models.SoftwarePurchase, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+softwarePurchaseColumns+` FROM software_purchases WHERE user_id = $1 AND status = 'paid' ORDER BY paid_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SoftwarePurchase
	for rows.Next() {
		p, err := scanSoftwarePurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountUserRedemptions mirrors PurchaseRepo.CountUserRedemptions for the
// software product line — how many paid software purchases this account
// already has against a given promo code.
func (r *SoftwarePurchaseRepo) CountUserRedemptions(ctx context.Context, userID, promoCodeID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM software_purchases WHERE user_id = $1 AND promo_code_id = $2 AND status = 'paid'`,
		userID, promoCodeID,
	).Scan(&count)
	return count, err
}

func (r *SoftwarePurchaseRepo) ListAll(ctx context.Context) ([]models.SoftwarePurchase, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+softwarePurchaseColumns+` FROM software_purchases ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SoftwarePurchase
	for rows.Next() {
		p, err := scanSoftwarePurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
