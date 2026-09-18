package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type PurchaseRepo struct{ db *sql.DB }

func NewPurchaseRepo(db *sql.DB) *PurchaseRepo { return &PurchaseRepo{db: db} }

const purchaseColumns = `id, course_id, user_id, email, amount_cents, currency, status, razorpay_order_id, razorpay_payment_id, access_token, promo_code_id, promo_code, discount_cents, created_at, paid_at`

func scanPurchase(row interface{ Scan(...any) error }) (models.Purchase, error) {
	var p models.Purchase
	var userID sql.NullInt64
	var promoCodeID sql.NullInt64
	err := row.Scan(&p.ID, &p.CourseID, &userID, &p.Email, &p.AmountCents, &p.Currency, &p.Status,
		&p.RazorpayOrderID, &p.RazorpayPaymentID, &p.AccessToken, &promoCodeID, &p.PromoCode, &p.DiscountCents,
		&p.CreatedAt, &p.PaidAt)
	p.UserID = userID.Int64
	if promoCodeID.Valid {
		p.PromoCodeID = &promoCodeID.Int64
	}
	return p, err
}

// Create records a new Razorpay order in "created" state, before the buyer
// completes checkout. UserID is 0 for a guest checkout (not logged in).
func (r *PurchaseRepo) Create(ctx context.Context, p models.Purchase) (int64, error) {
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
		INSERT INTO purchases (course_id, user_id, email, amount_cents, currency, status, razorpay_order_id, access_token, promo_code_id, promo_code, discount_cents)
		VALUES ($1,$2,$3,$4,$5,'created',$6,$7,$8,$9,$10) RETURNING id`,
		p.CourseID, userID, p.Email, p.AmountCents, p.Currency, p.RazorpayOrderID, p.AccessToken,
		promoCodeID, p.PromoCode, p.DiscountCents,
	).Scan(&id)
	return id, err
}

// ListPaidByUserID returns every paid course purchase made by a given
// account, newest first — the source for the "My Courses" section of
// /account.
func (r *PurchaseRepo) ListPaidByUserID(ctx context.Context, userID int64) ([]models.Purchase, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+purchaseColumns+` FROM purchases WHERE user_id = $1 AND status = 'paid' ORDER BY paid_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Purchase
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PurchaseRepo) GetByRazorpayOrderID(ctx context.Context, orderID string) (models.Purchase, error) {
	p, err := scanPurchase(r.db.QueryRowContext(ctx, `SELECT `+purchaseColumns+` FROM purchases WHERE razorpay_order_id = $1`, orderID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Purchase{}, ErrNotFound
	}
	return p, err
}

// GetPaidByAccessToken looks up a purchase by its bearer access token,
// only returning it if payment has actually been verified.
func (r *PurchaseRepo) GetPaidByAccessToken(ctx context.Context, token string) (models.Purchase, error) {
	p, err := scanPurchase(r.db.QueryRowContext(ctx,
		`SELECT `+purchaseColumns+` FROM purchases WHERE access_token = $1 AND status = 'paid'`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Purchase{}, ErrNotFound
	}
	return p, err
}

// MarkPaid transitions an order to "paid" after the Razorpay signature has
// been verified server-side. Scoped to status='created' so this can never
// re-process an order twice.
func (r *PurchaseRepo) MarkPaid(ctx context.Context, orderID, paymentID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE purchases SET status = 'paid', razorpay_payment_id = $1, paid_at = now()
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

// ListByUserID returns every purchase (any status) made by a given
// account, newest first — for the admin's per-user detail view.
func (r *PurchaseRepo) ListByUserID(ctx context.Context, userID int64) ([]models.Purchase, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+purchaseColumns+` FROM purchases WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Purchase
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListPaidUserIDsByCourseID returns the distinct account IDs of everyone
// with a paid purchase of a course — the audience for a "this course was
// updated" notification. A guest checkout (no account) has no user_id and
// is skipped, since there's no account to notify.
func (r *PurchaseRepo) ListPaidUserIDsByCourseID(ctx context.Context, courseID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT user_id FROM purchases
		WHERE course_id = $1 AND status = 'paid' AND user_id IS NOT NULL`, courseID)
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

func (r *PurchaseRepo) ListAll(ctx context.Context) ([]models.Purchase, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+purchaseColumns+` FROM purchases ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Purchase
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
