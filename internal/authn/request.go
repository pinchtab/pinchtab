package authn

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

type Method string

const (
	MethodNone   Method = ""
	MethodHeader Method = "header"
	MethodCookie Method = "cookie"
)

type Credentials struct {
	Value  string
	Method Method
}

// CookieName is the dashboard auth cookie used for browser APIs that cannot
// attach Authorization headers directly, such as EventSource and WebSocket.
const CookieName = "pinchtab_auth_token"

// CredentialsFromRequest extracts the auth token and the mechanism it arrived on.
// Authorization headers take precedence over the dashboard auth cookie.
func CredentialsFromRequest(r *http.Request) Credentials {
	if r == nil {
		return Credentials{}
	}

	if token := tokenFromAuthorizationHeader(r.Header.Get("Authorization")); token != "" {
		return Credentials{Value: token, Method: MethodHeader}
	}

	cookie, err := r.Cookie(CookieName)
	if err == nil {
		if value := normalizeCookieValue(cookie.Value); value != "" {
			return Credentials{Value: value, Method: MethodCookie}
		}
	}
	if value := cookieValueFromHeaders(r.Header.Values("Cookie"), CookieName); value != "" {
		return Credentials{Value: value, Method: MethodCookie}
	}
	return Credentials{}
}

// TokenFromRequest extracts the bearer token from the request.
// Authorization headers take precedence over the dashboard auth cookie.
func TokenFromRequest(r *http.Request) string {
	return CredentialsFromRequest(r).Value
}

// ClientIP returns the immediate peer IP address for audit and rate-limiting
// decisions. Reverse proxy headers are ignored unless a trusted-proxy model is
// added explicitly.
func ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func tokenFromAuthorizationHeader(auth string) string {
	auth = strings.TrimSpace(auth)
	if auth == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return auth
}

func normalizeCookieValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if decoded, err := url.QueryUnescape(value); err == nil {
		return strings.TrimSpace(decoded)
	}
	return value
}

func cookieValueFromHeaders(headers []string, name string) string {
	for _, header := range headers {
		for _, part := range strings.Split(header, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			key, value, ok := strings.Cut(part, "=")
			if !ok || strings.TrimSpace(key) != name {
				continue
			}
			if normalized := normalizeCookieValue(value); normalized != "" {
				return normalized
			}
		}
	}
	return ""
}
