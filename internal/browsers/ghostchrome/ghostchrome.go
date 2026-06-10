// Package ghostchrome registers the ghost-chrome browser provider: a
// static-first browser that serves reads from a lightweight gost-dom fetcher
// and escalates to Chrome when a page needs rendering. Launch args, binary
// discovery, and capability queries delegate to the Chrome base; the
// static-first routing lives in the bridgekit sub-package.
package ghostchrome

import (
	"context"
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

// Browser implements browsers.Browser as a stub over Chrome.
type Browser struct {
	chrome chrome.Browser
}

func (Browser) ID() string          { return "ghost-chrome" }
func (Browser) DisplayName() string { return "Ghost + Chrome" }

func (b Browser) Capabilities() browsers.CapabilitySet {
	return b.chrome.Capabilities()
}

func (b Browser) DiscoverBinary() browsers.BinaryDiscovery {
	return b.chrome.DiscoverBinary()
}

func (b *Browser) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck {
	return []browsers.DoctorCheck{
		{
			ID:          "handle_decisions",
			Description: "Verify ghost-chrome handle/skip behavior and state-changing safety",
			Fn: func(ctx context.Context, cfg interface{}) browsers.DoctorCheckResult {
				// Static shapes should be handled
				staticShapes := []string{browsers.ShapeStaticRead, browsers.ShapeStaticSnapshot}
				// Interactive shapes should be skipped
				skipShapes := []string{
					browsers.ShapeRenderedRead, browsers.ShapeVisual,
					browsers.ShapeInteraction, browsers.ShapeSessionState,
					browsers.ShapeNetworkControl, browsers.ShapeDownloadUpload,
				}

				var issues []string

				for _, shape := range staticShapes {
					d := b.CanHandle(browsers.RequestIntent{Shape: shape})
					if d.Decision != browsers.DecisionHandle {
						issues = append(issues, fmt.Sprintf("%s: expected handle, got %s", shape, d.Decision))
					}
				}

				for _, shape := range skipShapes {
					d := b.CanHandle(browsers.RequestIntent{Shape: shape})
					if d.Decision != browsers.DecisionSkip {
						issues = append(issues, fmt.Sprintf("%s: expected skip, got %s", shape, d.Decision))
					}
				}

				// State-changing safety: static-read with StateChanging=true should be skipped
				d := b.CanHandle(browsers.RequestIntent{Shape: browsers.ShapeStaticRead, StateChanging: true})
				if d.Decision != browsers.DecisionSkip {
					issues = append(issues, fmt.Sprintf("state-changing static-read: expected skip, got %s", d.Decision))
				}

				if len(issues) > 0 {
					return browsers.DoctorCheckResult{
						Status: browsers.DoctorWarn,
						Detail: strings.Join(issues, "; "),
					}
				}
				return browsers.DoctorCheckResult{
					Status: browsers.DoctorPass,
					Detail: "static shapes handled, interactive shapes skipped, state-changing safety enforced",
				}
			},
		},
	}
}

func (b Browser) BuildLaunchArgs(cfg browsers.LaunchConfig) ([]string, []string, error) {
	return b.chrome.BuildLaunchArgs(cfg)
}

func (Browser) SupportsRemoteCDP() bool { return true }

func (b Browser) GeoAlignment(geo browsers.GeoConfig) browsers.GeoStrategy {
	return b.chrome.GeoAlignment(geo)
}

func (Browser) ValidateTarget(_ browsers.TargetConfig) error { return nil }

func (Browser) ClassifyLaunchError(_ browsers.LaunchFailure) browsers.LaunchErrorKind {
	return browsers.LaunchErrorUnknown
}

func ghostCanHandle(intent browsers.RequestIntent) browsers.HandleDecision {
	if intent.StateChanging {
		return browsers.HandleDecision{
			Decision: browsers.DecisionSkip,
			Reason:   "state-changing requests require a full browser",
		}
	}
	switch intent.Shape {
	case browsers.ShapeStaticRead, browsers.ShapeStaticSnapshot:
		return browsers.HandleDecision{Decision: browsers.DecisionHandle}
	default:
		return browsers.HandleDecision{
			Decision: browsers.DecisionSkip,
			Reason:   "ghost requires Chrome for " + intent.Shape,
		}
	}
}

func (b *Browser) CanHandle(intent browsers.RequestIntent) browsers.HandleDecision {
	return ghostCanHandle(intent)
}

func (Browser) NewRuntimeInstance(browserCtx context.Context, headless bool) browsers.RuntimeInstance {
	return NewInstance(browserCtx, headless)
}

func init() { browsers.Register(&Browser{}) }
