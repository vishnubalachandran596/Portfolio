package handlers

import (
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestFinalAmountCents(t *testing.T) {
	cases := []struct{ base, discount, want int }{
		{99900, 0, 99900},
		{99900, 9990, 89910},
		{99900, 99900, 0},
		{99900, 150000, 0}, // a discount can never go below zero
		{99900, -500, 99900},
		{-1, 0, 0},
		{0, 0, 0},
	}
	for _, c := range cases {
		if got := finalAmountCents(c.base, c.discount); got != c.want {
			t.Errorf("finalAmountCents(%d,%d)=%d want %d", c.base, c.discount, got, c.want)
		}
	}
}

func TestCourseFixedPrice(t *testing.T) {
	if got := (models.Course{MinPriceCents: 149900}).FixedPriceCents(); got != 149900 {
		t.Errorf("got %d", got)
	}
	if got := (models.Course{MinPriceCents: -5}).FixedPriceCents(); got != 0 {
		t.Errorf("negative price must clamp to 0, got %d", got)
	}
}

// TestAmountNeverFromClient guards the pay-what-you-want removal: the two
// order handlers may read only the buyer's email and promo code from the
// form, and the browser script must not send an amount.
func TestAmountNeverFromClient(t *testing.T) {
	re := regexp.MustCompile(`FormValue\("([^"]+)"\)`)
	allowed := map[string]bool{"email": true, "promo_code": true, "code": true} // "code" is the /account/claim form
	for _, f := range []string{"checkout.go", "software_checkout.go", "purchase_common.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if !allowed[m[1]] {
				t.Errorf("%s reads form field %q; only email, promo_code and the claim code are allowed", f, m[1])
			}
		}
		if strings.Contains(string(src), "amount_cents") || strings.Contains(string(src), "Atoi(r.Form") {
			t.Errorf("%s must not read an amount from the request", f)
		}
	}
	js, err := os.ReadFile("../../web/static/js/main.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	start := strings.Index(src, `getElementById("purchase-btn")`)
	mid := strings.Index(src, "Optional contribution after finishing a course")
	sw := strings.Index(src, `getElementById("software-purchase-btn")`)
	end := strings.Index(src, "Optional article contribution")
	if start < 0 || mid < start || sw < mid || end < sw {
		t.Fatal("could not locate course and software checkout code in main.js")
	}
	if strings.Contains(src[start:mid], "amount_cents") || strings.Contains(src[sw:end], "amount_cents") {
		t.Error("main.js course/software checkout must not send amount_cents")
	}
}

func TestGuestCheckoutTemplatesRender(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	base := BaseData{
		SiteName: "Vishnu Balachandran",
		Nav:      NavLinks{Home: "/", Courses: "/courses", Refund: "/refund"},
	}
	course := models.Course{ID: 1, Slug: "java-interview-preparation", Title: "Java Interview Preparation", IsPaid: true, IsPublished: true, MinPriceCents: 149900}
	sw := models.Software{ID: 1, Slug: "temple", Title: "Temple Accounts", PriceCents: 200000, IsPublished: true, LicenceProduct: "temple"}

	cases := []struct {
		page string
		data any
		want []string
	}{
		{"course_detail", CourseDetailData{BaseData: base, Course: course, PaymentsEnabled: true, ResolvedMinCents: 149900, ResolvedMinWhole: 1499, ResolvedMinDisplay: "Rs 1,499"},
			[]string{`id="purchase-email"`, "claim code", "purchase-promo-apply", "You pay"}},
		{"course_detail", CourseDetailData{BaseData: base, Course: course, PaymentsEnabled: true, ResolvedMinCents: 149900, ResolvedMinDisplay: "Rs 1,499", UserEmail: "me@example.com"},
			[]string{"me@example.com"}},
		{"software_detail", SoftwareDetailData{BaseData: base, Software: sw, PaymentsEnabled: true},
			[]string{"Get licence"}},
		{"quiz_result", QuizResultData{BaseData: base, Course: course, Attempt: models.QuizAttempt{Passed: true, ScorePercent: 90}, PaymentsEnabled: true},
			[]string{"course-contribute-btn", "Download Certificate"}},
		{"purchase_success", PurchaseSuccessData{BaseData: base, Kind: "course", Title: "Java", Receipt: "C000012", Email: "g@example.com", Amount: "Rs 1,499", ClaimCode: "ABCDE-FGHIJ-KLMNO-PQRST", NextURL: "/courses/java/learn", NextText: "Open the course"},
			[]string{"ABCDE-FGHIJ-KLMNO-PQRST", "/account/claim", "C000012"}},
		{"purchase_success", PurchaseSuccessData{BaseData: base, Kind: "software", Title: "Temple", Receipt: "S000003", Email: "g@example.com", Amount: "Rs 2,000", Attached: true, Licence: true, NextURL: "/software/temple", NextText: "Go"},
			[]string{"attached to your account", "/account/licences"}},
		{"account_claim", AccountClaimData{BaseData: base}, []string{`name="code"`}},
		{"account_claim", AccountClaimData{BaseData: base, Error: "That code was not found, or it has already been used."}, []string{"not found"}},
		{"account_claim", AccountClaimData{BaseData: base, Claimed: []repo.ClaimedItem{{Kind: "course", Title: "Java", Slug: "java"}}}, []string{"Added to your account", "/courses/java/learn"}},
		{"certificate_verify", CertificateVerifyData{BaseData: BaseData{SiteName: "VB", NoIndex: true}, Valid: true, Name: "Jose Garcia", Course: "Java", Date: "05 March 2026", Score: 85, Pass: 70, Code: "ABCD-EF01-2345-6789"},
			[]string{"Certificate verified", "Jose Garcia", `name="robots" content="noindex, nofollow"`}},
		{"certificate_verify", CertificateVerifyData{BaseData: BaseData{SiteName: "VB", NoIndex: true}, Code: ""}, []string{"Certificate not found"}},
	}
	for i, c := range cases {
		rec := httptest.NewRecorder()
		r.Render(rec, 200, c.page, c.data)
		body := rec.Body.String()
		if strings.Contains(body, "<!-- render error:") {
			t.Fatalf("case %d %s: %s", i, c.page, body)
		}
		for _, w := range c.want {
			if !strings.Contains(body, w) {
				t.Errorf("case %d %s: missing %q", i, c.page, w)
			}
		}
	}
}
