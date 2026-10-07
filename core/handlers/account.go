package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

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

// AccountSoftware pairs a paid software purchase with the software item
// it unlocks, so the template can link straight to the download without
// a second lookup per row -- mirrors AccountCourse.
type AccountSoftware struct {
	Title                    string
	Version                  string
	DownloadLink             string
	LicenseTerms             string
	PostPurchaseInstructions string
	PaidAt                   string
	Amount                   string
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

// AccountTestimonial reflects the logged-in visitor's own submitted
// testimonial, if any — "none" (no submission yet, show the form),
// "pending" (submitted, awaiting admin approval), or "published" (live on
// the About page).
type AccountTestimonial struct {
	Status      string
	AuthorName  string
	AuthorTitle string
	Body        string
}

type AccountData struct {
	BaseData
	Bookings    []AccountBooking
	Courses     []AccountCourse
	Software    []AccountSoftware
	Payments    []AccountPayment
	Messages    []AccountMessage
	Testimonial AccountTestimonial
	Error       string
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
	contribs, err := a.ArticleContributions.ListPaidByUserID(ctx, uid)
	if err != nil {
		a.serverError(w, "load article contributions", err)
		return
	}
	softwarePurchases, err := a.SoftwarePurchases.ListPaidByUserID(ctx, uid)
	if err != nil {
		a.serverError(w, "load software purchases", err)
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
		BaseData:    a.base(r, "My Account — "+a.Cfg.SiteName, "Your consultancy bookings and purchased courses.", "account"),
		Testimonial: a.accountTestimonial(ctx, uid),
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
	for _, c := range contribs {
		article, err := a.Articles.GetByID(ctx, c.ArticleID)
		title := "(article removed)"
		if err == nil {
			title = article.Title
		}
		paidAt := c.CreatedAt.In(a.Timezone).Format("Jan 2, 2006")
		if c.PaidAt != nil {
			paidAt = c.PaidAt.In(a.Timezone).Format("Jan 2, 2006")
		}
		data.Payments = append(data.Payments, AccountPayment{
			When:        paidAt,
			Description: "Article: " + title,
			Amount:      models.FormatMoney(c.AmountCents, c.Currency),
			ReceiptURL:  "/account/articles/" + strconv.FormatInt(c.ID, 10) + "/receipt.pdf",
		})
	}
	for _, p := range softwarePurchases {
		sw, err := a.Software.GetByID(ctx, p.SoftwareID)
		if err != nil {
			continue
		}
		paidAt := ""
		if p.PaidAt != nil {
			paidAt = p.PaidAt.In(a.Timezone).Format("Jan 2, 2006")
		}
		data.Software = append(data.Software, AccountSoftware{
			Title:                    sw.Title,
			Version:                  sw.Version,
			DownloadLink:             sw.DownloadLink,
			LicenseTerms:             sw.LicenseTerms,
			PostPurchaseInstructions: sw.PostPurchaseInstructions,
			PaidAt:                   paidAt,
			Amount:                   models.FormatMoney(p.AmountCents, p.Currency),
		})
		data.Payments = append(data.Payments, AccountPayment{
			When:        paidAt,
			Description: "Software: " + sw.Title + " " + sw.Version,
			Amount:      models.FormatMoney(p.AmountCents, p.Currency),
			ReceiptURL:  "/account/software/" + strconv.FormatInt(p.ID, 10) + "/receipt.pdf",
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

	// A $0/100%-off checkout never touches Razorpay at all (see
	// CheckoutCreateOrder's amountCents == 0 branch) — RazorpayPaymentID
	// is only ever set once a real Razorpay payment is verified, so its
	// presence is a reliable way to tell the two apart on the receipt.
	paymentMethod := "Free enrollment (no payment)"
	orderID := ""
	if p.RazorpayPaymentID != "" {
		paymentMethod = "Razorpay"
		orderID = p.RazorpayOrderID
	}

	pdfBytes, err := receipt.Generate(receipt.Data{
		ReceiptNo:     receipt.ReceiptNoForPurchase(p.ID),
		IssuedAt:      issuedAt,
		PayerEmail:    p.Email,
		Description:   "Course: " + course.Title,
		AmountCents:   p.AmountCents,
		Currency:      p.Currency,
		PromoCode:     p.PromoCode,
		PromoText:     p.PromoText,
		DiscountCents: p.DiscountCents,
		PaymentMethod: paymentMethod,
		OrderID:       orderID,
		PaymentID:     p.RazorpayPaymentID,
		SiteName:      a.Cfg.SiteName,
		IssuerName:    a.issuerName(ctx),
	})
	if err != nil {
		a.serverError(w, "generate receipt", err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="receipt-`+receipt.ReceiptNoForPurchase(p.ID)+`.pdf"`)
	w.Write(pdfBytes)
}

// AccountSoftwareReceipt streams a PDF payment receipt for one of the
// logged-in visitor's own paid software purchases — mirrors
// AccountPurchaseReceipt.
func (a *App) AccountSoftwareReceipt(w http.ResponseWriter, r *http.Request) {
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
	purchases, err := a.SoftwarePurchases.ListPaidByUserID(ctx, uid)
	if err != nil {
		a.serverError(w, "load software purchases", err)
		return
	}
	var p *models.SoftwarePurchase
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
	sw, err := a.Software.GetByID(ctx, p.SoftwareID)
	if err != nil {
		a.serverError(w, "load software", err)
		return
	}
	issuedAt := p.CreatedAt
	if p.PaidAt != nil {
		issuedAt = *p.PaidAt
	}

	paymentMethod := "Free enrollment (no payment)"
	orderID := ""
	if p.RazorpayPaymentID != "" {
		paymentMethod = "Razorpay"
		orderID = p.RazorpayOrderID
	}

	pdfBytes, err := receipt.Generate(receipt.Data{
		ReceiptNo:     receipt.ReceiptNoForSoftwarePurchase(p.ID),
		IssuedAt:      issuedAt,
		PayerEmail:    p.Email,
		Description:   "Software: " + sw.Title + " " + sw.Version,
		AmountCents:   p.AmountCents,
		Currency:      p.Currency,
		PromoCode:     p.PromoCode,
		PromoText:     p.PromoText,
		DiscountCents: p.DiscountCents,
		PaymentMethod: paymentMethod,
		OrderID:       orderID,
		PaymentID:     p.RazorpayPaymentID,
		SiteName:      a.Cfg.SiteName,
		IssuerName:    a.issuerName(ctx),
	})
	if err != nil {
		a.serverError(w, "generate receipt", err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="receipt-`+receipt.ReceiptNoForSoftwarePurchase(p.ID)+`.pdf"`)
	w.Write(pdfBytes)
}

// AccountArticleReceipt streams a PDF receipt for one of the logged-in
// visitor's own paid article contributions.
func (a *App) AccountArticleReceipt(w http.ResponseWriter, r *http.Request) {
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
	c, err := a.ArticleContributions.GetByID(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load article contribution", err)
		return
	}
	if c.UserID != uid || c.Status != models.ContributionStatusPaid {
		a.NotFound(w, r)
		return
	}
	article, err := a.Articles.GetByID(ctx, c.ArticleID)
	title := "(article removed)"
	if err == nil {
		title = article.Title
	}
	issuedAt := c.CreatedAt
	if c.PaidAt != nil {
		issuedAt = *c.PaidAt
	}

	pdfBytes, err := receipt.Generate(receipt.Data{
		ReceiptNo:   receipt.ReceiptNoForArticleContribution(c.ID),
		IssuedAt:    issuedAt,
		PayerEmail:  c.Email,
		Description: "Article contribution: " + title,
		AmountCents: c.AmountCents,
		Currency:    c.Currency,
		PaymentID:   c.RazorpayPaymentID,
		SiteName:    a.Cfg.SiteName,
		IssuerName:  a.issuerName(ctx),
	})
	if err != nil {
		a.serverError(w, "generate receipt", err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="receipt-`+receipt.ReceiptNoForArticleContribution(c.ID)+`.pdf"`)
	w.Write(pdfBytes)
}

// accountTestimonial loads the status of a visitor's own testimonial
// submission, if any, for the /account page's "Share your experience"
// section. Defaults AuthorName to the account's own name so the form
// isn't blank the first time.
func (a *App) accountTestimonial(ctx context.Context, uid int64) AccountTestimonial {
	t, err := a.Testimonials.GetByUserID(ctx, uid)
	if err == nil {
		status := "pending"
		if t.IsPublished {
			status = "published"
		}
		return AccountTestimonial{Status: status, AuthorName: t.AuthorName, AuthorTitle: t.AuthorTitle, Body: t.Body}
	}

	name := ""
	if u, err := a.Users.GetByID(ctx, uid); err == nil {
		name = u.Name
	}
	return AccountTestimonial{Status: "none", AuthorName: name}
}

// AccountTestimonialSubmit lets a logged-in visitor submit their own
// testimonial. It always starts unpublished, regardless of anything
// submitted — visible only once an admin approves it — and only accepts
// one submission per account (resubmitting isn't offered; edits go
// through the admin, same as anything else on this page).
func (a *App) AccountTestimonialSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid, ok := a.Auth.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login?next=/account", http.StatusSeeOther)
		return
	}

	if _, err := a.Testimonials.GetByUserID(ctx, uid); err == nil {
		http.Redirect(w, r, "/account#testimonial", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	authorName := strings.TrimSpace(r.FormValue("author_name"))
	authorTitle := strings.TrimSpace(r.FormValue("author_title"))
	body := strings.TrimSpace(r.FormValue("body"))
	if authorName == "" || body == "" {
		data := AccountData{
			BaseData:    a.base(r, "My Account — "+a.Cfg.SiteName, "Your consultancy bookings and purchased courses.", "account"),
			Testimonial: AccountTestimonial{Status: "none", AuthorName: authorName, AuthorTitle: authorTitle, Body: body},
			Error:       "Name and testimonial text are both required.",
		}
		a.View.Render(w, http.StatusUnprocessableEntity, "account", data)
		return
	}

	if _, err := a.Testimonials.CreateFromUser(ctx, uid, authorName, authorTitle, body); err != nil {
		a.serverError(w, "submit testimonial", err)
		return
	}
	http.Redirect(w, r, "/account#testimonial", http.StatusSeeOther)
}
