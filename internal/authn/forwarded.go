package authn

import (
	"net/http"
	"strings"
)

// RequestScheme reports the scheme the client used to reach the server. When
// trustProxy is set, forwarded proxy headers take precedence over the local
// connection state so terminated-TLS deployments are detected correctly.
//
// This is the single source of truth for "should this cookie be Secure?" and
// "is this cookie-backed request same-origin?" so trusted-proxy behavior cannot
// drift between the two decisions.
func RequestScheme(r *http.Request, trustProxy bool) string {
	if r == nil {
		return "http"
	}
	if trustProxy {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
			return strings.ToLower(firstForwardedValue(forwarded))
		}
		if proto := forwardedDirective(r.Header.Get("Forwarded"), "proto"); proto != "" {
			return strings.ToLower(proto)
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// RequestHost reports the host the client used to reach the server, honoring
// forwarded proxy headers when trustProxy is set.
func RequestHost(r *http.Request, trustProxy bool) string {
	if r == nil {
		return ""
	}
	if trustProxy {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
			return firstForwardedValue(forwarded)
		}
		if host := forwardedDirective(r.Header.Get("Forwarded"), "host"); host != "" {
			return host
		}
	}
	return strings.TrimSpace(r.Host)
}

// firstForwardedValue returns the first, client-most entry of a comma-separated
// X-Forwarded-* header.
func firstForwardedValue(header string) string {
	return strings.TrimSpace(strings.Split(header, ",")[0])
}

// forwardedDirective extracts a single directive (e.g. "proto" or "host") from
// the RFC 7239 Forwarded header, returning the first match unquoted.
func forwardedDirective(header, name string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !strings.EqualFold(key, name) {
			continue
		}
		return strings.Trim(value, `"`)
	}
	return ""
}
