// Package handlers provides HTTP request handlers for the bridge server.
package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/idutil"
)

type Handlers struct {
	Bridge       bridge.BridgeAPI
	Config       *config.RuntimeConfig
	Profiles     bridge.ProfileService
	Dashboard    *dashboard.Dashboard
	Orchestrator bridge.OrchestratorService
	IdMgr        *idutil.Manager
}

func New(b bridge.BridgeAPI, cfg *config.RuntimeConfig, p bridge.ProfileService, d *dashboard.Dashboard, o bridge.OrchestratorService) *Handlers {
	return &Handlers{
		Bridge:       b,
		Config:       cfg,
		Profiles:     p,
		Dashboard:    d,
		Orchestrator: o,
		IdMgr:        idutil.NewManager(),
	}
}

// ensureChrome ensures Chrome is initialized before handling requests that need it
func (h *Handlers) ensureChrome() error {
	return h.Bridge.EnsureChrome(h.Config)
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux, doShutdown func()) {
	mux.HandleFunc("GET /health", h.HandleHealth)
	mux.HandleFunc("POST /ensure-chrome", h.HandleEnsureChrome)
	mux.HandleFunc("GET /tabs", h.HandleTabs)
	mux.HandleFunc("POST /tabs/{id}/navigate", h.HandleTabNavigate)
	mux.HandleFunc("GET /tabs/{id}/snapshot", h.HandleTabSnapshot)
	mux.HandleFunc("GET /tabs/{id}/screenshot", h.HandleTabScreenshot)
	mux.HandleFunc("POST /tabs/{id}/action", h.HandleTabAction)
	mux.HandleFunc("POST /tabs/{id}/actions", h.HandleTabActions)
	mux.HandleFunc("GET /tabs/{id}/text", h.HandleTabText)
	mux.HandleFunc("POST /tabs/{id}/evaluate", h.HandleTabEvaluate)
	mux.HandleFunc("GET /snapshot", h.HandleSnapshot)
	mux.HandleFunc("GET /screenshot", h.HandleScreenshot)
	mux.HandleFunc("GET /tabs/{id}/pdf", h.HandleTabPDF)
	mux.HandleFunc("POST /tabs/{id}/pdf", h.HandleTabPDF)
	mux.HandleFunc("GET /text", h.HandleText)
	mux.HandleFunc("POST /navigate", h.HandleNavigate)
	mux.HandleFunc("GET /navigate", h.HandleNavigate)
	mux.HandleFunc("GET /nav", h.HandleNavigate)
	mux.HandleFunc("POST /action", h.HandleAction)
	mux.HandleFunc("POST /actions", h.HandleActions)
	mux.HandleFunc("POST /evaluate", h.HandleEvaluate)
	mux.HandleFunc("POST /tab", h.HandleTab)
	mux.HandleFunc("POST /tab/lock", h.HandleTabLock)
	mux.HandleFunc("POST /tab/unlock", h.HandleTabUnlock)
	mux.HandleFunc("POST /tabs/{id}/lock", h.HandleTabLockByID)
	mux.HandleFunc("POST /tabs/{id}/unlock", h.HandleTabUnlockByID)
	mux.HandleFunc("GET /tabs/{id}/cookies", h.HandleTabGetCookies)
	mux.HandleFunc("POST /tabs/{id}/cookies", h.HandleTabSetCookies)
	mux.HandleFunc("GET /cookies", h.HandleGetCookies)
	mux.HandleFunc("POST /cookies", h.HandleSetCookies)
	mux.HandleFunc("GET /stealth/status", h.HandleStealthStatus)
	mux.HandleFunc("POST /fingerprint/rotate", h.HandleFingerprintRotate)
	mux.HandleFunc("GET /tabs/{id}/download", h.HandleTabDownload)
	mux.HandleFunc("POST /tabs/{id}/upload", h.HandleTabUpload)
	mux.HandleFunc("GET /download", h.HandleDownload)
	mux.HandleFunc("POST /upload", h.HandleUpload)
	mux.HandleFunc("GET /screencast", h.HandleScreencast)
	mux.HandleFunc("GET /screencast/tabs", h.HandleScreencastAll)
	mux.HandleFunc("GET /welcome", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(assets.WelcomeHTML))
	})

	if h.Profiles != nil {
		h.Profiles.RegisterHandlers(mux)
	}
	if h.Dashboard != nil {
		h.Dashboard.RegisterHandlers(mux)
	}
	if h.Orchestrator != nil {
		h.Orchestrator.RegisterHandlers(mux)
	}

	if doShutdown != nil {
		mux.HandleFunc("POST /shutdown", h.HandleShutdown(doShutdown))
	}
}
