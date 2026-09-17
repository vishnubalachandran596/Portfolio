package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type PurchaseRepo struct{ db *sql.DB }

func NewPurchaseRepo(db *sql.DB) *PurchaseRepo { return &PurchaseRepo{db: db} }

const purchaseColumns = `id, course_id, email, amount_cents, currency, status, razorpay_order_id, razorpay_payment_id, access_token, created_at, paid_at`

func scanPurchase(row interface{ Scan(...any) error }) (models.Purchase, error) {
	var p models.Purchase
	err := row.Scan(&p.ID, &p.CourseID, &p.Email, &p.AmountCents, &p.Currency, &p.Status,
		&p.RazorpayOrderID, &p.RazorpayPaymentID, &p.AccessToken, &p.CreatedAt, &p.PaidAt)
	return p, err
}

// Create records a new Razorpay order in "created" state, before the buyer
// completes checkout.
func (r *PurchaseRepo) Create(ctx context.Context, p models.Purchase) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO purchases (course_id, email, amount_cents, currency, status, razorpay_order_id, access_token)
		VALUES ($1,$2,$3,$4,'created',$5,$6) RETURNING id`,
		p.CourseID, p.Email, p.AmountCents, p.Currency, p.RazorpayOrderID, p.AccessToken,
	).Scan(&id)
	return id, err
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
