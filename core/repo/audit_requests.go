package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type AuditRequestRepo struct{ db *sql.DB }

func NewAuditRequestRepo(db *sql.DB) *AuditRequestRepo { return &AuditRequestRepo{db: db} }

const auditRequestColumns = `id, name, email, company, brief, amount_cents, currency, status, razorpay_order_id, razorpay_payment_id, created_at, paid_at`

func scanAuditRequest(row interface{ Scan(...any) error }) (models.AuditRequest, error) {
	var a models.AuditRequest
	var paidAt sql.NullTime
	err := row.Scan(&a.ID, &a.Name, &a.Email, &a.Company, &a.Brief, &a.AmountCents, &a.Currency, &a.Status,
		&a.RazorpayOrderID, &a.RazorpayPaymentID, &a.CreatedAt, &paidAt)
	if paidAt.Valid {
		a.PaidAt = &paidAt.Time
	}
	return a, err
}

// Create records a new audit request in "pending_payment" state, before
// the buyer completes checkout — mirrors BookingRepo.CreatePending.
func (r *AuditRequestRepo) Create(ctx context.Context, a models.AuditRequest) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO audit_requests (name, email, company, brief, amount_cents, currency, status, razorpay_order_id)
		VALUES ($1,$2,$3,$4,$5,$6,'pending_payment',$7) RETURNING id`,
		a.Name, a.Email, a.Company, a.Brief, a.AmountCents, a.Currency, a.RazorpayOrderID,
	).Scan(&id)
	return id, err
}

func (r *AuditRequestRepo) GetByRazorpayOrderID(ctx context.Context, orderID string) (models.AuditRequest, error) {
	a, err := scanAuditRequest(r.db.QueryRowContext(ctx, `SELECT `+auditRequestColumns+` FROM audit_requests WHERE razorpay_order_id = $1`, orderID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.AuditRequest{}, ErrNotFound
	}
	return a, err
}

// MarkPaid transitions a request to "paid", scoped to pending_payment so
// it can never be processed twice.
func (r *AuditRequestRepo) MarkPaid(ctx context.Context, orderID, paymentID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE audit_requests SET status = 'paid', razorpay_payment_id = $1, paid_at = now()
		WHERE razorpay_order_id = $2 AND status = 'pending_payment'`,
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

// ListAll returns every audit request, newest first, for the admin list.
func (r *AuditRequestRepo) ListAll(ctx context.Context) ([]models.AuditRequest, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+auditRequestColumns+` FROM audit_requests ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AuditRequest
	for rows.Next() {
		a, err := scanAuditRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
