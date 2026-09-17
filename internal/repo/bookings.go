package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/internal/models"
)

type BookingRepo struct{ db *sql.DB }

func NewBookingRepo(db *sql.DB) *BookingRepo { return &BookingRepo{db: db} }

const bookingColumns = `id, name, email, message, slot_start, slot_end, status, created_at`

func scanBooking(row interface{ Scan(...any) error }) (models.Booking, error) {
	var b models.Booking
	err := row.Scan(&b.ID, &b.Name, &b.Email, &b.Message, &b.SlotStart, &b.SlotEnd, &b.Status, &b.CreatedAt)
	return b, err
}

// ErrSlotTaken is returned when a visitor tries to book a slot that was
// already reserved by someone else (unique constraint on slot_start).
var ErrSlotTaken = errors.New("slot already booked")

func (r *BookingRepo) Create(ctx context.Context, b models.Booking) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO bookings (name, email, message, slot_start, slot_end, status)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		b.Name, b.Email, b.Message, b.SlotStart, b.SlotEnd, models.BookingStatusPending,
	).Scan(&id)
	if err != nil && isUniqueViolation(err) {
		return 0, ErrSlotTaken
	}
	return id, err
}

func (r *BookingRepo) ListUpcoming(ctx context.Context) ([]models.Booking, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE status != 'cancelled' AND slot_start >= now() - interval '1 day' ORDER BY slot_start`)
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

// BookedSlots returns the start times already reserved (pending or
// confirmed) within the given window, used to filter out taken slots.
func (r *BookingRepo) BookedSlots(ctx context.Context, from, to time.Time) (map[time.Time]bool, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT slot_start FROM bookings
		WHERE status != 'cancelled' AND slot_start >= $1 AND slot_start < $2`, from, to)
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
