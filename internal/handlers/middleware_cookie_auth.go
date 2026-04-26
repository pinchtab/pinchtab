package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

func isPublicDashboardPath(path string) bool {
	switch path {
	case "/", "/login", "/dashboard", "/dashboard/":
		return true
	}
	return strings.HasPrefix(path, "/dashboard/") || path == "/dashboard/favicon.png"
}

func isPublicAuthPath(path string) bool {
	switch path {
	case "/api/auth/login", "/api/auth/logout":
		return true
	default:
		return false
	}
}

func cookieAuthAllowed(r *http.Request) bool {
	path := strings.TrimSpace(r.URL.Path)
	switch r.Method {
	case http.MethodGet:
		switch {
		case path == "/health",
			path == "/metrics",
			path == "/api/metrics",
			path == "/api/activity",
			path == "/api/agents",
			path == "/api/events",
			path == "/api/config",
			path == "/sessions",
			strings.HasPrefix(path, "/sessions/"),
			path == "/profiles",
			path == "/instances",
			path == "/instances/tabs",
			path == "/instances/metrics":
			return true
		case strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/tabs"),
			strings.HasPrefix(path, "/api/agents/") && !strings.HasSuffix(path, "/events"),
			strings.HasPrefix(path, "/api/agents/") && strings.HasSuffix(path, "/events"),
			strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/logs"),
			strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/logs/stream"),
			strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/proxy/screencast"),
			strings.HasPrefix(path, "/tabs/") && strings.HasSuffix(path, "/screenshot"),
			strings.HasPrefix(path, "/tabs/") && strings.HasSuffix(path, "/pdf"):
			return true
		}
	case http.MethodPost:
		switch {
		case path == "/api/auth/elevate":
			return true
		case strings.HasPrefix(path, "/api/agents/") && strings.HasSuffix(path, "/events"):
			return true
		case path == "/action":
			return true
		case path == "/instances/start":
			return true
		case path == "/instances/launch":
			return true
		case strings.HasPrefix(path, "/tabs/") && strings.HasSuffix(path, "/close"):
			return true
		case strings.HasPrefix(path, "/instances/") && strings.HasSuffix(path, "/stop"):
			return true
		case path == "/profiles":
			return true
		}
	case http.MethodPut:
		return path == "/api/config"
	case http.MethodPatch:
		return strings.HasPrefix(path, "/profiles/")
	case http.MethodDelete:
		return strings.HasPrefix(path, "/profiles/")
	}
	return false
}

func cookieElevationRequired(r *http.Request, cfg *config.RuntimeConfig) bool {
	if cfg == nil || !cfg.Sessions.Dashboard.RequireElevation {
		return false
	}
	path := strings.TrimSpace(r.URL.Path)
	switch r.Method {
	case http.MethodPut:
		return path == "/api/config"
	case http.MethodPost:
		return path == "/shutdown"
	}
	return false
}

func cookieOriginAllowed(r *http.Request, trustProxy bool) bool {
	if isWebSocketUpgrade(r) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		return origin != "" && sameOriginRequest(origin, r, trustProxy)
	}

	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return sameOriginRequest(origin, r, trustProxy)
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		return sameOriginRequest(referer, r, trustProxy)
	}
	return false
}

func cookieSecureSetting(cfg *config.RuntimeConfig) *bool {
	if cfg == nil {
		return nil
	}
	return cfg.CookieSecure
}

func sameOriginRequest(origin string, r *http.Request, trustProxy ...bool) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	trust := len(trustProxy) > 0 && trustProxy[0]
	return strings.EqualFold(parsed.Scheme, requestScheme(r, trust)) && strings.EqualFold(parsed.Host, requestHost(r, trust))
}

func isWebSocketUpgrade(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
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
