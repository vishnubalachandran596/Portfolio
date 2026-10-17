// Package authutil wraps password hashing so handlers don't import bcrypt
// directly.
package authutil

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

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
