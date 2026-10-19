package handlers

import (
	"net/http"
	"net/url"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/scheduler"
)

// The private manage link (/booking/manage/{token}) is how a no-login
// visitor views, cancels or reschedules their booking. The site sends no
// email, so the token is shown once on screen after booking and only its
// SHA-256 hash is stored. Possession of the link is the credential: every
// handler looks the booking up by that hash and nothing else.

type BookingManageData struct {
	BaseData
	Token          string
	Booking        models.Booking
	When           string
	Duration       int
	CanChange      bool // confirmed and still in the future
	Notice         string
	Error          string
	Days           []scheduler.DaySlots
	WasRescheduled bool
	PreviousWhen   string
}

// manageGuard wraps the manage routes: never cached, never indexed, and the
// secret URL is not leaked through the Referer header.
func manageGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func managePath(token string) string { return "/booking/manage/" + url.PathEscape(token) }

// loadManaged resolves the token to a booking, or renders a 404.
func (a *App) loadManaged(w http.ResponseWriter, r *http.Request) (models.Booking, string, bool) {
	token := r.PathValue("token")
	b, err := a.Bookings.GetByManageTokenHash(r.Context(), authutil.HashKey(token))
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
		} else {
			a.serverError(w, "load managed booking", err)
		}
		return models.Booking{}, "", false
	}
	return b, token, true
}

func (a *App) manageData(r *http.Request, b models.Booking, token string) BookingManageData {
	d := BookingManageData{
		BaseData:  a.base(r, "Your booking "+b.Code+" — "+a.Cfg.SiteName, "", "consultancy"),
		Token:     token,
		Booking:   b,
		When:      b.SlotStart.In(a.Timezone).Format("Mon, Jan 2 2006 · 3:04 PM MST"),
		Duration:  int(b.SlotEnd.Sub(b.SlotStart).Minutes()),
		CanChange: b.Status == models.BookingStatusConfirmed && b.SlotStart.After(time.Now()),
		Error:     r.URL.Query().Get("error"),
	}
	switch r.URL.Query().Get("notice") {
	case "rescheduled":
		d.Notice = "Your booking was moved to the new time."
	case "cancelled":
		d.Notice = "Your booking was cancelled and the time is free again."
	}
	if b.PreviousSlotStart != nil && b.Status == models.BookingStatusConfirmed {
		d.WasRescheduled = true
		d.PreviousWhen = b.PreviousSlotStart.In(a.Timezone).Format("Mon, Jan 2 2006 · 3:04 PM")
	}
	return d
}

// BookingManagePage shows the booking behind a manage link.
func (a *App) BookingManagePage(w http.ResponseWriter, r *http.Request) {
	b, token, ok := a.loadManaged(w, r)
	if !ok {
		return
	}
	a.View.Render(w, http.StatusOK, "booking_manage", a.manageData(r, b, token))
}

// BookingManageCancel cancels the booking and frees its slot.
func (a *App) BookingManageCancel(w http.ResponseWriter, r *http.Request) {
	b, token, ok := a.loadManaged(w, r)
	if !ok {
		return
	}
	if err := a.Bookings.CancelByManageTokenHash(r.Context(), authutil.HashKey(token)); err != nil {
		if err == repo.ErrNotFound {
			http.Redirect(w, r, managePath(token)+"?error="+url.QueryEscape("This booking can no longer be cancelled."), http.StatusSeeOther)
			return
		}
		a.serverError(w, "cancel booking", err)
		return
	}
	a.Telegram.Send("❌ Booking cancelled (" + b.Code + ")\nName: " + b.Name + "\nWas: " + b.SlotStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM"))
	http.Redirect(w, r, managePath(token)+"?notice=cancelled", http.StatusSeeOther)
}

// BookingManageReschedulePage lists the open slots for moving the booking.
func (a *App) BookingManageReschedulePage(w http.ResponseWriter, r *http.Request) {
	b, token, ok := a.loadManaged(w, r)
	if !ok {
		return
	}
	d := a.manageData(r, b, token)
	if !d.CanChange {
		http.Redirect(w, r, managePath(token), http.StatusSeeOther)
		return
	}
	days, err := a.availableDays(r)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	d.Days = days
	a.View.Render(w, http.StatusOK, "booking_manage_reschedule", d)
}

// BookingManageRescheduleSubmit moves the booking, applying the same rules
// as the signed-in reschedule (offered slot, future, same length) plus the
// 10:00-20:00 IST day and the 30-day window.
func (a *App) BookingManageRescheduleSubmit(w http.ResponseWriter, r *http.Request) {
	b, token, ok := a.loadManaged(w, r)
	if !ok {
		return
	}
	back := managePath(token) + "/reschedule?error="
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	newStart, err := time.Parse(time.RFC3339, r.FormValue("slot"))
	if err != nil {
		http.Redirect(w, r, back+url.QueryEscape("Pick a valid time."), http.StatusSeeOther)
		return
	}
	rules, err := a.Availability.ListActive(r.Context())
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	if msg, _ := a.bookableSlotError(rules, newStart); msg != "" {
		http.Redirect(w, r, back+url.QueryEscape(msg), http.StatusSeeOther)
		return
	}
	duration := b.SlotEnd.Sub(b.SlotStart)
	if err := a.Bookings.RescheduleByManageTokenHash(r.Context(), authutil.HashKey(token), newStart, newStart.Add(duration)); err != nil {
		switch err {
		case repo.ErrSlotTaken:
			http.Redirect(w, r, back+url.QueryEscape("That time was just taken, please pick another."), http.StatusSeeOther)
		case repo.ErrNotFound:
			http.Redirect(w, r, managePath(token)+"?error="+url.QueryEscape("This booking can no longer be changed."), http.StatusSeeOther)
		default:
			a.serverError(w, "reschedule booking", err)
		}
		return
	}
	a.Telegram.Send("🔁 Visitor rescheduled their booking (" + b.Code + ")\nName: " + b.Name +
		"\nWas: " + b.SlotStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM") +
		"\nNow: " + newStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM"))
	http.Redirect(w, r, managePath(token)+"?notice=rescheduled", http.StatusSeeOther)
}
