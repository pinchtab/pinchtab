package authn

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SetSessionCookie stores the opaque dashboard session id in an HttpOnly
// same-site cookie so browser APIs can authenticate without exposing the
// underlying bearer token to JavaScript.
func SetSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, maxLifetime time.Duration) {
	if maxLifetime <= 0 {
		maxLifetime = DefaultSessionMaxLifetime
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    url.QueryEscape(strings.TrimSpace(sessionID)),
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(maxLifetime.Seconds()),
		Expires:  time.Now().Add(maxLifetime),
	})
}

// ClearSessionCookie expires the dashboard auth cookie.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func isSecureRequest(r *http.Request) bool {
	if r != nil && r.TLS != nil {
		return true
	}
	if r == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
