package handlers

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/scheduler"
)

// availableDays computes the next 30 days of bookable slots from the
// active availability rules minus what's already taken — shared by the
// booking page and every reschedule flow.
func (a *App) availableDays(r *http.Request) ([]scheduler.DaySlots, error) {
	ctx := r.Context()
	rules, err := a.Availability.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	from := time.Now().In(a.Timezone)
	to := from.AddDate(0, 0, models.BookingWindowDays+1)
	taken, err := a.Bookings.BookedSlots(ctx, from, to)
	if err != nil {
		return nil, err
	}
	return scheduler.ComputeDays(rules, taken, a.Timezone, models.BookingWindowDays), nil
}

// bookableSlotError is the single server-side gate for a requested slot
// (new booking and every reschedule): it must be one the rules actually
// offer, inside the 10:00-20:00 IST day, in the future and no more than 30
// days ahead. It returns the visitor-facing reason, or "" when the slot is
// fine, plus the slot length in minutes.
func (a *App) bookableSlotError(rules []models.AvailabilityRule, slotStart time.Time) (string, int) {
	minutes := models.ConsultancyDurationMinutes
	local := slotStart.In(a.Timezone)
	for _, rr := range rules {
		if rr.Weekday == local.Weekday() && rr.SlotMinutes > 0 {
			minutes = rr.SlotMinutes
			break
		}
	}
	now := time.Now()
	switch {
	case !slotStart.After(now):
		return "That time has already passed, please pick another.", minutes
	case slotStart.After(now.AddDate(0, 0, models.BookingWindowDays)):
		return "Bookings open up to 30 days ahead, please pick an earlier day.", minutes
	case !a.slotIsOffered(rules, slotStart),
		!scheduler.WithinBookingHours(slotStart, time.Duration(minutes)*time.Minute, a.Timezone):
		return "That time slot is not available, please pick another.", minutes
	}
	return "", minutes
}

// --- Admin: reschedule any booking --------------------------------------

type AdminBookingRescheduleData struct {
	BaseData
	Booking models.Booking
	Days    []scheduler.DaySlots
	Error   string
}

func (a *App) AdminBookingReschedulePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	booking, err := a.Bookings.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load booking", err)
		return
	}
	days, err := a.availableDays(r)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_booking_reschedule", AdminBookingRescheduleData{
		BaseData: a.base(r, "Reschedule Booking — Admin", "", "admin"),
		Booking:  booking,
		Days:     days,
		Error:    r.URL.Query().Get("error"),
	})
}

// AdminBookingRescheduleSubmit moves the booking to a new slot the admin
// picked, marks it unacknowledged so the visitor sees a notice on
// /account, and lets them know via Telegram.
func (a *App) AdminBookingRescheduleSubmit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	slotRaw := r.FormValue("slot")
	newStart, err := time.Parse(time.RFC3339, slotRaw)
	if err != nil {
		http.Redirect(w, r, "/admin/bookings/"+strconv.FormatInt(id, 10)+"/reschedule?error=Pick+a+valid+time", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	booking, err := a.Bookings.GetByID(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load booking", err)
		return
	}
	duration := booking.SlotEnd.Sub(booking.SlotStart)
	newEnd := newStart.Add(duration)

	if err := a.Bookings.Reschedule(ctx, id, newStart, newEnd); err != nil {
		if err == repo.ErrSlotTaken {
			http.Redirect(w, r, "/admin/bookings/"+strconv.FormatInt(id, 10)+"/reschedule?error=That+time+is+already+booked", http.StatusSeeOther)
			return
		}
		a.serverError(w, "reschedule booking", err)
		return
	}

	a.Telegram.Send("🔁 Rescheduled a booking\nName: " + booking.Name + "\nEmail: " + booking.Email +
		"\nWas: " + booking.SlotStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM") +
		"\nNow: " + newStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM"))
	a.emailBooking(ctx, models.EmailBookingRescheduled, booking, newStart, newEnd, "")

	http.Redirect(w, r, "/admin/bookings", http.StatusSeeOther)
}

// --- Visitor: acknowledge or self-reschedule ----------------------------

// AccountAcknowledgeReschedule dismisses the reschedule notice on the
// visitor's own booking, keeping the new (admin-picked) time.
func (a *App) AccountAcknowledgeReschedule(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.Auth.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login?next=/account", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Bookings.AcknowledgeReschedule(r.Context(), id, uid); err != nil {
		a.serverError(w, "acknowledge reschedule", err)
		return
	}
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

type AccountRescheduleData struct {
	BaseData
	Booking models.Booking
	Days    []scheduler.DaySlots
	Error   string
}

// AccountReschedulePage lets a visitor pick a different available time
// for their own booking — typically because an admin-initiated reschedule
// doesn't work for them.
func (a *App) AccountReschedulePage(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.Auth.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login?next=/account", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	booking, err := a.Bookings.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load booking", err)
		return
	}
	if booking.UserID != uid {
		a.NotFound(w, r)
		return
	}
	days, err := a.availableDays(r)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	a.View.Render(w, http.StatusOK, "account_reschedule", AccountRescheduleData{
		BaseData: a.base(r, "Reschedule Your Booking — "+a.Cfg.SiteName, "", "account"),
		Booking:  booking,
		Days:     days,
		Error:    r.URL.Query().Get("error"),
	})
}

func (a *App) AccountRescheduleSubmit(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.Auth.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login?next=/account", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	newStart, err := time.Parse(time.RFC3339, r.FormValue("slot"))
	if err != nil {
		http.Redirect(w, r, "/account/bookings/"+strconv.FormatInt(id, 10)+"/reschedule?error=Pick+a+valid+time", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	booking, err := a.Bookings.GetByID(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load booking", err)
		return
	}
	if booking.UserID != uid {
		a.NotFound(w, r)
		return
	}
	rules, err := a.Availability.ListActive(ctx)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	if msg, _ := a.bookableSlotError(rules, newStart); msg != "" {
		http.Redirect(w, r, "/account/bookings/"+strconv.FormatInt(id, 10)+"/reschedule?error="+url.QueryEscape(msg), http.StatusSeeOther)
		return
	}
	duration := booking.SlotEnd.Sub(booking.SlotStart)

	if err := a.Bookings.SelfReschedule(ctx, id, uid, newStart, newStart.Add(duration)); err != nil {
		if err == repo.ErrSlotTaken {
			http.Redirect(w, r, "/account/bookings/"+strconv.FormatInt(id, 10)+"/reschedule?error=That+time+was+just+taken%2C+pick+another", http.StatusSeeOther)
			return
		}
		a.serverError(w, "reschedule booking", err)
		return
	}

	a.Telegram.Send("🔁 Visitor rescheduled their own booking\nEmail: " + booking.Email +
		"\nNow: " + newStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM"))
	a.emailBooking(ctx, models.EmailBookingRescheduled, booking, newStart, newStart.Add(duration), "")

	http.Redirect(w, r, "/account", http.StatusSeeOther)
}
