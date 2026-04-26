package handlers

import (
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/session"
)

func sessionRequestAllowed(r *http.Request, sess *session.Session) bool {
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	path := strings.TrimSpace(r.URL.Path)
	if method == http.MethodPost && sessionRevokePath(path) {
		return true
	}
	if sessionAdminRoute(method, path) {
		return false
	}
	if sess == nil {
		return false
	}

	grants := normalizedSessionGrants(sess.Grants)
	if len(grants) == 0 || grants["*"] {
		return true
	}

	for grant := range grants {
		if sessionGrantAllows(grant, method, path) {
			return true
		}
	}
	return false
}

func normalizedSessionGrants(grants []string) map[string]bool {
	out := make(map[string]bool, len(grants))
	for _, grant := range grants {
		normalized := strings.ToLower(strings.TrimSpace(grant))
		if normalized == "" {
			continue
		}
		out[normalized] = true
	}
	return out
}

func sessionAdminRoute(method, path string) bool {
	switch {
	case method == http.MethodGet && path == "/api/config":
		return true
	case method == http.MethodPut && path == "/api/config":
		return true
	case method == http.MethodPost && path == "/shutdown":
		return true
	case method == http.MethodPost && (path == "/browser/restart" || path == "/ensure-chrome"):
		return true
	case method == http.MethodPost && path == "/fingerprint/rotate":
		return true
	case method == http.MethodGet && path == "/api/events":
		return true
	case method == http.MethodGet && path == "/api/metrics":
		return true
	case method == http.MethodGet && path == "/api/agents":
		return true
	case method == http.MethodGet && strings.HasPrefix(path, "/api/agents/") && !strings.HasSuffix(path, "/events"):
		return true
	case path == "/sessions" || strings.HasPrefix(path, "/sessions/"):
		return path != "/sessions/me"
	case path == "/instances" || strings.HasPrefix(path, "/instances/"):
		return true
	case path == "/profiles" || strings.HasPrefix(path, "/profiles/"):
		return true
	case method == http.MethodGet && path == "/cache/status":
		return true
	case method == http.MethodPost && path == "/cache/clear":
		return true
	default:
		return false
	}
}

func sessionRevokePath(path string) bool {
	if !strings.HasPrefix(path, "/sessions/") || !strings.HasSuffix(path, "/revoke") {
		return false
	}
	sessionID := strings.TrimSuffix(strings.TrimPrefix(path, "/sessions/"), "/revoke")
	return strings.Trim(sessionID, "/") != ""
}

func sessionGrantAllows(grant, method, path string) bool {
	switch grant {
	case "browse":
		return sessionBrowseGrantAllows(method, path)
	case "network":
		return sessionNetworkGrantAllows(method, path)
	case "media":
		return sessionMediaGrantAllows(method, path)
	case "cookies":
		return sessionCookiesGrantAllows(method, path)
	case "clipboard":
		return sessionClipboardGrantAllows(method, path)
	case "evaluate":
		return sessionEvaluateGrantAllows(method, path)
	case "storage":
		return sessionStorageGrantAllows(method, path)
	case "console":
		return sessionConsoleGrantAllows(method, path)
	case "solve":
		return sessionSolveGrantAllows(method, path)
	case "tasks":
		return sessionTasksGrantAllows(method, path)
	case "activity":
		return sessionActivityGrantAllows(method, path)
	default:
		return false
	}
}

func sessionBrowseGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		switch {
		case path == "/tabs",
			path == "/navigate",
			path == "/action",
			path == "/snapshot",
			path == "/screenshot",
			path == "/text",
			path == "/openapi.json",
			path == "/help",
			path == "/health",
			path == "/sessions/me":
			return true
		case tabRouteHasSuffix(path, "/snapshot"),
			tabRouteHasSuffix(path, "/screenshot"),
			tabRouteHasSuffix(path, "/text"),
			tabRouteHasSuffix(path, "/metrics"):
			return true
		}
	case http.MethodPost:
		switch {
		case path == "/tab",
			path == "/navigate",
			path == "/back",
			path == "/forward",
			path == "/reload",
			path == "/action",
			path == "/actions",
			path == "/macro",
			path == "/find",
			path == "/wait",
			path == "/dialog",
			path == "/lock",
			path == "/unlock":
			return true
		case tabRouteHasSuffix(path, "/navigate"),
			tabRouteHasSuffix(path, "/back"),
			tabRouteHasSuffix(path, "/forward"),
			tabRouteHasSuffix(path, "/reload"),
			tabRouteHasSuffix(path, "/action"),
			tabRouteHasSuffix(path, "/actions"),
			tabRouteHasSuffix(path, "/find"),
			tabRouteHasSuffix(path, "/wait"),
			tabRouteHasSuffix(path, "/dialog"),
			tabRouteHasSuffix(path, "/lock"),
			tabRouteHasSuffix(path, "/unlock"):
			return true
		}
	}
	return false
}

func sessionNetworkGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		switch {
		case path == "/network",
			path == "/network/stream",
			path == "/network/export",
			path == "/network/export/stream":
			return true
		case strings.HasPrefix(path, "/network/"):
			return true
		case tabRouteHasSuffix(path, "/network"),
			tabRouteHasSuffix(path, "/network/stream"),
			tabRouteHasSuffix(path, "/network/export"),
			tabRouteHasSuffix(path, "/network/export/stream"):
			return true
		case strings.HasPrefix(path, "/tabs/") && strings.Contains(path, "/network/"):
			return true
		}
	case http.MethodPost:
		return path == "/network/clear"
	}
	return false
}

func sessionMediaGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		switch {
		case path == "/pdf",
			path == "/download",
			path == "/screencast",
			path == "/screencast/tabs":
			return true
		case tabRouteHasSuffix(path, "/pdf"),
			tabRouteHasSuffix(path, "/download"):
			return true
		}
	case http.MethodPost:
		switch {
		case path == "/pdf",
			path == "/upload":
			return true
		case tabRouteHasSuffix(path, "/pdf"),
			tabRouteHasSuffix(path, "/upload"):
			return true
		}
	}
	return false
}

func sessionCookiesGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet, http.MethodPost:
		return path == "/cookies" || tabRouteHasSuffix(path, "/cookies")
	default:
		return false
	}
}

func sessionClipboardGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/clipboard/read" || path == "/clipboard/paste"
	case http.MethodPost:
		return path == "/clipboard/write" || path == "/clipboard/copy"
	default:
		return false
	}
}

func sessionEvaluateGrantAllows(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	return path == "/evaluate" || tabRouteHasSuffix(path, "/evaluate")
}

func sessionStorageGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/storage" || path == "/state/list" || path == "/state/show"
	case http.MethodPost:
		return path == "/storage" || path == "/state/save" || path == "/state/load" || path == "/state/clean"
	case http.MethodDelete:
		return path == "/storage" || path == "/state"
	default:
		return false
	}
}

func sessionConsoleGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/console" || path == "/errors"
	case http.MethodPost:
		return path == "/console/clear" || path == "/errors/clear"
	default:
		return false
	}
}

func sessionSolveGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/solvers" || path == "/config/autosolver"
	case http.MethodPost:
		switch {
		case path == "/solve" || strings.HasPrefix(path, "/solve/"):
			return true
		case tabRouteHasSuffix(path, "/solve") || (strings.HasPrefix(path, "/tabs/") && strings.Contains(path, "/solve/")):
			return true
		}
	}
	return false
}

func sessionTasksGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/tasks" || path == "/scheduler/stats" || strings.HasPrefix(path, "/tasks/")
	case http.MethodPost:
		return path == "/tasks" || path == "/tasks/batch" || (strings.HasPrefix(path, "/tasks/") && strings.HasSuffix(path, "/cancel"))
	default:
		return false
	}
}

func sessionActivityGrantAllows(method, path string) bool {
	switch method {
	case http.MethodGet:
		return path == "/api/activity" || (strings.HasPrefix(path, "/api/agents/") && strings.HasSuffix(path, "/events"))
	case http.MethodPost:
		return strings.HasPrefix(path, "/api/agents/") && strings.HasSuffix(path, "/events")
	default:
		return false
	}
}

func tabRouteHasSuffix(path, suffix string) bool {
	return strings.HasPrefix(path, "/tabs/") && strings.HasSuffix(path, suffix)
}
