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
	PriceDisplay string
}

// ConsultancyPage is the informational page — the only place on the site
// with a "Book Now" call to action, which leads to /booking.
func (a *App) ConsultancyPage(w http.ResponseWriter, r *http.Request) {
	base := a.base(r, "Consultancy — "+a.Cfg.SiteName, "Book a 1:1 consultancy session for architecture reviews, technical leadership, or career guidance.", "consultancy")
	a.View.Render(w, http.StatusOK, "consultancy", ConsultancyData{
		BaseData:     base,
		PriceDisplay: models.ConsultancyRegionalPriceDisplay(base.Country),
	})
}

type BookingData struct {
	BaseData
	Days         []scheduler.DaySlots
	Success      bool
	Error        string
	PriceDisplay string
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
	rules, err := a.Availability.ListActive(ctx)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}

	from := time.Now().In(a.Timezone)
	to := from.AddDate(0, 0, 15)
	taken, err := a.Bookings.BookedSlots(ctx, from, to)
	if err != nil {
		a.serverError(w, "load bookings", err)
		return
	}

	days := scheduler.ComputeDays(rules, taken, a.Timezone, 14)
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
		Currency:     currency,
		AmountCents:  cents,
		KeyID:        a.Razorpay.KeyID,
		UserEmail:    userEmail,
		UserName:     userNameVal,
	})
}

// BookingCheckoutOrder creates a Razorpay order for one hour of
// consultancy and, in the same step, provisionally reserves the chosen
// slot (status=pending_payment) so a second buyer can't race for it.
func (a *App) BookingCheckoutOrder(w http.ResponseWriter, r *http.Request) {
	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments are not configured yet."})
		return
	}
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

	if err := r.ParseForm(); err != nil {
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

	receipt := fmt.Sprintf("booking-%d-%d", uid, time.Now().UnixNano())
	orderID, err := a.Razorpay.CreateOrder(cents, currency, receipt)
	if err != nil {
		log.Printf("razorpay create order (booking): %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start checkout. Please try again."})
		return
	}

	_, err = a.Bookings.CreatePending(ctx, models.Booking{
		UserID:          uid,
		Name:            user.Name,
		Email:           user.Email,
		Message:         message,
		SlotStart:       slotStart,
		SlotEnd:         slotStart.Add(time.Duration(slotMinutes) * time.Minute),
		AmountCents:     cents,
		Currency:        currency,
		RazorpayOrderID: orderID,
	})
	if err != nil {
		if err == repo.ErrSlotTaken {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "That time was just booked by someone else, please pick another."})
			return
		}
		a.serverError(w, "reserve booking", err)
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
