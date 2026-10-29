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

func TestValidPersonName(t *testing.T) {
	for _, ok := range []string{"Vishnu", "Asha Nair", "Anne-Marie D'Souza", "A. K. Nair", "John Ronald Tolkien", "Li"} {
		if !validPersonName(ok) {
			t.Fatalf("%q should be accepted", ok)
		}
	}
	for _, bad := range []string{
		"", "A", "Перечисление Руб", "Asha  Nair", "Asha Nair Kumar Menon", "Asha123", "Asha_Nair",
		"https://x.buzz", "Asha@x.com", "Ашa Nair", strings.Repeat("a", 31),
	} {
		if validPersonName(bad) {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

func TestEnglishText(t *testing.T) {
	for _, ok := range []string{"Hello, I need a review of our system design. Thanks!", "Café meeting at 5pm", "Budget ₹5,000 / month", "line\nbreak"} {
		if !englishText(ok) {
			t.Fatalf("%q should be accepted", ok)
		}
	}
	for _, bad := range []string{"Привет, приз ждёт вас", "你好", "مرحبا", "hello \u0000"} {
		if englishText(bad) {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

func TestValidPhone(t *testing.T) {
	if !validPhone("") || !validPhone("+91 98765-43210") || !validPhone("(044) 2345.6789") {
		t.Fatal("valid phones rejected")
	}
	if validPhone("call me") || validPhone(strings.Repeat("1", 31)) {
		t.Fatal("invalid phone accepted")
	}
}

func TestContactSubmitErrorCodes(t *testing.T) {
	fx := newEmailFixture(t)
	good := func() url.Values {
		return url.Values{"name": {"Asha Nair"}, "email": {"a@b.test"}, "message": {"Hello there"}}
	}
	cases := []struct {
		field, value, want string
	}{
		{"name", "Привет приз", "/contact?error=name"},
		{"message", "Привет, приз ждёт вас", "/contact?error=english"},
		{"phone", "call me", "/contact?error=phone"},
		{"email", "пользователь@пример.рф", "/contact?error=1"},
	}
	for _, c := range cases {
		v := good()
		v.Set(c.field, c.value)
		if got := postForm(fx.app.ContactSubmit, "/contact", v).Header().Get("Location"); got != c.want {
			t.Fatalf("%s=%q: got %q want %q", c.field, c.value, got, c.want)
		}
	}
}

func TestCheckFieldRules(t *testing.T) {
	name := fieldSpec{"Your name", fieldName, 0, true}
	line := fieldSpec{"Title", fieldLine, 100, false}
	text := fieldSpec{"Message", fieldText, 20, true}
	cases := []struct {
		spec fieldSpec
		in   string
		ok   bool
	}{
		{name, "Anne-Marie D'Souza", true}, {name, "", false}, {name, "Привет Мир", false},
		{name, "Win $1000 now http://x", false}, {name, "A B C D", false},
		{line, "", true}, {line, "Senior Engineer", true}, {line, "a\nb", false}, {line, "工程师", false},
		{text, "Hello there", true}, {text, "", false}, {text, "你好", false}, {text, strings.Repeat("a", 21), false},
	}
	for _, c := range cases {
		if got := checkField(c.spec, c.in) == ""; got != c.ok {
			t.Errorf("checkField(%q, %q) ok=%v, want %v", c.spec.Label, c.in, got, c.ok)
		}
	}
	if validEmail("josé@example.com") || !validEmail("jo@example.com") {
		t.Error("validEmail must be ASCII only")
	}
}

func TestSafeNextValueBlocksOffSiteTargets(t *testing.T) {
	bad := []string{"//evil.com", "/\\evil.com", "/\t/evil.com", "https://evil.com", "evil.com", "/a\nb", ""}
	for _, b := range bad {
		if got := safeNextValue(b); got != "/" {
			t.Errorf("safeNextValue(%q) = %q, want /", b, got)
		}
	}
	if got := safeNextValue("/courses/x?a=1"); got != "/courses/x?a=1" {
		t.Errorf("good path changed: %q", got)
	}
}
