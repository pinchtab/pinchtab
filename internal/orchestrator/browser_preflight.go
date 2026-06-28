package orchestrator

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/runtimekit"
)

// BrowserUnavailableReason reports whether the request's target browser has no
// resolvable binary, with a user-facing reason. It lets the proxy fail fast and
// clearly instead of polling into a generic "instance not ready after 10s"
// timeout when the real problem is simply that no browser is installed — the
// opaque-503 cold start surfaced by install-UX testing, on the API path as well
// as the CLI.
//
// It deliberately avoids explicit per-request browser overrides, since those may
// select a different target/provider than the instance's default launch path.
// For the normal implicit path it mirrors the launch-time resolution in
// bridge/runtime.InitBrowser, including default-target promotion, so it can't
// diverge from what the instance would actually try to launch.
func (o *Orchestrator) BrowserUnavailableReason(r *http.Request) (string, bool) {
	if o == nil || o.runtimeCfg == nil {
		return "", false
	}
	if ExtractRequestedBrowser(r) != "" {
		return "", false
	}
	if strings.TrimSpace(o.runtimeCfg.CDPAttachURL) != "" {
		return "", false // attaching to an external CDP endpoint; no local binary needed
	}
	effective := runtimekit.ResolveEffectiveBrowser(o.runtimeCfg)
	provider := effective.ID
	if _, ok := browsers.Get(provider); !ok {
		return "", false
	}
	if override := strings.TrimSpace(effective.Binary); override != "" {
		if info, err := os.Stat(override); err != nil || info.IsDir() {
			return fmt.Sprintf("configured browser executable does not point at a usable executable: %s — "+
				"set the active browser config to a real browser or unset it for auto-discovery (run `pinchtab doctor`)", override), true
		}
		return "", false
	}
	if effective.Binary == "" {
		return fmt.Sprintf("no %s browser found on this host — install Chrome/Chromium "+
			"(e.g. `apt-get install -y chromium`) or set a browser binary in config (run `pinchtab doctor`)", provider), true
	}
	return "", false
}
