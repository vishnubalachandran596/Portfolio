package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

func testIST() *time.Location { return time.FixedZone("IST", 5*3600+1800) }

func allWeekRules(start, end string) []models.AvailabilityRule {
	var rules []models.AvailabilityRule
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		rules = append(rules, models.AvailabilityRule{Weekday: wd, StartTime: start, EndTime: end, SlotMinutes: 60, Active: true})
	}
	return rules
}

// futureAt returns an IST wall-clock time n days from now.
func futureAt(loc *time.Location, days, hour int) time.Time {
	n := time.Now().In(loc).AddDate(0, 0, days)
	return time.Date(n.Year(), n.Month(), n.Day(), hour, 0, 0, 0, loc)
}

func TestBookableSlotError(t *testing.T) {
	loc := testIST()
	a := &App{Timezone: loc}
	rules := allWeekRules("10:00", "20:00")

	cases := []struct {
		name    string
		slot    time.Time
		wantErr string // substring; "" means bookable
	}{
		{"tomorrow 10:00", futureAt(loc, 1, 10), ""},
		{"tomorrow 19:00 (last start)", futureAt(loc, 1, 19), ""},
		{"day 29 is inside the window", futureAt(loc, 29, 11), ""},
		{"past", time.Now().Add(-2 * time.Hour).Truncate(time.Hour), "passed"},
		{"yesterday", futureAt(loc, -1, 12), "passed"},
		{"over 30 days", futureAt(loc, 31, 12), "30 days"},
		{"09:00 before opening", futureAt(loc, 2, 9), "not available"},
		{"20:00 after closing", futureAt(loc, 2, 20), "not available"},
		{"half past is not an hourly slot", futureAt(loc, 2, 11).Add(30 * time.Minute), "not available"},
	}
	for _, c := range cases {
		msg, mins := a.bookableSlotError(rules, c.slot)
		if c.wantErr == "" {
			if msg != "" {
				t.Errorf("%s: unexpected rejection %q", c.name, msg)
			}
			if mins != 60 {
				t.Errorf("%s: slot minutes = %d, want 60", c.name, mins)
			}
			continue
		}
		if !strings.Contains(msg, c.wantErr) {
			t.Errorf("%s: got %q, want it to contain %q", c.name, msg, c.wantErr)
		}
	}
}

// A wide admin rule cannot open slots outside 10:00-20:00 IST.
func TestBookableSlotErrorClampsWideRule(t *testing.T) {
	loc := testIST()
	a := &App{Timezone: loc}
	rules := allWeekRules("06:00", "23:00")
	if msg, _ := a.bookableSlotError(rules, futureAt(loc, 2, 8)); msg == "" {
		t.Error("08:00 must be rejected even though the rule starts at 06:00")
	}
	if msg, _ := a.bookableSlotError(rules, futureAt(loc, 2, 21)); msg == "" {
		t.Error("21:00 must be rejected even though the rule runs to 23:00")
	}
}

// With no rule for the weekday nothing is bookable (admin rules still apply).
func TestBookableSlotErrorNeedsRule(t *testing.T) {
	loc := testIST()
	a := &App{Timezone: loc}
	slot := futureAt(loc, 3, 11)
	var rules []models.AvailabilityRule
	for _, r := range allWeekRules("10:00", "20:00") {
		if r.Weekday != slot.Weekday() {
			rules = append(rules, r)
		}
	}
	if msg, _ := a.bookableSlotError(rules, slot); msg == "" {
		t.Error("a weekday without an availability rule must not be bookable")
	}
}

func TestNewBookingCodeFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := models.NewBookingCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 9 || !strings.HasPrefix(c, "BK-") {
			t.Fatalf("bad code %q", c)
		}
		if strings.ContainsAny(c[3:], "01OI") {
			t.Fatalf("ambiguous character in %q", c)
		}
		seen[c] = true
	}
	if len(seen) < 190 {
		t.Errorf("codes are not random enough: %d unique of 200", len(seen))
	}
}
