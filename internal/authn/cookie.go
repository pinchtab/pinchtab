package authn

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SetSessionCookie stores the opaque dashboard session id in an HttpOnly
// same-site cookie so browser APIs can authenticate without exposing the
// underlying bearer token to JavaScript.
func SetSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, maxLifetime time.Duration, trustProxy bool) {
	if maxLifetime <= 0 {
		maxLifetime = DefaultSessionMaxLifetime
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    url.QueryEscape(strings.TrimSpace(sessionID)),
		Path:     "/",
		HttpOnly: true,
		Secure:   sessionCookieSecure(r, trustProxy),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(maxLifetime.Seconds()),
		Expires:  time.Now().Add(maxLifetime),
	})
}

// ClearSessionCookie expires the dashboard auth cookie.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request, trustProxy bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   sessionCookieSecure(r, trustProxy),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func sessionCookieSecure(r *http.Request, trustProxy bool) bool {
	if requestScheme(r, trustProxy) == "https" {
		return true
	}
	return !isLoopbackHost(requestHost(r, trustProxy))
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

func requestHost(r *http.Request, trustProxy bool) string {
	if r == nil {
		return ""
	}
	if trustProxy {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
			return strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
		if forwarded := strings.TrimSpace(r.Header.Get("Forwarded")); forwarded != "" {
			for _, part := range strings.Split(forwarded, ";") {
				key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
				if !ok || !strings.EqualFold(key, "host") {
					continue
				}
				return strings.Trim(value, `"`)
			}
		}
	}
	return strings.TrimSpace(r.Host)
}

func isLoopbackHost(host string) bool {
	host = hostOnly(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hostOnly(hostport string) string {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return ""
	}
	if host, port, err := net.SplitHostPort(hostport); err == nil {
		if port != "" {
			return strings.Trim(host, "[]")
		}
	}
	if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		return strings.Trim(hostport, "[]")
	}
	if strings.Count(hostport, ":") > 1 {
		return strings.Trim(hostport, "[]")
	}
	return hostport
}
