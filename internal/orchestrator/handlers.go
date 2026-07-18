package orchestrator

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// Allows reports whether the given capability is permitted by the orchestrator's
// current security settings. Centralises the per-capability dispatch so callers
// don't need their own switch over routes.Capability.
func (o *Orchestrator) Allows(cap routes.Capability) bool {
	switch cap {
	case routes.CapEvaluate:
		return o.AllowsEvaluate()
	case routes.CapMacro:
		return o.AllowsMacro()
	case routes.CapScreencast:
		return o.AllowsScreencast()
	case routes.CapDownload:
		return o.AllowsDownload()
	case routes.CapCookies:
		return o.AllowsCookies()
	case routes.CapUpload:
		return o.AllowsUpload()
	case routes.CapStateExport:
		return o.AllowsStateExport()
	case routes.CapNetworkIntercept:
		return o.AllowsNetworkIntercept()
	default:
		return false
	}
}

func registerCapabilityRoute(mux *http.ServeMux, route string, enabled bool, feature, setting, code string, next http.HandlerFunc) {
	if enabled {
		mux.HandleFunc(route, next)
		return
	}
	mux.HandleFunc(route, httpx.DisabledEndpointHandler(feature, setting, code))
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
	cookiesMeta, _ := routes.Meta(routes.CapCookies)
	registerCapabilityRoute(mux, "POST /instances/{id}/cookies", o.Allows(routes.CapCookies), cookiesMeta.Label, cookiesMeta.Setting, cookiesMeta.DisabledCode, o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/audit", o.proxyToInstance)
	mux.HandleFunc("POST /instances/{id}/scrape", o.proxyToInstance)
	screencastMeta, _ := routes.Meta(routes.CapScreencast)
	registerCapabilityRoute(mux, "GET /instances/{id}/proxy/screencast", o.Allows(routes.CapScreencast), screencastMeta.Label, screencastMeta.Setting, screencastMeta.DisabledCode, o.handleProxyScreencast)
	registerCapabilityRoute(mux, "GET /instances/{id}/screencast", o.Allows(routes.CapScreencast), screencastMeta.Label, screencastMeta.Setting, screencastMeta.DisabledCode, o.proxyToInstance)

	// Tab operations - generic proxy (all route to the appropriate instance).
	// Sourced from the shared route catalogue to stay in sync with bridge and strategy.
	for _, route := range routes.TabScopedRoutes() {
		mux.HandleFunc(route, o.proxyTabRequest)
	}
	for cap, eps := range routes.TabScopedCapabilityRoutes() {
		meta, ok := routes.Meta(cap)
		if !ok {
			continue
		}
		enabled := o.Allows(cap)
		for _, ep := range eps {
			registerCapabilityRoute(mux, ep.TabRoute(), enabled, meta.Label, meta.Setting, meta.DisabledCode, o.proxyTabRequest)
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
