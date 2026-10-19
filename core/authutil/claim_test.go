package authutil

import "testing"

func TestClaimCodeRoundTrip(t *testing.T) {
	code, err := NewClaimCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != ClaimCodeLen || code != NormalizeClaimCode(code) {
		t.Fatalf("stored form must be %d normalised chars, got %q", ClaimCodeLen, code)
	}
	shown := FormatClaimCode(code)
	if len(shown) != ClaimCodeLen+3 {
		t.Fatalf("display form wrong: %q", shown)
	}
	if got := NormalizeClaimCode(shown); got != code {
		t.Fatalf("display form must normalise back to the stored one: %q vs %q", got, code)
	}
	other, _ := NewClaimCode()
	if other == code {
		t.Fatal("claim codes must be random")
	}
}

func TestNormalizeClaimCode(t *testing.T) {
	good := "0123456789ABCDEF0123"
	cases := []struct{ in, want string }{
		{good, good},
		{"01234-56789-abcde-f0123", "0123456789ABCDEF0123"},
		{"  0123456789abcdef0123 ", "0123456789ABCDEF0123"},
		{"", ""},
		{"0123456789ABCDEF012", ""},   // too short
		{"0123456789ABCDEF01234", ""}, // too long
		{"0123456789ABCDEF012G", ""},  // not hex
		{"0123456789ABCDEF012\n", ""}, // control character
		{"' OR 1=1 --", ""},
	}
	for _, c := range cases {
		if got := NormalizeClaimCode(c.in); got != c.want {
			t.Errorf("NormalizeClaimCode(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
