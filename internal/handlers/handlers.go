package handlers

import (
	"net/http"
	"os"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
)

type Handlers struct {
	Bridge       bridge.BridgeAPI
	Config       *config.RuntimeConfig
	Profiles     bridge.ProfileService
	Dashboard    *dashboard.Dashboard
	Orchestrator bridge.OrchestratorService
}

func New(b bridge.BridgeAPI, cfg *config.RuntimeConfig, p bridge.ProfileService, d *dashboard.Dashboard, o bridge.OrchestratorService) *Handlers {
	return &Handlers{
		Bridge:       b,
		Config:       cfg,
		Profiles:     p,
		Dashboard:    d,
		Orchestrator: o,
	}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux, doShutdown func()) {
	mux.HandleFunc("GET /health", h.HandleHealth)
	mux.HandleFunc("GET /tabs", h.HandleTabs)
	mux.HandleFunc("GET /snapshot", h.HandleSnapshot)
	mux.HandleFunc("GET /screenshot", h.HandleScreenshot)
	mux.HandleFunc("GET /text", h.HandleText)
	mux.HandleFunc("POST /navigate", h.HandleNavigate)
	mux.HandleFunc("POST /action", h.HandleAction)
	mux.HandleFunc("POST /actions", h.HandleActions)
	mux.HandleFunc("POST /evaluate", h.HandleEvaluate)
	mux.HandleFunc("POST /tab", h.HandleTab)
	mux.HandleFunc("POST /tab/lock", h.HandleTabLock)
	mux.HandleFunc("POST /tab/unlock", h.HandleTabUnlock)
	mux.HandleFunc("GET /cookies", h.HandleGetCookies)
	mux.HandleFunc("POST /cookies", h.HandleSetCookies)
	mux.HandleFunc("GET /stealth/status", h.HandleStealthStatus)
	mux.HandleFunc("POST /fingerprint/rotate", h.HandleFingerprintRotate)
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
	if h.Orchestrator != nil && os.Getenv("BRIDGE_NO_DASHBOARD") == "" {
		h.Orchestrator.RegisterHandlers(mux)
	}

	if doShutdown != nil {
		mux.HandleFunc("POST /shutdown", h.HandleShutdown(doShutdown))
	}
}
