// Package handlers provides HTTP request handlers for the bridge server.
package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/idutil"
	"github.com/pinchtab/pinchtab/internal/semantic"
)

type Handlers struct {
	Bridge       bridge.BridgeAPI
	Config       *config.RuntimeConfig
	Profiles     bridge.ProfileService
	Dashboard    *dashboard.Dashboard
	Orchestrator bridge.OrchestratorService
	IdMgr        *idutil.Manager
	Matcher      semantic.ElementMatcher
	IntentCache  *semantic.IntentCache
	Recovery     *semantic.RecoveryEngine
	Router       *engine.Router // optional; nil ⇒ chrome-only
}

func New(b bridge.BridgeAPI, cfg *config.RuntimeConfig, p bridge.ProfileService, d *dashboard.Dashboard, o bridge.OrchestratorService) *Handlers {
	matcher := semantic.NewCombinedMatcher(semantic.NewHashingEmbedder(128))
	intentCache := semantic.NewIntentCache(200, 10*time.Minute)

	h := &Handlers{
		Bridge:       b,
		Config:       cfg,
		Profiles:     p,
		Dashboard:    d,
		Orchestrator: o,
		IdMgr:        idutil.NewManager(),
		Matcher:      matcher,
		IntentCache:  intentCache,
	}

	// Wire up the recovery engine with callbacks that delegate back to
	// the handler's bridge without introducing circular imports.
	h.Recovery = semantic.NewRecoveryEngine(
		semantic.DefaultRecoveryConfig(),
		matcher,
		intentCache,
		// SnapshotRefresher
		func(ctx context.Context, tabID string) error {
			h.refreshRefCache(ctx, tabID)
			return nil
		},
		// NodeIDResolver
		func(tabID, ref string) (int64, bool) {
			cache := h.Bridge.GetRefCache(tabID)
			if cache == nil {
				return 0, false
			}
			nid, ok := cache.Refs[ref]
			return nid, ok
		},
		// DescriptorBuilder
		func(tabID string) []semantic.ElementDescriptor {
			nodes := h.resolveSnapshotNodes(tabID)
			descs := make([]semantic.ElementDescriptor, len(nodes))
			for i, n := range nodes {
				descs[i] = semantic.ElementDescriptor{
					Ref:   n.Ref,
					Role:  n.Role,
					Name:  n.Name,
					Value: n.Value,
				}
			}
			return descs
		},
	)

	return h
}

// ensureChrome ensures Chrome is initialized before handling requests that need it
func (h *Handlers) ensureChrome() error {
	return h.Bridge.EnsureChrome(h.Config)
}

// useLite returns true when the engine router routes this operation to lite.
func (h *Handlers) useLite(op engine.Capability, url string) bool {
	return h.Router != nil && h.Router.UseLite(op, url)
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux, doShutdown func()) {
	mux.HandleFunc("GET /health", h.HandleHealth)
	mux.HandleFunc("POST /ensure-chrome", h.HandleEnsureChrome)
	mux.HandleFunc("GET /tabs", h.HandleTabs)
	mux.HandleFunc("POST /tabs/{id}/navigate", h.HandleTabNavigate)
	mux.HandleFunc("POST /tabs/{id}/back", h.HandleTabBack)
	mux.HandleFunc("POST /tabs/{id}/forward", h.HandleTabForward)
	mux.HandleFunc("POST /tabs/{id}/reload", h.HandleTabReload)
	mux.HandleFunc("GET /tabs/{id}/snapshot", h.HandleTabSnapshot)
	mux.HandleFunc("GET /tabs/{id}/screenshot", h.HandleTabScreenshot)
	mux.HandleFunc("POST /tabs/{id}/action", h.HandleTabAction)
	mux.HandleFunc("POST /tabs/{id}/actions", h.HandleTabActions)
	mux.HandleFunc("GET /tabs/{id}/text", h.HandleTabText)
	mux.HandleFunc("GET /tabs/{id}/metrics", h.HandleTabMetrics)
	mux.HandleFunc("GET /metrics", h.HandleMetrics)
	mux.HandleFunc("GET /snapshot", h.HandleSnapshot)
	mux.HandleFunc("GET /screenshot", h.HandleScreenshot)
	mux.HandleFunc("GET /tabs/{id}/pdf", h.HandleTabPDF)
	mux.HandleFunc("POST /tabs/{id}/pdf", h.HandleTabPDF)
	mux.HandleFunc("GET /pdf", h.HandlePDF)
	mux.HandleFunc("POST /pdf", h.HandlePDF)
	mux.HandleFunc("GET /text", h.HandleText)
	mux.HandleFunc("GET /help", h.HandleHelp)
	mux.HandleFunc("GET /openapi.json", h.HandleOpenAPI)
	mux.HandleFunc("POST /navigate", h.HandleNavigate)
	mux.HandleFunc("GET /navigate", h.HandleNavigate)

	mux.HandleFunc("POST /back", h.HandleBack)
	mux.HandleFunc("POST /forward", h.HandleForward)
	mux.HandleFunc("POST /reload", h.HandleReload)
	mux.HandleFunc("POST /action", h.HandleAction)
	mux.HandleFunc("GET /action", h.HandleAction)
	mux.HandleFunc("POST /actions", h.HandleActions)
	mux.HandleFunc("POST /macro", h.HandleMacro)
	mux.HandleFunc("POST /tab", h.HandleTab)
	mux.HandleFunc("POST /tab/lock", h.HandleTabLock)
	mux.HandleFunc("POST /tab/unlock", h.HandleTabUnlock)
	mux.HandleFunc("POST /tabs/{id}/lock", h.HandleTabLockByID)
	mux.HandleFunc("POST /tabs/{id}/unlock", h.HandleTabUnlockByID)
	mux.HandleFunc("GET /tabs/{id}/cookies", h.HandleTabGetCookies)
	mux.HandleFunc("POST /tabs/{id}/cookies", h.HandleTabSetCookies)
	mux.HandleFunc("GET /cookies", h.HandleGetCookies)
	mux.HandleFunc("POST /cookies", h.HandleSetCookies)
	mux.HandleFunc("POST /fingerprint/rotate", h.HandleFingerprintRotate)
	mux.HandleFunc("GET /tabs/{id}/download", h.HandleTabDownload)
	mux.HandleFunc("POST /tabs/{id}/upload", h.HandleTabUpload)
	mux.HandleFunc("GET /download", h.HandleDownload)
	mux.HandleFunc("POST /upload", h.HandleUpload)
	mux.HandleFunc("POST /tabs/{id}/find", h.HandleFind)
	mux.HandleFunc("POST /find", h.HandleFind)
	mux.HandleFunc("GET /screencast", h.HandleScreencast)
	mux.HandleFunc("GET /screencast/tabs", h.HandleScreencastAll)
	mux.HandleFunc("POST /tabs/{id}/evaluate", h.HandleTabEvaluate)
	mux.HandleFunc("POST /evaluate", h.HandleEvaluate)
	mux.HandleFunc("GET /network", h.HandleNetwork)
	mux.HandleFunc("GET /network/stream", h.HandleNetworkStream)
	mux.HandleFunc("GET /network/{requestId}", h.HandleNetworkByID)
	mux.HandleFunc("POST /network/clear", h.HandleNetworkClear)
	mux.HandleFunc("GET /tabs/{id}/network", h.HandleTabNetwork)
	mux.HandleFunc("GET /tabs/{id}/network/stream", h.HandleTabNetworkStream)
	mux.HandleFunc("GET /tabs/{id}/network/{requestId}", h.HandleTabNetworkByID)
	mux.HandleFunc("POST /dialog", h.HandleDialog)
	mux.HandleFunc("POST /tabs/{id}/dialog", h.HandleTabDialog)
	mux.HandleFunc("POST /wait", h.HandleWait)
	mux.HandleFunc("POST /tabs/{id}/wait", h.HandleTabWait)
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
