package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func TestExtraPurchaseIDs(t *testing.T) {
	paid := models.PurchaseStatusPaid
	got := extraPurchaseIDs([]models.SoftwarePurchase{
		{ID: 9, UserID: 1, SoftwareID: 5, Status: paid},
		{ID: 4, UserID: 1, SoftwareID: 5, Status: paid},
		{ID: 6, UserID: 1, SoftwareID: 5, Status: "created"},
		{ID: 7, UserID: 1, SoftwareID: 6, Status: paid}, // other item: first
		{ID: 8, UserID: 2, SoftwareID: 5, Status: paid}, // other user: first
		{ID: 10, UserID: 0, SoftwareID: 5, Status: paid},
		{ID: 11, UserID: 0, SoftwareID: 5, Status: paid}, // guests are never extra
	})
	if len(got) != 1 || !got[9] {
		t.Fatalf("only purchase 9 is extra, got %v", got)
	}
}

func TestNewLicenceTemplates(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	base := BaseData{SiteName: "Site", Nav: NavLinks{Home: "/", Software: "/software", Refund: "/refund"}}
	sw := models.Software{ID: 1, Slug: "temple", Title: "Temple", Version: "1", LicenceProduct: "temple", PriceCents: 200000, IsPublished: true}

	render := func(page string, data any) string {
		rec := httptest.NewRecorder()
		r.Render(rec, 200, page, data)
		if rec.Code != 200 {
			t.Fatalf("%s: status %d", page, rec.Code)
		}
		return rec.Body.String()
	}

	with := render("account_licences", AccountLicencesData{
		BaseData: base, Ready: true, PaymentsEnabled: true,
		Buy: []licenceBuy{{Slug: "temple", Title: "Temple", Price: "₹2,000"}},
	})
	for _, want := range []string{`id="new-licence-temple"`, `data-software-buy`, `data-slug="temple"`, `id="lic-promo-temple"`, `checkout.razorpay.com`, "Get a new licence"} {
		if !strings.Contains(with, want) {
			t.Errorf("account_licences missing %q", want)
		}
	}
	off := render("account_licences", AccountLicencesData{BaseData: base, Buy: []licenceBuy{{Slug: "temple", Title: "Temple", Price: "x"}}})
	if strings.Contains(off, "data-software-buy") || strings.Contains(off, "checkout.razorpay.com") {
		t.Error("payments off: no buy button or Razorpay script expected")
	}
	if none := render("account_licences", AccountLicencesData{BaseData: base}); strings.Contains(none, "Get a new licence") {
		t.Error("no paid purchase: no new-licence box expected")
	}

	owner := render("software_detail", SoftwareDetailData{BaseData: base, Software: sw, HasAccess: true, OwnsSoftware: true})
	if !strings.Contains(owner, "Need another device?") || !strings.Contains(owner, `href="/account/licences#new-licence-temple"`) {
		t.Error("owner page missing the new-licence link")
	}
	admin := render("software_detail", SoftwareDetailData{BaseData: base, Software: sw, HasAccess: true})
	if strings.Contains(admin, "Need another device?") {
		t.Error("admin without a purchase must not see the new-licence link")
	}
}
