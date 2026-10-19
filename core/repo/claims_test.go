package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// testDB opens DATABASE_URL (the local dev Postgres with every migration
// applied) or skips. These tests insert uniquely named rows and remove them.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Skip("cannot open database: ", err)
	}
	if err := db.Ping(); err != nil {
		t.Skip("database unreachable: ", err)
	}
	var has bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='purchases' AND column_name='claim_code')`).Scan(&has); err != nil || !has {
		t.Skip("migration 087 not applied")
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestClaimGuestPurchases(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := fmt.Sprintf("w2atest%d", time.Now().UnixNano())
	var courseID, swID, userA, userB, pCourse, pSoft, lic int64

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM licences WHERE software_id = $1`, swID)
		db.Exec(`DELETE FROM purchases WHERE course_id = $1`, courseID)
		db.Exec(`DELETE FROM software_purchases WHERE software_id = $1`, swID)
		db.Exec(`DELETE FROM courses WHERE id = $1`, courseID)
		db.Exec(`DELETE FROM software WHERE id = $1`, swID)
		db.Exec(`DELETE FROM users WHERE email LIKE $1`, tag+"%")
	})
	must(db.QueryRow(`INSERT INTO courses (slug, title) VALUES ($1,$1) RETURNING id`, tag).Scan(&courseID))
	must(db.QueryRow(`INSERT INTO software (slug, title) VALUES ($1,$1) RETURNING id`, tag).Scan(&swID))
	must(db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ($1,'x') RETURNING id`, tag+"a@example.com").Scan(&userA))
	must(db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ($1,'x') RETURNING id`, tag+"b@example.com").Scan(&userB))

	guestEmail := tag + "guest@example.com"
	code1, code2, code3 := "AAAAAAAAAAAAAAAAAAAA", "BBBBBBBBBBBBBBBBBBBB", "CCCCCCCCCCCCCCCCCCCC"
	must(db.QueryRow(`INSERT INTO purchases (course_id,email,amount_cents,currency,status,razorpay_order_id,access_token,claim_code,paid_at)
		VALUES ($1,$2,100,'INR','paid',$3,$3,$4,now()) RETURNING id`, courseID, guestEmail, tag+"o1", code1).Scan(&pCourse))
	must(db.QueryRow(`INSERT INTO software_purchases (software_id,email,amount_cents,currency,status,razorpay_order_id,access_token,claim_code,paid_at)
		VALUES ($1,$2,100,'INR','paid',$3,$3,$4,now()) RETURNING id`, swID, guestEmail, tag+"o2", code2).Scan(&pSoft))
	must(db.QueryRow(`INSERT INTO licences (lic_code, software_id, purchase_id) VALUES ($1,$2,$3) RETURNING id`, tag, swID, pSoft).Scan(&lic))
	// An unpaid guest order must never be claimable.
	var pUnpaid int64
	must(db.QueryRow(`INSERT INTO purchases (course_id,email,amount_cents,currency,status,razorpay_order_id,access_token,claim_code)
		VALUES ($1,$2,100,'INR','created',$3,$3,$4) RETURNING id`, courseID, guestEmail, tag+"o3", code3).Scan(&pUnpaid))

	r := NewClaimRepo(db)

	// Unknown code and unpaid order: not found.
	if _, err := r.ClaimByCode(ctx, userA, "DDDDDDDDDDDDDDDDDDDD"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown code: %v", err)
	}
	if _, err := r.ClaimByCode(ctx, userA, code3); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unpaid order must not be claimable: %v", err)
	}

	// A course claim attaches it and wipes the code (single use).
	it, err := r.ClaimByCode(ctx, userA, code1)
	if err != nil || it.Kind != "course" || it.Slug != tag {
		t.Fatalf("claim course: %+v %v", it, err)
	}
	var owner sql.NullInt64
	var left string
	must(db.QueryRow(`SELECT user_id, claim_code FROM purchases WHERE id=$1`, pCourse).Scan(&owner, &left))
	if owner.Int64 != userA || left != "" {
		t.Fatalf("course not attached/wiped: owner=%v code=%q", owner, left)
	}
	if _, err := r.ClaimByCode(ctx, userB, code1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a code must work only once: %v", err)
	}

	// A software claim attaches the purchase AND its licence.
	it, err = r.ClaimByCode(ctx, userB, code2)
	if err != nil || it.Kind != "software" {
		t.Fatalf("claim software: %+v %v", it, err)
	}
	must(db.QueryRow(`SELECT user_id FROM licences WHERE id=$1`, lic).Scan(&owner))
	if owner.Int64 != userB {
		t.Fatalf("licence not attached to claimant: %v", owner)
	}

	// Google-verified email attach: only unclaimed, paid purchases of that
	// email, case-insensitively; nothing else is touched.
	var pLater int64
	must(db.QueryRow(`INSERT INTO purchases (course_id,email,amount_cents,currency,status,razorpay_order_id,access_token,claim_code,paid_at)
		VALUES ($1,$2,100,'INR','paid',$3,$3,$4,now()) RETURNING id`, courseID, guestEmail, tag+"o4", "EEEEEEEEEEEEEEEEEEEE").Scan(&pLater))
	items, err := r.AttachByEmail(ctx, userA, " "+upper(guestEmail)+" ")
	if err != nil || len(items) != 1 {
		t.Fatalf("attach by email: %+v %v", items, err)
	}
	must(db.QueryRow(`SELECT user_id FROM purchases WHERE id=$1`, pLater).Scan(&owner))
	if owner.Int64 != userA {
		t.Fatalf("email attach missed the purchase: %v", owner)
	}
	must(db.QueryRow(`SELECT user_id FROM purchases WHERE id=$1`, pUnpaid).Scan(&owner))
	if owner.Valid {
		t.Fatal("email attach must not touch unpaid orders")
	}
	if again, _ := r.AttachByEmail(ctx, userB, guestEmail); len(again) != 0 {
		t.Fatalf("already-claimed purchases must not move: %+v", again)
	}
	if none, _ := r.AttachByEmail(ctx, userB, ""); len(none) != 0 {
		t.Fatal("empty email must attach nothing")
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
