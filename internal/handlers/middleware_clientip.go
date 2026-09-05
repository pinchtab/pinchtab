package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/config"
)

// ClientIPMiddleware makes the trusted-proxy decision once, above every consumer
// of it — the activity recorder, the limiters and every audit line — and carries
// the answer on the request context.
func ClientIPMiddleware(live *config.Live, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := live.Get()
		trustProxy := cfg != nil && cfg.TrustProxyHeaders
		hops := 0
		if cfg != nil {
			hops = cfg.TrustedProxyHops
		}
		ip := authn.ResolveClientIP(r, trustProxy, hops)
		next.ServeHTTP(w, r.WithContext(authn.WithClientIP(r.Context(), ip)))
	})
}
