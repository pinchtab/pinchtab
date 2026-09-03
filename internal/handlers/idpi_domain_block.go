package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/remedy"
	"github.com/pinchtab/pinchtab/internal/security"
)

// idpiDomainBlockedCode is the one code every IDPI domain block reports, so a
// client can switch on it whether the tab drifted, a requested URL was refused,
// or a navigation was blocked. The nav site used to answer with the generic
// "error" code, which left the most common way to hit the allowlist as the one
// case a consumer could not classify.
const idpiDomainBlockedCode = "idpi_domain_blocked"

// The drifted-tab block describes where the tab IS, not what the caller asked
// for, so its remedy must not name the allowlist: widening it is precisely the
// change the operator's allowlist exists to prevent, and an agent that follows
// that lead erodes the posture instead of recovering. Going back does recover —
// no navigation or history endpoint consults this guard, so `back` can always
// reach a tab the read verbs have locked out.
const idpiDriftedTabHint = "the tab navigated to a domain outside security.allowedDomains, so this page was never read — the block describes where the tab is, not what you asked for. Do not widen the allowlist to get past it. Going back returns the tab to the previous allowed page; when there is no history entry, navigate to an allowed URL or close the tab."

var idpiDriftedTabRemedy = remedy.Declare("pinchtab back")

// A refused target URL is the opposite case: nothing navigated, there is nothing
// to recover from, and the allowlist genuinely is the only lever — so this is the
// only remedy that names it, and the only place that guidance is rendered.
const idpiRefusedURLHint = "the requested URL is outside security.allowedDomains, so the request was refused and nothing navigated. Allowing it widens what automation may reach — see docs/guides/security.md."

// writeIDPIDomainBlocked is the single writer of an IDPI domain-block refusal, so
// the status and code cannot differ between the three sites that produce one.
func writeIDPIDomainBlocked(w http.ResponseWriter, message string, details map[string]any) {
	httpx.ErrorCode(w, http.StatusForbidden, idpiDomainBlockedCode, message, false, details)
}

func idpiDriftedTabDetails(url string) map[string]any {
	return idpiBlockDetails(url, idpiDriftedTabHint, idpiDriftedTabRemedy.Remedy())
}

func idpiRefusedURLDetails(url string) map[string]any {
	return idpiBlockDetails(url, idpiRefusedURLHint, idpiAllowlistRemedy(url))
}

func idpiBlockDetails(url, hint string, r remedy.Remedy) map[string]any {
	details := remedy.Details(hint, r)
	details["url"] = url
	// The domain is a discrete field as well as prose so MCP and dashboard
	// consumers do not have to parse the sentence to learn what was blocked.
	if host := security.ExtractHost(url); host != "" {
		details["domain"] = host
	}
	return details
}

// allowlistWidening appends to the current allowlist rather than replacing it, and carries
// the restart because the security block is snapshotted at boot.
var allowlistWidening = remedy.Declare(
	`pinchtab config set security.allowedDomains "$(pinchtab config get security.allowedDomains),<domain>" && pinchtab server restart`)

// idpiAllowlistRemedy is the copy-pasteable widening of the allowlist. A hostless target
// (about:blank) cannot be allowlisted at all, so it gets no remedy instead of one that
// cannot work.
func idpiAllowlistRemedy(url string) remedy.Remedy {
	host := security.ExtractHost(url)
	if host == "" {
		return remedy.None
	}
	return allowlistWidening.Fill(host)
}
