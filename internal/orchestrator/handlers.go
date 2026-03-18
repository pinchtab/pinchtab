package orchestrator

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

func registerCapabilityRoute(mux *http.ServeMux, route string, enabled bool, feature, setting, code string, next http.HandlerFunc) {
	if enabled {
		mux.HandleFunc(route, next)
		return
	}
	mux.HandleFunc(route, httpx.DisabledEndpointHandler(feature, setting, code))
}

func (o *Orchestrator) RegisterHandlers(mux *http.ServeMux) {
	// Profile management
	mux.HandleFunc("POST /profiles/{id}/start", o.handleStartByID)
	mux.HandleFunc("POST /profiles/{id}/stop", o.handleStopByID)
	mux.HandleFunc("GET /profiles/{id}/instance", o.handleProfileInstance)

	// Instance management
	mux.HandleFunc("GET /instances", o.handleList)
	mux.HandleFunc("GET /instances/{id}", o.handleGetInstance)
	mux.HandleFunc("GET /instances/tabs", o.handleAllTabs)
	mux.HandleFunc("GET /instances/metrics", o.handleAllMetrics)
	mux.HandleFunc("POST /instances/start", o.handleStartInstance)
	mux.HandleFunc("POST /instances/launch", o.handleLaunchByName)
	mux.HandleFunc("POST /instances/attach", o.handleAttachInstance)
	mux.HandleFunc("POST /instances/attach-bridge", o.handleAttachBridge)
	mux.HandleFunc("POST /instances/{id}/start", o.handleStartByInstanceID)
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

	// Tab operations - generic proxy (all route to the appropriate instance)
	for _, route := range []string{
		"POST /tabs/{id}/navigate",
		"GET /tabs/{id}/snapshot",
		"GET /tabs/{id}/screenshot",
		"POST /tabs/{id}/action",
		"POST /tabs/{id}/actions",
		"GET /tabs/{id}/text",
		"GET /tabs/{id}/pdf",
		"POST /tabs/{id}/pdf",
		"POST /tabs/{id}/lock",
		"POST /tabs/{id}/unlock",
		"GET /tabs/{id}/cookies",
		"POST /tabs/{id}/cookies",
		"GET /tabs/{id}/metrics",
		"POST /tabs/{id}/find",
		"POST /tabs/{id}/back",
		"POST /tabs/{id}/forward",
		"POST /tabs/{id}/reload",
	} {
		mux.HandleFunc(route, o.proxyTabRequest)
	}
	registerCapabilityRoute(mux, "POST /tabs/{id}/evaluate", o.AllowsEvaluate(), "evaluate", "security.allowEvaluate", "evaluate_disabled", o.proxyTabRequest)
	registerCapabilityRoute(mux, "GET /tabs/{id}/download", o.AllowsDownload(), "download", "security.allowDownload", "download_disabled", o.proxyTabRequest)
	registerCapabilityRoute(mux, "POST /tabs/{id}/upload", o.AllowsUpload(), "upload", "security.allowUpload", "upload_disabled", o.proxyTabRequest)
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
