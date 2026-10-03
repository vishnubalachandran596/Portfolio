package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

func generateAccessToken() (string, error) {
	return authutil.RandomHex(32)
}

func accessCookieName(courseID int64) string {
	return "ca_" + strconv.FormatInt(courseID, 10)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// CheckoutCreateOrder creates a Razorpay order for a paid course. The
// buyer picks an amount client-side (pay-what-you-want, floored at the
// course's minimum); we re-validate that floor server-side — never trust
// a client-supplied amount below the minimum, though paying *more* is
// always fine.
func (a *App) CheckoutCreateOrder(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	if err != nil || !course.IsPaid || !course.IsPublished {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Course not found."})
		return
	}

	// The purchase widget submits via fetch() with a FormData body, which
	// the browser always sends as multipart/form-data — ParseForm alone
	// never reads that (it only parses application/x-www-form-urlencoded),
	// so ParseMultipartForm is required here. ErrNotMultipart just means a
	// urlencoded body came in instead, which ParseMultipartForm already
	// parsed via its own internal ParseForm call.
	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form submission."})
		return
	}
	// A Google-verified account is mandatory for every checkout — paid or
	// free — both so a purchase can always be tied to a real account (no
	// drifting "my courses" list) and so a promo code's one-redemption-
	// per-user rule below has something stable to key off. There is no
	// guest-checkout path anymore.
	uid, ok := a.Auth.UserID(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Please log in with Google to continue.", "login_required": "1"})
		return
	}
	userID := uid
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Please log in with Google to continue.", "login_required": "1"})
		return
	}
	email := user.Email
	// Courses sell in INR only — see models.CurrencyINR.
	currency, minCents := models.CurrencyINR, course.MinPriceCents

	amountCents, _ := strconv.Atoi(r.FormValue("amount_cents"))
	if amountCents < minCents {
		amountCents = minCents
	}

	discountCents, promo, perr := a.resolvePromoCode(r.Context(), r.FormValue("promo_code"), models.PromoProductCourse, course.ID, userID, email, amountCents)
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That promo code isn't valid."})
		return
	}
	amountCents -= discountCents
	if amountCents < 0 {
		amountCents = 0
	}

	token, err := generateAccessToken()
	if err != nil {
		a.serverError(w, "generate access token", err)
		return
	}

	purchase := models.Purchase{
		CourseID:      course.ID,
		UserID:        userID,
		Email:         email,
		AmountCents:   amountCents,
		Currency:      currency,
		AccessToken:   token,
		DiscountCents: discountCents,
	}
	if promo != nil {
		purchase.PromoCodeID = &promo.ID
		purchase.PromoCode = promo.Code
	}

	// A $0 course (the default — every course is "contribute as you like")
	// never touches Razorpay at all: it's recorded straight as paid, and
	// the visitor gets instant access. Razorpay is only involved once a
	// visitor actually chooses to contribute something.
	if amountCents == 0 {
		purchase.RazorpayOrderID = "free-" + token
		if _, err := a.Purchases.Create(r.Context(), purchase); err != nil {
			a.serverError(w, "record free enrollment", err)
			return
		}
		if err := a.Purchases.MarkPaid(r.Context(), purchase.RazorpayOrderID, ""); err != nil {
			a.serverError(w, "mark free enrollment paid", err)
			return
		}
		if promo != nil {
			if err := a.PromoCodes.IncrementRedemption(r.Context(), promo.ID); err != nil {
				log.Printf("increment promo redemption: %v", err)
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name:     accessCookieName(course.ID),
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   a.Cfg.IsProduction(),
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().AddDate(2, 0, 0),
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"free":     true,
			"redirect": "/courses/" + slug + "/learn",
		})
		return
	}

	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Contributions aren't enabled yet — leave the amount at 0 for free access."})
		return
	}

	receipt := fmt.Sprintf("course-%d-%d", course.ID, time.Now().UnixNano())
	orderID, err := a.Razorpay.CreateOrder(amountCents, currency, receipt)
	if err != nil {
		log.Printf("razorpay create order: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start checkout. Please try again."})
		return
	}
	purchase.RazorpayOrderID = orderID
	if _, err := a.Purchases.Create(r.Context(), purchase); err != nil {
		a.serverError(w, "record purchase", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id":    orderID,
		"amount":      amountCents,
		"currency":    currency,
		"key_id":      a.Razorpay.KeyID,
		"course_name": course.Title,
		"email":       email,
	})
}

// CheckoutVerify is called by the client after Razorpay's Checkout.js
// reports success. It verifies the HMAC signature server-side — this is
// the only step that actually proves payment happened, so access is
// granted here, never from the client-side "success" callback alone.
func (a *App) CheckoutVerify(w http.ResponseWriter, r *http.Request) {
	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments are not configured yet."})
		return
	}

	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	if err != nil || !course.IsPaid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Course not found."})
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

	ctx := r.Context()
	purchase, err := a.Purchases.GetByRazorpayOrderID(ctx, body.OrderID)
	if err != nil || purchase.CourseID != course.ID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Order not found."})
		return
	}

	if err := a.Purchases.MarkPaid(ctx, body.OrderID, body.PaymentID); err != nil && err != repo.ErrNotFound {
		a.serverError(w, "mark purchase paid", err)
		return
	}
	if purchase.PromoCodeID != nil {
		if err := a.PromoCodes.IncrementRedemption(ctx, *purchase.PromoCodeID); err != nil {
			log.Printf("increment promo redemption: %v", err)
		}
	}

	promoLine := ""
	if purchase.PromoCode != "" {
		promoLine = fmt.Sprintf("\nPromo: %s (-%s)", purchase.PromoCode, models.FormatMoney(purchase.DiscountCents, purchase.Currency))
	}
	a.Telegram.Send(fmt.Sprintf("💰 Course purchase\nCourse: %s\nEmail: %s\nAmount: %s%s",
		course.Title, purchase.Email, models.FormatMoney(purchase.AmountCents, purchase.Currency), promoLine))

	http.SetCookie(w, &http.Cookie{
		Name:     accessCookieName(course.ID),
		Value:    purchase.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.Cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().AddDate(2, 0, 0),
	})

	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/courses/" + slug + "/learn"})
}

// courseAccess reports whether the current request may view a paid
// course's gated content: a logged-in admin, or a valid paid-purchase
// access token (from the cookie set after checkout, or a magic-link
// ?token= query param — set as a cookie immediately so it isn't needed
// again). watermark identifies who's viewing, for the copy-deterrent
// watermark on the served content.
func (a *App) courseAccess(w http.ResponseWriter, r *http.Request, course models.Course) (ok bool, watermark string) {
	if a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r) {
		return true, "Admin preview"
	}

	token := r.URL.Query().Get("token")
	fromQuery := token != ""
	if token == "" {
		if c, err := r.Cookie(accessCookieName(course.ID)); err == nil {
			token = c.Value
		}
	}
	if token == "" {
		return false, ""
	}

	purchase, err := a.Purchases.GetPaidByAccessToken(r.Context(), token)
	if err != nil || purchase.CourseID != course.ID {
		return false, ""
	}

	if fromQuery {
		http.SetCookie(w, &http.Cookie{
			Name:     accessCookieName(course.ID),
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   a.Cfg.IsProduction(),
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().AddDate(2, 0, 0),
		})
	}
	return true, purchase.Email
}
