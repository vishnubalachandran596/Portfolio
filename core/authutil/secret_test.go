package authutil

import "testing"

func TestSecretEquals(t *testing.T) {
	cases := []struct {
		got, want string
		ok        bool
	}{
		{"s3cret", "s3cret", true},
		{"s3cret", "s3cret ", false},
		{"", "", false}, // an unset secret never matches, even an empty guess
		{"anything", "", false},
		{"", "s3cret", false},
		{"S3CRET", "s3cret", false},
	}
	for _, c := range cases {
		if got := SecretEquals(c.got, c.want); got != c.ok {
			t.Errorf("SecretEquals(%q,%q)=%v want %v", c.got, c.want, got, c.ok)
		}
	}
}
