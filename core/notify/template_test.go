package notify

import (
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

func goodPurchaseHTML() string {
	return `<p>Hello {{.BuyerName}}</p><p><a href="{{.ReceiptURL}}">Receipt</a></p>`
}

func TestValidateAcceptsPlainTemplate(t *testing.T) {
	warnings, err := Validate(models.EmailPurchaseCourse, "Your course", goodPurchaseHTML(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	html, text, err := Execute(models.EmailPurchaseCourse, goodPurchaseHTML(), "", SampleData(models.EmailPurchaseCourse))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Asha Raman") || !strings.Contains(text, "Asha Raman") {
		t.Fatalf("html %q text %q", html, text)
	}
}

func TestValidateRejectsUnsafeTemplates(t *testing.T) {
	cases := []struct {
		name string
		kind string
		html string
		text string
	}{
		{"script", models.EmailPurchaseCourse, `<script></script>` + goodPurchaseHTML(), ""},
		{"onclick", models.EmailPurchaseCourse, `<a onclick="x" href="{{.ReceiptURL}}">x</a>`, ""},
		{"javascript", models.EmailPurchaseCourse, `<a href="javascript:alert(1)">x</a><a href="{{.ReceiptURL}}">r</a>`, ""},
		{"iframe", models.EmailPurchaseCourse, `<iframe src="https://example.com"></iframe>` + goodPurchaseHTML(), ""},
		{"form", models.EmailPurchaseCourse, `<form action="/x"></form>` + goodPurchaseHTML(), ""},
		{"define", models.EmailPurchaseCourse, `{{define "x"}}hi{{end}}` + goodPurchaseHTML(), ""},
		{"call", models.EmailPurchaseCourse, `{{printf "%s" .BuyerName}}` + goodPurchaseHTML(), ""},
		{"unknown", models.EmailPurchaseCourse, `<p>{{.Password}}</p>` + goodPurchaseHTML(), ""},
		{"missing link", models.EmailPurchaseCourse, `<p>Hello {{.BuyerName}}</p>`, ""},
		{"booking text", models.EmailBookingNew, `<p><a href="{{.ManageURL}}">manage</a></p>`, "no link here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Validate(tc.kind, "Subject", tc.html, tc.text); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestValidateWarnsOnHTTPSImage(t *testing.T) {
	html := `<img src="https://example.com/a.png" alt="">` + goodPurchaseHTML()
	warnings, err := Validate(models.EmailPurchaseCourse, "Subject", html, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "https://example.com/a.png") {
		t.Fatalf("warnings: %v", warnings)
	}
	if _, err := Validate(models.EmailPurchaseCourse, "Subject", `<img src="http://example.com/a.png">`+goodPurchaseHTML(), ""); err == nil {
		t.Fatal("http image should be rejected")
	}
}
