package handlers

import (
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func CorsMiddleware(live *config.Live, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := live.Get()
		allowedOrigin := corsAllowedOrigin(cfg, r)
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			if allowedOrigin != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Add("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == "OPTIONS" {
			if strings.TrimSpace(r.Header.Get("Origin")) != "" && allowedOrigin == "" && cfg != nil && strings.TrimSpace(cfg.Token) != "" {
				httpx.ErrorCode(w, 403, "cors_forbidden", "cross-origin requests are disabled when auth is enabled", false, nil)
				return
			}
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func corsAllowedOrigin(cfg *config.RuntimeConfig, r *http.Request) string {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || cfg == nil {
		return ""
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return "*"
	}
	if sameOriginRequest(origin, r, cfg.TrustProxyHeaders) {
		return origin
	}
	return ""
}
