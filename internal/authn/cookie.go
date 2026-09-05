package authn

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/config"
	"net/url"
	"strings"
	"time"
)

const defaultSessionCookieLifetime = 7 * 24 * time.Hour

type CookiePolicy struct {
	TrustProxy bool
	Secure     *bool
}

func CookiePolicyFor(cfg *config.RuntimeConfig) CookiePolicy {
	if cfg == nil {
		return CookiePolicy{}
	}
	return CookiePolicy{TrustProxy: cfg.TrustProxyHeaders, Secure: cfg.CookieSecure}
}

func SetSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, maxLifetime time.Duration, policy CookiePolicy) {
	if maxLifetime <= 0 {
		maxLifetime = defaultSessionCookieLifetime
	}
	http.SetCookie(w, sessionCookie(r, policy, url.QueryEscape(strings.TrimSpace(sessionID)), int(maxLifetime.Seconds()), time.Now().Add(maxLifetime)))
}

func ClearSessionCookie(w http.ResponseWriter, r *http.Request, policy CookiePolicy) {
	http.SetCookie(w, sessionCookie(r, policy, "", -1, time.Unix(0, 0)))
}

func sessionCookie(r *http.Request, policy CookiePolicy, value string, maxAge int, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   policy.secure(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
		Expires:  expires,
	}
}

func (p CookiePolicy) secure(r *http.Request) bool {
	if p.Secure != nil {
		return *p.Secure
	}
	return RequestIsHTTPS(r, p.TrustProxy)
}

func RequestIsHTTPS(r *http.Request, trustProxy bool) bool {
	return RequestScheme(r, trustProxy) == "https"
}
