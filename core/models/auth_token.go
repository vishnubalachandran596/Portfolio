package models

import "time"

// Auth token purposes.
const (
	AuthTokenSignup = "signup"
	AuthTokenReset  = "reset"
)

// AuthToken is one emailed one-time link. Only the hash of the token is
// stored. A signup token also carries the pending name and password hash,
// so the account does not exist until the link is used.
type AuthToken struct {
	ID           int64
	Purpose      string
	Email        string
	Name         string
	PasswordHash string
	NextPath     string
	ExpiresAt    time.Time
}
