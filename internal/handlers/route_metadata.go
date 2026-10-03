package handlers

import (
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
)

// routeMetadataFor builds the route metadata for a resolved request: one attempt
// per browser actually consulted. A CanHandle downgrade counts as two — the
// intent browser that declined, then chrome — and marks the route escalated.
func routeMetadataFor(routing browserRouting) *browserops.RouteMetadata {
	route := &browserops.RouteMetadata{
		RequestedBrowser: routing.Browser,
		UsedBrowser:      routing.Browser,
	}
	if routing.RequestBrowser != "" {
		route.RequestedBrowser = routing.RequestBrowser
	}

	intentBrowser := routing.IntentBrowser
	if intentBrowser == "" {
		intentBrowser = routing.Browser
	}

	if intentBrowser != routing.Browser {
		route.Escalated = true
		route.Attempts = []browserops.RouteAttempt{
			{Browser: intentBrowser, Reason: routing.Decision.Reason},
			{Browser: routing.Browser, Accepted: true},
		}
		return route
	}

	route.Attempts = []browserops.RouteAttempt{{
		Browser:  routing.Browser,
		Accepted: routing.Decision.Decision == browsers.DecisionHandle,
		Reason:   routing.Decision.Reason,
	}}
	return route
}

func withRoute(resp map[string]any, route *browserops.RouteMetadata) map[string]any {
	if !route.Trivial() {
		resp["route"] = route
	}
	return resp
}
