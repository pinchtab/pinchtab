package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

// scanInspectContentForIDPI runs the IDPI prompt-injection scan over the page
// content the inspect family returns.
//
// /text, /snapshot, /find, /pdf, /scrape and /capture all pass page-derived
// content through the scanner. /html and /styles did not, which left the
// scanner trivially bypassable: on a page whose text /text refuses with a 403,
// asking for the markup instead returned the same payload verbatim, with no
// scan, no advisory header, and no trust-boundary notice.
//
// Scans without wrapping, matching scanFindCorpusForIDPI. Wrapping raw markup
// in the trust-boundary markers would leave the html field unparseable for
// callers that hand it to an HTML parser, and blocking plus the advisory
// headers are the half that closes the bypass. In strict mode a detected threat
// blocks the request (writes HTTP 403 and returns blocked=true); otherwise the
// response headers and the returned warning carry the advisory. A no-op
// (returns "", false) when IDPI content scanning is disabled.
func (h *Handlers) scanInspectContentForIDPI(w http.ResponseWriter, kind inspectKind, corpus string) (warning string, blocked bool) {
	if !h.Config.IDPI.Enabled || !h.Config.IDPI.ScanContent {
		return "", false
	}
	if strings.TrimSpace(corpus) == "" {
		return "", false
	}

	result := h.ContentGuard.ScanOnly(corpus)
	if result.Blocked {
		httpx.Error(w, http.StatusForbidden,
			fmt.Errorf("%s blocked by IDPI scanner: %s%s", kind, result.BlockReason, idpiScannerHint()))
		return "", true
	}
	result.SetHeaders(w)
	return result.Warning, false
}

// inspectTrustBoundary attaches the standing boundary to the kinds that
// disclose page-derived content, the same two the scanner reads; /title and
// /url carry neither a scan nor a boundary, for the reason inspectScanCorpus
// records.
func (h *Handlers) inspectTrustBoundary(kind inspectKind) trustBoundary {
	if kind != inspectKindHTML && kind != inspectKindStyles {
		return trustBoundary{}
	}
	return h.trustBoundary()
}

// inspectScanCorpus returns the page-derived text an inspect response would
// disclose, or "" for the kinds that disclose none.
//
// /title and /url are deliberately excluded. Both are already carried in the
// header line of a /snapshot response, which is scanned, and blocking a request
// for the current URL would break navigation bookkeeping for a payload the
// caller can neither read at length nor act on.
func inspectScanCorpus(kind inspectKind, payload inspectPayload) string {
	switch kind {
	case inspectKindHTML:
		return payload.HTML
	case inspectKindStyles:
		return styleValuesCorpus(payload.Styles)
	default:
		return ""
	}
}

// styleValuesCorpus flattens computed style values into a scannable corpus.
// Property names are fixed CSS identifiers and cannot carry a payload; the
// values can, via content: strings and url() targets.
func styleValuesCorpus(styles map[string]any) string {
	if len(styles) == 0 {
		return ""
	}
	keys := make([]string, 0, len(styles))
	for k := range styles {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		if v, ok := styles[k].(string); ok && v != "" {
			sb.WriteString(v)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
