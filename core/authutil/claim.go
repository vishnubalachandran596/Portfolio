package authutil

import "strings"

// ClaimCodeLen is the number of hex characters in a claim code (80 bits).
const ClaimCodeLen = 20

// NewClaimCode returns a fresh claim code in its stored form: upper-case
// hex, no separators. Show it to the buyer with FormatClaimCode.
func NewClaimCode() (string, error) {
	h, err := RandomHex(ClaimCodeLen / 2)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(h), nil
}

// NormalizeClaimCode turns what a person typed (any case, with spaces or
// hyphens) into the stored form. It returns "" for anything that cannot be
// a claim code, so callers can reject early without touching the database.
func NormalizeClaimCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'F':
			b.WriteRune(r)
		case r == '-', r == ' ', r == '\t':
			// separators are allowed and ignored
		default:
			return ""
		}
	}
	if b.Len() != ClaimCodeLen {
		return ""
	}
	return b.String()
}

// FormatClaimCode groups a stored code as XXXXX-XXXXX-XXXXX-XXXXX for
// display. Anything that is not a full code is returned unchanged.
func FormatClaimCode(code string) string {
	if len(code) != ClaimCodeLen {
		return code
	}
	return code[0:5] + "-" + code[5:10] + "-" + code[10:15] + "-" + code[15:20]
}
