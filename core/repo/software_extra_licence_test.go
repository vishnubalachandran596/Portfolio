package repo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// An account can hold several paid purchases of one software item; each
// yields its own licence, and CountEarlierPaid tells the first from the rest.
func TestExtraLicencePurchases(t *testing.T) {
	db := testClaimsDB(t)
	ctx := context.Background()
	tag := fmt.Sprintf("xlictest%d", time.Now().UnixNano())
	var swID, uid, p1, p2, p3 int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM licences WHERE software_id = $1`, swID)
		db.Exec(`DELETE FROM software_purchases WHERE software_id = $1`, swID)
		db.Exec(`DELETE FROM software WHERE id = $1`, swID)
		db.Exec(`DELETE FROM users WHERE email LIKE $1`, tag+"%")
	})
	must(db.QueryRow(`INSERT INTO software (slug, title, licence_product) VALUES ($1,$1,'p') RETURNING id`, tag).Scan(&swID))
	must(db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ($1,'x') RETURNING id`, tag+"@example.com").Scan(&uid))
	ins := func(order, status string) int64 {
		var id int64
		must(db.QueryRow(`INSERT INTO software_purchases (software_id,user_id,email,amount_cents,currency,status,razorpay_order_id,access_token,paid_at)
			VALUES ($1,$2,$3,100,'INR',$4,$5,$5,now()) RETURNING id`, swID, uid, tag+"@example.com", status, tag+order).Scan(&id))
		return id
	}
	p1, p2 = ins("a", "paid"), ins("b", "paid")
	p3 = ins("c", "created") // unpaid orders never count

	pr, lr := NewSoftwarePurchaseRepo(db), NewLicenceRepo(db)
	for _, id := range []int64{p1, p2} {
		must(lr.EnsureForPurchase(ctx, swID, id, uid))
		must(lr.EnsureForPurchase(ctx, swID, id, uid)) // retry is a no-op
	}
	list, err := lr.ListByUser(ctx, uid)
	must(err)
	if len(list) != 2 {
		t.Fatalf("want 2 licences for 2 purchases, got %d", len(list))
	}
	if n, _ := pr.CountEarlierPaid(ctx, uid, swID, p1); n != 0 {
		t.Fatalf("first purchase is not extra, earlier=%d", n)
	}
	if n, _ := pr.CountEarlierPaid(ctx, uid, swID, p2); n != 1 {
		t.Fatalf("second purchase is extra, earlier=%d", n)
	}
	if n, _ := pr.CountEarlierPaid(ctx, uid, swID, p3+1); n != 2 {
		t.Fatalf("unpaid order must not count, earlier=%d", n)
	}
	l, err := lr.GetByPurchase(ctx, p2)
	must(err)
	if l.PurchaseID != p2 {
		t.Fatalf("GetByPurchase returned purchase %d", l.PurchaseID)
	}
	if _, err := lr.GetByPurchase(ctx, p3); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no licence for unpaid purchase: %v", err)
	}
}
