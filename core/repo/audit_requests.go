package repo

import (
	"context"
	"database/sql"

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

// Create, GetByRazorpayOrderID, and MarkPaid used to back a paid
// self-checkout flow (Create on order-creation, MarkPaid after Razorpay
// verification). That flow was removed — the Architecture Review is
// scoped over a call now, with no payment gateway involved anywhere
// except course purchases (see docs/ARCHITECTURE.md §6) — so those
// methods were removed too. ListAll stays: it's still read by the admin
// payments ledger (admin_payments.go) to show the real historical rows
// from when self-checkout was live.

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
