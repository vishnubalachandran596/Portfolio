package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/geo"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/scheduler"
)

type ConsultancyData struct {
	BaseData
	PriceDisplay         string
	HourlyIsFree         bool
	AuditPriceDisplay    string
	RetainerRangeDisplay string
}

// ConsultancyPage is the informational page — three tiers, none of them
// self-checkout from here anymore: the Architecture Review and the
// Fractional Advisory retainer both route to /contact to scope on a
// call first (see AuditCheckoutOrder's doc comment for why the paid
// self-checkout backend still exists even though this page no longer
// links to it), and the hourly session books directly via /booking.
func (a *App) ConsultancyPage(w http.ResponseWriter, r *http.Request) {
	base := a.base(r, "Technical Consultant — Architecture Reviews & Fractional Advisory | "+a.Cfg.SiteName, "Hire a technical consultant for a fixed-price architecture review, an ongoing fractional advisory retainer, or a 1:1 hourly session — from an 8+ year banking and fintech platform architect.", "consultancy")

	a.View.Render(w, http.StatusOK, "consultancy", ConsultancyData{
		BaseData:             base,
		PriceDisplay:         models.ConsultancyRegionalPriceDisplay(base.Country),
		HourlyIsFree:         models.ConsultancyPriceCentsUSD <= 0,
		AuditPriceDisplay:    models.AuditRegionalPriceDisplay(base.Country),
		RetainerRangeDisplay: models.RetainerRangeDisplay(base.Country),
	})
}

// AuditCheckoutOrder creates a Razorpay order for the fixed-price
// Architecture Review package — no calendar slot involved, just an
// intake (name, email, company, brief) and a fixed amount resolved
// server-side from the visitor's region, mirroring BookingCheckoutOrder.
//
// The consultancy page no longer links here: the Architecture Review is
// now scoped over a call first (like the retainer), not self-checkout.
// This handler, its route, and the audit_requests table are left intact
// rather than deleted — real historical rows exist from when self-checkout
// was live, and the fastest way back to self-checkout (if that's ever
// wanted again) is re-adding the form to consultancy.html, not rebuilding
// this from scratch.
func (a *App) AuditCheckoutOrder(w http.ResponseWriter, r *http.Request) {
	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments are not configured yet — reach out directly instead."})
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form submission."})
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	company := strings.TrimSpace(r.FormValue("company"))
	brief := strings.TrimSpace(r.FormValue("brief"))
	if name == "" || email == "" || !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Name and a valid email are required."})
		return
	}

	currency, cents := models.AuditRegionalPrice(geo.CountryFromRequest(r))

	receipt := fmt.Sprintf("audit-%d", time.Now().UnixNano())
	orderID, err := a.Razorpay.CreateOrder(cents, currency, receipt)
	if err != nil {
		log.Printf("razorpay create order (audit): %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start checkout. Please try again."})
		return
	}

	if _, err := a.AuditRequests.Create(r.Context(), models.AuditRequest{
		Name: name, Email: email, Company: company, Brief: brief,
		AmountCents: cents, Currency: currency, RazorpayOrderID: orderID,
	}); err != nil {
		a.serverError(w, "record audit request", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id": orderID,
		"amount":   cents,
		"currency": currency,
		"key_id":   a.Razorpay.KeyID,
		"email":    email,
		"name":     name,
	})
}

func (a *App) AuditCheckoutVerify(w http.ResponseWriter, r *http.Request) {
	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments are not configured yet."})
		return
	}
	var body struct {
		OrderID   string `json:"razorpay_order_id"`
		PaymentID string `json:"razorpay_payment_id"`
		Signature string `json:"razorpay_signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request."})
		return
	}
	if !a.Razorpay.VerifySignature(body.OrderID, body.PaymentID, body.Signature) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Payment verification failed."})
		return
	}
	if err := a.AuditRequests.MarkPaid(r.Context(), body.OrderID, body.PaymentID); err != nil && err != repo.ErrNotFound {
		a.serverError(w, "mark audit request paid", err)
		return
	}
	if req, aerr := a.AuditRequests.GetByRazorpayOrderID(r.Context(), body.OrderID); aerr == nil {
		a.Telegram.Send(fmt.Sprintf("🏗️ Architecture Review purchased\nName: %s\nEmail: %s\nCompany: %s\nAmount: %s\nBrief: %s",
			req.Name, req.Email, req.Company, models.FormatMoney(req.AmountCents, req.Currency), req.Brief))
	}
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/consultancy?audit=1"})
}

type BookingData struct {
	BaseData
	Days         []scheduler.DaySlots
	Success      bool
	Error        string
	PriceDisplay string
	IsFree       bool
	Currency     string
	AmountCents  int
	KeyID        string
	UserEmail    string
	UserName     string
}

// BookingPage requires a logged-in visitor (see RequireUser in routes.go)
// and shows the consultancy price up front before any slot is picked.
func (a *App) BookingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days, err := a.availableDays(r)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}

	base := a.base(r, "Book Consultancy — "+a.Cfg.SiteName, "Book a 1:1 consultancy session.", "consultancy")
	currency, cents := models.ConsultancyRegionalPrice(base.Country)

	userEmail, userNameVal := "", ""
	if uid, ok := a.Auth.UserID(r); ok {
		if u, err := a.Users.GetByID(ctx, uid); err == nil {
			userEmail = u.Email
			userNameVal = u.Name
		}
	}

	a.View.Render(w, http.StatusOK, "booking", BookingData{
		BaseData:     base,
		Days:         days,
		Success:      r.URL.Query().Get("booked") == "1",
		Error:        r.URL.Query().Get("error"),
		PriceDisplay: models.FormatMoney(cents, currency) + "/hour",
		IsFree:       cents <= 0,
		Currency:     currency,
		AmountCents:  cents,
		KeyID:        a.Razorpay.KeyID,
		UserEmail:    userEmail,
		UserName:     userNameVal,
	})
}

// BookingCheckoutOrder creates a Razorpay order for one hour of
// consultancy and, in the same step, provisionally reserves the chosen
// slot (status=pending_payment) so a second buyer can't race for it. When
// the final price is $0 (see the free/paid branch below), Razorpay is
// skipped entirely, so its own configured-or-not state doesn't gate this
// handler at the top anymore — only the paid branch checks it.
func (a *App) BookingCheckoutOrder(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.Auth.UserID(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Please log in to book a session."})
		return
	}
	ctx := r.Context()
	user, err := a.Users.GetByID(ctx, uid)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Please log in to book a session."})
		return
	}

	// See the matching comment in checkout.go's CheckoutCreateOrder: this
	// form is submitted via fetch() with a FormData body (multipart), so
	// ParseMultipartForm is required — plain ParseForm never reads it.
	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form submission."})
		return
	}
	message := strings.TrimSpace(r.FormValue("message"))
	slotRaw := strings.TrimSpace(r.FormValue("slot"))
	slotStart, err := time.Parse(time.RFC3339, slotRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That time slot is no longer valid, please pick another."})
		return
	}

	rules, err := a.Availability.ListActive(ctx)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	if !a.slotIsOffered(rules, slotStart) || slotStart.Before(time.Now()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That time slot is not available, please pick another."})
		return
	}
	slotMinutes := models.ConsultancyDurationMinutes
	for _, rr := range rules {
		if rr.Weekday == slotStart.In(a.Timezone).Weekday() {
			slotMinutes = rr.SlotMinutes
			break
		}
	}

	currency, cents := models.ConsultancyRegionalPrice(geo.CountryFromRequest(r))

	discountCents, promo, perr := a.resolvePromoCode(ctx, r.FormValue("promo_code"), models.PromoProductConsultancy, 0, cents)
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That promo code isn't valid."})
		return
	}
	cents -= discountCents

	// Hourly consultancy is free for now (see models.ConsultancyPriceCentsUSD/INR)
	// — same "skip Razorpay entirely" pattern as a $0 course purchase in
	// checkout.go: a synthetic order id, straight to confirmed, no payment
	// step at all. Still reuses CreatePending/MarkPaid rather than a new
	// code path, so the slot-race protection (unique constraint on
	// slot_start) applies exactly the same as a paid booking.
	var orderID string
	if cents <= 0 {
		token, terr := generateAccessToken()
		if terr != nil {
			a.serverError(w, "generate free booking token", terr)
			return
		}
		orderID = "free-" + token
	} else {
		if !a.Razorpay.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments are not configured yet."})
			return
		}
		receipt := fmt.Sprintf("booking-%d-%d", uid, time.Now().UnixNano())
		oid, oerr := a.Razorpay.CreateOrder(cents, currency, receipt)
		if oerr != nil {
			log.Printf("razorpay create order (booking): %v", oerr)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start checkout. Please try again."})
			return
		}
		orderID = oid
	}

	booking := models.Booking{
		UserID:          uid,
		Name:            user.Name,
		Email:           user.Email,
		Message:         message,
		SlotStart:       slotStart,
		SlotEnd:         slotStart.Add(time.Duration(slotMinutes) * time.Minute),
		AmountCents:     cents,
		Currency:        currency,
		RazorpayOrderID: orderID,
		DiscountCents:   discountCents,
	}
	if promo != nil {
		booking.PromoCodeID = &promo.ID
		booking.PromoCode = promo.Code
	}
	_, err = a.Bookings.CreatePending(ctx, booking)
	if err != nil {
		if err == repo.ErrSlotTaken {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "That time was just booked by someone else, please pick another."})
			return
		}
		a.serverError(w, "reserve booking", err)
		return
	}

	if cents <= 0 {
		if err := a.Bookings.MarkPaid(ctx, orderID, ""); err != nil {
			a.serverError(w, "confirm free booking", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"free": true, "redirect": "/booking?booked=1"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id": orderID,
		"amount":   cents,
		"currency": currency,
		"key_id":   a.Razorpay.KeyID,
		"email":    user.Email,
		"name":     user.Name,
	})
}

func (a *App) BookingCheckoutVerify(w http.ResponseWriter, r *http.Request) {
	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments are not configured yet."})
		return
	}
	var body struct {
		OrderID   string `json:"razorpay_order_id"`
		PaymentID string `json:"razorpay_payment_id"`
		Signature string `json:"razorpay_signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request."})
		return
	}
	if !a.Razorpay.VerifySignature(body.OrderID, body.PaymentID, body.Signature) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Payment verification failed."})
		return
	}
	if err := a.Bookings.MarkPaid(r.Context(), body.OrderID, body.PaymentID); err != nil && err != repo.ErrNotFound {
		a.serverError(w, "mark booking paid", err)
		return
	}

	if booking, berr := a.Bookings.GetByRazorpayOrderID(r.Context(), body.OrderID); berr == nil {
		if booking.PromoCodeID != nil {
			if err := a.PromoCodes.IncrementRedemption(r.Context(), *booking.PromoCodeID); err != nil {
				log.Printf("increment promo redemption: %v", err)
			}
		}
		promoLine := ""
		if booking.PromoCode != "" {
			promoLine = fmt.Sprintf("\nPromo: %s (-%s)", booking.PromoCode, models.FormatMoney(booking.DiscountCents, booking.Currency))
		}
		a.Telegram.Send(fmt.Sprintf("📅 Consultancy booking\nName: %s\nEmail: %s\nWhen: %s\nAmount: %s%s",
			booking.Name, booking.Email, booking.SlotStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM MST"),
			models.FormatMoney(booking.AmountCents, booking.Currency), promoLine))
	}

	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/booking?booked=1"})
}

type ContactData struct {
	BaseData
	Profile models.Profile
}

func (a *App) ContactPage(w http.ResponseWriter, r *http.Request) {
	profile, err := a.Profile.Get(r.Context())
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	a.View.Render(w, http.StatusOK, "contact", ContactData{
		BaseData: a.base(r, "Contact — "+a.Cfg.SiteName, "Get in touch, or book a consultancy session directly.", "contact"),
		Profile:  profile,
	})
}
