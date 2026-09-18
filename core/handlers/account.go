package handlers

import (
	"net/http"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// AccountCourse pairs a paid purchase with the course it unlocks, so the
// template can link straight to /courses/{slug}/learn without a second
// lookup per row.
type AccountCourse struct {
	CourseTitle string
	CourseSlug  string
	PaidAt      string
	Amount      string
}

type AccountPayment struct {
	When        string
	Description string
	Amount      string
}

type AccountMessage struct {
	When string
	Body string
}

type AccountData struct {
	BaseData
	Bookings []AccountBooking
	Courses  []AccountCourse
	Payments []AccountPayment
	Messages []AccountMessage
}

type AccountBooking struct {
	ID             int64
	When           string
	Status         string
	WasRescheduled bool
	PreviousWhen   string
}

// AccountPage requires a logged-in visitor (see RequireUser in routes.go)
// and lists every booking, paid course purchase, payment, and admin
// message tied to their account, so they can find any of it again at any
// time rather than only right after checkout. Opening this page also
// marks any unread admin messages as read, the same way opening an inbox
// does.
func (a *App) AccountPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid, ok := a.Auth.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login?next=/account", http.StatusSeeOther)
		return
	}

	bookings, err := a.Bookings.ListByUserID(ctx, uid)
	if err != nil {
		a.serverError(w, "load bookings", err)
		return
	}
	purchases, err := a.Purchases.ListPaidByUserID(ctx, uid)
	if err != nil {
		a.serverError(w, "load purchases", err)
		return
	}
	messages, err := a.AdminMessages.ListByUserID(ctx, uid)
	if err != nil {
		a.serverError(w, "load messages", err)
		return
	}
	if err := a.AdminMessages.MarkAllReadByUserID(ctx, uid); err != nil {
		a.serverError(w, "mark messages read", err)
		return
	}

	data := AccountData{
		BaseData: a.base(r, "My Account — "+a.Cfg.SiteName, "Your consultancy bookings and purchased courses.", "account"),
	}
	for _, b := range bookings {
		row := AccountBooking{
			ID:     b.ID,
			When:   b.SlotStart.In(a.Timezone).Format("Mon, Jan 2 2006 · 3:04 PM"),
			Status: b.Status,
		}
		if !b.RescheduleAcknowledged && b.PreviousSlotStart != nil {
			row.WasRescheduled = true
			row.PreviousWhen = b.PreviousSlotStart.In(a.Timezone).Format("Mon, Jan 2 2006 · 3:04 PM")
		}
		data.Bookings = append(data.Bookings, row)
		if b.Status == models.BookingStatusConfirmed && b.AmountCents > 0 {
			data.Payments = append(data.Payments, AccountPayment{
				When:        b.SlotStart.In(a.Timezone).Format("Jan 2, 2006"),
				Description: "1:1 consultancy session",
				Amount:      models.FormatMoney(b.AmountCents, b.Currency),
			})
		}
	}
	for _, p := range purchases {
		course, err := a.Courses.GetByID(ctx, p.CourseID)
		if err != nil {
			continue
		}
		paidAt := ""
		if p.PaidAt != nil {
			paidAt = p.PaidAt.In(a.Timezone).Format("Jan 2, 2006")
		}
		data.Courses = append(data.Courses, AccountCourse{
			CourseTitle: course.Title,
			CourseSlug:  course.Slug,
			PaidAt:      paidAt,
			Amount:      models.FormatMoney(p.AmountCents, p.Currency),
		})
		data.Payments = append(data.Payments, AccountPayment{
			When:        paidAt,
			Description: "Course: " + course.Title,
			Amount:      models.FormatMoney(p.AmountCents, p.Currency),
		})
	}
	for _, m := range messages {
		data.Messages = append(data.Messages, AccountMessage{
			When: m.CreatedAt.In(a.Timezone).Format("Mon, Jan 2 2006 · 3:04 PM"),
			Body: m.Body,
		})
	}

	a.View.Render(w, http.StatusOK, "account", data)
}
