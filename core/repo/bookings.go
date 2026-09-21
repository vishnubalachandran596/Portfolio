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

const bookingColumns = `id, user_id, name, email, message, slot_start, slot_end, status, amount_cents, currency, razorpay_order_id, razorpay_payment_id, previous_slot_start, rescheduled_at, reschedule_acknowledged, promo_code_id, promo_code, discount_cents, created_at`

func scanBooking(row interface{ Scan(...any) error }) (models.Booking, error) {
	var b models.Booking
	var userID sql.NullInt64
	var prevSlot, rescheduledAt sql.NullTime
	var promoCodeID sql.NullInt64
	err := row.Scan(&b.ID, &userID, &b.Name, &b.Email, &b.Message, &b.SlotStart, &b.SlotEnd, &b.Status,
		&b.AmountCents, &b.Currency, &b.RazorpayOrderID, &b.RazorpayPaymentID,
		&prevSlot, &rescheduledAt, &b.RescheduleAcknowledged, &promoCodeID, &b.PromoCode, &b.DiscountCents, &b.CreatedAt)
	b.UserID = userID.Int64
	if prevSlot.Valid {
		b.PreviousSlotStart = &prevSlot.Time
	}
	if rescheduledAt.Valid {
		b.RescheduledAt = &rescheduledAt.Time
	}
	if promoCodeID.Valid {
		b.PromoCodeID = &promoCodeID.Int64
	}
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
	var promoCodeID any
	if b.PromoCodeID != nil {
		promoCodeID = *b.PromoCodeID
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO bookings (user_id, name, email, message, slot_start, slot_end, status, amount_cents, currency, razorpay_order_id, promo_code_id, promo_code, discount_cents)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		b.UserID, b.Name, b.Email, b.Message, b.SlotStart, b.SlotEnd, models.BookingStatusPendingPayment,
		b.AmountCents, b.Currency, b.RazorpayOrderID, promoCodeID, b.PromoCode, b.DiscountCents,
	).Scan(&id)
	if err != nil && isUniqueViolation(err) {
		return 0, ErrSlotTaken
	}
	return id, err
}

// MarkPaid confirms a pending booking — historically only called after a
// Razorpay signature was verified, now called directly from
// BookingConfirm since there's no payment step at all. Still scoped to
// status='pending_payment' so a replayed call is a no-op (ErrNotFound)
// rather than double-processing.
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

func (r *BookingRepo) GetByID(ctx context.Context, id int64) (models.Booking, error) {
	b, err := scanBooking(r.db.QueryRowContext(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Booking{}, ErrNotFound
	}
	return b, err
}

// Reschedule moves a confirmed booking to a new slot, admin-initiated:
// records the old slot and marks the change unacknowledged so /account can
// surface a notice until the visitor sees it and either accepts it or
// picks a different time themselves. ErrSlotTaken if the new slot is
// already claimed by another booking.
func (r *BookingRepo) Reschedule(ctx context.Context, id int64, newStart, newEnd time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE bookings
		SET previous_slot_start = slot_start, slot_start = $1, slot_end = $2,
		    rescheduled_at = now(), reschedule_acknowledged = FALSE
		WHERE id = $3`,
		newStart, newEnd, id)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrSlotTaken
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
	return nil
}

// SelfReschedule lets the booking's own account move it to a different
// available slot — e.g. after an admin-initiated reschedule doesn't work
// for them — scoped to user_id so they can't touch someone else's
// booking. Clears any pending reschedule notice since this is now their
// own deliberate choice.
func (r *BookingRepo) SelfReschedule(ctx context.Context, id, userID int64, newStart, newEnd time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE bookings
		SET slot_start = $1, slot_end = $2, reschedule_acknowledged = TRUE
		WHERE id = $3 AND user_id = $4`,
		newStart, newEnd, id, userID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrSlotTaken
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
	return nil
}

// AcknowledgeReschedule dismisses the reschedule notice on the visitor's
// own booking without changing the slot.
func (r *BookingRepo) AcknowledgeReschedule(ctx context.Context, id, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bookings SET reschedule_acknowledged = TRUE WHERE id = $1 AND user_id = $2`, id, userID)
	return err
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
