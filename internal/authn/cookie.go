package authn

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultSessionCookieLifetime = 7 * 24 * time.Hour

// SetSessionCookie stores the opaque dashboard session id in an HttpOnly
// same-site cookie so browser APIs can authenticate without exposing the
// underlying bearer token to JavaScript.
func SetSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, maxLifetime time.Duration, trustProxy bool, cookieSecure *bool) {
	if maxLifetime <= 0 {
		maxLifetime = defaultSessionCookieLifetime
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    url.QueryEscape(strings.TrimSpace(sessionID)),
		Path:     "/",
		HttpOnly: true,
		Secure:   sessionCookieSecure(r, trustProxy, cookieSecure),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(maxLifetime.Seconds()),
		Expires:  time.Now().Add(maxLifetime),
	})
}

// ClearSessionCookie expires the dashboard auth cookie.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request, trustProxy bool, cookieSecure *bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   sessionCookieSecure(r, trustProxy, cookieSecure),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func sessionCookieSecure(r *http.Request, trustProxy bool, cookieSecure *bool) bool {
	if cookieSecure != nil {
		return *cookieSecure
	}
	return RequestIsHTTPS(r, trustProxy)
}

func RequestIsHTTPS(r *http.Request, trustProxy bool) bool {
	return requestScheme(r, trustProxy) == "https"
}

func requestScheme(r *http.Request, trustProxy bool) string {
	if r == nil {
		return "http"
	}
	if trustProxy {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
			return strings.ToLower(strings.TrimSpace(strings.Split(forwarded, ",")[0]))
		}
		if forwarded := strings.TrimSpace(r.Header.Get("Forwarded")); forwarded != "" {
			for _, part := range strings.Split(forwarded, ";") {
				key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
				if !ok || !strings.EqualFold(key, "proto") {
					continue
				}
				return strings.ToLower(strings.Trim(value, `"`))
			}
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
