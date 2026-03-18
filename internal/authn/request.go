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
	if err != nil {
		return Credentials{}
	}

	value := strings.TrimSpace(cookie.Value)
	if value == "" {
		return Credentials{}
	}
	if decoded, err := url.QueryUnescape(value); err == nil {
		return Credentials{Value: strings.TrimSpace(decoded), Method: MethodCookie}
	}
	return Credentials{Value: value, Method: MethodCookie}
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
