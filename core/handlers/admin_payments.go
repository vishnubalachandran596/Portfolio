package handlers

import (
	"net/http"
	"sort"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type adminPaymentRow struct {
	When        string
	whenSort    int64
	Type        string // "Course", "Consultancy", "Audit", "Article", or "Software"
	Description string
	Email       string
	Amount      string
	PaymentID   string
	PromoCode   string
	Discount    string
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
	auditRequests, err := a.AuditRequests.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load audit requests", err)
		return
	}
	articleContribs, err := a.ArticleContributions.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load article contributions", err)
		return
	}
	softwarePurchases, err := a.SoftwarePurchases.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load software purchases", err)
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
		row := adminPaymentRow{
			When:        p.PaidAt.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM"),
			whenSort:    p.PaidAt.Unix(),
			Type:        "Course",
			Description: title,
			Email:       p.Email,
			Amount:      models.FormatMoney(p.AmountCents, p.Currency),
			PaymentID:   p.RazorpayPaymentID,
			PromoCode:   p.PromoCode,
		}
		if p.PromoCode != "" {
			row.Discount = models.FormatMoney(p.DiscountCents, p.Currency)
		}
		rows = append(rows, row)
		totalCents[p.Currency] += p.AmountCents
	}
	for _, b := range bookings {
		if b.Status != models.BookingStatusConfirmed || b.RazorpayPaymentID == "" {
			continue
		}
		row := adminPaymentRow{
			When:        b.SlotStart.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM"),
			whenSort:    b.CreatedAt.Unix(),
			Type:        "Consultancy",
			Description: "1:1 session · " + b.SlotStart.In(a.Timezone).Format("Mon, Jan 2"),
			Email:       b.Email,
			Amount:      models.FormatMoney(b.AmountCents, b.Currency),
			PaymentID:   b.RazorpayPaymentID,
			PromoCode:   b.PromoCode,
		}
		if b.PromoCode != "" {
			row.Discount = models.FormatMoney(b.DiscountCents, b.Currency)
		}
		rows = append(rows, row)
		totalCents[b.Currency] += b.AmountCents
	}
	for _, aud := range auditRequests {
		if aud.Status != models.AuditStatusPaid || aud.PaidAt == nil {
			continue
		}
		desc := "Architecture Review"
		if aud.Company != "" {
			desc += " · " + aud.Company
		}
		rows = append(rows, adminPaymentRow{
			When:        aud.PaidAt.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM"),
			whenSort:    aud.PaidAt.Unix(),
			Type:        "Audit",
			Description: desc,
			Email:       aud.Email,
			Amount:      models.FormatMoney(aud.AmountCents, aud.Currency),
			PaymentID:   aud.RazorpayPaymentID,
		})
		totalCents[aud.Currency] += aud.AmountCents
	}
	for _, c := range articleContribs {
		if c.Status != models.ContributionStatusPaid || c.PaidAt == nil {
			continue
		}
		article, aerr := a.Articles.GetByID(ctx, c.ArticleID)
		title := "(article removed)"
		if aerr == nil {
			title = article.Title
		}
		rows = append(rows, adminPaymentRow{
			When:        c.PaidAt.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM"),
			whenSort:    c.PaidAt.Unix(),
			Type:        "Article",
			Description: title,
			Email:       c.Email,
			Amount:      models.FormatMoney(c.AmountCents, c.Currency),
			PaymentID:   c.RazorpayPaymentID,
		})
		totalCents[c.Currency] += c.AmountCents
	}
	for _, p := range softwarePurchases {
		if p.Status != models.PurchaseStatusPaid || p.PaidAt == nil {
			continue
		}
		sw, swerr := a.Software.GetByID(ctx, p.SoftwareID)
		title := "(software removed)"
		if swerr == nil {
			title = sw.Title + " " + sw.Version
		}
		row := adminPaymentRow{
			When:        p.PaidAt.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM"),
			whenSort:    p.PaidAt.Unix(),
			Type:        "Software",
			Description: title,
			Email:       p.Email,
			Amount:      models.FormatMoney(p.AmountCents, p.Currency),
			PaymentID:   p.RazorpayPaymentID,
			PromoCode:   p.PromoCode,
		}
		if p.PromoCode != "" {
			row.Discount = models.FormatMoney(p.DiscountCents, p.Currency)
		}
		rows = append(rows, row)
		totalCents[p.Currency] += p.AmountCents
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].whenSort > rows[j].whenSort })

	total := models.FormatMoneyTotals(totalCents)
	if total == "" {
		total = "$0"
	}

	a.View.Render(w, http.StatusOK, "admin_payments_list", AdminPaymentsListData{
		BaseData: a.base(r, "Payments — Admin", "", "admin"),
		Payments: rows,
		Total:    total,
	})
}
