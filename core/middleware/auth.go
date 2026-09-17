// Package middleware provides HTTP middleware: admin session auth and
// request logging.
package middleware

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey string

const adminIDKey ctxKey = "admin_id"
const userIDKey ctxKey = "user_id"

const SessionCookieName = "portfolio_admin_session"
const UserSessionCookieName = "portfolio_session"

type sessionClaims struct {
	AdminID int64 `json:"admin_id"`
	jwt.RegisteredClaims
}

type userSessionClaims struct {
	UserID int64 `json:"user_id"`
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

// --- Visitor (non-admin) sessions ---
//
// A separate cookie/JWT from the admin session above: a visitor logs in
// to book consultancy or buy a course, which is a different privilege
// level from the admin portal. When a visitor's email happens to match an
// admin account, the login handler issues *both* cookies, so they get
// their visitor session and full admin access together.

func (a *Auth) IssueUserSession(w http.ResponseWriter, userID int64) {
	claims := userSessionClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(a.secret)
	if err != nil {
		log.Printf("issue user session: %v", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     UserSessionCookieName,
		Value:    signed,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(a.ttl),
	})
}

func (a *Auth) ClearUserSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     UserSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func (a *Auth) userIDFromRequest(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie(UserSessionCookieName)
	if err != nil || cookie.Value == "" {
		return 0, false
	}
	claims := &userSessionClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		return a.secret, nil
	})
	if err != nil || !token.Valid {
		return 0, false
	}
	return claims.UserID, true
}

// RequireUser redirects to /login?next=<path> when there's no valid
// visitor session, otherwise calls next with the user id attached to the
// request context.
func (a *Auth) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.userIDFromRequest(r)
		if !ok {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserIDFromContext reads the authenticated visitor id set by RequireUser.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}

// IsUserAuthenticated reports whether the request carries a valid visitor
// session, without redirecting.
func (a *Auth) IsUserAuthenticated(r *http.Request) bool {
	_, ok := a.userIDFromRequest(r)
	return ok
}

// UserID reads the visitor id straight from the session cookie, for
// handlers that want it without having gone through RequireUser (e.g. the
// shared page chrome, which renders on pages a visitor doesn't have to be
// logged in to view).
func (a *Auth) UserID(r *http.Request) (int64, bool) {
	return a.userIDFromRequest(r)
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
