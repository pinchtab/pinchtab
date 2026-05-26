package routing

import (
	"context"
	"fmt"

	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
)

// Request carries the inputs for browser routing.
type Request struct {
	Ctx         context.Context
	URL         string
	Intent      browsers.RequestIntent
	Browser     string                    // resolved browser name
	LiteFetcher ghostchrome.StaticFetcher // nil if static browser unavailable
}

// Result carries the routing decision.
type Result struct {
	UsedBrowser string
	Escalated   bool
	Quality     int
	Route       *browserops.RouteMetadata
	GhostResult *ghostchrome.GhostResult // non-nil when ghost was tried
}

// Route evaluates browser routing for a request. It looks up the browser,
// checks CanHandle, and delegates to composed browser logic when applicable.
func Route(req Request) (Result, error) {
	b, ok := browsers.Get(req.Browser)
	if !ok {
		return Result{
			UsedBrowser: req.Browser,
			Route:       browserops.SingleBrowserRoute(req.Browser),
		}, nil
	}

	d := b.CanHandle(req.Intent)
	switch d.Decision {
	case browsers.DecisionFail:
		return Result{}, fmt.Errorf("browser %q failed: %s", req.Browser, d.Reason)
	case browsers.DecisionSkip:
		return Result{
			UsedBrowser: "chrome",
			Escalated:   true,
			Route: &browserops.RouteMetadata{
				RequestedBrowser: req.Browser,
				UsedBrowser:      "chrome",
				Escalated:        true,
				FallbackAttempts: 1,
				Attempts: []browserops.RouteAttempt{
					{Browser: req.Browser, Accepted: false, Reason: d.Reason},
					{Browser: "chrome", Accepted: true, Reason: "fallback from skip"},
				},
			},
		}, nil
	}

	if gc, ok := b.(*ghostchrome.Browser); ok {
		gr := gc.Route(ghostchrome.RouteRequest{
			Ctx:    req.Ctx,
			Lite:   req.LiteFetcher,
			URL:    req.URL,
			Intent: req.Intent,
		})
		fallback := 0
		if gr.Escalated {
			fallback = 1
		}
		return Result{
			UsedBrowser: gr.UsedBrowser,
			Escalated:   gr.Escalated,
			Quality:     gr.Quality,
			GhostResult: gr.GhostResult,
			Route: &browserops.RouteMetadata{
				RequestedBrowser: req.Browser,
				UsedBrowser:      gr.UsedBrowser,
				Escalated:        gr.Escalated,
				Quality:          gr.Quality,
				FallbackAttempts: fallback,
				Attempts:         convertAttempts(gr.Attempts),
			},
		}, nil
	}

	return Result{
		UsedBrowser: req.Browser,
		Route:       browserops.SingleBrowserRoute(req.Browser),
	}, nil
}

func convertAttempts(attempts []ghostchrome.Attempt) []browserops.RouteAttempt {
	out := make([]browserops.RouteAttempt, len(attempts))
	for i, a := range attempts {
		out[i] = browserops.RouteAttempt{
			Browser:  a.Browser,
			Accepted: a.Accepted,
			Reason:   a.Reason,
		}
	}
	return out
}
