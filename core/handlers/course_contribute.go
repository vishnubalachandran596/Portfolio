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

const (
	// A voluntary contribution is at least Rs 1 and at most Rs 1,00,000.
	courseContributeMinCents = 100
	courseContributeMaxCents = 10000000
)

// validContribution reports whether a learner-chosen amount is in range.
// This is the one place a client-supplied amount is accepted, and only for
// the optional contribution, never for the course price.
func validContribution(cents int) bool {
	return cents >= courseContributeMinCents && cents <= courseContributeMaxCents
}

// contributorEmail resolves who is contributing: the signed-in account, else
// the email on the purchase whose access cookie the caller holds. ok=false
// when the caller has no real access (admin preview included).
func (a *App) contributorEmail(w http.ResponseWriter, r *http.Request, course models.Course) (userID int64, email string, ok bool) {
	hasAccess, who, _ := a.courseAccess(w, r, course)
	if !hasAccess || who == "Admin preview" {
		return 0, "", false
	}
	if uid, has := a.Auth.UserID(r); has {
		if u, err := a.Users.GetByID(r.Context(), uid); err == nil {
			return uid, u.Email, true
		}
	}
	return 0, who, who != ""
}

// CourseContributeOrder starts a Razorpay order for the optional
// contribution shown after a course is finished. The course price is fixed
// and unrelated; this amount is the learner's own choice, range-checked
// here. Like every create/verify pair, payment is only recorded as paid by
// CourseContributeVerify after the HMAC signature checks out.
func (a *App) CourseContributeOrder(w http.ResponseWriter, r *http.Request) {
	course, err := a.Courses.GetBySlug(r.Context(), r.PathValue("slug"))
	if err != nil || !course.IsPaid || !course.IsPublished {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Course not found."})
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form submission."})
		return
	}
	userID, email, ok := a.contributorEmail(w, r, course)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Only learners enrolled in this course can contribute."})
		return
	}
	amountCents, _ := strconv.Atoi(r.FormValue("amount_cents"))
	if !validContribution(amountCents) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Enter an amount between " +
			models.FormatMoney(courseContributeMinCents, models.CurrencyINR) + " and " +
			models.FormatMoney(courseContributeMaxCents, models.CurrencyINR) + "."})
		return
	}
	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Contributions aren't enabled yet."})
		return
	}
	orderID, err := a.Razorpay.CreateOrder(amountCents, models.CurrencyINR, fmt.Sprintf("contrib-%d-%d", course.ID, time.Now().UnixNano()))
	if err != nil {
		log.Printf("razorpay create contribution order: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not start checkout. Please try again."})
		return
	}
	if _, err := a.CourseContributions.Create(r.Context(), models.CourseContribution{
		CourseID: course.ID, UserID: userID, Email: email, AmountCents: amountCents,
		Currency: models.CurrencyINR, RazorpayOrderID: orderID,
	}); err != nil {
		a.serverError(w, "record course contribution", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"order_id": orderID, "amount": amountCents, "currency": models.CurrencyINR,
		"key_id": a.Razorpay.KeyID, "course_name": course.Title, "email": email,
	})
}

// CourseContributeVerify marks a contribution paid after HMAC verification.
func (a *App) CourseContributeVerify(w http.ResponseWriter, r *http.Request) {
	if !a.Razorpay.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payments are not configured yet."})
		return
	}
	course, err := a.Courses.GetBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
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
	row, err := a.CourseContributions.GetByRazorpayOrderID(ctx, body.OrderID)
	if err != nil || row.CourseID != course.ID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Order not found."})
		return
	}
	if row.Status != "paid" {
		if err := a.CourseContributions.MarkPaid(ctx, body.OrderID, body.PaymentID); err != nil && err != repo.ErrNotFound {
			a.serverError(w, "mark course contribution paid", err)
			return
		}
		a.Telegram.Send(fmt.Sprintf("Course contribution\nCourse: %s\nEmail: %s\nAmount: %s",
			course.Title, row.Email, models.FormatMoney(row.AmountCents, row.Currency)))
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1", "message": "Thank you for your contribution."})
}

// CheckoutQuote previews the order summary for a course: fixed price, the
// promo discount (resolved by the same resolvePromoCode the order handler
// uses, so the preview can never disagree with what is charged) and the
// total. It creates nothing and is not rate-limited like an order start.
func (a *App) CheckoutQuote(w http.ResponseWriter, r *http.Request) {
	course, err := a.Courses.GetBySlug(r.Context(), r.PathValue("slug"))
	if err != nil || !course.IsPaid || !course.IsPublished {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Course not found."})
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form submission."})
		return
	}
	var userID int64
	email := cleanEmail(r.FormValue("email"))
	if uid, ok := a.Auth.UserID(r); ok {
		if u, err := a.Users.GetByID(r.Context(), uid); err == nil {
			userID, email = uid, u.Email
		}
	}
	base := course.FixedPriceCents()
	discount, promo, perr := a.resolvePromoCode(r.Context(), r.FormValue("promo_code"), models.PromoProductCourse, course.ID, userID, email, base)
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errPromoMessage})
		return
	}
	total := finalAmountCents(base, discount)
	out := map[string]any{
		"price": models.FormatMoney(base, models.CurrencyINR), "discount_cents": discount,
		"discount":    models.FormatMoney(discount, models.CurrencyINR),
		"total_cents": total, "total": models.FormatMoney(total, models.CurrencyINR),
	}
	if promo != nil {
		out["promo_code"], out["percent"] = promo.Code, promo.DiscountPercent
	}
	writeJSON(w, http.StatusOK, out)
}
