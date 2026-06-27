package orchestrator

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/runtimekit"
	"github.com/pinchtab/pinchtab/internal/config"
)

// BrowserUnavailableReason reports whether the request's target browser has no
// resolvable binary, with a user-facing reason. It lets the proxy fail fast and
// clearly instead of polling into a generic "instance not ready after 10s"
// timeout when the real problem is simply that no browser is installed — the
// opaque-503 cold start surfaced by install-UX testing, on the API path as well
// as the CLI.
//
// It deliberately speaks only to the simple default/single-provider case. An
// explicit per-request browser, a multi-target config, or a CDP attach all fall
// through (return false) so existing behavior and HTTP error mapping are
// unchanged. It mirrors the launch-time resolution in bridge/runtime.InitBrowser
// (explicit browser.binary wins over discovery) so it can't diverge from what
// the instance would actually try to launch.
func (o *Orchestrator) BrowserUnavailableReason(r *http.Request) (string, bool) {
	if o == nil || o.runtimeCfg == nil {
		return "", false
	}
	if ExtractRequestedBrowser(r) != "" || len(o.runtimeCfg.Targets) > 0 {
		return "", false
	}
	if strings.TrimSpace(o.runtimeCfg.CDPAttachURL) != "" {
		return "", false // attaching to an external CDP endpoint; no local binary needed
	}
	provider := config.NormalizeBrowser(o.runtimeCfg.DefaultBrowser)
	if _, ok := browsers.Get(provider); !ok {
		return "", false
	}
	if override := strings.TrimSpace(o.runtimeCfg.BrowserBinary); override != "" {
		if info, err := os.Stat(override); err != nil || info.IsDir() {
			return fmt.Sprintf("configured browser.binary does not point at a usable executable: %s — "+
				"set it to a real browser or unset it for auto-discovery (run `pinchtab doctor`)", override), true
		}
		return "", false
	}
	if runtimekit.FindBrowserBinary(provider) == "" {
		return fmt.Sprintf("no %s browser found on this host — install Chrome/Chromium "+
			"(e.g. `apt-get install -y chromium`) or set browser.binary (run `pinchtab doctor`)", provider), true
	}
	return "", false
}
