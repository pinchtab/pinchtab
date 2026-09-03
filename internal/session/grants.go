package session

import (
	"fmt"
	"slices"
	"strings"
)

// The grant names a session may hold. This is the ONE definition, beside the
// route list and for the same reason: the middleware's per-grant matchers and the
// API's validator both derive from it, so a twelfth grant cannot be enforceable
// but unsettable, or settable but unenforced — which is exactly how eleven
// matchers came to exist behind a field nothing could write.
const (
	GrantBrowse    = "browse"
	GrantNetwork   = "network"
	GrantMedia     = "media"
	GrantCookies   = "cookies"
	GrantClipboard = "clipboard"
	GrantEvaluate  = "evaluate"
	GrantStorage   = "storage"
	GrantConsole   = "console"
	GrantSolve     = "solve"
	GrantTasks     = "tasks"
	GrantActivity  = "activity"

	// GrantAll is the explicit spelling of "no narrowing", accepted so a caller
	// can say it rather than having to omit the field. It means what an empty
	// grant list means, and it is not a matcher: nothing enforces it, because
	// there is nothing to narrow.
	GrantAll = "*"
)

var grantNames = []string{
	GrantBrowse,
	GrantNetwork,
	GrantMedia,
	GrantCookies,
	GrantClipboard,
	GrantEvaluate,
	GrantStorage,
	GrantConsole,
	GrantSolve,
	GrantTasks,
	GrantActivity,
}

// GrantNames returns the grants that narrow a session, in a stable order. The
// copy is deliberate: a caller that mutated the slice would change what one
// process enforces without changing what it accepts.
func GrantNames() []string { return slices.Clone(grantNames) }

// NormalizeGrant is how a grant name is read everywhere: case folded and
// trimmed, so a capitalised grant scopes the session rather than silently
// matching no route at all.
func NormalizeGrant(grant string) string {
	return strings.ToLower(strings.TrimSpace(grant))
}

// ValidateGrants normalizes a requested grant list and refuses an unknown name.
// Refusing is the point: a mistyped grant that is dropped silently produces a
// session the caller believes is scoped and which is not, which is a worse
// outcome than the request failing.
func ValidateGrants(grants []string) ([]string, error) {
	out := make([]string, 0, len(grants))
	for _, raw := range grants {
		grant := NormalizeGrant(raw)
		if grant == "" {
			continue
		}
		if grant != GrantAll && !slices.Contains(grantNames, grant) {
			return nil, fmt.Errorf("unknown grant %q (valid grants: %s, or %q for all)",
				raw, strings.Join(grantNames, ", "), GrantAll)
		}
		if !slices.Contains(out, grant) {
			out = append(out, grant)
		}
	}
	return out, nil
}
