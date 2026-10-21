package handlers

import (
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/repo"
)

func TestAccountDeletePageRenders(t *testing.T) {
	base := BaseData{SiteName: "S", Nav: NavLinks{Home: "/"}}
	body := renderDL(t, "account_delete", AccountDeleteData{
		BaseData: base, Email: "a@example.com", HasPassword: true,
		Summary: repo.AccountDeletionSummary{Courses: 2, Software: 1, Certificates: 1, UpcomingSlots: 1},
	})
	for _, want := range []string{
		"All your purchases will be lost", "a@example.com", `name="password"`, `name="confirm"`,
		"1 upcoming consultancy booking", `action="/account/delete"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	body = renderDL(t, "account_delete", AccountDeleteData{BaseData: base, Email: "g@example.com"})
	if !strings.Contains(body, `name="email"`) || strings.Contains(body, `name="password"`) {
		t.Error("google-only account should confirm by email")
	}
}
