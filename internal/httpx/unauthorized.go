package httpx

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/session"
)

const (
	CodeMissingToken = "missing_token"
	CodeBadToken     = "bad_token"
	CodeBadSession   = "bad_session"
)

func UnauthorizedSession(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Session realm=%q, error=%q", "pinchtab", CodeBadSession))
	ErrorCode(w, http.StatusUnauthorized, CodeBadSession, "invalid or expired agent session", false, nil)
}

// Unauthorized is the one producer of the bearer 401 pair — the WWW-Authenticate
// header and the coded body — for every missing_token/bad_token site. presented
// is the credential the caller actually sent under the Bearer scheme, when one
// was sent as a header value; it selects the wrong-scheme guidance and is never
// echoed back.
func Unauthorized(w http.ResponseWriter, code, presented string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer realm=%q, error=%q", "pinchtab", code))
	ErrorCode(w, http.StatusUnauthorized, code, "unauthorized", false, map[string]any{
		"hint": UnauthorizedHint(code, presented),
	})
}

// UnauthorizedHint derives the guidance from what is known: a bearer value
// carrying the agent-session prefix is unambiguously a session token under the
// wrong scheme, an empty credential wants PINCHTAB_TOKEN named, and a rejected
// one wants the it-belongs-to-another-host cue. No remedy anywhere: a
// credential's value is a secret the product cannot spell into a runnable line.
func UnauthorizedHint(code, presented string) string {
	if strings.HasPrefix(strings.TrimSpace(presented), session.IDPrefix) {
		return "that looks like an agent session token; send it as 'Authorization: Session <token>', not Bearer"
	}
	if code == CodeMissingToken {
		return "no credential was sent; set PINCHTAB_TOKEN — it pairs with a remote destination on one line: PINCHTAB_TOKEN=<token> pinchtab --server <url> ..."
	}
	return "this host rejected the token; when targeting a remote server with --server, the token must belong to that host — otherwise the CLI sends PINCHTAB_TOKEN or the local config's server.token"
}
