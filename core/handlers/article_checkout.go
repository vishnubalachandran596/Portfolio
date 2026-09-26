package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/geo"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// ArticleContributeOrder starts a Razorpay order for an optional article
// contribution. The amount is floored at one whole currency unit; $0 is
// rejected because this box does not grant access — skipping it is how
// a visitor pays nothing. HMAC verification happens in
// ArticleContributeVerify, never from the client success callback.
func (a *App) ArticleContributeOrder(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	article, err := a.Articles.GetBySlug(r.Context(), slug)
	if err != nil || !article.IsPublished {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Article not found."})
		return
	}

	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form submission."})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	var userID int64
	if uid, ok := a.Auth.UserID(r); ok {
		userID = uid
		if u, err := a.Users.GetByID(r.Context(), uid); err == nil {
			email = u.Email
		}
	}
	if email == "" || !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "A valid email is required."})
		return
	}
	if userID == 0 {
		id, err := a.guestUserForEmail(r, email, "")
		if err != nil {
			a.serverError(w, "provision guest contributor", err)
			return
		}
		userID = id
	}

	currency := models.ContributionCurrency(geo.CountryFromRequest(r), a.Cfg.CheckoutCurrency)
	amountCents, _ := strconv.Atoi(r.FormValue("amount_cents"))
	if amountCents < models.ArticleContributeMinCents {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Enter an amount of at least " + models.FormatMoney(models.ArticleContributeMinCents, currency) + "."})
		return
	}

	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Contributions aren't enabled yet."})
		return
	}

	receipt := fmt.Sprintf("article-%d-%d", article.ID, time.Now().UnixNano())
	orderID, err := a.Razorpay.CreateOrder(amountCents, currency, receipt)
	if err != nil {
		log.Printf("razorpay create article order: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start checkout. Please try again."})
		return
	}

	if _, err := a.ArticleContributions.Create(r.Context(), models.ArticleContribution{
		ArticleID:       article.ID,
		UserID:          userID,
		Email:           email,
		AmountCents:     amountCents,
		Currency:        currency,
		RazorpayOrderID: orderID,
	}); err != nil {
		a.serverError(w, "record article contribution", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id":      orderID,
		"amount":        amountCents,
		"currency":      currency,
		"key_id":        a.Razorpay.KeyID,
		"article_title": article.Title,
		"email":         email,
	})
}

// ArticleContributeVerify records a paid contribution after HMAC
// verification. The same user may contribute again — each verify writes
// its own paid_at.
func (a *App) ArticleContributeVerify(w http.ResponseWriter, r *http.Request) {
	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments are not configured yet."})
		return
	}

	slug := r.PathValue("slug")
	article, err := a.Articles.GetBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Article not found."})
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
	row, err := a.ArticleContributions.GetByRazorpayOrderID(ctx, body.OrderID)
	if err != nil || row.ArticleID != article.ID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Order not found."})
		return
	}

	if row.Status != models.ContributionStatusPaid {
		if err := a.ArticleContributions.MarkPaid(ctx, body.OrderID, body.PaymentID); err != nil && err != repo.ErrNotFound {
			a.serverError(w, "mark article contribution paid", err)
			return
		}
	}

	a.Telegram.Send(fmt.Sprintf("☕ Article contribution\nArticle: %s\nEmail: %s\nAmount: %s",
		article.Title, row.Email, models.FormatMoney(row.AmountCents, row.Currency)))

	writeJSON(w, http.StatusOK, map[string]string{"ok": "1", "redirect": "/articles/" + slug + "?contributed=1"})
}

func (a *App) guestUserForEmail(r *http.Request, email, name string) (int64, error) {
	randomPassword, err := authutil.RandomHex(32)
	if err != nil {
		return 0, err
	}
	hash, err := authutil.HashPassword(randomPassword)
	if err != nil {
		return 0, err
	}
	return a.Users.GetOrCreateGuest(r.Context(), strings.ToLower(email), hash, name)
}
