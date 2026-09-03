package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/remedy"
	"github.com/pinchtab/pinchtab/internal/session"
)

// sessionScopeRefusal says WHICH of the two causes refused the request. The 403
// presented two causes and could only ever have one, so it carried neither; with
// grants reachable an operator has to be able to tell "this credential may never
// do that" from "this session was not given that grant", because the remedies are
// a different credential and a different session.
type sessionScopeRefusal struct {
	hint   string
	remedy remedy.Remedy
}

// The admin case has no remedy line: the fix is to send a different credential on
// whatever verb the caller wanted, which is not one command, and a remedy naming a
// command the caller did not ask for is worse than none. The scope case has one — a
// session that carries the grant — so that one is published as a command.
var (
	grantedSessionCmd   = remedy.Declare("pinchtab session create --agent-id <id> --grant <grant>")
	adminRouteFormat    = "admin verbs are not available to an agent session whatever its grants: this request authenticated with PINCHTAB_SESSION, and %s %s needs the server token"
	outsideGrantsFormat = "this session holds %s, and %s %s is outside them"
	coveringGrantFormat = "; the grant that covers it is %q"
	noCoveringGrantMsg  = "; no grant covers it, so it is reachable only with the server token"
)

func sessionRequestAllowed(r *http.Request, sess *session.Session) bool {
	_, refused := sessionRequestRefusal(r, sess)
	return !refused
}

// sessionRequestRefusal is the one decision and its reason. Returning the reason
// beside the verdict is what stops the two from drifting: a refusal that has to be
// re-derived by the caller describes the rule as the caller remembers it.
func sessionRequestRefusal(r *http.Request, sess *session.Session) (sessionScopeRefusal, bool) {
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	path := strings.TrimSpace(r.URL.Path)
	if method == http.MethodPost && sessionRevokePath(path) {
		return sessionScopeRefusal{}, false
	}
	if sessionAdminRoute(method, path) {
		return sessionScopeRefusal{
			hint:   fmt.Sprintf(adminRouteFormat, method, path),
			remedy: remedy.None,
		}, true
	}
	if sess == nil {
		return sessionScopeRefusal{hint: fmt.Sprintf(adminRouteFormat, method, path), remedy: remedy.None}, true
	}

	grants := normalizedSessionGrants(sess.Grants)
	if len(grants) == 0 || grants[session.GrantAll] {
		return sessionScopeRefusal{}, false
	}

	for grant := range grants {
		if sessionGrantAllows(grant, method, path) {
			return sessionScopeRefusal{}, false
		}
	}
	return sessionScopeRefusal{
		hint:   outsideGrantsRefusalHint(grants, method, path),
		remedy: grantedSessionCmd.Remedy(),
	}, true
}

// outsideGrantsRefusalHint names the grants the session holds and, when one exists, the
// grant that would have covered the route — the caller cannot look that mapping up
// from outside, and a refusal they cannot act on is retried unchanged.
func outsideGrantsRefusalHint(held map[string]bool, method, path string) string {
	names := make([]string, 0, len(held))
	for grant := range held {
		names = append(names, grant)
	}
	sort.Strings(names)

	hint := fmt.Sprintf(outsideGrantsFormat, strings.Join(names, ", "), method, path)
	if covering := grantCovering(method, path); covering != "" {
		return hint + fmt.Sprintf(coveringGrantFormat, covering)
	}
	return hint + noCoveringGrantMsg
}

// grantCovering returns the first grant whose matcher admits this route, walking
// the canonical list so the answer cannot name a grant the API would refuse.
func grantCovering(method, path string) string {
	for _, grant := range session.GrantNames() {
		if sessionGrantAllows(grant, method, path) {
			return grant
		}
	}
	return ""
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
	case method == http.MethodPost && (path == "/browser/restart" || path == "/ensure-browser"):
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

// sessionGrantMatchers binds each grant NAME to the routes it admits. It is a map
// keyed by the vocabulary's own constants rather than a switch over string
// literals, so the enforced set can be compared with the settable set instead of
// being trusted to match it.
var sessionGrantMatchers = map[string]func(method, path string) bool{
	session.GrantBrowse:    sessionBrowseGrantAllows,
	session.GrantNetwork:   sessionNetworkGrantAllows,
	session.GrantMedia:     sessionMediaGrantAllows,
	session.GrantCookies:   sessionCookiesGrantAllows,
	session.GrantClipboard: sessionClipboardGrantAllows,
	session.GrantEvaluate:  sessionEvaluateGrantAllows,
	session.GrantStorage:   sessionStorageGrantAllows,
	session.GrantConsole:   sessionConsoleGrantAllows,
	session.GrantSolve:     sessionSolveGrantAllows,
	session.GrantTasks:     sessionTasksGrantAllows,
	session.GrantActivity:  sessionActivityGrantAllows,
}

func sessionGrantAllows(grant, method, path string) bool {
	matcher, ok := sessionGrantMatchers[grant]
	return ok && matcher(method, path)
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
			path == "/screencast/tabs",
			path == "/record/status":
			return true
		case tabRouteHasSuffix(path, "/pdf"),
			tabRouteHasSuffix(path, "/download"):
			return true
		}
	case http.MethodPost:
		switch {
		case path == "/pdf",
			path == "/upload",
			path == "/record/start",
			path == "/record/stop":
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
		return path == "/storage" || path == "/state" || path == "/state/list" || path == "/state/show"
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
