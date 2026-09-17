package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type BookingRepo struct{ db *sql.DB }

func NewBookingRepo(db *sql.DB) *BookingRepo { return &BookingRepo{db: db} }

const bookingColumns = `id, user_id, name, email, message, slot_start, slot_end, status, amount_cents, currency, razorpay_order_id, razorpay_payment_id, created_at`

func scanBooking(row interface{ Scan(...any) error }) (models.Booking, error) {
	var b models.Booking
	var userID sql.NullInt64
	err := row.Scan(&b.ID, &userID, &b.Name, &b.Email, &b.Message, &b.SlotStart, &b.SlotEnd, &b.Status,
		&b.AmountCents, &b.Currency, &b.RazorpayOrderID, &b.RazorpayPaymentID, &b.CreatedAt)
	b.UserID = userID.Int64
	return b, err
}

// ErrSlotTaken is returned when a visitor tries to reserve a slot that's
// already confirmed or has a live (non-stale) pending payment.
var ErrSlotTaken = errors.New("slot already booked")

// CreatePending reserves a slot for payment: a Razorpay order has already
// been created for it, so this INSERT is what actually claims the slot —
// the UNIQUE(slot_start) constraint rejects a racing second buyer.
func (r *BookingRepo) CreatePending(ctx context.Context, b models.Booking) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO bookings (user_id, name, email, message, slot_start, slot_end, status, amount_cents, currency, razorpay_order_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		b.UserID, b.Name, b.Email, b.Message, b.SlotStart, b.SlotEnd, models.BookingStatusPendingPayment,
		b.AmountCents, b.Currency, b.RazorpayOrderID,
	).Scan(&id)
	if err != nil && isUniqueViolation(err) {
		return 0, ErrSlotTaken
	}
	return id, err
}

func (r *BookingRepo) GetByRazorpayOrderID(ctx context.Context, orderID string) (models.Booking, error) {
	b, err := scanBooking(r.db.QueryRowContext(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE razorpay_order_id = $1`, orderID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Booking{}, ErrNotFound
	}
	return b, err
}

// MarkPaid confirms a pending booking once Razorpay's signature has been
// verified. Scoped to status='pending_payment' so a replayed verify call
// is a no-op (ErrNotFound) rather than double-processing.
func (r *BookingRepo) MarkPaid(ctx context.Context, orderID, paymentID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE bookings SET status = $1, razorpay_payment_id = $2
		WHERE razorpay_order_id = $3 AND status = $4`,
		models.BookingStatusConfirmed, paymentID, orderID, models.BookingStatusPendingPayment)
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

func (r *BookingRepo) ListUpcoming(ctx context.Context) ([]models.Booking, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE status = 'confirmed' AND slot_start >= now() - interval '1 day' ORDER BY slot_start`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBookings(rows)
}

// ListByUserID returns every booking (any status) made by a given account,
// newest slot first — the source for the "My Bookings" section of
// /account.
func (r *BookingRepo) ListByUserID(ctx context.Context, userID int64) ([]models.Booking, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE user_id = $1 ORDER BY slot_start DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBookings(rows)
}

func (r *BookingRepo) ListAll(ctx context.Context) ([]models.Booking, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+bookingColumns+` FROM bookings ORDER BY slot_start DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBookings(rows)
}

func scanBookings(rows *sql.Rows) ([]models.Booking, error) {
	var out []models.Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BookedSlots returns the start times that aren't offerable: confirmed
// bookings, plus pending-payment reservations less than 20 minutes old
// (an abandoned checkout frees the slot automatically without a cleanup
// job).
func (r *BookingRepo) BookedSlots(ctx context.Context, from, to time.Time) (map[time.Time]bool, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT slot_start FROM bookings
		WHERE slot_start >= $1 AND slot_start < $2
		  AND (status = 'confirmed' OR (status = 'pending_payment' AND created_at > now() - interval '20 minutes'))`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	taken := map[time.Time]bool{}
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		taken[t.UTC()] = true
	}
	return taken, rows.Err()
}

func (r *BookingRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bookings SET status = $1 WHERE id = $2`, status, id)
	return err
}

func isUniqueViolation(err error) bool {
	// pgx wraps *pgconn.PgError; checking the message avoids an extra import
	// for a single, low-risk error classification.
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value") || strings.Contains(msg, "SQLSTATE 23505")
}
