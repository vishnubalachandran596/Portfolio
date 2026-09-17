// Package middleware provides HTTP middleware: admin session auth and
// request logging.
package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey string

const adminIDKey ctxKey = "admin_id"

const SessionCookieName = "portfolio_admin_session"

type sessionClaims struct {
	AdminID int64 `json:"admin_id"`
	jwt.RegisteredClaims
}

// Auth issues and validates the admin session JWT stored in an HttpOnly
// cookie.
type Auth struct {
	secret []byte
	ttl    time.Duration
	secure bool
}

func NewAuth(secret []byte, ttl time.Duration, secureCookies bool) *Auth {
	return &Auth{secret: secret, ttl: ttl, secure: secureCookies}
}

func (a *Auth) IssueSession(w http.ResponseWriter, adminID int64) {
	claims := sessionClaims{
		AdminID: adminID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(a.secret)
	if err != nil {
		log.Printf("issue session: %v", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(a.ttl),
	})
}

func (a *Auth) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

// adminIDFromRequest validates the session cookie and returns the admin id.
func (a *Auth) adminIDFromRequest(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return 0, false
	}
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		return a.secret, nil
	})
	if err != nil || !token.Valid {
		return 0, false
	}
	return claims.AdminID, true
}

// RequireAdmin redirects to /admin/login when there is no valid session,
// otherwise calls next with the admin id attached to the request context.
func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.adminIDFromRequest(r)
		if !ok {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), adminIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdminIDFromContext reads the authenticated admin id set by RequireAdmin.
func AdminIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(adminIDKey).(int64)
	return id, ok
}

// IsAuthenticated reports whether the request already carries a valid
// session, without redirecting (used by the login page to bounce logged-in
// admins straight to the dashboard).
func (a *Auth) IsAuthenticated(r *http.Request) bool {
	_, ok := a.adminIDFromRequest(r)
	return ok
}

// Logging logs method, path, status, and duration for each request.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
