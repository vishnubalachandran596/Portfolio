package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/receipt"
	"github.com/vishnubalachandran596/portfolio/core/repo"
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
	ReceiptURL  string
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
				ReceiptURL:  "/account/bookings/" + strconv.FormatInt(b.ID, 10) + "/receipt.pdf",
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
			ReceiptURL:  "/account/purchases/" + strconv.FormatInt(p.ID, 10) + "/receipt.pdf",
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

func (a *App) issuerName(ctx context.Context) string {
	issuer := a.Cfg.SiteName
	if profile, err := a.Profile.Get(ctx); err == nil && profile.Name != "" {
		issuer = profile.Name
	}
	return issuer
}

// AccountBookingReceipt streams a PDF payment receipt for one of the
// logged-in visitor's own confirmed, paid bookings.
func (a *App) AccountBookingReceipt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid, ok := a.Auth.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	b, err := a.Bookings.GetByID(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load booking", err)
		return
	}
	if b.UserID != uid || b.Status != models.BookingStatusConfirmed || b.AmountCents <= 0 {
		a.NotFound(w, r)
		return
	}

	pdfBytes, err := receipt.Generate(receipt.Data{
		ReceiptNo:   receipt.ReceiptNoForBooking(b.ID),
		IssuedAt:    b.SlotStart,
		PayerEmail:  b.Email,
		Description: "1:1 consultancy session — " + b.SlotStart.In(a.Timezone).Format("Jan 2, 2006"),
		AmountCents: b.AmountCents,
		Currency:    b.Currency,
		PaymentID:   b.RazorpayPaymentID,
		SiteName:    a.Cfg.SiteName,
		IssuerName:  a.issuerName(ctx),
	})
	if err != nil {
		a.serverError(w, "generate receipt", err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="receipt-`+receipt.ReceiptNoForBooking(b.ID)+`.pdf"`)
	w.Write(pdfBytes)
}

// AccountPurchaseReceipt streams a PDF payment receipt for one of the
// logged-in visitor's own paid course purchases.
func (a *App) AccountPurchaseReceipt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid, ok := a.Auth.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	purchases, err := a.Purchases.ListPaidByUserID(ctx, uid)
	if err != nil {
		a.serverError(w, "load purchases", err)
		return
	}
	var p *models.Purchase
	for i := range purchases {
		if purchases[i].ID == id {
			p = &purchases[i]
			break
		}
	}
	if p == nil {
		a.NotFound(w, r)
		return
	}
	course, err := a.Courses.GetByID(ctx, p.CourseID)
	if err != nil {
		a.serverError(w, "load course", err)
		return
	}
	issuedAt := p.CreatedAt
	if p.PaidAt != nil {
		issuedAt = *p.PaidAt
	}

	pdfBytes, err := receipt.Generate(receipt.Data{
		ReceiptNo:   receipt.ReceiptNoForPurchase(p.ID),
		IssuedAt:    issuedAt,
		PayerEmail:  p.Email,
		Description: "Course: " + course.Title,
		AmountCents: p.AmountCents,
		Currency:    p.Currency,
		PaymentID:   p.RazorpayPaymentID,
		SiteName:    a.Cfg.SiteName,
		IssuerName:  a.issuerName(ctx),
	})
	if err != nil {
		a.serverError(w, "generate receipt", err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="receipt-`+receipt.ReceiptNoForPurchase(p.ID)+`.pdf"`)
	w.Write(pdfBytes)
}
