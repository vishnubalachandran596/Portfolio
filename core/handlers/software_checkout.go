package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

func softwareAccessCookieName(softwareID int64) string {
	return "sa_" + strconv.FormatInt(softwareID, 10)
}

// SoftwareCheckoutOrder creates a Razorpay order for a fixed-price
// software purchase. Unlike CheckoutCreateOrder (courses, pay-what-you-
// want), the amount is never taken from the client — it's always the
// software's own PriceCents, less any valid promo discount. Mirrors
// CheckoutCreateOrder's structure otherwise, including the login
// requirement and the $0-after-discount free path.
func (a *App) SoftwareCheckoutOrder(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	sw, err := a.Software.GetBySlug(r.Context(), slug)
	if err != nil || !sw.IsPublished {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Software not found."})
		return
	}

	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form submission."})
		return
	}
	uid, ok := a.Auth.UserID(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Please log in with Google to continue.", "login_required": "1"})
		return
	}
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Please log in with Google to continue.", "login_required": "1"})
		return
	}
	email := user.Email
	currency := models.CurrencyINR
	amountCents := sw.PriceCents

	discountCents, promo, perr := a.resolvePromoCode(r.Context(), r.FormValue("promo_code"), models.PromoProductSoftware, 0, uid, email, amountCents)
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

	purchase := models.SoftwarePurchase{
		SoftwareID:    sw.ID,
		UserID:        uid,
		Email:         email,
		AmountCents:   amountCents,
		Currency:      currency,
		AccessToken:   token,
		DiscountCents: discountCents,
	}
	if promo != nil {
		purchase.PromoCodeID = &promo.ID
		purchase.PromoCode = promo.Code
		purchase.PromoText = promo.BannerText
		if purchase.PromoText == "" {
			purchase.PromoText = promo.Description
		}
	}

	// A 100%-off promo can bring a fixed-price item to zero — handled the
	// same way a $0 course enrollment is: recorded straight as paid,
	// Razorpay never involved.
	if amountCents == 0 {
		purchase.RazorpayOrderID = "free-" + token
		if _, err := a.SoftwarePurchases.Create(r.Context(), purchase); err != nil {
			a.serverError(w, "record free software purchase", err)
			return
		}
		if err := a.SoftwarePurchases.MarkPaid(r.Context(), purchase.RazorpayOrderID, ""); err != nil {
			a.serverError(w, "mark free software purchase paid", err)
			return
		}
		if promo != nil {
			if err := a.PromoCodes.IncrementRedemption(r.Context(), promo.ID); err != nil {
				log.Printf("increment promo redemption: %v", err)
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name:     softwareAccessCookieName(sw.ID),
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   a.Cfg.IsProduction(),
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().AddDate(2, 0, 0),
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"free":     true,
			"redirect": "/software/" + slug,
		})
		return
	}

	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments aren't enabled yet."})
		return
	}

	receipt := fmt.Sprintf("software-%d-%d", sw.ID, time.Now().UnixNano())
	orderID, err := a.Razorpay.CreateOrder(amountCents, currency, receipt)
	if err != nil {
		log.Printf("razorpay create order: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start checkout. Please try again."})
		return
	}
	purchase.RazorpayOrderID = orderID
	if _, err := a.SoftwarePurchases.Create(r.Context(), purchase); err != nil {
		a.serverError(w, "record software purchase", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id":      orderID,
		"amount":        amountCents,
		"currency":      currency,
		"key_id":        a.Razorpay.KeyID,
		"software_name": sw.Title,
		"email":         email,
	})
}

// SoftwareCheckoutVerify mirrors CheckoutVerify: the Razorpay signature is
// the only step that actually proves payment, so access (here: the
// download link + license/post-purchase instructions) is granted here,
// never from the client-side "success" callback alone.
func (a *App) SoftwareCheckoutVerify(w http.ResponseWriter, r *http.Request) {
	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments are not configured yet."})
		return
	}

	slug := r.PathValue("slug")
	sw, err := a.Software.GetBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Software not found."})
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
	purchase, err := a.SoftwarePurchases.GetByRazorpayOrderID(ctx, body.OrderID)
	if err != nil || purchase.SoftwareID != sw.ID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Order not found."})
		return
	}

	if err := a.SoftwarePurchases.MarkPaid(ctx, body.OrderID, body.PaymentID); err != nil && err != repo.ErrNotFound {
		a.serverError(w, "mark software purchase paid", err)
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
	a.Telegram.Send(fmt.Sprintf("💾 Software purchase\nSoftware: %s %s\nEmail: %s\nAmount: %s%s",
		sw.Title, sw.Version, purchase.Email, models.FormatMoney(purchase.AmountCents, purchase.Currency), promoLine))

	http.SetCookie(w, &http.Cookie{
		Name:     softwareAccessCookieName(sw.ID),
		Value:    purchase.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.Cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().AddDate(2, 0, 0),
	})

	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/software/" + slug})
}

// softwareAccess reports whether the current request may see a paid
// software item's download link: a logged-in admin, or a valid
// paid-purchase access token (cookie, or a ?token= query param set as a
// cookie immediately). Mirrors courseAccess.
func (a *App) softwareAccess(w http.ResponseWriter, r *http.Request, sw models.Software) bool {
	if a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r) {
		return true
	}

	token := r.URL.Query().Get("token")
	fromQuery := token != ""
	if token == "" {
		if c, err := r.Cookie(softwareAccessCookieName(sw.ID)); err == nil {
			token = c.Value
		}
	}
	if token == "" {
		return false
	}

	purchase, err := a.SoftwarePurchases.GetPaidByAccessToken(r.Context(), token)
	if err != nil || purchase.SoftwareID != sw.ID {
		return false
	}

	if fromQuery {
		http.SetCookie(w, &http.Cookie{
			Name:     softwareAccessCookieName(sw.ID),
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   a.Cfg.IsProduction(),
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().AddDate(2, 0, 0),
		})
	}
	return true
}
