// Package scheduler turns weekly availability rules into a concrete list of
// bookable time slots for the next N days, filtering out slots already
// taken by another booking.
package scheduler

import (
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type DaySlots struct {
	Date  time.Time
	Label string
	Slots []models.TimeSlot
}

// ComputeDays returns the next `days` calendar days (in loc) that have at
// least one matching availability rule, each populated with the slots that
// rule implies. Slots in the past (today, earlier than now) and slots
// present in `taken` are marked unavailable rather than dropped, so the UI
// can show a full, honest picture of the week.
func ComputeDays(rules []models.AvailabilityRule, taken map[time.Time]bool, loc *time.Location, days int) []DaySlots {
	now := time.Now().In(loc)
	byWeekday := map[time.Weekday][]models.AvailabilityRule{}
	for _, r := range rules {
		byWeekday[r.Weekday] = append(byWeekday[r.Weekday], r)
	}

	var out []DaySlots
	for d := 0; d < days; d++ {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, d)
		rulesToday := byWeekday[day.Weekday()]
		if len(rulesToday) == 0 {
			continue
		}

		var slots []models.TimeSlot
		for _, rule := range rulesToday {
			slots = append(slots, expandRule(rule, day, loc)...)
		}
		if len(slots) == 0 {
			continue
		}

		for i := range slots {
			if slots[i].Start.Before(now) {
				slots[i].Available = false
				continue
			}
			if taken[slots[i].Start.UTC()] {
				slots[i].Available = false
			}
		}

		out = append(out, DaySlots{Date: day, Label: day.Format("Mon, Jan 2"), Slots: slots})
	}
	return out
}

func expandRule(rule models.AvailabilityRule, day time.Time, loc *time.Location) []models.TimeSlot {
	start, err := parseClock(rule.StartTime, day, loc)
	if err != nil {
		return nil
	}
	end, err := parseClock(rule.EndTime, day, loc)
	if err != nil {
		return nil
	}
	step := time.Duration(rule.SlotMinutes) * time.Minute
	if step <= 0 {
		step = 30 * time.Minute
	}

	// Public booking hours are a hard clamp on top of the admin rules: a
	// rule can narrow the day but never open slots outside 10:00-20:00.
	open := time.Date(day.Year(), day.Month(), day.Day(), models.BookingFirstStartHour, 0, 0, 0, loc)
	closeAt := time.Date(day.Year(), day.Month(), day.Day(), models.BookingLastEndHour, 0, 0, 0, loc)
	if start.Before(open) {
		start = open
	}
	if end.After(closeAt) {
		end = closeAt
	}

	var slots []models.TimeSlot
	for t := start; t.Add(step).Compare(end) <= 0; t = t.Add(step) {
		slots = append(slots, models.TimeSlot{Start: t, End: t.Add(step), Available: true})
	}
	return slots
}

// WithinBookingHours reports whether a slot that starts at start and lasts
// d lies inside the public booking day (10:00 to 20:00 in loc).
func WithinBookingHours(start time.Time, d time.Duration, loc *time.Location) bool {
	l := start.In(loc)
	open := time.Date(l.Year(), l.Month(), l.Day(), models.BookingFirstStartHour, 0, 0, 0, loc)
	closeAt := time.Date(l.Year(), l.Month(), l.Day(), models.BookingLastEndHour, 0, 0, 0, loc)
	return !l.Before(open) && !l.Add(d).After(closeAt)
}

func parseClock(hhmm string, day time.Time, loc *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation("15:04", hhmm, loc)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(day.Year(), day.Month(), day.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc), nil
}
