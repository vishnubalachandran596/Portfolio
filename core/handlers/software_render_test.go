package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

// TestSoftwareTemplatesRender smoke-tests the new software_list,
// software_detail, admin_software_list, and admin_software_form templates
// with realistic data -- catching runtime template errors (undefined
// fields, bad range targets) that a bare parse (view.New() in every other
// *_render_test.go) wouldn't.
func TestSoftwareTemplatesRender(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	base := BaseData{
		SiteName: "Vishnu Balachandran",
		Nav:      NavLinks{Home: "/", Courses: "/courses", Software: "/software", Consultancy: "/consultancy"},
	}
	sw := models.Software{
		ID: 1, Slug: "temple-accounts-billing", Title: "Temple Accounts & Billing", Version: "1.0.0",
		Summary:     "A local order and expense tracker for temples, no subscription.",
		Description: "A local order and expense tracker for temples.\n\nSecond paragraph of the description.", DownloadLink: "https://drive.google.com/drive/folders/abc",
		PriceCents: 200000, LicenseTerms: "Licensed per device.",
		PostPurchaseInstructions: "Email support@example.com with your receipt for your license key.",
		IsPublished:              true,
	}

	listData := SoftwareListData{BaseData: base, Software: []models.Software{sw}}
	emptyListData := SoftwareListData{BaseData: base}
	detailData := SoftwareDetailData{BaseData: base, Software: sw, PaymentsEnabled: true, UserEmail: "buyer@example.com"}
	detailDataAccess := SoftwareDetailData{BaseData: base, Software: sw, HasAccess: true}

	adminBase := base
	adminListData := AdminSoftwareListData{BaseData: adminBase, Software: []models.Software{sw}}
	adminFormData := AdminSoftwareFormData{BaseData: adminBase, Software: sw, IsNew: false}
	adminFormNewData := AdminSoftwareFormData{BaseData: adminBase, IsNew: true}

	loggedOut := SoftwareDetailData{BaseData: base, Software: sw, PaymentsEnabled: true}
	owner := SoftwareDetailData{BaseData: base, Software: sw, HasAccess: true, OwnsSoftware: true, UserEmail: "buyer@example.com"}
	swLic := sw
	swLic.LicenceProduct = "temple"
	ownerLic := SoftwareDetailData{BaseData: base, Software: swLic, HasAccess: true, OwnsSoftware: true, UserEmail: "buyer@example.com"}
	promo := models.PromoCode{ID: 1, Code: "X", Product: models.PromoProductSoftware, DiscountPercent: 10, SoftwareIDs: []int64{1}}
	promoList := AdminPromoListData{BaseData: base, SoftwarePromos: []models.PromoCode{promo}}
	promoForm := AdminPromoFormData{BaseData: base, Promo: promo, SoftwareItems: []models.Software{sw}}

	// Extra, content-asserting checks on the purchase-box states.
	for _, c := range []struct {
		name string
		data SoftwareDetailData
		want string
	}{
		{"logged out", loggedOut, `href="/login?next=/software/temple-accounts-billing"`},
		{"logged in", detailData, `id="software-purchase-btn"`},
		{"owner", ownerLic, `href="/account/licences"`},
	} {
		rec := httptest.NewRecorder()
		r.Render(rec, 200, "software_detail", c.data)
		body := rec.Body.String()
		if !strings.Contains(body, c.want) || (c.name != "owner" && !strings.Contains(body, "Get licence")) {
			t.Fatalf("%s: missing %q or Get licence button", c.name, c.want)
		}
	}

	cases := []struct {
		page string
		data any
	}{
		{"software_detail", loggedOut},
		{"software_detail", owner},
		{"admin_promo_list", promoList},
		{"admin_promo_form", promoForm},
		{"software_list", listData},
		{"software_list", emptyListData},
		{"software_detail", detailData},
		{"software_detail", detailDataAccess},
		{"admin_software_list", adminListData},
		{"admin_software_form", adminFormData},
		{"admin_software_form", adminFormNewData},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		r.Render(rec, 200, c.page, c.data)
		body := rec.Body.String()
		if strings.Contains(body, "<!-- render error:") {
			t.Fatalf("%s: %s", c.page, body)
		}
	}
}
