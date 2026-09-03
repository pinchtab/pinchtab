package authn

import (
	"context"
	"net"
	"net/http"
	"strings"
)

const forwardedForDirective = "for"

type clientIPKey struct{}

// resolvedClientIP distinguishes "the chain resolved a value" from "nothing ran",
// which a bare string on the context cannot: absent must fall back to the peer
// address, and a resolved empty value must not be mistaken for it.
type resolvedClientIP struct{ value string }

// ResolveClientIP adjudicates the client identity for a request. It is the one
// place a forwarding header is read for that decision; every consumer reads the
// answer back through ClientIP.
func ResolveClientIP(r *http.Request, trustProxy bool) string {
	if r == nil {
		return ""
	}
	if trustProxy {
		if forwarded := forwardedClientIP(r); forwarded != "" {
			return forwarded
		}
	}
	return peerIP(r)
}

func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, resolvedClientIP{value: ip})
}

func forwardedClientIP(r *http.Request) string {
	if header := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); header != "" {
		if ip := forwardedIP(firstForwardedValue(header)); ip != "" {
			return ip
		}
	}
	return forwardedIP(forwardedDirective(r.Header.Get("Forwarded"), forwardedForDirective))
}

func forwardedIP(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil && host != "" {
		value = host
	}
	return strings.Trim(value, "[]")
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
