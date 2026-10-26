package view

import (
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/notify"
)

func TestBuiltinEmailTemplatesRender(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range models.EmailKinds {
		data := notify.SampleData(kind)
		html, text, err := r.RenderEmail(kind, data)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if strings.TrimSpace(html) == "" || strings.TrimSpace(text) == "" {
			t.Fatalf("%s: empty render", kind)
		}
		if strings.Contains(html, "<script") || strings.Contains(html, "javascript:") {
			t.Fatalf("%s: unsafe html", kind)
		}
		switch kind {
		case models.EmailBookingNew, models.EmailBookingRescheduled, models.EmailBookingCancelled, models.EmailConnectRequest:
			if !strings.Contains(html, data.ManageURL) || !strings.Contains(text, data.ManageURL) {
				t.Fatalf("%s: manage url missing", kind)
			}
		case models.EmailNewsletterConfirm:
			if !strings.Contains(html, data.UnsubscribeURL) || !strings.Contains(text, data.UnsubscribeURL) {
				t.Fatalf("%s: unsubscribe url missing", kind)
			}
		default:
			if !strings.Contains(html, data.ReceiptURL) && !strings.Contains(html, data.AccessURL) {
				t.Fatalf("%s: receipt or access url missing", kind)
			}
			if !strings.Contains(html, data.Amount) {
				t.Fatalf("%s: formatted amount missing", kind)
			}
		}
	}
}
