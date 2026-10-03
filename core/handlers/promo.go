package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// errPromoInvalid covers every reason a submitted code can't be used right
// now (not found, expired, exhausted, inactive, or not scoped to this
// product/course) — deliberately not distinguished for the buyer, so a
// scoping check can't be used to enumerate valid codes.
var errPromoInvalid = errors.New("promo code is invalid or expired")

// resolvePromoCode looks up a promo code by its submitted text and, if
// it's usable, applies to the product/course being checked out, allowed
// for this account's email (user-scoped private codes), and under this
// account's own per-user redemption limit, returns the discount in cents
// (never more than amountCents) plus the code's own record so the caller
// can label the purchase/booking and bump its redemption count once
// payment is verified. An empty raw code is not an error — it just means
// no discount applies. courseID is ignored for the consultancy product.
// userID/email identify the account attempting to redeem — checkout
// requires a signed-in account unconditionally, so these are never zero.
func (a *App) resolvePromoCode(ctx context.Context, raw, product string, courseID, userID int64, email string, amountCents int) (discountCents int, promo *models.PromoCode, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil, nil
	}
	p, gerr := a.PromoCodes.GetByCode(ctx, raw)
	if gerr != nil {
		if gerr == repo.ErrNotFound {
			return 0, nil, errPromoInvalid
		}
		return 0, nil, gerr
	}
	if !p.Usable() || p.Product != product {
		return 0, nil, errPromoInvalid
	}
	if product == models.PromoProductCourse && len(p.CourseIDs) > 0 && !p.HasCourse(courseID) {
		return 0, nil, errPromoInvalid
	}
	if !p.AllowsEmail(strings.ToLower(strings.TrimSpace(email))) {
		return 0, nil, errPromoInvalid
	}
	if p.MaxRedemptionsPerUser > 0 {
		count, cerr := a.Purchases.CountUserRedemptions(ctx, userID, p.ID)
		if cerr != nil {
			return 0, nil, cerr
		}
		if count >= p.MaxRedemptionsPerUser {
			return 0, nil, errPromoInvalid
		}
	}
	discount := amountCents * p.DiscountPercent / 100
	if discount > amountCents {
		discount = amountCents
	}
	return discount, &p, nil
}
