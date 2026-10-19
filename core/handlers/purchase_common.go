package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

const (
	throttleCheckout = "checkout"
	throttleClaim    = "claim"

	// A guest can start this many checkouts an hour per IP before a
	// rising delay applies (each start creates a payment order).
	checkoutMaxPerHour = 30
	claimMaxFails      = 5
)

// checkoutBuyer is who a checkout is for: a signed-in account (UserID set,
// the account's own email) or a guest (UserID 0, a typed email).
type checkoutBuyer struct {
	UserID int64
	Email  string
}

func (b checkoutBuyer) Guest() bool { return b.UserID == 0 }

// newClaimCode returns the claim code to store on the purchase: a fresh
// code for a guest, "" for a signed-in buyer (nothing to claim).
func (b checkoutBuyer) newClaimCode() (string, error) {
	if !b.Guest() {
		return "", nil
	}
	return authutil.NewClaimCode()
}

// resolveBuyer works out who is checking out. A signed-in user keeps their
// account email and any email field is ignored. Otherwise the form's
// "email" is required and validated; guests are rate limited per IP. On
// failure it has already written the JSON error and returns ok=false.
func (a *App) resolveBuyer(w http.ResponseWriter, r *http.Request) (checkoutBuyer, bool) {
	if uid, ok := a.Auth.UserID(r); ok {
		if user, err := a.Users.GetByID(r.Context(), uid); err == nil {
			return checkoutBuyer{UserID: uid, Email: user.Email}, true
		}
	}
	email := cleanEmail(r.FormValue("email"))
	if !validEmail(email) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Please enter a valid email address."})
		return checkoutBuyer{}, false
	}
	ik := authutil.HashKey(authutil.ClientIP(r))
	if _, locked := a.throttled(r, throttleCheckout, ik); locked {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many checkout attempts. Please wait a few minutes and try again."})
		return checkoutBuyer{}, false
	}
	a.recordFailure(r, throttleCheckout, checkoutMaxPerHour, ik)
	return checkoutBuyer{Email: email}, true
}

// finalAmountCents is the amount actually charged: the server-side base
// price less a promo discount, never below zero and never above the base.
func finalAmountCents(baseCents, discountCents int) int {
	if baseCents < 0 {
		baseCents = 0
	}
	if discountCents < 0 {
		discountCents = 0
	}
	if discountCents > baseCents {
		discountCents = baseCents
	}
	return baseCents - discountCents
}

func guestTag(userID int64) string {
	if userID == 0 {
		return " (guest)"
	}
	return ""
}

// setAccessCookie stores a purchase's bearer access token for two years.
func (a *App) setAccessCookie(w http.ResponseWriter, name, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.Cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().AddDate(2, 0, 0),
	})
}

// PurchaseSuccessData drives the post-payment page.
type PurchaseSuccessData struct {
	BaseData
	Kind      string // "course" or "software"
	Title     string
	Receipt   string
	Email     string
	Amount    string
	PromoLine string
	// ClaimCode is the formatted code, set only while the purchase is an
	// unclaimed guest purchase.
	ClaimCode string
	// Attached is true when the purchase already belongs to an account.
	Attached bool
	NextURL  string
	NextText string
	Licence  bool
}

func (a *App) renderPurchaseSuccess(w http.ResponseWriter, r *http.Request, d PurchaseSuccessData) {
	d.BaseData = a.base(r, "Thank you - "+d.Title, "", "")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	a.View.Render(w, http.StatusOK, "purchase_success", d)
}

func promoLineOf(code string, discount int, currency string) string {
	if code == "" {
		return ""
	}
	return fmt.Sprintf("Promo code %s (-%s)", code, models.FormatMoney(discount, currency))
}

// CoursePurchaseSuccess shows the receipt and, for a guest purchase, the
// claim code. Access is the purchase's own access cookie, so the code is
// only ever shown to the browser that paid.
func (a *App) CoursePurchaseSuccess(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	course, err := a.Courses.GetBySlug(r.Context(), slug)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	c, cerr := r.Cookie(accessCookieName(course.ID))
	if cerr != nil {
		http.Redirect(w, r, "/courses/"+slug, http.StatusSeeOther)
		return
	}
	p, err := a.Purchases.GetPaidByAccessToken(r.Context(), c.Value)
	if err != nil || p.CourseID != course.ID {
		http.Redirect(w, r, "/courses/"+slug, http.StatusSeeOther)
		return
	}
	a.renderPurchaseSuccess(w, r, PurchaseSuccessData{
		Kind:      "course",
		Title:     course.Title,
		Receipt:   fmt.Sprintf("C%06d", p.ID),
		Email:     p.Email,
		Amount:    models.FormatMoney(p.AmountCents, p.Currency),
		PromoLine: promoLineOf(p.PromoCode, p.DiscountCents, p.Currency),
		ClaimCode: claimDisplay(p.UserID, p.ClaimCode),
		Attached:  p.UserID != 0,
		NextURL:   "/courses/" + slug + "/learn",
		NextText:  "Open the course",
	})
}

// SoftwarePurchaseSuccess is CoursePurchaseSuccess for software.
func (a *App) SoftwarePurchaseSuccess(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	sw, err := a.Software.GetBySlug(r.Context(), slug)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	c, cerr := r.Cookie(softwareAccessCookieName(sw.ID))
	if cerr != nil {
		http.Redirect(w, r, "/software/"+slug, http.StatusSeeOther)
		return
	}
	p, err := a.SoftwarePurchases.GetPaidByAccessToken(r.Context(), c.Value)
	if err != nil || p.SoftwareID != sw.ID {
		http.Redirect(w, r, "/software/"+slug, http.StatusSeeOther)
		return
	}
	a.renderPurchaseSuccess(w, r, PurchaseSuccessData{
		Kind:      "software",
		Title:     sw.Title,
		Receipt:   fmt.Sprintf("S%06d", p.ID),
		Email:     p.Email,
		Amount:    models.FormatMoney(p.AmountCents, p.Currency),
		PromoLine: promoLineOf(p.PromoCode, p.DiscountCents, p.Currency),
		ClaimCode: claimDisplay(p.UserID, p.ClaimCode),
		Attached:  p.UserID != 0,
		NextURL:   "/software/" + slug,
		NextText:  "Go to the download page",
		Licence:   sw.LicenceProduct != "",
	})
}

func claimDisplay(userID int64, code string) string {
	if userID != 0 || code == "" {
		return ""
	}
	return authutil.FormatClaimCode(code)
}

// AccountClaimData drives /account/claim.
type AccountClaimData struct {
	BaseData
	Error   string
	Claimed []repo.ClaimedItem
}

// AccountClaimPage shows the claim-code form.
func (a *App) AccountClaimPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	a.View.Render(w, http.StatusOK, "account_claim", AccountClaimData{
		BaseData: a.base(r, "Claim a purchase - "+a.Cfg.SiteName, "", "account"),
	})
}

// AccountClaimSubmit attaches the purchase behind a claim code to the
// signed-in account. Throttled in the database (kind 'claim', per account
// and per IP) and generic about failures, so codes cannot be guessed.
func (a *App) AccountClaimSubmit(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.Auth.UserID(r)
	w.Header().Set("Cache-Control", "no-store")
	render := func(status int, d AccountClaimData) {
		d.BaseData = a.base(r, "Claim a purchase - "+a.Cfg.SiteName, "", "account")
		a.View.Render(w, status, "account_claim", d)
	}
	uk, ik := "u:"+fmt.Sprint(uid), authutil.HashKey(authutil.ClientIP(r))
	if until, locked := a.throttled(r, throttleClaim, uk, ik); locked {
		render(http.StatusTooManyRequests, AccountClaimData{Error: lockedMessage(until)})
		return
	}
	if err := r.ParseForm(); err != nil {
		render(http.StatusBadRequest, AccountClaimData{Error: "Invalid form submission."})
		return
	}
	code := authutil.NormalizeClaimCode(r.FormValue("code"))
	if code == "" {
		a.recordFailure(r, throttleClaim, claimMaxFails, uk, ik)
		render(http.StatusBadRequest, AccountClaimData{Error: "That code was not found, or it has already been used."})
		return
	}
	item, err := a.Claims.ClaimByCode(r.Context(), uid, code)
	if errors.Is(err, repo.ErrNotFound) {
		a.recordFailure(r, throttleClaim, claimMaxFails, uk, ik)
		render(http.StatusBadRequest, AccountClaimData{Error: "That code was not found, or it has already been used."})
		return
	}
	if err != nil {
		a.serverError(w, "claim purchase", err)
		return
	}
	a.clearFailures(r, throttleClaim, uk, ik)
	render(http.StatusOK, AccountClaimData{Claimed: []repo.ClaimedItem{item}})
}
