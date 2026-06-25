package doctor

import (
	"context"
	"sort"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
)

type BrowserInfo struct {
	Name         string        `json:"name"`
	Registered   bool          `json:"registered"`
	Configured   bool          `json:"configured"`
	IsDefault    bool          `json:"isDefault,omitempty"`
	Status       string        `json:"status"`       // "ready", "missing", "needs-config"
	StatusDetail string        `json:"statusDetail"` // one-line summary
	Checks       []CheckResult `json:"checks,omitempty"`
	Handles      []string      `json:"handles,omitempty"`
	SkipsOrFails []string      `json:"skipsOrFails,omitempty"`
}

type BrowsersReport struct {
	ConfiguredBrowsers []string      `json:"configuredBrowsers"`
	DefaultBrowser     string        `json:"defaultBrowser"`
	KnownBrowsers      []string      `json:"knownBrowsers"`
	Browsers           []BrowserInfo `json:"browsers"`
}

var allShapes = []string{
	browsers.ShapeStaticRead,
	browsers.ShapeStaticSnapshot,
	browsers.ShapeRenderedRead,
	browsers.ShapeVisual,
	browsers.ShapeInteraction,
	browsers.ShapeSessionState,
	browsers.ShapeNetworkControl,
	browsers.ShapeDownloadUpload,
}

// ReportBrowsers builds a BrowsersReport by merging the browser registry with
// the runtime configuration. It is read-only and never modifies the provided
// config or enables additional browsers. It runs doctor checks and probes
// CanHandle for every registered browser.
func ReportBrowsers(ctx context.Context, cfg *config.RuntimeConfig) BrowsersReport {
	known := browsers.IDs()
	configured := cfgBrowsersAvailable(cfg)
	defaultBrowser := cfgDefaultBrowser(cfg)

	// Build a deduplicated union of configured + known browser IDs; the
	// sort below makes the report order deterministic.
	seen := map[string]bool{}
	var union []string
	for _, id := range configured {
		if !seen[id] {
			seen[id] = true
			union = append(union, id)
		}
	}
	for _, id := range known {
		if !seen[id] {
			seen[id] = true
			union = append(union, id)
		}
	}
	sort.Strings(union)

	infos := make([]BrowserInfo, 0, len(union))
	for _, id := range union {
		info := BrowserInfo{
			Name:       id,
			Configured: contains(configured, id),
			IsDefault:  id == defaultBrowser,
		}

		b, ok := browsers.Get(id)
		if ok {
			info.Registered = true

			env := doctorEnvForBrowser(cfg, id)
			for _, dc := range b.DoctorChecks(browsers.TargetConfig{Provider: id}) {
				info.Checks = append(info.Checks, browserCheckResult(ctx, dc, env))
			}

			for _, shape := range allShapes {
				d := b.CanHandle(browsers.RequestIntent{Shape: shape})
				if d.Decision == browsers.DecisionHandle {
					info.Handles = append(info.Handles, shape)
				} else {
					info.SkipsOrFails = append(info.SkipsOrFails, shape)
				}
			}
		}

		deriveBrowserStatus(&info)
		infos = append(infos, info)
	}

	return BrowsersReport{
		ConfiguredBrowsers: configured,
		DefaultBrowser:     defaultBrowser,
		KnownBrowsers:      known,
		Browsers:           infos,
	}
}

func deriveBrowserStatus(info *BrowserInfo) {
	if !info.Registered {
		info.Status = "missing"
		info.StatusDetail = "not a known browser"
		return
	}

	for _, c := range info.Checks {
		if c.Status == StatusFail {
			info.Status = "missing"
			info.StatusDetail = c.Detail
			if info.StatusDetail == "" {
				info.StatusDetail = c.ErrMsg
			}
			return
		}
	}

	for _, c := range info.Checks {
		if c.Status == StatusWarn {
			info.Status = "needs-config"
			info.StatusDetail = c.Detail
			return
		}
	}

	info.Status = "ready"
	for _, c := range info.Checks {
		if c.Status == StatusPass && c.Detail != "" {
			info.StatusDetail = c.Detail
			return
		}
	}

	// Special case for ghost-chrome or composed browsers.
	if info.Name == "ghost-chrome" {
		info.StatusDetail = "ghost -> chrome"
	} else if info.StatusDetail == "" {
		info.StatusDetail = "registered"
	}
}

func cfgBrowsersAvailable(cfg *config.RuntimeConfig) []string {
	if cfg == nil || len(cfg.BrowsersAvailable) == 0 {
		return nil
	}
	return cfg.BrowsersAvailable
}

func cfgDefaultBrowser(cfg *config.RuntimeConfig) string {
	return defaultBrowserForDoctor(cfg)
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func errMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// doctorEnvForBrowser resolves the provider's effective config (its target's
// binary/cloak settings) before building the check env — checking cloak
// against the global chrome binary yields meaningless PASS/FAIL results.
// Providers without a configured target fall back to the global config.
func doctorEnvForBrowser(cfg *config.RuntimeConfig, browserID string) *browsers.DoctorEnv {
	if cfg == nil {
		return nil
	}
	matches := config.TargetsForBrowser(cfg, browserID)
	name := ""
	switch len(matches) {
	case 0:
	case 1:
		name = matches[0]
	default:
		dt := config.ResolveDefaultTarget(cfg)
		for _, m := range matches {
			if m == dt {
				name = dt
				break
			}
		}
	}
	if name != "" {
		if resolved, err := config.ResolveExplicitBrowserTarget(cfg, name); err == nil {
			return buildDoctorEnv(resolved.Config)
		}
	}
	return buildDoctorEnv(cfg)
}
