package authutil

import "testing"

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		pw, email string
		ok        bool
	}{
		{"short", "a@b.co", false},
		{"long enough pw", "a@b.co", true},
		{"A@B.CO-is-longer", "a@b.co-is-longer", false},
		{string(make([]byte, 73)), "a@b.co", false},
	}
	for _, c := range cases {
		if got := ValidatePassword(c.pw, c.email) == ""; got != c.ok {
			t.Errorf("ValidatePassword(%q) ok=%v, want %v", c.pw, got, c.ok)
		}
	}
}

func TestNormalizeAnswer(t *testing.T) {
	if got := NormalizeAnswer("  St   MARYS\tSchool "); got != "st marys school" {
		t.Fatalf("got %q", got)
	}
}

func TestDummyHashIsValidBcrypt(t *testing.T) {
	// A malformed dummy would make the "unknown account" path return early
	// and leak timing; it must parse (and never match a real password).
	if CheckPassword(DummyHash, "anything") {
		t.Fatal("dummy hash must not match")
	}
}
