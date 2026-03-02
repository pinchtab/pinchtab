package orchestrator

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (o *Orchestrator) RegisterHandlers(mux *http.ServeMux) {
	// Profile management
	mux.HandleFunc("POST /profiles/{id}/start", o.handleStartByID)
	mux.HandleFunc("POST /profiles/{id}/stop", o.handleStopByID)
	mux.HandleFunc("GET /profiles/{id}/instance", o.handleProfileInstance)

	// Instance management
	mux.HandleFunc("GET /instances", o.handleList)
	mux.HandleFunc("GET /instances/{id}", o.handleGetInstance)
	mux.HandleFunc("GET /instances/tabs", o.handleAllTabs)
	mux.HandleFunc("POST /instances/start", o.handleStartInstance)
	mux.HandleFunc("POST /instances/launch", o.handleLaunchByName)
	mux.HandleFunc("POST /instances/{id}/start", o.handleStartByInstanceID)
	mux.HandleFunc("POST /instances/{id}/stop", o.handleStopByInstanceID)
	mux.HandleFunc("GET /instances/{id}/logs", o.handleLogsByID)
	mux.HandleFunc("GET /instances/{id}/tabs", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/proxy/screencast", o.handleProxyScreencast)
	mux.HandleFunc("POST /instances/{id}/tabs/open", o.handleInstanceTabOpen)
	mux.HandleFunc("POST /instances/{id}/tab", o.proxyToInstance)
	mux.HandleFunc("GET /instances/{id}/screencast", o.proxyToInstance)

	// Tab operations - custom handlers
	mux.HandleFunc("POST /tabs/{id}/close", o.handleTabClose)

	// Tab operations - generic proxy (all route to the appropriate instance)
	tabProxyRoutes := []string{
		"POST /tabs/{id}/navigate",
		"GET /tabs/{id}/snapshot",
		"GET /tabs/{id}/screenshot",
		"POST /tabs/{id}/action",
		"POST /tabs/{id}/actions",
		"GET /tabs/{id}/text",
		"POST /tabs/{id}/evaluate",
		"GET /tabs/{id}/pdf",
		"POST /tabs/{id}/pdf",
		"GET /tabs/{id}/download",
		"POST /tabs/{id}/upload",
		"POST /tabs/{id}/lock",
		"POST /tabs/{id}/unlock",
		"GET /tabs/{id}/cookies",
		"POST /tabs/{id}/cookies",
	}
	for _, route := range tabProxyRoutes {
		mux.HandleFunc(route, o.proxyTabRequest)
	}
}

func (o *Orchestrator) handleList(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, 200, o.List())
}

func (o *Orchestrator) handleAllTabs(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, 200, o.AllTabs())
}
