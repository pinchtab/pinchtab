// Package ghostchrome registers a Ghost+Chrome stub browser provider.
// It delegates capability and discovery queries to the Chrome base but
// refuses to launch, serving as a placeholder for future implementation.
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

// Attempt records a single browser that was considered during routing.
type Attempt struct {
	Browser  string
	Accepted bool
	Reason   string
}

// RouteResult carries the routing decision from ghost-chrome.
type RouteResult struct {
	UsedBrowser string       // "ghost" or "chrome"
	Escalated   bool         // true if Ghost was tried but rejected
	GhostResult *GhostResult // non-nil when Ghost was tried
	Quality     int
	Attempts    []Attempt
}

// RouteRequest holds the inputs for a routing decision.
type RouteRequest struct {
	Ctx    context.Context
	Lite   StaticFetcher
	URL    string
	Intent browsers.RequestIntent
}

// Route orchestrates Ghost → Chrome escalation. Ghost is tried first for
// request shapes it can handle; if the result passes ShouldAccept the Ghost
// result is returned. Otherwise the request escalates to Chrome. Shapes
// that Ghost cannot handle skip straight to Chrome with no Ghost attempt.
func (b *Browser) Route(req RouteRequest) RouteResult {
	ghost := &GhostAdapter{}
	d := ghost.CanHandle(req.Intent)

	// If Ghost can't handle this shape, go straight to Chrome.
	if d.Decision != browsers.DecisionHandle {
		return RouteResult{
			UsedBrowser: "chrome",
			Attempts: []Attempt{
				{Browser: "ghost", Accepted: false, Reason: d.Reason},
				{Browser: "chrome", Accepted: true},
			},
		}
	}

	// Try Ghost.
	gr := ghost.Try(req.Ctx, req.Lite, req.URL)

	if gr.ShouldAccept() {
		return RouteResult{
			UsedBrowser: "ghost",
			GhostResult: gr,
			Quality:     gr.Quality,
			Attempts: []Attempt{
				{Browser: "ghost", Accepted: true, Reason: gr.FormatReason()},
			},
		}
	}

	// Escalate to Chrome.
	return RouteResult{
		UsedBrowser: "chrome",
		Escalated:   true,
		GhostResult: gr,
		Quality:     gr.Quality,
		Attempts: []Attempt{
			{Browser: "ghost", Accepted: false, Reason: gr.FormatReason()},
			{Browser: "chrome", Accepted: true, Reason: "escalated from ghost"},
		},
	}
}

func init() { browsers.Register(&Browser{}) }
