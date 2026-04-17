package orchestrator

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

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
	// Profile management
	if !skipLaunch {
		mux.HandleFunc("POST /profiles/{id}/start", o.handleStartByID)
	}
	mux.HandleFunc("POST /profiles/{id}/stop", o.handleStopByID)
	mux.HandleFunc("GET /profiles/{id}/instance", o.handleProfileInstance)

	// Instance management
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
	registerCapabilityRoute(mux, "GET /instances/{id}/proxy/screencast", o.AllowsScreencast(), "screencast", "security.allowScreencast", "screencast_disabled", o.handleProxyScreencast)
	registerCapabilityRoute(mux, "GET /instances/{id}/screencast", o.AllowsScreencast(), "screencast", "security.allowScreencast", "screencast_disabled", o.proxyToInstance)

	// Tab operations - custom handlers
	mux.HandleFunc("POST /tabs/{id}/close", o.handleTabClose)

	// Tab operations - generic proxy (all route to the appropriate instance).
	// Sourced from routes.Core() catalogue to stay in sync with bridge and strategy.
	for _, route := range routes.TabScopedRoutes() {
		mux.HandleFunc(route, o.proxyTabRequest)
	}
	// Tab-scoped capability-gated routes.
	for cap, eps := range routes.TabScopedCapabilityRoutes() {
		var enabled bool
		var feature, setting, code string
		switch cap {
		case routes.CapEvaluate:
			enabled = o.AllowsEvaluate()
			feature, setting, code = "evaluate", "security.allowEvaluate", "evaluate_disabled"
		case routes.CapDownload:
			enabled = o.AllowsDownload()
			feature, setting, code = "download", "security.allowDownload", "download_disabled"
		case routes.CapUpload:
			enabled = o.AllowsUpload()
			feature, setting, code = "upload", "security.allowUpload", "upload_disabled"
		case routes.CapScreencast:
			enabled = o.AllowsScreencast()
			feature, setting, code = "screencast", "security.allowScreencast", "screencast_disabled"
		case routes.CapMacro:
			enabled = o.AllowsMacro()
			feature, setting, code = "macro", "security.allowMacro", "macro_disabled"
		case routes.CapStateExport:
			enabled = o.AllowsStateExport()
			feature, setting, code = "stateExport", "security.allowStateExport", "state_export_disabled"
		}
		for _, ep := range eps {
			registerCapabilityRoute(mux, ep.TabRoute(), enabled, feature, setting, code, o.proxyTabRequest)
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
	httpx.JSON(w, 200, o.AllTabs())
}

func (o *Orchestrator) handleAllMetrics(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, o.AllMetrics())
}
