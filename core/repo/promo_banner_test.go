package repo

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestParseScopeItems(t *testing.T) {
	got := parseScopeItems(`[{"t":"Java","u":"/courses/java"}]`, `[{"t":"Temple","u":"/software/temple"}]`)
	if len(got) != 2 || got[0].Title != "Java" || got[1].URL != "/software/temple" {
		t.Fatalf("got %+v", got)
	}
	if len(parseScopeItems(`[]`, `not json`)) != 0 {
		t.Fatal("empty / bad JSON must give no items")
	}
}

// The scope titles ride in the banner query itself: scoped code lists its
// published items, an unscoped code lists none.
func TestListActivePublicScopeItems(t *testing.T) {
	db := testClaimsDB(t)
	ctx := context.Background()
	tag := fmt.Sprintf("bnr%d", time.Now().UnixNano())
	var courseID, hiddenID, swID, scoped, open int64
	t.Cleanup(func() {
		db.Exec(`DELETE FROM promo_codes WHERE code LIKE $1`, tag+"%")
		db.Exec(`DELETE FROM courses WHERE slug LIKE $1`, tag+"%")
		db.Exec(`DELETE FROM software WHERE slug LIKE $1`, tag+"%")
	})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.QueryRow(`INSERT INTO courses (slug, title, is_published) VALUES ($1,'Pub course',TRUE) RETURNING id`, tag+"c").Scan(&courseID))
	must(db.QueryRow(`INSERT INTO courses (slug, title, is_published) VALUES ($1,'Draft course',FALSE) RETURNING id`, tag+"d").Scan(&hiddenID))
	must(db.QueryRow(`INSERT INTO software (slug, title, is_published) VALUES ($1,'Pub app',TRUE) RETURNING id`, tag+"s").Scan(&swID))
	ins := `INSERT INTO promo_codes (code, product, discount_percent, starts_at, expires_at, is_active, is_public)
		VALUES ($1,'course',10, now() - interval '1 day', now() + interval '1 day', TRUE, TRUE) RETURNING id`
	must(db.QueryRow(ins, tag+"SCOPED").Scan(&scoped))
	must(db.QueryRow(ins, tag+"OPEN").Scan(&open))
	_, err := db.Exec(`INSERT INTO promo_code_courses (promo_code_id, course_id) VALUES ($1,$2),($1,$3)`, scoped, courseID, hiddenID)
	must(err)
	_, err = db.Exec(`INSERT INTO promo_code_software (promo_code_id, software_id) VALUES ($1,$2)`, scoped, swID)
	must(err)

	list, err := NewPromoCodeRepo(db).ListActivePublic(ctx)
	must(err)
	for _, p := range list {
		switch p.ID {
		case scoped:
			if len(p.ScopeItems) != 2 || p.ScopeItems[0].Title != "Pub course" || p.ScopeItems[0].URL != "/courses/"+tag+"c" || p.ScopeItems[1].Title != "Pub app" {
				t.Fatalf("scoped items: %+v", p.ScopeItems)
			}
		case open:
			if len(p.ScopeItems) != 0 {
				t.Fatalf("unscoped code must list nothing: %+v", p.ScopeItems)
			}
		}
	}
}
