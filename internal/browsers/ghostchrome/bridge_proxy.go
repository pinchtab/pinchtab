package ghostchrome

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome/staticfetch"
)

// ChromeBridge is the subset of bridge.BridgeAPI that BridgeProxy needs
// for tab context resolution, action execution, and tab creation.
// Defined locally to avoid an import cycle with the bridge package
// (bridge → config → browsers/all → ghostchrome).
type ChromeBridge interface {
	TabContext(tabID string) (ctx context.Context, resolvedID string, err error)
	CreateTab(url string) (tabID string, ctx context.Context, cancel context.CancelFunc, err error)
	ExecuteAction(ctx context.Context, kind string, req ActionRequest) (map[string]any, error)
	AvailableActions() []string
}

// ActionRequest mirrors the fields from bridge.ActionRequest that the
// proxy needs for static-action routing. The server layer is responsible
// for type-asserting the real bridge.ActionRequest when calling through.
type ActionRequest struct {
	TabID string
	Kind  string
	Ref   string
	Text  string
	Value string
}

// Action kind constants matching bridge.Action* values.
const (
	ActionClick        = "click"
	ActionType         = "type"
	ActionFill         = "fill"
	ActionPress        = "press"
	ActionHover        = "hover"
	ActionScroll       = "scroll"
	ActionKeyboardType = "keyboard-type"
)

// BridgeProxy wraps a ChromeBridge + *staticfetch.Browser to provide
// transparent ghost-chrome routing. Lite tabs are lazily escalated to
// Chrome when Chrome-only operations are requested.
type BridgeProxy struct {
	chrome        ChromeBridge         // underlying Chrome bridge (all unhandled methods forwarded here)
	lite          *staticfetch.Browser // static fetch browser (may be nil → pure passthrough)
	ensureBrowser func() error         // ensures the browser runtime is running before escalation
	tabMap        tabMapping           // lite tabID → Chrome tabID
	escalationMu  sync.Map             // lite tabID → *sync.Mutex (serializes per-tab escalation)
}

// NewBridgeProxy creates a BridgeProxy that transparently routes between
// the static fetch browser and the Chrome bridge. The ensureBrowser
// function is called before escalating a lite tab to Chrome; it should
// call the bridge browser-init path with the appropriate config.
func NewBridgeProxy(chrome ChromeBridge, lite *staticfetch.Browser, ensureBrowser func() error) *BridgeProxy {
	return &BridgeProxy{
		chrome:        chrome,
		lite:          lite,
		ensureBrowser: ensureBrowser,
	}
}

// tabEscalationLock returns a per-tab mutex that serializes the
// check-then-escalate sequence so concurrent callers for the same
// lite tab don't create duplicate Chrome tabs.
func (p *BridgeProxy) tabEscalationLock(tabID string) *sync.Mutex {
	v, _ := p.escalationMu.LoadOrStore(tabID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// TabContext resolves a tab ID to a context. For lite tabs it lazily
// escalates to Chrome by navigating Chrome to the lite tab's URL.
func (p *BridgeProxy) TabContext(tabID string) (context.Context, string, error) {
	// Try the Chrome bridge first.
	ctx, resolved, err := p.chrome.TabContext(tabID)
	if err == nil {
		return ctx, resolved, nil
	}

	if p.lite == nil {
		return nil, "", err
	}

	mu := p.tabEscalationLock(tabID)
	mu.Lock()
	defer mu.Unlock()

	// Re-check cached mapping under the lock.
	if chromeID, ok := p.tabMap.get(tabID); ok {
		ctx, resolved, mapErr := p.chrome.TabContext(chromeID)
		if mapErr == nil {
			return ctx, resolved, nil
		}
		// Cached mapping is stale — clear it.
		p.tabMap.clear(tabID)
		slog.Debug("cleared stale lite→chrome mapping", "liteTab", tabID, "chromeTab", chromeID)
	}

	// Look up the URL from the static browser.
	url, found := p.lite.TabURL(tabID)
	if !found {
		return nil, "", err // return original error
	}

	// Escalate: ensure Chrome is running and create a tab.
	if p.ensureBrowser != nil {
		if ensureErr := p.ensureBrowser(); ensureErr != nil {
			return nil, "", ensureErr
		}
	}

	chromeTabID, tabCtx, _, createErr := p.chrome.CreateTab(url)
	if createErr != nil {
		return nil, "", createErr
	}

	p.tabMap.set(tabID, chromeTabID)
	slog.Debug("escalated lite tab to Chrome", "liteTab", tabID, "chromeTab", chromeTabID, "url", url)
	return tabCtx, chromeTabID, nil
}

// ExecuteAction intercepts click and type/fill actions when the static
// browser can handle them (ref-based targeting). All other actions
// delegate to chromeFallback if provided, otherwise to the Chrome bridge.
// The chromeFallback allows the adapter layer to pass the full
// bridge.ActionRequest to Chrome without the proxy needing to know about it.
func (p *BridgeProxy) ExecuteAction(ctx context.Context, kind string, req ActionRequest, chromeFallback func(ctx context.Context) (map[string]any, error)) (map[string]any, error) {
	if p.canHandleStaticAction(kind, req.Ref, req.TabID) {
		switch kind {
		case ActionClick:
			if err := p.lite.Click(ctx, req.TabID, req.Ref); err != nil {
				return nil, err
			}
			return map[string]any{"clicked": true}, nil

		case ActionType, ActionFill:
			text := req.Text
			if kind == ActionFill && text == "" {
				text = req.Value
			}
			if err := p.lite.Type(ctx, req.TabID, req.Ref, text); err != nil {
				return nil, err
			}
			return map[string]any{"typed": true, "len": len([]rune(text))}, nil
		}
	}

	if chromeFallback != nil {
		return chromeFallback(ctx)
	}
	return p.chrome.ExecuteAction(ctx, kind, req)
}

// ReleaseTab scrubs all per-tab proxy state: the lite→Chrome mapping, the
// escalation mutex, and the static tab itself. Deleting the escalation mutex
// while another goroutine holds it is safe: the held mutex keeps working, and
// a racing newcomer gets a fresh mutex, re-checks state under it, and finds
// the tab gone (tab-not-found).
func (p *BridgeProxy) ReleaseTab(tabID string) {
	p.tabMap.clear(tabID)
	p.escalationMu.Delete(tabID)
	if p.lite != nil {
		p.lite.CloseTab(tabID)
	}
}

// canHandleStaticAction returns true when a static browser action is
// feasible: the action kind is click or type/fill, the request targets
// an element by ref (not a CSS selector), AND the tab exists in the
// static browser. Escalated tabs are closed in the static browser once
// their ref cache is built (see BridgeAdapter.TabContext), so they fail
// the TabURL check and route to Chrome.
func (p *BridgeProxy) canHandleStaticAction(kind string, ref string, tabID string) bool {
	if p.lite == nil || ref == "" {
		return false
	}
	if tabID != "" {
		if _, ok := p.lite.TabURL(tabID); !ok {
			return false
		}
	}
	switch kind {
	case ActionClick, ActionType, ActionFill:
		return true
	default:
		return false
	}
}

// AvailableActions returns the combined set of available actions from both
// the Chrome bridge and the static browser.
func (p *BridgeProxy) AvailableActions() []string {
	chrome := p.chrome.AvailableActions()
	if p.lite == nil {
		return chrome
	}

	// Ensure click and type are in the list (they may already be if
	// Chrome is running, but add them if Chrome hasn't started yet).
	set := make(map[string]struct{}, len(chrome)+2)
	for _, a := range chrome {
		set[a] = struct{}{}
	}
	set[ActionClick] = struct{}{}
	set[ActionType] = struct{}{}

	result := make([]string, 0, len(set))
	for a := range set {
		result = append(result, a)
	}
	// Deterministic order: handlers interpolate this into "valid values:"
	// error messages, which must not vary per call.
	sort.Strings(result)
	return result
}

// StaticBrowser returns the underlying static fetch browser as a
// BrowserRuntime. Returns nil if no lite browser is configured.
func (p *BridgeProxy) StaticBrowser() browserops.BrowserRuntime {
	if p.lite == nil {
		return nil
	}
	return p.lite
}

// ChromeTabID returns the Chrome tab ID for an escalated lite tab.
// Returns ("", false) if no mapping exists.
func (p *BridgeProxy) ChromeTabID(liteTabID string) (string, bool) {
	return p.tabMap.get(liteTabID)
}

// TabURL returns the URL of a lite tab, if it exists.
func (p *BridgeProxy) TabURL(tabID string) (string, bool) {
	if p.lite == nil {
		return "", false
	}
	return p.lite.TabURL(tabID)
}

// ---------- tab mapping ----------

// tabMapping tracks lite-tabID → Chrome-tabID associations created by
// lazy Chrome escalation. Thread-safe.
type tabMapping struct {
	mu sync.RWMutex
	m  map[string]string
}

func (t *tabMapping) get(liteID string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.m == nil {
		return "", false
	}
	id, ok := t.m[liteID]
	return id, ok
}

func (t *tabMapping) set(liteID, chromeID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = make(map[string]string)
	}
	t.m[liteID] = chromeID
}

func (t *tabMapping) clear(liteID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, liteID)
}
