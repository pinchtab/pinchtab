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

// browserCrash returns the crash recorded for the browser context this request
// is being served on, if any. The lookup is scoped to that context's generation:
// crash state is process-global, so a crash belonging to a browser that has
// since been replaced — or to another instance — must not answer for a healthy
// one. HasCrashDiagnostics cannot serve here: it is a monotonic lifetime flag
// that stays true until process exit.
func (h *Handlers) browserCrash() (bridge.CrashEvent, bool) {
	if h == nil || h.Bridge == nil {
		return bridge.CrashEvent{}, false
	}
	return bridge.CrashForBrowserContext(h.Bridge.BrowserContext())
}

// annotateBrowserCrash rewrites a failing response's message and details to name
// the browser crash behind it. Without a crash on the live browser context both
// come back untouched, so uncrashed responses stay byte-identical.
func (h *Handlers) annotateBrowserCrash(message string, details map[string]any) (string, map[string]any) {
	crash, ok := h.browserCrash()
	if !ok {
		return message, details
	}

	return crashAnnotation(message, details, crash,
		"this error is a symptom of the dead browser, not of your selector or timeout; restart it with: pinchtab server restart")
}

// crashAnnotation is the one shape a crash takes in an error body: the reason in
// the message, browserCrashed/browserCrashReason as discrete fields, and a hint
// that says what the caller should do about it.
func crashAnnotation(message string, details map[string]any, crash bridge.CrashEvent, remedy string) (string, map[string]any) {
	annotated := make(map[string]any, len(details)+3)
	for k, v := range details {
		annotated[k] = v
	}
	annotated["browserCrashed"] = true
	annotated["browserCrashReason"] = crash.Reason
	annotated["hint"] = fmt.Sprintf("the browser crashed (%s at %s) — %s", crash.Reason, crash.Time.Format(time.RFC3339), remedy)
	return fmt.Sprintf("%s (browser crashed: %s)", message, crash.Reason), annotated
}

// errorWithCrashContext is httpx.Error plus the crash annotation.
func (h *Handlers) errorWithCrashContext(w http.ResponseWriter, status int, err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	annotated, details := h.annotateBrowserCrash(message, nil)
	if len(details) == 0 {
		httpx.Error(w, status, err)
		return
	}
	httpx.ErrorCode(w, status, "browser_crashed", annotated, false, details)
}

// errorCodeWithCrashContext is httpx.ErrorCode plus the crash annotation. The
// code is preserved so clients keying on it keep working; the crash arrives as
// message text and details.
func (h *Handlers) errorCodeWithCrashContext(w http.ResponseWriter, status int, code, message string, retryable bool, details map[string]any) {
	annotated, annotatedDetails := h.annotateBrowserCrash(message, details)
	httpx.ErrorCode(w, status, code, annotated, retryable, annotatedDetails)
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
	guards  tabGuards
}

// bridgeBindings is the authoritative bridge route catalog. It is realized as a
// method so the per-binding handlers can close over the receiver h.
func (h *Handlers) bridgeBindings() []routeBinding {
	return []routeBinding{
		{pattern: "POST /navigate", root: h.HandleNavigate, tab: h.HandleTabNavigate, guards: guardHandoffPause},
		{pattern: "POST /back", root: h.HandleBack, tab: h.HandleTabBack, guards: guardHandoffPause},
		{pattern: "POST /forward", root: h.HandleForward, tab: h.HandleTabForward, guards: guardHandoffPause},
		{pattern: "POST /reload", root: h.HandleReload, tab: h.HandleTabReload, guards: guardHandoffPause},
		{pattern: "GET /snapshot", root: h.HandleSnapshot, tab: h.HandleTabSnapshot, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /frame", root: h.HandleFrame, tab: h.HandleTabFrame, guards: guardNone},
		{pattern: "POST /frame", root: h.HandleFrame, tab: h.HandleTabFrame, guards: guardNone},
		{pattern: "GET /screenshot", root: h.HandleScreenshot, tab: h.HandleTabScreenshot, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /annotate", root: h.HandleAnnotate, tab: h.HandleTabAnnotate, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /capture", root: h.HandleCapture, tab: h.HandleTabCapture, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /text", root: h.HandleText, tab: h.HandleTabText, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /title", root: h.HandleTitle, tab: h.HandleTabTitle, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /url", root: h.HandleURL, tab: h.HandleTabURL, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /html", root: h.HandleHTML, tab: h.HandleTabHTML, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /styles", root: h.HandleStyles, tab: h.HandleTabStyles, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /value", root: h.HandleGetValue, tab: h.HandleTabGetValue, guards: guardDomainPolicy},
		{pattern: "GET /attr", root: h.HandleGetAttr, tab: h.HandleTabGetAttr, guards: guardDomainPolicy},
		{pattern: "GET /count", root: h.HandleCount, tab: h.HandleTabCount, guards: guardDomainPolicy},
		{pattern: "GET /box", root: h.HandleGetBox, tab: h.HandleTabGetBox, guards: guardDomainPolicy},
		{pattern: "GET /visible", root: h.HandleGetVisible, tab: h.HandleTabGetVisible, guards: guardDomainPolicy},
		{pattern: "GET /enabled", root: h.HandleGetEnabled, tab: h.HandleTabGetEnabled, guards: guardDomainPolicy},
		{pattern: "GET /checked", root: h.HandleGetChecked, tab: h.HandleTabGetChecked, guards: guardDomainPolicy},
		{pattern: "GET /pdf", root: h.HandlePDF, tab: h.HandleTabPDF, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "POST /pdf", root: h.HandlePDF, tab: h.HandleTabPDF, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "POST /action", root: h.HandleAction, tab: h.HandleTabAction, guards: guardDialogBlocked | guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /actions", root: h.HandleActions, tab: h.HandleTabActions, guards: guardDomainPolicy}, // handoff-pause enforced PER STEP inside the batch loop, not as a request gate
		{pattern: "POST /dialog", root: h.HandleDialog, tab: h.HandleTabDialog, guards: guardHandoffPause},
		{pattern: "POST /wait", root: h.HandleWait, tab: h.HandleTabWait, guards: guardDomainPolicy},
		{pattern: "POST /find", root: h.HandleFind, tab: h.HandleFind, guards: guardDomainPolicy},
		{pattern: "POST /tab", root: h.HandleTab, guards: guardNone},
		{pattern: "POST /close", root: h.HandleClose, tab: h.HandleTabClose, guards: guardNone},
		{pattern: "POST /lock", root: h.HandleTabLock, tab: h.HandleTabLockByID, guards: guardNone},
		{pattern: "POST /unlock", root: h.HandleTabUnlock, tab: h.HandleTabUnlockByID, guards: guardNone},
		{pattern: "POST /handoff", tab: h.HandleTabHandoff, tabOnly: true, guards: guardDomainPolicy},
		{pattern: "POST /resume", tab: h.HandleTabResume, tabOnly: true, guards: guardDomainPolicy},
		{pattern: "GET /handoff", tab: h.HandleTabHandoffStatus, tabOnly: true, guards: guardNone},
		{pattern: "GET /cookies", root: h.HandleGetCookies, tab: h.HandleTabGetCookies, guards: guardDomainPolicy}, // no handoff-pause: reads the tab's cookies, changes nothing the human sees
		{pattern: "POST /cookies", root: h.HandleSetCookies, tab: h.HandleTabSetCookies, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "DELETE /cookies", root: h.HandleClearCookies, tab: h.HandleTabClearCookies, guards: guardNone},
		{pattern: "GET /metrics", root: h.HandleMetrics, tab: h.HandleTabMetrics, guards: guardNone},
		{pattern: "GET /timing", root: h.HandleTiming, tab: h.HandleTabTiming, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "GET /a11y/audit", root: h.HandleA11yAudit, tab: h.HandleTabA11yAudit, guards: guardDialogBlocked | guardDomainPolicy},
		{pattern: "POST /audit/page", root: h.HandleAuditPage, guards: guardNone},
		{pattern: "POST /audit", root: h.HandleAudit, guards: guardNone},
		{pattern: "POST /scrape", root: h.HandleScrape, guards: guardNone},
		{pattern: "GET /network", root: h.HandleNetwork, tab: h.HandleTabNetwork, guards: guardDomainPolicy},                                       // no handoff-pause: the /network reads serve the capture buffer, never the page
		{pattern: "GET /network/stream", root: h.HandleNetworkStream, tab: h.HandleTabNetworkStream, guards: guardDomainPolicy},                    // no handoff-pause: read probe
		{pattern: "GET /network/export", root: h.HandleNetworkExport, tab: h.HandleTabNetworkExport, guards: guardDomainPolicy},                    // no handoff-pause: read probe
		{pattern: "GET /network/export/stream", root: h.HandleNetworkExportStream, tab: h.HandleTabNetworkExportStream, guards: guardDomainPolicy}, // no handoff-pause: read probe
		{pattern: "GET /network/{requestId}", root: h.HandleNetworkByID, tab: h.HandleTabNetworkByID, guards: guardDomainPolicy},                   // no handoff-pause: read probe
		{pattern: "POST /network/clear", root: h.HandleNetworkClear, guards: guardNone},
		{pattern: "GET /network/route", root: h.HandleNetworkRouteList, tab: h.HandleTabNetworkRouteList, guards: guardNone},
		{pattern: "POST /network/route", root: h.HandleNetworkRoute, tab: h.HandleTabNetworkRoute, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "DELETE /network/route", root: h.HandleNetworkUnroute, tab: h.HandleTabNetworkUnroute, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "GET /console", root: h.HandleGetConsoleLogs, guards: guardDomainPolicy},
		{pattern: "POST /console/clear", root: h.HandleClearConsoleLogs, guards: guardNone},
		{pattern: "GET /errors", root: h.HandleGetErrorLogs, guards: guardDomainPolicy},
		{pattern: "POST /errors/clear", root: h.HandleClearErrorLogs, guards: guardNone},
		{pattern: "GET /clipboard/read", root: h.HandleClipboardRead, guards: guardNone},
		{pattern: "POST /clipboard/write", root: h.HandleClipboardWrite, guards: guardNone},
		{pattern: "POST /clipboard/copy", root: h.HandleClipboardCopy, guards: guardNone},
		{pattern: "GET /clipboard/paste", root: h.HandleClipboardPaste, guards: guardNone},
		{pattern: "GET /stealth/status", root: h.HandleStealthStatus, guards: guardNone},
		{pattern: "POST /fingerprint/rotate", root: h.HandleFingerprintRotate, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "GET /solvers", root: h.HandleListSolvers, guards: guardNone},
		{pattern: "GET /config/autosolver", root: h.HandleAutoSolverConfig, guards: guardNone},
		{pattern: "POST /solve", root: h.HandleSolve, tab: h.HandleTabSolve, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /solve/{name}", root: h.HandleSolve, tab: h.HandleTabSolve, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /emulation/viewport", root: h.HandleSetViewport, tab: h.HandleTabSetViewport, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /emulation/geolocation", root: h.HandleSetGeolocation, tab: h.HandleTabSetGeolocation, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /emulation/offline", root: h.HandleSetOffline, tab: h.HandleTabSetOffline, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /emulation/headers", root: h.HandleSetHeaders, tab: h.HandleTabSetHeaders, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /emulation/credentials", root: h.HandleSetCredentials, tab: h.HandleTabSetCredentials, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /emulation/media", root: h.HandleSetMedia, tab: h.HandleTabSetMedia, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /cache/clear", root: h.HandleCacheClear, guards: guardNone},
		{pattern: "GET /cache/status", root: h.HandleCacheStatus, guards: guardNone},
		{pattern: "POST /storage", root: h.HandleStorage, tab: h.HandleTabStorageSet, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "DELETE /storage", root: h.HandleStorage, tab: h.HandleTabStorageDelete, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "GET /storage", root: h.HandleStorage, tab: h.HandleTabStorageGet, guards: guardDomainPolicy}, // no handoff-pause: read probe; the POST/DELETE siblings carry it
		{pattern: "GET /state", root: h.HandleStateCurrent, guards: guardDomainPolicy},                          // no handoff-pause: captures a snapshot, writes nothing to the tab
		{pattern: "GET /state/list", root: h.HandleStateList, guards: guardNone},
		{pattern: "GET /state/show", root: h.HandleStateShow, guards: guardNone},
		{pattern: "POST /state/save", root: h.HandleStateSave, guards: guardDomainPolicy}, // no handoff-pause: POST writes a file, not the tab; /state/load is the write and carries it
		{pattern: "POST /state/load", root: h.HandleStateLoad, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "DELETE /state", root: h.HandleStateDelete, guards: guardNone},
		{pattern: "POST /state/clean", root: h.HandleStateClean, guards: guardNone},
		{pattern: "POST /evaluate", root: h.HandleEvaluate, tab: h.HandleTabEvaluate, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /macro", root: h.HandleMacro, guards: guardDomainPolicy}, // handoff-pause enforced PER STEP inside the macro loop, not as a request gate
		{pattern: "GET /download", root: h.HandleDownload, tab: h.HandleTabDownload, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "POST /upload", root: h.HandleUpload, tab: h.HandleTabUpload, guards: guardDomainPolicy | guardHandoffPause},
		{pattern: "GET /screencast", root: h.HandleScreencast, guards: guardDomainPolicy},
		{pattern: "GET /screencast/tabs", root: h.HandleScreencastAll, guards: guardNone},
		{pattern: "POST /record/start", root: h.HandleRecordStart, guards: guardNone},
		{pattern: "POST /record/stop", root: h.HandleRecordStop, guards: guardNone},
		{pattern: "GET /record/status", root: h.HandleRecordStatus, guards: guardNone},
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
