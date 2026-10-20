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

// TestPromoSoftwareScope checks that a software promo code with a scope
// only resolves for the listed software items, an unscoped one resolves
// for any, and the scope round-trips through the repo. Skips without
// DATABASE_URL.
func TestPromoSoftwareScope(t *testing.T) {
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

	tag := fmt.Sprintf("W3S%d", time.Now().UnixNano())
	var swA, swB int64
	for i, dst := range []*int64{&swA, &swB} {
		if err := db.QueryRow(`INSERT INTO software (slug, title) VALUES ($1, $2) RETURNING id`,
			fmt.Sprintf("%s-%d", tag, i), "Scope test").Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM promo_codes WHERE code LIKE $1`, tag+"%")
		db.Exec(`DELETE FROM software WHERE slug LIKE $1`, tag+"%")
	})

	mk := func(suffix string, ids []int64) string {
		p := models.PromoCode{
			Code: tag + suffix, Product: models.PromoProductSoftware, DiscountPercent: 50, IsActive: true,
			StartsAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), SoftwareIDs: ids,
		}
		if _, err := a.PromoCodes.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p.Code
	}
	scoped := mk("ONE", []int64{swA})
	anyItem := mk("ANY", nil)

	if _, p, err := a.resolvePromoCode(ctx, scoped, models.PromoProductSoftware, swA, 0, "g@example.com", 10000); err != nil || p == nil || !p.HasSoftware(swA) {
		t.Fatalf("scoped code for its own item: p=%v err=%v", p, err)
	}
	if _, _, err := a.resolvePromoCode(ctx, scoped, models.PromoProductSoftware, swB, 0, "g@example.com", 10000); err != errPromoInvalid {
		t.Fatalf("scoped code for another item: err=%v", err)
	}
	for _, id := range []int64{swA, swB} {
		if d, _, err := a.resolvePromoCode(ctx, anyItem, models.PromoProductSoftware, id, 0, "g@example.com", 10000); err != nil || d != 5000 {
			t.Fatalf("unscoped code for %d: d=%d err=%v", id, d, err)
		}
	}

	// Editing the scope replaces it; switching product drops stale scope.
	got, err := a.PromoCodes.GetByCode(ctx, scoped)
	if err != nil {
		t.Fatal(err)
	}
	got.SoftwareIDs = []int64{swB}
	if err := a.PromoCodes.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	if got, _ = a.PromoCodes.GetByCode(ctx, scoped); len(got.SoftwareIDs) != 1 || got.SoftwareIDs[0] != swB {
		t.Fatalf("scope after update: %v", got.SoftwareIDs)
	}
}
