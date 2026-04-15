// Package handlers provides HTTP request handlers for the bridge server.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/idpi"
	"github.com/pinchtab/pinchtab/internal/ids"
	"github.com/pinchtab/semantic"
	"github.com/pinchtab/semantic/recovery"
)

type Handlers struct {
	Bridge       bridge.BridgeAPI
	Config       *config.RuntimeConfig
	Profiles     bridge.ProfileService
	Dashboard    *dashboard.Dashboard
	Orchestrator bridge.OrchestratorService
	IdMgr        *ids.Manager
	Matcher      semantic.ElementMatcher
	IntentCache  *recovery.IntentCache
	Recovery     *recovery.RecoveryEngine
	Router       *engine.Router // optional; nil ⇒ chrome-only
	IDPIGuard    idpi.Guard
	Version      string // build version injected at startup
	clipboard    clipboardStore

	// Optional dependency injection (for unit testing)
	evalJS      func(ctx context.Context, expression string, out *string) error
	evalRuntime func(ctx context.Context, expression string, out any, opts ...chromedp.EvaluateOption) error
}

func New(b bridge.BridgeAPI, cfg *config.RuntimeConfig, p bridge.ProfileService, d *dashboard.Dashboard, o bridge.OrchestratorService) *Handlers {
	matcher := semantic.NewCombinedMatcher(semantic.NewHashingEmbedder(128))
	intentCache := recovery.NewIntentCache(200, 10*time.Minute)

	h := &Handlers{
		Bridge:       b,
		Config:       cfg,
		Profiles:     p,
		Dashboard:    d,
		Orchestrator: o,
		IdMgr:        ids.NewManager(),
		Matcher:      matcher,
		IntentCache:  intentCache,
		IDPIGuard:    idpi.NewGuard(cfg.IDPI, cfg.AllowedDomains),
	}

	// Wire up the recovery engine with callbacks that delegate back to
	// the handler's bridge without introducing circular imports.
	h.Recovery = recovery.NewRecoveryEngine(
		recovery.DefaultRecoveryConfig(),
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
			target, ok := cache.Lookup(ref)
			return target.BackendNodeID, ok
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

	// Default evalJS backed by chromedp for production
	h.evalJS = func(ctx context.Context, expression string, out *string) error {
		return chromedp.Run(ctx, chromedp.Evaluate(expression, out))
	}
	h.evalRuntime = func(ctx context.Context, expression string, out any, opts ...chromedp.EvaluateOption) error {
		return chromedp.Run(ctx, chromedp.Evaluate(expression, out, opts...))
	}

	// Clean up .tmp export files orphaned by a previous crash.
	go CleanupStaleTmpExports(cfg.StateDir)

	return h
}

type restartStatusProvider interface {
	RestartStatus() (bool, time.Duration)
}

// ensureChrome ensures Chrome is initialized before handling requests that need it
func (h *Handlers) ensureChrome() error {
	return h.Bridge.EnsureChrome(h.Config)
}

func (h *Handlers) ensureChromeOrRespond(w http.ResponseWriter) bool {
	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return false
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return false
	}
	return true
}

func (h *Handlers) bridgeRestartStatus() (bool, time.Duration) {
	provider, ok := h.Bridge.(restartStatusProvider)
	if !ok {
		return false, 0
	}
	return provider.RestartStatus()
}

func (h *Handlers) writeBridgeUnavailable(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, bridge.ErrBrowserDraining) {
		return false
	}
	draining, retryAfter := h.bridgeRestartStatus()
	if !draining {
		retryAfter = time.Second
	}
	seconds := int((retryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	httpx.ErrorCode(w, http.StatusServiceUnavailable, "browser_draining", err.Error(), true, map[string]any{"retryAfterSeconds": seconds})
	return true
}

// useLite returns true when the engine router routes this operation to lite.
func (h *Handlers) useLite(op engine.Capability, url string) bool {
	return h.Router != nil && h.Router.UseLite(op, url)
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux, doShutdown func()) {
	mux.HandleFunc("GET /health", h.HandleHealth)
	mux.HandleFunc("POST /ensure-chrome", h.HandleEnsureChrome)
	mux.HandleFunc("POST /browser/restart", h.HandleBrowserRestart)
	mux.HandleFunc("GET /tabs", h.HandleTabs)
	mux.HandleFunc("POST /tabs/{id}/navigate", h.HandleTabNavigate)
	mux.HandleFunc("POST /tabs/{id}/back", h.HandleTabBack)
	mux.HandleFunc("POST /tabs/{id}/forward", h.HandleTabForward)
	mux.HandleFunc("POST /tabs/{id}/reload", h.HandleTabReload)
	mux.HandleFunc("GET /tabs/{id}/snapshot", h.HandleTabSnapshot)
	mux.HandleFunc("GET /tabs/{id}/frame", h.HandleTabFrame)
	mux.HandleFunc("POST /tabs/{id}/frame", h.HandleTabFrame)
	mux.HandleFunc("GET /tabs/{id}/screenshot", h.HandleTabScreenshot)
	mux.HandleFunc("POST /tabs/{id}/action", h.HandleTabAction)
	mux.HandleFunc("POST /tabs/{id}/actions", h.HandleTabActions)
	mux.HandleFunc("POST /tabs/{id}/handoff", h.HandleTabHandoff)
	mux.HandleFunc("POST /tabs/{id}/resume", h.HandleTabResume)
	mux.HandleFunc("GET /tabs/{id}/handoff", h.HandleTabHandoffStatus)
	mux.HandleFunc("GET /tabs/{id}/text", h.HandleTabText)
	mux.HandleFunc("GET /tabs/{id}/metrics", h.HandleTabMetrics)
	mux.HandleFunc("GET /metrics", h.HandleMetrics)
	mux.HandleFunc("GET /snapshot", h.HandleSnapshot)
	mux.HandleFunc("GET /frame", h.HandleFrame)
	mux.HandleFunc("POST /frame", h.HandleFrame)
	mux.HandleFunc("GET /screenshot", h.HandleScreenshot)
	mux.HandleFunc("GET /tabs/{id}/pdf", h.HandleTabPDF)
	mux.HandleFunc("POST /tabs/{id}/pdf", h.HandleTabPDF)
	mux.HandleFunc("GET /pdf", h.HandlePDF)
	mux.HandleFunc("POST /pdf", h.HandlePDF)
	mux.HandleFunc("GET /text", h.HandleText)
	mux.HandleFunc("GET /openapi.json", h.HandleOpenAPI)
	mux.HandleFunc("GET /help", h.HandleOpenAPI) // alias
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
	mux.HandleFunc("POST /lock", h.HandleTabLock)
	mux.HandleFunc("POST /unlock", h.HandleTabUnlock)
	mux.HandleFunc("POST /tabs/{id}/lock", h.HandleTabLockByID)
	mux.HandleFunc("POST /tabs/{id}/unlock", h.HandleTabUnlockByID)
	mux.HandleFunc("GET /tabs/{id}/cookies", h.HandleTabGetCookies)
	mux.HandleFunc("POST /tabs/{id}/cookies", h.HandleTabSetCookies)
	mux.HandleFunc("GET /cookies", h.HandleGetCookies)
	mux.HandleFunc("POST /cookies", h.HandleSetCookies)
	mux.HandleFunc("GET /solvers", h.HandleListSolvers)
	mux.HandleFunc("POST /solve", h.HandleSolve)
	mux.HandleFunc("POST /solve/{name}", h.HandleSolve)
	mux.HandleFunc("POST /tabs/{id}/solve", h.HandleTabSolve)
	mux.HandleFunc("POST /tabs/{id}/solve/{name}", h.HandleTabSolve)
	mux.HandleFunc("POST /fingerprint/rotate", h.HandleFingerprintRotate)
	mux.HandleFunc("GET /stealth/status", h.HandleStealthStatus)
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
	mux.HandleFunc("GET /clipboard/read", h.HandleClipboardRead)
	mux.HandleFunc("POST /clipboard/write", h.HandleClipboardWrite)
	mux.HandleFunc("POST /clipboard/copy", h.HandleClipboardCopy)
	mux.HandleFunc("GET /clipboard/paste", h.HandleClipboardPaste)
	mux.HandleFunc("GET /network", h.HandleNetwork)
	mux.HandleFunc("GET /network/stream", h.HandleNetworkStream)
	mux.HandleFunc("GET /network/export", h.HandleNetworkExport)
	mux.HandleFunc("GET /network/export/stream", h.HandleNetworkExportStream)
	mux.HandleFunc("GET /network/{requestId}", h.HandleNetworkByID)
	mux.HandleFunc("POST /network/clear", h.HandleNetworkClear)
	mux.HandleFunc("GET /tabs/{id}/network", h.HandleTabNetwork)
	mux.HandleFunc("GET /tabs/{id}/network/stream", h.HandleTabNetworkStream)
	mux.HandleFunc("GET /tabs/{id}/network/export", h.HandleTabNetworkExport)
	mux.HandleFunc("GET /tabs/{id}/network/export/stream", h.HandleTabNetworkExportStream)
	mux.HandleFunc("GET /tabs/{id}/network/{requestId}", h.HandleTabNetworkByID)
	mux.HandleFunc("POST /dialog", h.HandleDialog)
	mux.HandleFunc("POST /tabs/{id}/dialog", h.HandleTabDialog)
	mux.HandleFunc("POST /wait", h.HandleWait)
	mux.HandleFunc("POST /tabs/{id}/wait", h.HandleTabWait)
	mux.HandleFunc("GET /console", h.HandleGetConsoleLogs)
	mux.HandleFunc("POST /console/clear", h.HandleClearConsoleLogs)
	mux.HandleFunc("GET /errors", h.HandleGetErrorLogs)
	mux.HandleFunc("POST /errors/clear", h.HandleClearErrorLogs)
	mux.HandleFunc("POST /cache/clear", h.HandleCacheClear)
	mux.HandleFunc("GET /cache/status", h.HandleCacheStatus)

	// Storage (current origin only)
	mux.HandleFunc("GET /storage", h.HandleStorage)
	mux.HandleFunc("POST /storage", h.HandleStorage)
	mux.HandleFunc("DELETE /storage", h.HandleStorage)
	mux.HandleFunc("GET /tabs/{id}/storage", h.HandleTabStorageGet)
	mux.HandleFunc("POST /tabs/{id}/storage", h.HandleTabStorageSet)
	mux.HandleFunc("DELETE /tabs/{id}/storage", h.HandleTabStorageDelete)

	// State management
	mux.HandleFunc("GET /state/list", h.HandleStateList)
	mux.HandleFunc("GET /state/show", h.HandleStateShow)
	mux.HandleFunc("POST /state/save", h.HandleStateSave)
	mux.HandleFunc("POST /state/load", h.HandleStateLoad)
	mux.HandleFunc("DELETE /state", h.HandleStateDelete)
	mux.HandleFunc("POST /state/clean", h.HandleStateClean)

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
