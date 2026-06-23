package doctor

import (
	"fmt"
	"io"
)

// BrowserLegend explains the overview status markers used by `pinchtab doctor browser`.
const BrowserLegend = "Legend: ✓ ready  ~ needs config  ✗ not available"

// BrowserInstallHints maps a browser name to a one-line install/setup hint shown
// when that browser is not ready. Shared so the doctor commands cannot drift.
var BrowserInstallHints = map[string]string{
	"chrome":       "Install Google Chrome: https://www.google.com/chrome/ or via package manager (apt install google-chrome-stable / brew install --cask google-chrome)",
	"cloak":        "CloakBrowser requires a custom build. See: docs/guides/cloakbrowser.md",
	"ghost-chrome": "ghost-chrome is built-in (uses Chrome with static-first routing). Ensure Chrome is installed.",
}

// CheckMarker renders a per-check status marker (OK/FAIL/WARN/SKIP) for report rows.
func CheckMarker(s CheckStatus) string {
	return statusMarker(s)
}

// CheckDetail returns a check's detail, falling back to its error message when
// the detail is empty.
func CheckDetail(c CheckResult) string {
	detail := c.Detail
	if detail == "" && c.ErrMsg != "" {
		detail = c.ErrMsg
	}
	return detail
}

// WriteBrowserCheckRow writes one indented check row ("    <marker> <name>: <detail>")
// as used by both the browser overview and the browsers report.
func WriteBrowserCheckRow(w io.Writer, c CheckResult) {
	_, _ = fmt.Fprintf(w, "    %s %s: %s\n", CheckMarker(c.Status), c.Name, CheckDetail(c))
}

// BrowserStatusMarker maps a BrowserInfo.Status to the overview symbol:
// ✓ ready, ~ needs config, ✗ anything else (missing/unknown).
func BrowserStatusMarker(status string) string {
	switch status {
	case "ready":
		return "✓"
	case "needs-config":
		return "~"
	default:
		return "✗"
	}
}
