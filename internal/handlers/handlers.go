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
	"github.com/pinchtab/pinchtab/internal/routes"
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
	CurrentTabs     *CurrentTabStore
	Version         string
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
		CurrentTabs:     NewCurrentTabStore(),
		credentialStore: newCredentialStore(),
		recorder:        &recorder{},
	}

	h.recorder.captureFrame = func(ctx context.Context, quality int) ([]byte, error) {
		return h.Bridge.CaptureScreenshot(ctx, "jpeg", quality, nil)
	}

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

	h.evalJS = func(ctx context.Context, expression string, out *string) error {
		return h.Bridge.Evaluate(ctx, expression, out, bridge.EvalOpts{})
	}
	h.autoSolverRunner = h.runAutoSolver
	h.evalRuntime = func(ctx context.Context, expression string, out any, opts bridge.EvalOpts) error {
		return h.Bridge.Evaluate(ctx, expression, out, opts)
	}

	if notifier, ok := h.Bridge.(tabRemovalNotifier); ok {
		notifier.AddTabRemovedHook(h.credentialStore.RemoveTab)
	}

	return h
}

// StartBackgroundCleanup launches best-effort startup cleanup of stale export
// and upload temp files. It is a process-wide side effect and must be invoked
// explicitly by the server bootstrap, not by New, so constructing a Handlers
// (e.g. in tests) does not spawn filesystem work.
func (h *Handlers) StartBackgroundCleanup() {
	if h == nil || h.Config == nil {
		return
	}
	go CleanupStaleTmpExports(h.Config.StateDir)
	go CleanupStaleUploads(h.Config.StateDir)
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

func (h *Handlers) ensureBrowser(cfg *config.RuntimeConfig) error {
	if cfg == nil {
		cfg = h.Config
	}
	return h.Bridge.EnsureBrowser(cfg)
}

func (h *Handlers) ensureBrowserOrRespond(w http.ResponseWriter, cfg *config.RuntimeConfig) bool {
	if err := h.ensureBrowser(cfg); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return false
		}
		httpx.Error(w, 500, fmt.Errorf("browser initialization: %w", err))
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
	h.registerBridgeRoutes(mux)
	h.registerSpecialRoutes(mux, doShutdown)

	if h.Profiles != nil {
		h.Profiles.RegisterHandlers(mux)
	}
	if h.Dashboard != nil {
		h.Dashboard.RegisterHandlers(mux)
	}
	if h.Orchestrator != nil {
		h.Orchestrator.RegisterHandlers(mux)
	}
}

// muxRegistrar is the subset of *http.ServeMux used by route registration so a
// test recorder can capture the registered pattern set for catalog-parity checks.
type muxRegistrar interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// routeBinding pairs one catalog Endpoint.Route() with the handlers that serve
// its root and /tabs/{id}/... forms. It is the single authoritative source for
// bridge route→handler wiring: each route string appears exactly once here
// instead of being repeated across parallel root/tab/tab-only tables.
//
//   - root:    handler for the root form (e.g. "POST /navigate"); nil ⇒ tabOnly
//     and no root route is registered.
//   - tab:     handler for the "POST /tabs/{id}/navigate" form; set iff the
//     catalog Endpoint is TabScoped.
//   - tabOnly: true when the endpoint is registered ONLY in its /tabs/{id}/...
//     form because the operation is inherently tab-bound (handoff/resume).
type routeBinding struct {
	pattern string
	root    http.HandlerFunc
	tab     http.HandlerFunc
	tabOnly bool
}

// bridgeBindings is the authoritative bridge route catalog. It is realized as a
// method so the per-binding handlers can close over the receiver h.
func (h *Handlers) bridgeBindings() []routeBinding {
	return []routeBinding{
		{pattern: "POST /navigate", root: h.HandleNavigate, tab: h.HandleTabNavigate},
		{pattern: "POST /back", root: h.HandleBack, tab: h.HandleTabBack},
		{pattern: "POST /forward", root: h.HandleForward, tab: h.HandleTabForward},
		{pattern: "POST /reload", root: h.HandleReload, tab: h.HandleTabReload},
		{pattern: "GET /snapshot", root: h.HandleSnapshot, tab: h.HandleTabSnapshot},
		{pattern: "GET /frame", root: h.HandleFrame, tab: h.HandleTabFrame},
		{pattern: "POST /frame", root: h.HandleFrame, tab: h.HandleTabFrame},
		{pattern: "GET /screenshot", root: h.HandleScreenshot, tab: h.HandleTabScreenshot},
		{pattern: "GET /annotate", root: h.HandleAnnotate, tab: h.HandleTabAnnotate},
		{pattern: "GET /capture", root: h.HandleCapture, tab: h.HandleTabCapture},
		{pattern: "GET /text", root: h.HandleText, tab: h.HandleTabText},
		{pattern: "GET /title", root: h.HandleTitle, tab: h.HandleTabTitle},
		{pattern: "GET /url", root: h.HandleURL, tab: h.HandleTabURL},
		{pattern: "GET /html", root: h.HandleHTML, tab: h.HandleTabHTML},
		{pattern: "GET /styles", root: h.HandleStyles, tab: h.HandleTabStyles},
		{pattern: "GET /value", root: h.HandleGetValue, tab: h.HandleTabGetValue},
		{pattern: "GET /attr", root: h.HandleGetAttr, tab: h.HandleTabGetAttr},
		{pattern: "GET /count", root: h.HandleCount, tab: h.HandleTabCount},
		{pattern: "GET /box", root: h.HandleGetBox, tab: h.HandleTabGetBox},
		{pattern: "GET /visible", root: h.HandleGetVisible, tab: h.HandleTabGetVisible},
		{pattern: "GET /enabled", root: h.HandleGetEnabled, tab: h.HandleTabGetEnabled},
		{pattern: "GET /checked", root: h.HandleGetChecked, tab: h.HandleTabGetChecked},
		{pattern: "GET /pdf", root: h.HandlePDF, tab: h.HandleTabPDF},
		{pattern: "POST /pdf", root: h.HandlePDF, tab: h.HandleTabPDF},
		{pattern: "POST /action", root: h.HandleAction, tab: h.HandleTabAction},
		{pattern: "POST /actions", root: h.HandleActions, tab: h.HandleTabActions},
		{pattern: "POST /dialog", root: h.HandleDialog, tab: h.HandleTabDialog},
		{pattern: "POST /wait", root: h.HandleWait, tab: h.HandleTabWait},
		{pattern: "POST /find", root: h.HandleFind, tab: h.HandleFind},
		{pattern: "POST /tab", root: h.HandleTab},
		{pattern: "POST /close", root: h.HandleClose, tab: h.HandleTabClose},
		{pattern: "POST /lock", root: h.HandleTabLock, tab: h.HandleTabLockByID},
		{pattern: "POST /unlock", root: h.HandleTabUnlock, tab: h.HandleTabUnlockByID},
		{pattern: "POST /handoff", tab: h.HandleTabHandoff, tabOnly: true},
		{pattern: "POST /resume", tab: h.HandleTabResume, tabOnly: true},
		{pattern: "GET /handoff", tab: h.HandleTabHandoffStatus, tabOnly: true},
		{pattern: "GET /cookies", root: h.HandleGetCookies, tab: h.HandleTabGetCookies},
		{pattern: "POST /cookies", root: h.HandleSetCookies, tab: h.HandleTabSetCookies},
		{pattern: "DELETE /cookies", root: h.HandleClearCookies, tab: h.HandleTabClearCookies},
		{pattern: "GET /metrics", root: h.HandleMetrics, tab: h.HandleTabMetrics},
		{pattern: "GET /timing", root: h.HandleTiming, tab: h.HandleTabTiming},
		{pattern: "GET /a11y/audit", root: h.HandleA11yAudit, tab: h.HandleTabA11yAudit},
		{pattern: "POST /audit/page", root: h.HandleAuditPage},
		{pattern: "GET /network", root: h.HandleNetwork, tab: h.HandleTabNetwork},
		{pattern: "GET /network/stream", root: h.HandleNetworkStream, tab: h.HandleTabNetworkStream},
		{pattern: "GET /network/export", root: h.HandleNetworkExport, tab: h.HandleTabNetworkExport},
		{pattern: "GET /network/export/stream", root: h.HandleNetworkExportStream, tab: h.HandleTabNetworkExportStream},
		{pattern: "GET /network/{requestId}", root: h.HandleNetworkByID, tab: h.HandleTabNetworkByID},
		{pattern: "POST /network/clear", root: h.HandleNetworkClear},
		{pattern: "GET /network/route", root: h.HandleNetworkRouteList, tab: h.HandleTabNetworkRouteList},
		{pattern: "POST /network/route", root: h.HandleNetworkRoute, tab: h.HandleTabNetworkRoute},
		{pattern: "DELETE /network/route", root: h.HandleNetworkUnroute, tab: h.HandleTabNetworkUnroute},
		{pattern: "GET /console", root: h.HandleGetConsoleLogs},
		{pattern: "POST /console/clear", root: h.HandleClearConsoleLogs},
		{pattern: "GET /errors", root: h.HandleGetErrorLogs},
		{pattern: "POST /errors/clear", root: h.HandleClearErrorLogs},
		{pattern: "GET /clipboard/read", root: h.HandleClipboardRead},
		{pattern: "POST /clipboard/write", root: h.HandleClipboardWrite},
		{pattern: "POST /clipboard/copy", root: h.HandleClipboardCopy},
		{pattern: "GET /clipboard/paste", root: h.HandleClipboardPaste},
		{pattern: "GET /stealth/status", root: h.HandleStealthStatus},
		{pattern: "POST /fingerprint/rotate", root: h.HandleFingerprintRotate},
		{pattern: "GET /solvers", root: h.HandleListSolvers},
		{pattern: "GET /config/autosolver", root: h.HandleAutoSolverConfig},
		{pattern: "POST /solve", root: h.HandleSolve, tab: h.HandleTabSolve},
		{pattern: "POST /solve/{name}", root: h.HandleSolve, tab: h.HandleTabSolve},
		{pattern: "POST /emulation/viewport", root: h.HandleSetViewport, tab: h.HandleTabSetViewport},
		{pattern: "POST /emulation/geolocation", root: h.HandleSetGeolocation, tab: h.HandleTabSetGeolocation},
		{pattern: "POST /emulation/offline", root: h.HandleSetOffline, tab: h.HandleTabSetOffline},
		{pattern: "POST /emulation/headers", root: h.HandleSetHeaders, tab: h.HandleTabSetHeaders},
		{pattern: "POST /emulation/credentials", root: h.HandleSetCredentials, tab: h.HandleTabSetCredentials},
		{pattern: "POST /emulation/media", root: h.HandleSetMedia, tab: h.HandleTabSetMedia},
		{pattern: "POST /cache/clear", root: h.HandleCacheClear},
		{pattern: "GET /cache/status", root: h.HandleCacheStatus},
		{pattern: "POST /storage", root: h.HandleStorage, tab: h.HandleTabStorageSet},
		{pattern: "DELETE /storage", root: h.HandleStorage, tab: h.HandleTabStorageDelete},
		{pattern: "GET /storage", root: h.HandleStorage, tab: h.HandleTabStorageGet},
		{pattern: "GET /state", root: h.HandleStateCurrent},
		{pattern: "GET /state/list", root: h.HandleStateList},
		{pattern: "GET /state/show", root: h.HandleStateShow},
		{pattern: "POST /state/save", root: h.HandleStateSave},
		{pattern: "POST /state/load", root: h.HandleStateLoad},
		{pattern: "DELETE /state", root: h.HandleStateDelete},
		{pattern: "POST /state/clean", root: h.HandleStateClean},
		{pattern: "POST /evaluate", root: h.HandleEvaluate, tab: h.HandleTabEvaluate},
		{pattern: "POST /macro", root: h.HandleMacro},
		{pattern: "GET /download", root: h.HandleDownload, tab: h.HandleTabDownload},
		{pattern: "POST /upload", root: h.HandleUpload, tab: h.HandleTabUpload},
		{pattern: "GET /screencast", root: h.HandleScreencast},
		{pattern: "GET /screencast/tabs", root: h.HandleScreencastAll},
		{pattern: "POST /record/start", root: h.HandleRecordStart},
		{pattern: "POST /record/stop", root: h.HandleRecordStop},
		{pattern: "GET /record/status", root: h.HandleRecordStatus},
	}
}

// tabOnlyRoutes lists catalog endpoints registered ONLY in their /tabs/{id}/...
// form: they are TabScoped but have no root-level handler because the operation
// is inherently tab-bound (handoff/resume). Keyed by Endpoint.Route(). Derived
// from the single bridgeBindings catalog so it can never drift from the wiring.
var tabOnlyRoutes = func() map[string]bool {
	m := map[string]bool{}
	for _, b := range (*Handlers)(nil).bridgeBindings() {
		if b.tabOnly {
			m[b.pattern] = true
		}
	}
	return m
}()

// specialCaseRoutes are routes registered outside the catalog loop: meta/docs
// endpoints, management routes, GET aliases of POST verbs, the ungated tab-state
// view, and the conditional shutdown route. The parity test treats the live
// route set as {catalog-derived} ∪ specialCaseRoutes, so this is the only
// hand-maintained registration list that must track registerSpecialRoutes.
var specialCaseRoutes = []string{
	"GET /health",
	"POST /ensure-browser",
	"POST /ensure-chrome",
	"POST /browser/restart",
	"GET /tabs",
	"GET /openapi.json",
	"GET /help",
	"GET /navigate",
	"GET /action",
	"GET /tabs/{id}/state",
	"POST /shutdown",
}

// registerBridgeRoutes registers the bridge API surface by walking the shared
// routes catalog and resolving each endpoint against the single bridgeBindings
// table: a root route for every endpoint except tab-only ones, plus the
// /tabs/{id}/... variant for every TabScoped endpoint. A missing or incomplete
// binding panics so a catalog entry can never be silently left unrouted.
func (h *Handlers) registerBridgeRoutes(mux muxRegistrar) {
	bind := map[string]routeBinding{}
	for _, b := range h.bridgeBindings() {
		bind[b.pattern] = b
	}
	for _, ep := range routes.Core() {
		b, ok := bind[ep.Route()]
		if !ok {
			panic("handlers: no binding for catalog route " + ep.Route())
		}
		if !b.tabOnly {
			if b.root == nil {
				panic("handlers: no root handler for catalog route " + ep.Route())
			}
			mux.HandleFunc(ep.Route(), b.root)
		}
		if ep.TabScoped {
			if b.tab == nil {
				panic("handlers: no tab handler for catalog route " + ep.Route())
			}
			mux.HandleFunc(ep.TabRoute(), b.tab)
		}
	}
}

func (h *Handlers) registerSpecialRoutes(mux muxRegistrar, doShutdown func()) {
	mux.HandleFunc("GET /health", h.HandleHealth)
	mux.HandleFunc("POST /ensure-browser", h.HandleEnsureBrowser)
	// Back-compat alias: older orchestrators retry lazy init via the pre-rename
	// path; without it a version-skewed pair 404s. Keeps the legacy "chrome_ready"
	// status for orchestrators that string-match it.
	mux.HandleFunc("POST /ensure-chrome", h.HandleEnsureChrome)
	mux.HandleFunc("POST /browser/restart", h.HandleBrowserRestart)
	mux.HandleFunc("GET /tabs", h.HandleTabs)
	mux.HandleFunc("GET /openapi.json", h.HandleOpenAPI)
	mux.HandleFunc("GET /help", h.HandleOpenAPI)
	mux.HandleFunc("GET /navigate", h.HandleNavigate)
	mux.HandleFunc("GET /action", h.HandleAction)
	// GET /state is the capability-gated full browser state; GET /tabs/{id}/state
	// is the ungated lightweight tab-runtime readiness view, so it is not a
	// TabScoped catalog entry and is registered explicitly here.
	mux.HandleFunc("GET /tabs/{id}/state", h.HandleTabState)
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
		return d, nil
	case browsers.DecisionFail:
		return d, fmt.Errorf("browser %q failed: %s", browserName, d.Reason)
	default:
		return d, nil
	}
}
