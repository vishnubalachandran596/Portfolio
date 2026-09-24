// Package authutil wraps password hashing so handlers don't import bcrypt
// directly.
package authutil

import (
	"crypto/rand"
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
