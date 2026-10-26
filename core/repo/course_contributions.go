package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// CourseContributionRepo stores the optional voluntary contribution a
// learner can make after finishing a course (migration 087).
type CourseContributionRepo struct{ db *sql.DB }

func NewCourseContributionRepo(db *sql.DB) *CourseContributionRepo {
	return &CourseContributionRepo{db: db}
}

const courseContributionColumns = `id, course_id, COALESCE(user_id, 0), email, amount_cents, currency, status, razorpay_order_id, razorpay_payment_id, created_at, paid_at`

func scanCourseContribution(row interface{ Scan(...any) error }) (models.CourseContribution, error) {
	var c models.CourseContribution
	var paidAt sql.NullTime
	err := row.Scan(&c.ID, &c.CourseID, &c.UserID, &c.Email, &c.AmountCents, &c.Currency, &c.Status,
		&c.RazorpayOrderID, &c.RazorpayPaymentID, &c.CreatedAt, &paidAt)
	if paidAt.Valid {
		c.PaidAt = &paidAt.Time
	}
	return c, err
}

func (r *CourseContributionRepo) Create(ctx context.Context, c models.CourseContribution) (int64, error) {
	var userID any
	if c.UserID > 0 {
		userID = c.UserID
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO course_contributions (course_id, user_id, email, amount_cents, currency, status, razorpay_order_id)
		VALUES ($1,$2,$3,$4,$5,'created',$6) RETURNING id`,
		c.CourseID, userID, c.Email, c.AmountCents, c.Currency, c.RazorpayOrderID).Scan(&id)
	return id, err
}

func (r *CourseContributionRepo) GetByID(ctx context.Context, id int64) (models.CourseContribution, error) {
	c, err := scanCourseContribution(r.db.QueryRowContext(ctx, `SELECT `+courseContributionColumns+` FROM course_contributions WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.CourseContribution{}, ErrNotFound
	}
	return c, err
}

func (r *CourseContributionRepo) GetByRazorpayOrderID(ctx context.Context, orderID string) (models.CourseContribution, error) {
	c, err := scanCourseContribution(r.db.QueryRowContext(ctx, `SELECT `+courseContributionColumns+` FROM course_contributions WHERE razorpay_order_id = $1`, orderID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.CourseContribution{}, ErrNotFound
	}
	return c, err
}

// MarkPaid flips a created order to paid once, after signature verification.
func (r *CourseContributionRepo) MarkPaid(ctx context.Context, orderID, paymentID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE course_contributions SET status='paid', razorpay_payment_id=$1, paid_at=now()
		WHERE razorpay_order_id=$2 AND status='created'`, paymentID, orderID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
