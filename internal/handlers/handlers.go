// Package handlers provides HTTP request handlers for the bridge server.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/contentguard"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/idpi"
	"github.com/pinchtab/pinchtab/internal/ids"
	"github.com/pinchtab/pinchtab/internal/navguard"
	"github.com/pinchtab/semantic"
	"github.com/pinchtab/semantic/recovery"
)

type Handlers struct {
	Bridge          bridge.BridgeAPI
	Config          *config.RuntimeConfig
	Profiles        bridge.ProfileService
	Dashboard       *dashboard.Dashboard
	Orchestrator    bridge.OrchestratorService
	IdMgr           *ids.Manager
	Matcher         semantic.ElementMatcher
	IntentCache     *recovery.IntentCache
	Recovery        *recovery.RecoveryEngine
	IDPIGuard       idpi.Guard
	ContentGuard    *contentguard.Scanner
	NavGuard        *navguard.Validator
	CurrentTabs     *CurrentTabStore
	Version         string // build version injected at startup
	clipboard       clipboardStore
	credentialStore *credentialStore

	// emptyPointerPolicy controls behavior when an identified caller omits
	// tabId and has no stored scoped current tab. See EmptyPointerPolicy.
	emptyPointerPolicy EmptyPointerPolicy

	recorder *recorder

	// Optional dependency injection (for unit testing)
	evalJS           func(ctx context.Context, expression string, out *string) error
	autoSolverRunner func(ctx context.Context, tabID string) error
	evalRuntime      func(ctx context.Context, expression string, out any, opts bridge.EvalOpts) error
}

func New(b bridge.BridgeAPI, cfg *config.RuntimeConfig, p bridge.ProfileService, d *dashboard.Dashboard, o bridge.OrchestratorService) *Handlers {
	matcher := semantic.NewCombinedMatcher(semantic.NewHashingEmbedder(128))
	intentCache := recovery.NewIntentCache(200, 10*time.Minute)

	idpiGuard := idpi.NewGuard(cfg.IDPI, cfg.AllowedDomains)
	h := &Handlers{
		Bridge:       b,
		Config:       cfg,
		Profiles:     p,
		Dashboard:    d,
		Orchestrator: o,
		IdMgr:        ids.NewManager(),
		Matcher:      matcher,
		IntentCache:  intentCache,
		IDPIGuard:    idpiGuard,
		ContentGuard: &contentguard.Scanner{
			Guard:       idpiGuard,
			WrapEnabled: cfg.IDPI.WrapContent,
		},
		NavGuard: &navguard.Validator{
			TrustedResolveCIDRs: navguard.ParseCIDRs(cfg.TrustedResolveCIDRs),
			TrustedProxyCIDRs:   navguard.BuildTrustedProxyCIDRs(cfg.TrustLoopbackProxy, cfg.TrustedProxyCIDRs),
			IDPIDomainAllowed:   idpiGuard.DomainAllowed,
		},
		CurrentTabs:     NewCurrentTabStore(),
		credentialStore: newCredentialStore(),
		recorder:        &recorder{},
	}

	h.recorder.captureFrame = func(ctx context.Context, quality int) ([]byte, error) {
		return h.Bridge.CaptureScreenshot(ctx, "jpeg", quality, nil)
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
			return semanticDescriptorsFromNodes(nodes)
		},
	)

	// Default evalJS backed by BridgeAPI for production
	h.evalJS = func(ctx context.Context, expression string, out *string) error {
		return h.Bridge.Evaluate(ctx, expression, out, bridge.EvalOpts{})
	}
	h.autoSolverRunner = h.runAutoSolver
	h.evalRuntime = func(ctx context.Context, expression string, out any, opts bridge.EvalOpts) error {
		return h.Bridge.Evaluate(ctx, expression, out, opts)
	}

	// Clean up .tmp export files orphaned by a previous crash.
	go CleanupStaleTmpExports(cfg.StateDir)
	// Clean up decoded base64 upload staging dirs left by older runs.
	go CleanupStaleUploads(cfg.StateDir)

	return h
}

// SetEmptyPointerPolicy configures behavior when an identified caller
// omits tabId and has no stored scoped current tab. Default is lazy.
func (h *Handlers) SetEmptyPointerPolicy(p EmptyPointerPolicy) {
	if h == nil {
		return
	}
	if p == "" {
		p = EmptyPointerLazy
	}
	h.emptyPointerPolicy = p
}

// EmptyPointerPolicy returns the active empty-pointer policy. Defaults to
// lazy when not configured.
func (h *Handlers) EmptyPointerPolicy() EmptyPointerPolicy {
	if h == nil || h.emptyPointerPolicy == "" {
		return EmptyPointerLazy
	}
	return h.emptyPointerPolicy
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

// armAutoCloseIfEnabled (re)arms the per-tab idle close timer when the
// instance has lifecycle policy "close_idle". Call when an authorized
// read/action request has finished using the tab.
func (h *Handlers) armAutoCloseIfEnabled(tabID string) {
	if h == nil || h.Bridge == nil || tabID == "" {
		return
	}
	if h.Config == nil || h.Config.TabLifecyclePolicy != "close_idle" {
		return
	}
	h.Bridge.ScheduleAutoClose(tabID)
}

// cancelAutoCloseIfEnabled stops a pending auto-close timer. Call from
// /navigate to indicate fresh work on the tab.
func (h *Handlers) cancelAutoCloseIfEnabled(tabID string) {
	if h == nil || h.Bridge == nil || tabID == "" {
		return
	}
	if h.Config == nil || h.Config.TabLifecyclePolicy != "close_idle" {
		return
	}
	h.Bridge.CancelAutoClose(tabID)
}

// clearTabFrameScope drops any active frame scope on a tab. Call from
// /navigate after a successful navigation: the previous page's frame
// tree is gone, so any FrameScope pointing into it would only cause
// stale-scope failures on the next /snap, /text, /wait, or /action.
func (h *Handlers) clearTabFrameScope(tabID string) {
	if h == nil || tabID == "" {
		return
	}
	if scopes := h.frameScopes(); scopes != nil {
		scopes.ClearFrameScope(tabID)
	}
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
	mux.HandleFunc("GET /tabs/{id}/capture", h.HandleTabCapture)
	mux.HandleFunc("POST /tabs/{id}/action", h.HandleTabAction)
	mux.HandleFunc("POST /tabs/{id}/actions", h.HandleTabActions)
	mux.HandleFunc("POST /tabs/{id}/handoff", h.HandleTabHandoff)
	mux.HandleFunc("POST /tabs/{id}/resume", h.HandleTabResume)
	mux.HandleFunc("GET /tabs/{id}/handoff", h.HandleTabHandoffStatus)
	mux.HandleFunc("GET /tabs/{id}/text", h.HandleTabText)
	mux.HandleFunc("GET /tabs/{id}/title", h.HandleTabTitle)
	mux.HandleFunc("GET /tabs/{id}/url", h.HandleTabURL)
	mux.HandleFunc("GET /tabs/{id}/html", h.HandleTabHTML)
	mux.HandleFunc("GET /tabs/{id}/styles", h.HandleTabStyles)
	mux.HandleFunc("GET /tabs/{id}/value", h.HandleTabGetValue)
	mux.HandleFunc("GET /tabs/{id}/attr", h.HandleTabGetAttr)
	mux.HandleFunc("GET /tabs/{id}/count", h.HandleTabCount)
	mux.HandleFunc("GET /tabs/{id}/metrics", h.HandleTabMetrics)
	mux.HandleFunc("GET /metrics", h.HandleMetrics)
	mux.HandleFunc("GET /snapshot", h.HandleSnapshot)
	mux.HandleFunc("GET /frame", h.HandleFrame)
	mux.HandleFunc("POST /frame", h.HandleFrame)
	mux.HandleFunc("GET /screenshot", h.HandleScreenshot)
	mux.HandleFunc("GET /capture", h.HandleCapture)
	mux.HandleFunc("GET /tabs/{id}/pdf", h.HandleTabPDF)
	mux.HandleFunc("POST /tabs/{id}/pdf", h.HandleTabPDF)
	mux.HandleFunc("GET /pdf", h.HandlePDF)
	mux.HandleFunc("POST /pdf", h.HandlePDF)
	mux.HandleFunc("GET /text", h.HandleText)
	mux.HandleFunc("GET /title", h.HandleTitle)
	mux.HandleFunc("GET /url", h.HandleURL)
	mux.HandleFunc("GET /html", h.HandleHTML)
	mux.HandleFunc("GET /styles", h.HandleStyles)
	mux.HandleFunc("GET /value", h.HandleGetValue)
	mux.HandleFunc("GET /attr", h.HandleGetAttr)
	mux.HandleFunc("GET /count", h.HandleCount)
	mux.HandleFunc("GET /box", h.HandleGetBox)
	mux.HandleFunc("GET /tabs/{id}/box", h.HandleTabGetBox)
	mux.HandleFunc("GET /visible", h.HandleGetVisible)
	mux.HandleFunc("GET /tabs/{id}/visible", h.HandleTabGetVisible)
	mux.HandleFunc("GET /enabled", h.HandleGetEnabled)
	mux.HandleFunc("GET /tabs/{id}/enabled", h.HandleTabGetEnabled)
	mux.HandleFunc("GET /checked", h.HandleGetChecked)
	mux.HandleFunc("GET /tabs/{id}/checked", h.HandleTabGetChecked)
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
	mux.HandleFunc("POST /close", h.HandleClose)
	mux.HandleFunc("POST /tabs/{id}/close", h.HandleTabClose)
	mux.HandleFunc("POST /lock", h.HandleTabLock)
	mux.HandleFunc("POST /unlock", h.HandleTabUnlock)
	mux.HandleFunc("POST /tabs/{id}/lock", h.HandleTabLockByID)
	mux.HandleFunc("POST /tabs/{id}/unlock", h.HandleTabUnlockByID)
	mux.HandleFunc("GET /tabs/{id}/cookies", h.HandleTabGetCookies)
	mux.HandleFunc("POST /tabs/{id}/cookies", h.HandleTabSetCookies)
	mux.HandleFunc("DELETE /tabs/{id}/cookies", h.HandleTabClearCookies)
	mux.HandleFunc("GET /cookies", h.HandleGetCookies)
	mux.HandleFunc("POST /cookies", h.HandleSetCookies)
	mux.HandleFunc("DELETE /cookies", h.HandleClearCookies)
	mux.HandleFunc("GET /solvers", h.HandleListSolvers)
	mux.HandleFunc("GET /config/autosolver", h.HandleAutoSolverConfig)
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
	mux.HandleFunc("POST /record/start", h.HandleRecordStart)
	mux.HandleFunc("POST /record/stop", h.HandleRecordStop)
	mux.HandleFunc("GET /record/status", h.HandleRecordStatus)
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
	mux.HandleFunc("POST /network/route", h.HandleNetworkRoute)
	mux.HandleFunc("DELETE /network/route", h.HandleNetworkUnroute)
	mux.HandleFunc("GET /network/route", h.HandleNetworkRouteList)
	mux.HandleFunc("POST /tabs/{id}/network/route", h.HandleTabNetworkRoute)
	mux.HandleFunc("DELETE /tabs/{id}/network/route", h.HandleTabNetworkUnroute)
	mux.HandleFunc("GET /tabs/{id}/network/route", h.HandleTabNetworkRouteList)
	mux.HandleFunc("POST /dialog", h.HandleDialog)
	mux.HandleFunc("POST /tabs/{id}/dialog", h.HandleTabDialog)
	mux.HandleFunc("POST /wait", h.HandleWait)
	mux.HandleFunc("POST /tabs/{id}/wait", h.HandleTabWait)
	mux.HandleFunc("GET /tabs/{id}/state", h.HandleTabState)
	mux.HandleFunc("GET /console", h.HandleGetConsoleLogs)
	mux.HandleFunc("POST /console/clear", h.HandleClearConsoleLogs)
	mux.HandleFunc("GET /errors", h.HandleGetErrorLogs)
	mux.HandleFunc("POST /errors/clear", h.HandleClearErrorLogs)
	mux.HandleFunc("POST /emulation/viewport", h.HandleSetViewport)
	mux.HandleFunc("POST /tabs/{id}/emulation/viewport", h.HandleTabSetViewport)
	mux.HandleFunc("POST /emulation/geolocation", h.HandleSetGeolocation)
	mux.HandleFunc("POST /tabs/{id}/emulation/geolocation", h.HandleTabSetGeolocation)
	mux.HandleFunc("POST /emulation/offline", h.HandleSetOffline)
	mux.HandleFunc("POST /tabs/{id}/emulation/offline", h.HandleTabSetOffline)
	mux.HandleFunc("POST /emulation/headers", h.HandleSetHeaders)
	mux.HandleFunc("POST /tabs/{id}/emulation/headers", h.HandleTabSetHeaders)
	mux.HandleFunc("POST /emulation/credentials", h.HandleSetCredentials)
	mux.HandleFunc("POST /tabs/{id}/emulation/credentials", h.HandleTabSetCredentials)
	mux.HandleFunc("POST /emulation/media", h.HandleSetMedia)
	mux.HandleFunc("POST /tabs/{id}/emulation/media", h.HandleTabSetMedia)

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
	mux.HandleFunc("GET /state", h.HandleStateCurrent)
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

// checkBrowserCanHandle is the single enforcement point for browser capability
// decisions. DecisionSkip returns without error — the caller should fallback to
// Chrome. DecisionFail returns an error for HTTP 400.
func checkBrowserCanHandle(browserName string, intent browsers.RequestIntent) (browsers.HandleDecision, error) {
	b, ok := browsers.Get(browserName)
	if !ok {
		return browsers.HandleDecision{Decision: browsers.DecisionHandle}, nil
	}
	d := b.CanHandle(intent)
	switch d.Decision {
	case browsers.DecisionSkip:
		return d, nil // caller should fallback to Chrome
	case browsers.DecisionFail:
		return d, fmt.Errorf("browser %q failed: %s", browserName, d.Reason)
	default:
		return d, nil
	}
}
