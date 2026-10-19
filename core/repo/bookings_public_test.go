package repo

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/db"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

const testEmailSuffix = "@w3-booking-test.invalid"

// testDB opens the migrated local database, or skips: these tests exercise
// the real unique index and exclusion constraint from migration 089, which a
// mock cannot prove. Rows are created far in the future under a reserved
// email domain and removed afterwards.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	d, err := db.Open(url)
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	if err := d.Ping(); err != nil {
		t.Skipf("database unreachable: %v", err)
	}
	var n int
	_ = d.QueryRow(`SELECT count(*) FROM pg_indexes WHERE indexname = 'uq_bookings_active_slot'`).Scan(&n)
	if n == 0 {
		t.Skip("migration 089 not applied")
	}
	clean := func() {
		_, _ = d.Exec(`DELETE FROM bookings WHERE email LIKE '%' || $1`, testEmailSuffix)
		_, _ = d.Exec(`DELETE FROM auth_throttle WHERE kind = 'booking-test'`)
	}
	clean()
	t.Cleanup(func() { clean(); d.Close() })
	return d
}

func slotAt(dayOffset, hour int) time.Time {
	return time.Date(2090, 1, 1, hour, 0, 0, 0, time.UTC).AddDate(0, 0, dayOffset)
}

func newB(email string, start time.Time, minutes int) models.Booking {
	return models.Booking{Name: "W3 Test", Email: email + testEmailSuffix, SlotStart: start, SlotEnd: start.Add(time.Duration(minutes) * time.Minute)}
}

func TestPublicBookingRejectsSameAndOverlappingSlots(t *testing.T) {
	r := NewBookingRepo(testDB(t))
	ctx := context.Background()
	start := slotAt(1, 5)

	if _, code, err := r.CreatePublic(ctx, newB("a", start, 60), authutil.HashKey("tok-a")); err != nil || len(code) != 9 {
		t.Fatalf("first booking: code=%q err=%v", code, err)
	}
	if _, _, err := r.CreatePublic(ctx, newB("b", start, 60), authutil.HashKey("tok-b")); err != ErrSlotTaken {
		t.Errorf("same start: want ErrSlotTaken, got %v", err)
	}
	// A booking starting inside an existing one overlaps even though the
	// start times differ (needs the exclusion constraint, not just the index).
	if _, _, err := r.CreatePublic(ctx, newB("c", start.Add(30*time.Minute), 60), authutil.HashKey("tok-c")); err != ErrSlotTaken {
		t.Errorf("overlapping start: want ErrSlotTaken, got %v", err)
	}
	// Back-to-back is fine: ranges are half-open.
	if _, _, err := r.CreatePublic(ctx, newB("d", start.Add(time.Hour), 60), authutil.HashKey("tok-d")); err != nil {
		t.Errorf("adjacent slot must be allowed: %v", err)
	}
}

// Two (here: twelve) simultaneous requests for one slot: exactly one wins.
func TestPublicBookingConcurrentRequests(t *testing.T) {
	r := NewBookingRepo(testDB(t))
	ctx := context.Background()
	start := slotAt(2, 6)

	var wins, taken, other int32
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			email := "race" + string(rune('a'+i))
			_, _, err := r.CreatePublic(ctx, newB(email, start, 60), authutil.HashKey("race-tok-"+email))
			switch err {
			case nil:
				atomic.AddInt32(&wins, 1)
			case ErrSlotTaken:
				atomic.AddInt32(&taken, 1)
			default:
				atomic.AddInt32(&other, 1)
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	close(gate)
	wg.Wait()
	if wins != 1 || taken != 11 || other != 0 {
		t.Fatalf("wins=%d taken=%d other=%d, want 1/11/0", wins, taken, other)
	}
}

func TestPublicBookingManageTokenFlow(t *testing.T) {
	r := NewBookingRepo(testDB(t))
	ctx := context.Background()
	start := slotAt(3, 5)
	hash := authutil.HashKey("manage-flow-token")

	id, code, err := r.CreatePublic(ctx, newB("flow", start, 60), hash)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByManageTokenHash(ctx, hash)
	if err != nil || got.ID != id || got.Code != code || got.Status != models.BookingStatusConfirmed {
		t.Fatalf("lookup by token: %+v err=%v", got, err)
	}
	if _, err := r.GetByManageTokenHash(ctx, authutil.HashKey("wrong-token")); err != ErrNotFound {
		t.Errorf("wrong token: want ErrNotFound, got %v", err)
	}
	if _, err := r.GetByManageTokenHash(ctx, ""); err != ErrNotFound {
		t.Errorf("empty hash must never match legacy rows: got %v", err)
	}

	// Reschedule keeps the booking and records the old slot.
	moved := start.Add(2 * time.Hour)
	if err := r.RescheduleByManageTokenHash(ctx, hash, moved, moved.Add(time.Hour)); err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	got, _ = r.GetByManageTokenHash(ctx, hash)
	if !got.SlotStart.Equal(moved) || got.PreviousSlotStart == nil || !got.PreviousSlotStart.Equal(start) {
		t.Errorf("reschedule not recorded: %+v", got)
	}
	// Rescheduling onto a taken slot is refused.
	if _, _, err := r.CreatePublic(ctx, newB("other", start, 60), authutil.HashKey("other-tok")); err != nil {
		t.Fatal(err)
	}
	if err := r.RescheduleByManageTokenHash(ctx, hash, start, start.Add(time.Hour)); err != ErrSlotTaken {
		t.Errorf("reschedule onto taken slot: want ErrSlotTaken, got %v", err)
	}
	// A wrong token can neither cancel nor move.
	if err := r.CancelByManageTokenHash(ctx, authutil.HashKey("wrong")); err != ErrNotFound {
		t.Errorf("cancel with wrong token: got %v", err)
	}

	// Cancel frees the slot, and a second cancel is a no-op error.
	if err := r.CancelByManageTokenHash(ctx, hash); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := r.CancelByManageTokenHash(ctx, hash); err != ErrNotFound {
		t.Errorf("second cancel: want ErrNotFound, got %v", err)
	}
	if _, _, err := r.CreatePublic(ctx, newB("reuse", moved, 60), authutil.HashKey("reuse-tok")); err != nil {
		t.Errorf("a cancelled slot must be bookable again: %v", err)
	}
}

func TestCountActiveByEmail(t *testing.T) {
	r := NewBookingRepo(testDB(t))
	ctx := context.Background()
	// The suffix check proves matching is case-insensitive.
	for i := 0; i < 2; i++ {
		if _, _, err := r.CreatePublic(ctx, newB("cap", slotAt(10+i, 5), 60), authutil.HashKey("cap-tok-"+string(rune('a'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	// slot_start > now(), so year-2090 rows count as upcoming.
	n, err := r.CountActiveByEmail(ctx, "CAP"+testEmailSuffix)
	if err != nil || n != 2 {
		t.Fatalf("count = %d err=%v, want 2", n, err)
	}
}

// Booking throttling reuses the auth_throttle table: after the limit the key
// is locked, and the lock is per hashed key.
func TestBookingThrottleLocks(t *testing.T) {
	th := NewAuthThrottleRepo(testDB(t))
	ctx := context.Background()
	ik := authutil.HashKey("203.0.113.9")
	ek := authutil.HashKey("spammer" + testEmailSuffix)
	const kind = "booking-test"

	for i := 0; i < 2; i++ {
		_ = th.Fail(ctx, kind, ik, 3)
		if until, _ := th.Locked(ctx, kind, ik); !until.IsZero() {
			t.Fatalf("locked too early after %d attempts", i+1)
		}
	}
	_ = th.Fail(ctx, kind, ik, 3)
	if until, err := th.Locked(ctx, kind, ik); err != nil || until.IsZero() {
		t.Fatalf("want locked after 3 attempts, got until=%v err=%v", until, err)
	}
	if until, _ := th.Locked(ctx, kind, ek); !until.IsZero() {
		t.Error("another key must not be locked")
	}
	if len(ik) != 64 {
		t.Errorf("throttle key should be a SHA-256 hex digest, got %d chars", len(ik))
	}
}
