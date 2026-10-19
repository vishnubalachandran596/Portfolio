package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// TestPromoInCourseCheckout exercises the real resolvePromoCode against the
// dev database: server-side discount on the fixed price, 100% codes, and a
// clear rejection for invalid, expired, used-up, per-account-used, private
// and wrong-product codes. It skips without DATABASE_URL.
func TestPromoInCourseCheckout(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", url)
	if err != nil || db.Ping() != nil {
		t.Skip("database unreachable")
	}
	defer db.Close()
	ctx := context.Background()
	a := &App{PromoCodes: repo.NewPromoCodeRepo(db), Purchases: repo.NewPurchaseRepo(db), SoftwarePurchases: repo.NewSoftwarePurchaseRepo(db)}

	tag := fmt.Sprintf("W2A%d", time.Now().UnixNano())
	mk := func(suffix string, p models.PromoCode) string {
		p.Code = tag + suffix
		if p.Product == "" {
			p.Product = models.PromoProductCourse
		}
		p.StartsAt = time.Now().Add(-time.Hour)
		if p.ExpiresAt.IsZero() {
			p.ExpiresAt = time.Now().Add(time.Hour)
		}
		if _, err := a.PromoCodes.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p.Code
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM promo_codes WHERE code LIKE $1`, tag+"%") })

	const base = 149900
	half := mk("HALF", models.PromoCode{DiscountPercent: 50, IsActive: true})
	full := mk("FULL", models.PromoCode{DiscountPercent: 100, IsActive: true})
	expired := mk("OLD", models.PromoCode{DiscountPercent: 50, IsActive: true, ExpiresAt: time.Now().Add(-time.Minute)})
	inactive := mk("OFF", models.PromoCode{DiscountPercent: 50, IsActive: false})
	usedUp := mk("USED", models.PromoCode{DiscountPercent: 50, IsActive: true, MaxRedemptions: 1})
	private := mk("PRIV", models.PromoCode{DiscountPercent: 50, IsActive: true, AllowedEmails: []string{"owner@example.com"}})
	software := mk("SW", models.PromoCode{DiscountPercent: 50, IsActive: true, Product: models.PromoProductSoftware})
	if _, err := db.Exec(`UPDATE promo_codes SET redemption_count = 1 WHERE code = $1`, usedUp); err != nil {
		t.Fatal(err)
	}

	// 50 percent: discount is server-computed and the charged total follows.
	d, p, err := a.resolvePromoCode(ctx, " "+half+" ", models.PromoProductCourse, 1, 0, "guest@example.com", base)
	if err != nil || p == nil || d != base/2 {
		t.Fatalf("half: d=%d p=%v err=%v", d, p, err)
	}
	if got := finalAmountCents(base, d); got != base-base/2 {
		t.Fatalf("charged total %d != discounted price %d", got, base-base/2)
	}
	// 100 percent: total is exactly zero (the free path, no payment order).
	d, _, err = a.resolvePromoCode(ctx, full, models.PromoProductCourse, 1, 0, "guest@example.com", base)
	if err != nil || finalAmountCents(base, d) != 0 {
		t.Fatalf("full: d=%d err=%v", d, err)
	}
	// No code: no discount, no error.
	if d, p, err = a.resolvePromoCode(ctx, "", models.PromoProductCourse, 1, 0, "g@example.com", base); err != nil || d != 0 || p != nil {
		t.Fatalf("empty code: %d %v %v", d, p, err)
	}
	// Every rejected code gives the same error, which the handlers turn into
	// errPromoMessage.
	for name, code := range map[string]string{"unknown": tag + "NOPE", "expired": expired, "inactive": inactive,
		"used up": usedUp, "private for a guest": private, "wrong product": software} {
		if _, _, err := a.resolvePromoCode(ctx, code, models.PromoProductCourse, 1, 0, "owner@example.com", base); err != errPromoInvalid {
			t.Errorf("%s: want errPromoInvalid, got %v", name, err)
		}
	}
	if errPromoMessage == "" {
		t.Error("rejection message must not be empty")
	}
}
