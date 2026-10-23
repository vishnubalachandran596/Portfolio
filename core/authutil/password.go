// Package authutil wraps password hashing so handlers don't import bcrypt
// directly.
package authutil

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(hash), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// SecretEquals compares two secrets in constant time. Both sides are hashed
// first so the comparison takes the same time whatever their lengths. An
// empty expected value never matches: an unset secret disables the login
// that depends on it instead of accepting an empty password.
func SecretEquals(got, want string) bool {
	if want == "" {
		return false
	}
	g, w := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

// RandomHex returns n cryptographically random bytes hex-encoded (2n
// characters). Shared by every place in this repo that needs an
// unguessable token: OAuth CSRF state, access tokens, certificate codes,
// and unusable placeholder passwords for accounts (OAuth-provisioned or
// admin-created) that never get a real one.
func RandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// MaxPasswordBytes is bcrypt's hard input limit; longer passwords would be
// silently truncated, so they are rejected up front.
const MaxPasswordBytes = 72

// ValidatePassword enforces length only (NIST guidance: no composition
// rules). It returns a user-facing message, or "" when the password is fine.
func ValidatePassword(password, email string) string {
	switch {
	case len(password) < 10:
		return "Use at least 10 characters for your password."
	case len(password) > MaxPasswordBytes:
		return "Use at most 72 characters for your password."
	case strings.EqualFold(password, email):
		return "Your password can't be the same as your email."
	}
	return ""
}

// NormalizeAnswer makes a security answer forgiving about case and spacing
// before it is hashed or compared.
func NormalizeAnswer(answer string) string {
	return strings.Join(strings.Fields(strings.ToLower(answer)), " ")
}

// HashKey returns the SHA-256 hex digest used as a throttle key, so raw
// emails and IP addresses are never stored.
func HashKey(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return hex.EncodeToString(sum[:])
}

// ClientIP returns the caller's address. Vercel overwrites the forwarding
// headers, so they are only trusted in that order of precedence.
func ClientIP(r *http.Request) string {
	for _, h := range []string{"X-Vercel-Forwarded-For", "X-Real-Ip", "X-Forwarded-For"} {
		if v := strings.TrimSpace(strings.Split(r.Header.Get(h), ",")[0]); v != "" {
			return v
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// DummyHash is a valid bcrypt hash of a throwaway string. Comparing against
// it when an account doesn't exist keeps the response time the same, so the
// login and reset pages don't reveal which emails are registered.
const DummyHash = "$2a$10$WeqYFIb86kn84JVhRPURXOtIr7M6GImo6XxkrqhncFNmkK5NOwtIa"

// TokenHash is the SHA-256 hex digest under which a bearer access token is
// stored; the raw token lives only in the holder's cookie.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
