package repo

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

// TestDeleteAccount checks the erasure rules against the dev Postgres: access
// revoked, personal rows deleted, payment rows kept but anonymous, and a
// later sign-up with the same email cannot reclaim anything.
func TestDeleteAccount(t *testing.T) {
	db := testClaimsDB(t)
	ctx := context.Background()
	tag := fmt.Sprintf("deltest%d", time.Now().UnixNano())
	var courseID, swID, uid, other, pCourse, pGuest, pSoft, lic, pPay int64
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
		db.Exec(`DELETE FROM quiz_attempts WHERE course_id = $1`, courseID)
		db.Exec(`DELETE FROM courses WHERE id = $1`, courseID)
		db.Exec(`DELETE FROM software WHERE id = $1`, swID)
		db.Exec(`DELETE FROM bookings WHERE email LIKE $1 OR name = 'Deleted user' AND message = ''`, tag+"%")
		db.Exec(`DELETE FROM users WHERE email LIKE $1`, tag+"%")
	})
	email := tag + "@example.com"
	must(db.QueryRow(`INSERT INTO courses (slug, title) VALUES ($1,$1) RETURNING id`, tag).Scan(&courseID))
	must(db.QueryRow(`INSERT INTO software (slug, title) VALUES ($1,$1) RETURNING id`, tag).Scan(&swID))
	must(db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ($1,'x') RETURNING id`, email).Scan(&uid))
	must(db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ($1,'x') RETURNING id`, tag+"o@example.com").Scan(&other))
	must(db.QueryRow(`INSERT INTO purchases (course_id,user_id,email,amount_cents,currency,status,razorpay_order_id,access_token,paid_at)
		VALUES ($1,$2,$3,100,'INR','paid',$4,$4,now()) RETURNING id`, courseID, uid, email, tag+"o1").Scan(&pCourse))
	must(db.QueryRow(`INSERT INTO purchases (course_id,email,amount_cents,currency,status,razorpay_order_id,access_token,claim_code,paid_at)
		VALUES ($1,$2,100,'INR','paid',$3,$3,'ZZZZZZZZZZZZZZZZZZZZ',now()) RETURNING id`, courseID, email, tag+"o2").Scan(&pGuest))
	must(db.QueryRow(`INSERT INTO software_purchases (software_id,user_id,email,amount_cents,currency,status,razorpay_order_id,access_token,paid_at)
		VALUES ($1,$2,$3,100,'INR','paid',$4,$4,now()) RETURNING id`, swID, uid, email, tag+"o3").Scan(&pSoft))
	must(db.QueryRow(`INSERT INTO licences (lic_code, software_id, purchase_id, user_id) VALUES ($1,$2,$3,$4) RETURNING id`, tag, swID, pSoft, uid).Scan(&lic))
	must(db.QueryRow(`INSERT INTO quiz_attempts (course_id,user_id,name,email,total_questions,correct_answers,score_percent,passed,certificate_code)
		VALUES ($1,$2,'N',$3,10,10,100,true,$4) RETURNING id`, courseID, uid, email, tag).Scan(new(int64)))
	must(db.QueryRow(`INSERT INTO bookings (user_id,name,email,message,slot_start,slot_end,status)
		VALUES ($1,'Real Name',$2,'secret',now()-interval '30 days',now()-interval '29 days','confirmed') RETURNING id`, uid, email).Scan(&pPay))
	// somebody else's purchase must be untouched
	var pOther int64
	must(db.QueryRow(`INSERT INTO purchases (course_id,user_id,email,amount_cents,currency,status,razorpay_order_id,access_token,paid_at)
		VALUES ($1,$2,$3,100,'INR','paid',$4,$4,now()) RETURNING id`, courseID, other, tag+"o@example.com", tag+"o4").Scan(&pOther))

	r := NewUserRepo(db)
	sum, err := r.DeletionSummary(ctx, uid)
	if err != nil || sum.Courses != 1 || sum.Software != 1 || sum.Certificates != 1 || sum.UpcomingSlots != 0 {
		t.Fatalf("summary: %+v %v", sum, err)
	}
	must(r.DeleteAccount(ctx, uid))

	var n int
	count := func(q string, a ...any) int {
		must(db.QueryRow(q, a...).Scan(&n))
		return n
	}
	if count(`SELECT count(*) FROM users WHERE id=$1`, uid) != 0 {
		t.Fatal("user not deleted")
	}
	if count(`SELECT count(*) FROM licences WHERE id=$1`, lic) != 0 || count(`SELECT count(*) FROM quiz_attempts WHERE course_id=$1`, courseID) != 0 {
		t.Fatal("licence / certificate not deleted")
	}
	for _, c := range []struct {
		table string
		id    int64
	}{{"purchases", pCourse}, {"purchases", pGuest}, {"software_purchases", pSoft}} {
		var e, tok, code string
		var u sql.NullInt64
		q := fmt.Sprintf(`SELECT email, access_token, claim_code, user_id FROM %s WHERE id=$1`, c.table)
		must(db.QueryRow(q, c.id).Scan(&e, &tok, &code, &u))
		if e != "deleted-account" || code != "" || u.Valid || tok == tag+"o1" || tok == tag+"o3" || tok == tag+"o2" {
			t.Fatalf("%s %d not anonymised/revoked: %q %q %q %v", c.table, c.id, e, tok, code, u)
		}
	}
	var bn, be, bm string
	must(db.QueryRow(`SELECT name,email,message FROM bookings WHERE id=$1`, pPay).Scan(&bn, &be, &bm))
	if bn != "Deleted user" || be != "deleted-account" || bm != "" {
		t.Fatalf("booking not anonymised: %q %q %q", bn, be, bm)
	}
	var oe string
	must(db.QueryRow(`SELECT email FROM purchases WHERE id=$1`, pOther).Scan(&oe))
	if oe != tag+"o@example.com" {
		t.Fatal("another user's purchase was modified")
	}
	// Signing up again with the same email must not bring anything back.
	var again int64
	must(db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ($1,'x') RETURNING id`, email).Scan(&again))
	items, err := NewClaimRepo(db).AttachByEmail(ctx, again, email)
	if err != nil || len(items) != 0 {
		t.Fatalf("re-signup reclaimed purchases: %+v %v", items, err)
	}
	if err := r.DeleteAccount(ctx, uid); err == nil {
		t.Fatal("deleting a missing account must error")
	}
}
