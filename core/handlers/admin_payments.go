package handlers

import (
	"net/http"
	"sort"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type adminPaymentRow struct {
	When        string
	whenSort    int64
	Type        string // "Course" or "Consultancy"
	Description string
	Email       string
	Amount      string
	PaymentID   string
}

type AdminPaymentsListData struct {
	BaseData
	Payments []adminPaymentRow
	Total    string
}

// AdminPaymentsList is a single combined ledger of every payment actually
// received — paid course purchases and confirmed (paid) consultancy
// bookings — since those live as two separate admin pages otherwise and
// neither on its own answers "what have I actually been paid".
func (a *App) AdminPaymentsList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	purchases, err := a.Purchases.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load purchases", err)
		return
	}
	bookings, err := a.Bookings.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load bookings", err)
		return
	}

	var rows []adminPaymentRow
	totalCents := map[string]int{}

	for _, p := range purchases {
		if p.Status != models.PurchaseStatusPaid || p.PaidAt == nil {
			continue
		}
		course, cerr := a.Courses.GetByID(ctx, p.CourseID)
		title := "(course removed)"
		if cerr == nil {
			title = course.Title
		}
		rows = append(rows, adminPaymentRow{
			When:        p.PaidAt.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM"),
			whenSort:    p.PaidAt.Unix(),
			Type:        "Course",
			Description: title,
			Email:       p.Email,
			Amount:      models.FormatMoney(p.AmountCents, p.Currency),
			PaymentID:   p.RazorpayPaymentID,
		})
		totalCents[p.Currency] += p.AmountCents
	}
	for _, b := range bookings {
		if b.Status != models.BookingStatusConfirmed || b.RazorpayPaymentID == "" {
			continue
		}
		rows = append(rows, adminPaymentRow{
			When:        b.SlotStart.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM"),
			whenSort:    b.CreatedAt.Unix(),
			Type:        "Consultancy",
			Description: "1:1 session · " + b.SlotStart.In(a.Timezone).Format("Mon, Jan 2"),
			Email:       b.Email,
			Amount:      models.FormatMoney(b.AmountCents, b.Currency),
			PaymentID:   b.RazorpayPaymentID,
		})
		totalCents[b.Currency] += b.AmountCents
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].whenSort > rows[j].whenSort })

	var totalParts []string
	for currency, cents := range totalCents {
		totalParts = append(totalParts, models.FormatMoney(cents, currency))
	}
	sort.Strings(totalParts)
	total := "$0"
	if len(totalParts) > 0 {
		total = totalParts[0]
		for _, p := range totalParts[1:] {
			total += " + " + p
		}
	}

	a.View.Render(w, http.StatusOK, "admin_payments_list", AdminPaymentsListData{
		BaseData: a.base(r, "Payments — Admin", "", "admin"),
		Payments: rows,
		Total:    total,
	})
}
