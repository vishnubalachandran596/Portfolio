// Package middleware provides HTTP middleware: admin session auth and
// request logging.
package middleware

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"sync"
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
	// TV is the user's token_version when the cookie was issued. A cookie
	// whose TV no longer matches the database row is rejected, which is how
	// a password reset signs every old session out without a session table.
	TV int `json:"tv"`
	jwt.RegisteredClaims
}

// Auth issues and validates the admin session JWT stored in an HttpOnly
// cookie.
type Auth struct {
	secret []byte
	ttl    time.Duration
	secure bool

	// verifyUser and verifyAdmin re-check a signed cookie against the
	// database. A valid signature alone is never enough: an admin who was
	// removed, or a user who reset their password, loses access at once.
	verifyUser  func(ctx context.Context, id int64) (tokenVersion int, ok bool)
	verifyAdmin func(ctx context.Context, id int64) bool
}

// SetVerifiers wires the database checks. Left unset (as in unit tests) the
// cookie signature alone decides.
func (a *Auth) SetVerifiers(
	user func(ctx context.Context, id int64) (int, bool),
	admin func(ctx context.Context, id int64) bool,
) {
	a.verifyUser, a.verifyAdmin = user, admin
}

// identity memoises the database checks for one request, so a page that
// asks "who is this?" several times (the shared chrome plus the handler)
// pays for at most one lookup per cookie. It is created per request by
// Attach and holds no state beyond that request.
type identity struct {
	userOnce  sync.Once
	userID    int64
	userOK    bool
	adminOnce sync.Once
	adminID   int64
	adminOK   bool
}

type identityKey struct{}

// Attach installs the per-request identity memo. Wrap the whole mux in it.
func (a *Auth) Attach(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), identityKey{}, &identity{})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func identityFrom(r *http.Request) *identity {
	id, _ := r.Context().Value(identityKey{}).(*identity)
	return id
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

// adminIDFromRequest returns the admin id for a valid cookie that still
// belongs to a row in the admins table.
func (a *Auth) adminIDFromRequest(r *http.Request) (int64, bool) {
	if mem := identityFrom(r); mem != nil {
		mem.adminOnce.Do(func() { mem.adminID, mem.adminOK = a.resolveAdmin(r) })
		return mem.adminID, mem.adminOK
	}
	return a.resolveAdmin(r)
}

func (a *Auth) resolveAdmin(r *http.Request) (int64, bool) {
	id, ok := a.adminCookieID(r)
	if !ok {
		return 0, false
	}
	if a.verifyAdmin != nil && !a.verifyAdmin(r.Context(), id) {
		return 0, false
	}
	return id, true
}

// adminCookieID validates the signature and expiry only.
func (a *Auth) adminCookieID(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return 0, false
	}
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		return a.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
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

// AdminID returns the authenticated admin's id from the session cookie,
// for handlers outside the RequireAdmin chain (which otherwise only
// exposes it via context) that still need to know which admin is asking.
func (a *Auth) AdminID(r *http.Request) (int64, bool) {
	return a.adminIDFromRequest(r)
}

// --- Visitor (non-admin) sessions ---
//
// A separate cookie/JWT from the admin session above: a visitor logs in
// to book consultancy or buy a course, which is a different privilege
// level from the admin portal. When a visitor's email happens to match an
// admin account, the login handler issues *both* cookies, so they get
// their visitor session and full admin access together.

func (a *Auth) IssueUserSession(w http.ResponseWriter, userID int64, tokenVersion int) {
	claims := userSessionClaims{
		UserID: userID,
		TV:     tokenVersion,
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
	if mem := identityFrom(r); mem != nil {
		mem.userOnce.Do(func() { mem.userID, mem.userOK = a.resolveUser(r) })
		return mem.userID, mem.userOK
	}
	return a.resolveUser(r)
}

func (a *Auth) resolveUser(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie(UserSessionCookieName)
	if err != nil || cookie.Value == "" {
		return 0, false
	}
	claims := &userSessionClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		return a.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return 0, false
	}
	if a.verifyUser != nil {
		tv, ok := a.verifyUser(r.Context(), claims.UserID)
		if !ok || tv != claims.TV {
			return 0, false
		}
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
