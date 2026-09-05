package orchestrator

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

func (o *Orchestrator) Allows(cap routes.Capability) bool {
	return o.cfg().CapabilityEnabled(cap)
}

func (o *Orchestrator) registerCapabilityRoute(mux *http.ServeMux, route string, cap routes.Capability, next http.HandlerFunc) {
	meta, ok := routes.Meta(cap)
	if !ok {
		return
	}
	if o.Allows(cap) {
		mux.HandleFunc(route, next)
		return
	}
	mux.HandleFunc(route, httpx.DisabledEndpointHandler(meta.Label, meta.Setting, meta.DisabledCode))
}

// RegisterHandlersNoLaunch registers all orchestrator handlers except
// local instance launch endpoints (start, launch). Used by the no-instance strategy.
func (o *Orchestrator) RegisterHandlersNoLaunch(mux *http.ServeMux) {
	o.registerHandlers(mux, true)
}

func (o *Orchestrator) RegisterHandlers(mux *http.ServeMux) {
	o.registerHandlers(mux, false)
}

func (o *Orchestrator) registerHandlers(mux *http.ServeMux, skipLaunch bool) {
	if !skipLaunch {
		mux.HandleFunc("POST /profiles/{id}/start", o.handleStartByID)
	}
	mux.HandleFunc("POST /profiles/{id}/stop", o.handleStopByID)
	mux.HandleFunc("GET /profiles/{id}/instance", o.handleProfileInstance)

	mux.HandleFunc("GET /instances", o.handleList)
	mux.HandleFunc("GET /instances/{id}", o.handleGetInstance)
	mux.HandleFunc("GET /instances/tabs", o.handleAllTabs)
	mux.HandleFunc("GET /instances/metrics", o.handleAllMetrics)
	// The front door answers the bare /metrics with its OWN counters, so an
	// instance's request counters need a path of their own; without it, moving
	// /metrics to the front door would just make the other layer invisible.
	mux.HandleFunc("GET /instances/{id}/metrics", o.proxyToInstance)
	if !skipLaunch {
		mux.HandleFunc("POST /instances/start", o.handleStartInstance)
		mux.HandleFunc("POST /instances/launch", o.handleLaunchByName)
	}
	mux.HandleFunc("POST /instances/attach", o.handleAttachInstance)
	mux.HandleFunc("POST /instances/attach-bridge", o.handleAttachBridge)
	if !skipLaunch {
		mux.HandleFunc("POST /instances/{id}/start", o.handleStartByInstanceID)
	}
	mux.HandleFunc("POST /instances/{id}/restart", o.handleRestartByInstanceID)
	mux.HandleFunc("POST /instances/{id}/stop", o.handleStopByInstanceID)
	mux.HandleFunc("GET /instances/{id}/logs", o.handleLogsByID)
	mux.HandleFunc("GET /instances/{id}/logs/stream", o.handleLogsStreamByID)
	mux.HandleFunc("GET /instances/{id}/tabs", o.handleInstanceTabs)
	mux.HandleFunc("POST /instances/{id}/tabs/open", o.handleInstanceTabOpen)
	mux.HandleFunc("POST /instances/{id}/tab", o.proxyToInstance)
	// Disposable, cookie-authenticated CLI runs access their isolated child
	// through these routes because a child's loopback URL is not reachable by
	// remote clients.
	mux.HandleFunc("POST /instances/{id}/close", o.proxyToInstance)
	o.registerCapabilityRoute(mux, "POST /instances/{id}/cookies", routes.CapCookies, o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/audit", o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/scrape", o.proxyToInstance)
	o.registerCapabilityRoute(mux, "GET /instances/{id}/proxy/screencast", routes.CapScreencast, o.handleProxyScreencast)
	o.registerCapabilityRoute(mux, "GET /instances/{id}/screencast", routes.CapScreencast, o.proxyToInstance)

	// Tab operations - generic proxy (all route to the appropriate instance).
	// Sourced from the shared route catalogue to stay in sync with bridge and strategy.
	for _, route := range routes.TabScopedRoutes() {
		mux.HandleFunc(route, o.proxyTabRequest)
	}
	for cap, eps := range routes.TabScopedCapabilityRoutes() {
		for _, ep := range eps {
			o.registerCapabilityRoute(mux, ep.TabRoute(), cap, o.proxyTabRequest)
		}
	}

	// Cache operations - per-instance (browser-wide shorthands are in strategy routes)
	mux.HandleFunc("POST /instances/{id}/cache/clear", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/cache/status", o.proxyToInstance)
}

func (o *Orchestrator) handleList(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, o.List())
}

func (o *Orchestrator) handleAllTabs(w http.ResponseWriter, r *http.Request) {
	fresh := r.URL.Query().Get("fresh") == "1"
	httpx.JSON(w, 200, o.allTabs(fresh))
}

func (o *Orchestrator) handleAllMetrics(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, o.AllMetrics())
}
