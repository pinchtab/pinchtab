package handlers

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

// InternalTokenHeader carries the shared secret on orchestrator → instance
// proxy hops. The instance verifies it against PINCHTAB_INTERNAL_TOKEN and
// marks the request context as trusted-internal-proxy when it matches.
const (
	InternalTokenHeader = "X-PinchTab-Internal-Token"
	InternalTokenEnv    = "PINCHTAB_INTERNAL_TOKEN"
)

type trustCtxKey struct{}

// MarkTrustedInternalProxy returns a context marked as a trusted internal
// proxy hop (orchestrator → instance after auth).
func MarkTrustedInternalProxy(ctx context.Context) context.Context {
	return context.WithValue(ctx, trustCtxKey{}, true)
}

// IsTrustedInternalProxy reports whether the request was marked as a trusted
// internal proxy hop.
func IsTrustedInternalProxy(r *http.Request) bool {
	if r == nil {
		return false
	}
	v, _ := r.Context().Value(trustCtxKey{}).(bool)
	return v
}

// TrustedInternalProxyStripMiddleware combines two responsibilities for
// instance ingress:
//
//  1. If the request carries a valid internal token, mark the context as a
//     trusted-internal-proxy hop. The token header is removed so it never
//     reaches handlers.
//  2. Otherwise, strip every X-PinchTab-* header from the request so a
//     public client cannot spoof internal metadata.
//
// Pass an empty secret to disable trust verification entirely (always
// strips). The secret is compared in constant time.
func TrustedInternalProxyStripMiddleware(secret string) func(http.Handler) http.Handler {
	secretBytes := []byte(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trusted := false
			if len(secretBytes) > 0 {
				if got := r.Header.Get(InternalTokenHeader); got != "" &&
					subtle.ConstantTimeCompare([]byte(got), secretBytes) == 1 {
					trusted = true
				}
			}
			r.Header.Del(InternalTokenHeader)

			if trusted {
				r = r.WithContext(MarkTrustedInternalProxy(r.Context()))
			} else {
				stripPinchtabHeaders(r.Header)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func stripPinchtabHeaders(h http.Header) {
	for name := range h {
		if strings.HasPrefix(http.CanonicalHeaderKey(name), "X-Pinchtab-") {
			h.Del(name)
		}
	}
}
