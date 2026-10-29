package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBuyerNameRefusesSpamNames(t *testing.T) {
	for _, bad := range []string{
		"Перечисление поступило, напишите на rotjgmyb@proton.me",
		"visit http://evil.example now",
		"www.evil.example",
		"line\nbreak",
		strings.Repeat("a", 61),
		"",
	} {
		if got := buyerName(bad, "x@y.test"); got != "there" {
			t.Fatalf("buyerName(%q) = %q, want there", bad, got)
		}
	}
	if got := buyerName("  Asha Nair ", "x@y.test"); got != "Asha Nair" {
		t.Fatalf("real name changed: %q", got)
	}
}

func postForm(h http.HandlerFunc, path string, v url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// The fixture has no database, so any of these reaching the store would panic:
// each must be turned away before it.
func TestContactSubmitRejectsBeforeStore(t *testing.T) {
	fx := newEmailFixture(t)
	cases := map[string]url.Values{
		"link name": {"name": {"Ура! Приз ждёт вас! https://x.buzz/abc"}, "email": {"a@b.test"}, "message": {"hi"}},
		"honeypot":  {"name": {"Asha"}, "email": {"a@b.test"}, "message": {"hi"}, "website": {"x"}},
		"bad email": {"name": {"Asha"}, "email": {"not-an-email"}, "message": {"hi"}},
		"short":     {"name": {"A"}, "email": {"a@b.test"}, "message": {"hi"}},
		"too long":  {"name": {"Asha"}, "email": {"a@b.test"}, "message": {strings.Repeat("x", 5001)}},
	}
	for name, v := range cases {
		rec := postForm(fx.app.ContactSubmit, "/contact", v)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s: status %d", name, rec.Code)
		}
	}
	if got := postForm(fx.app.ContactSubmit, "/contact", cases["honeypot"]).Header().Get("Location"); got != "/contact?sent=1" {
		t.Fatalf("honeypot should look successful, got %q", got)
	}
	if got := postForm(fx.app.ContactSubmit, "/contact", cases["bad email"]).Header().Get("Location"); got != "/contact?error=1" {
		t.Fatalf("bad email: %q", got)
	}
}

func TestSubscribeRejectsBeforeStore(t *testing.T) {
	fx := newEmailFixture(t)
	rec := postForm(fx.app.NewsletterSubscribe, "/subscribe", url.Values{"email": {"a@b.test"}, "source": {"home"}, "website": {"x"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("honeypot status %d", rec.Code)
	}
	rec = postForm(fx.app.NewsletterSubscribe, "/subscribe", url.Values{"email": {"nope"}, "source": {"home"}})
	if rec.Header().Get("Location") != "/" {
		t.Fatalf("bad email location %q", rec.Header().Get("Location"))
	}
}
