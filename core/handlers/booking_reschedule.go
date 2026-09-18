package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/scheduler"
)

// availableDays computes the next two weeks of bookable slots from the
// active availability rules minus what's already taken — shared by the
// booking page and both reschedule flows.
func (a *App) availableDays(r *http.Request) ([]scheduler.DaySlots, error) {
	ctx := r.Context()
	rules, err := a.Availability.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	from := time.Now().In(a.Timezone)
	to := from.AddDate(0, 0, 15)
	taken, err := a.Bookings.BookedSlots(ctx, from, to)
	if err != nil {
		return nil, err
	}
	return scheduler.ComputeDays(rules, taken, a.Timezone, 14), nil
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
	if !a.slotIsOffered(rules, newStart) || newStart.Before(time.Now()) {
		http.Redirect(w, r, "/account/bookings/"+strconv.FormatInt(id, 10)+"/reschedule?error=That+time+is+not+available", http.StatusSeeOther)
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

	http.Redirect(w, r, "/account", http.StatusSeeOther)
}
