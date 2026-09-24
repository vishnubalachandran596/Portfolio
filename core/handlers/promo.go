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
// it's usable and applies to the product being checked out, returns the
// discount in cents (never more than amountCents) plus the code's own
// record so the caller can label the purchase/booking and bump its
// redemption count once payment is verified. An empty raw code is not an
// error — it just means no discount applies. courseID is ignored for the
// consultancy product.
func (a *App) resolvePromoCode(ctx context.Context, raw, product string, courseID int64, amountCents int) (discountCents int, promo *models.PromoCode, err error) {
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
	if product == models.PromoProductCourse && !p.AppliesToAllCourses && !p.HasCourse(courseID) {
		return 0, nil, errPromoInvalid
	}
	discount := amountCents * p.DiscountPercent / 100
	if discount > amountCents {
		discount = amountCents
	}
	return discount, &p, nil
}
