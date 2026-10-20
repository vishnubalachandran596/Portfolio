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
// errPromoMessage is what the buyer is told for any rejected code.
const errPromoMessage = "That promo code is not valid: it may be misspelt, expired, used up, already used on this account, or not for this course."

var errPromoInvalid = errors.New("promo code is invalid or expired")

// resolvePromoCode looks up a promo code by its submitted text and, if
// it's usable, applies to the product/course being checked out, allowed
// for this account's email (user-scoped private codes), and under this
// account's own per-user redemption limit, returns the discount in cents
// (never more than amountCents) plus the code's own record so the caller
// can label the purchase/booking and bump its redemption count once
// payment is verified. An empty raw code is not an error — it just means
// no discount applies. itemID is the course ID for the course product and
// the software ID for the software product (an empty CourseIDs/SoftwareIDs
// scope means "any item"); it is ignored for consultancy. userID/email identify who is
// redeeming: userID is 0 for a guest checkout (email only), in which case
// private codes are refused and per-user limits count by email.
func (a *App) resolvePromoCode(ctx context.Context, raw, product string, itemID, userID int64, email string, amountCents int) (discountCents int, promo *models.PromoCode, err error) {
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
	if product == models.PromoProductCourse && len(p.CourseIDs) > 0 && !p.HasCourse(itemID) {
		return 0, nil, errPromoInvalid
	}
	if product == models.PromoProductSoftware && len(p.SoftwareIDs) > 0 && !p.HasSoftware(itemID) {
		return 0, nil, errPromoInvalid
	}
	// A guest (userID 0) only has a typed email, which proves nothing, so a
	// private (email-scoped) code is never honoured for one: the buyer must
	// sign in first.
	if userID == 0 && p.IsPrivate() {
		return 0, nil, errPromoInvalid
	}
	if !p.AllowsEmail(strings.ToLower(strings.TrimSpace(email))) {
		return 0, nil, errPromoInvalid
	}
	if p.MaxRedemptionsPerUser > 0 {
		// Per-user redemption counting is product-aware: software
		// purchases live in their own table (software_purchases), not
		// purchases, so each product line counts against its own history.
		var count int
		var cerr error
		switch {
		case userID == 0 && product == models.PromoProductSoftware:
			count, cerr = a.SoftwarePurchases.CountEmailRedemptions(ctx, email, p.ID)
		case userID == 0:
			count, cerr = a.Purchases.CountEmailRedemptions(ctx, email, p.ID)
		case product == models.PromoProductSoftware:
			count, cerr = a.SoftwarePurchases.CountUserRedemptions(ctx, userID, p.ID)
		default:
			count, cerr = a.Purchases.CountUserRedemptions(ctx, userID, p.ID)
		}
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
