package scheduler

import (
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

var ist = time.FixedZone("IST", 5*3600+1800)

// A rule wider than the public day (08:00-23:00) is clamped to 10:00-20:00:
// ten hourly slots, the first starting 10:00 and the last 19:00 IST.
func TestExpandRuleClampsToPublicDay(t *testing.T) {
	day := time.Date(2030, 1, 7, 0, 0, 0, 0, ist)
	slots := expandRule(models.AvailabilityRule{StartTime: "08:00", EndTime: "23:00", SlotMinutes: 60, Active: true}, day, ist)
	if len(slots) != 10 {
		t.Fatalf("want 10 hourly slots, got %d", len(slots))
	}
	if h := slots[0].Start.In(ist).Hour(); h != 10 {
		t.Errorf("first slot starts %d:00, want 10:00", h)
	}
	if h := slots[len(slots)-1].Start.In(ist).Hour(); h != 19 {
		t.Errorf("last slot starts %d:00, want 19:00", h)
	}
	if end := slots[len(slots)-1].End.In(ist).Hour(); end != 20 {
		t.Errorf("last slot ends %d:00, want 20:00", end)
	}
}

// A narrower admin rule still wins: the clamp only ever shrinks the day.
func TestExpandRuleKeepsNarrowerAdminRule(t *testing.T) {
	day := time.Date(2030, 1, 7, 0, 0, 0, 0, ist)
	slots := expandRule(models.AvailabilityRule{StartTime: "14:00", EndTime: "17:00", SlotMinutes: 60, Active: true}, day, ist)
	if len(slots) != 3 {
		t.Fatalf("want 3 slots, got %d", len(slots))
	}
}

func TestWithinBookingHours(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2030, 1, 7, h, m, 0, 0, ist) }
	hour := time.Hour
	cases := []struct {
		name  string
		start time.Time
		want  bool
	}{
		{"09:00 too early", at(9, 0), false},
		{"10:00 first slot", at(10, 0), true},
		{"19:00 last slot", at(19, 0), true},
		{"19:30 runs past 20:00", at(19, 30), false},
		{"20:00 too late", at(20, 0), false},
	}
	for _, c := range cases {
		if got := WithinBookingHours(c.start, hour, ist); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// ComputeDays covers exactly the requested number of days and marks taken
// and past slots unavailable instead of dropping them.
func TestComputeDaysThirtyDaysAndTaken(t *testing.T) {
	var rules []models.AvailabilityRule
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		rules = append(rules, models.AvailabilityRule{Weekday: wd, StartTime: "10:00", EndTime: "20:00", SlotMinutes: 60, Active: true})
	}
	days := ComputeDays(rules, map[time.Time]bool{}, ist, models.BookingWindowDays)
	if len(days) != models.BookingWindowDays {
		t.Fatalf("want %d days, got %d", models.BookingWindowDays, len(days))
	}
	last := days[len(days)-1]
	taken := map[time.Time]bool{last.Slots[0].Start.UTC(): true}
	days = ComputeDays(rules, taken, ist, models.BookingWindowDays)
	if days[len(days)-1].Slots[0].Available {
		t.Error("a taken slot must be unavailable")
	}
	if !days[len(days)-1].Slots[1].Available {
		t.Error("an untaken future slot must stay available")
	}
}
