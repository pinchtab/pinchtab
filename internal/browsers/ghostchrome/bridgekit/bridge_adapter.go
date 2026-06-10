// Package bridgekit provides BridgeAdapter, a bridge.BridgeAPI wrapper
// that encapsulates all ghost-chrome static-vs-Chrome routing logic.
//
// It lives in a sub-package of ghostchrome to break the import cycle
// that would occur if ghostchrome imported bridge directly
// (bridge -> config -> browsers/all -> ghostchrome -> bridge).
//
// The adapter is split across files by responsibility:
//   - bridge_adapter.go — core wiring: types, construction, provider
//     registration, tab-context resolution, action dispatch.
//   - bridge_adapter_refmap.go — escalated ref-cache population.
//   - bridge_adapter_content.go — static-first Navigate/Snapshot/Text
//     routing and escalation helpers.
//   - bridge_adapter_passthrough.go — thin delegations to the embedded
//     Chrome bridge.
package bridgekit

import (
	"context"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome/staticfetch"
	"github.com/pinchtab/pinchtab/internal/browsers/providerhooks"
	"github.com/pinchtab/pinchtab/internal/config"
)

// chromeActionAdapter wraps bridge.BridgeAPI to satisfy
// ghostchrome.ChromeBridge by translating between bridge.ActionRequest
// and ghostchrome.ActionRequest.
type chromeActionAdapter struct {
	bridge.BridgeAPI
}

// TabContext adapts the BridgeAPI.TabContext (which returns *bridge.TabHandle)
// to the ghostchrome.ChromeBridge interface (which returns context.Context).
func (a *chromeActionAdapter) TabContext(tabID string) (context.Context, string, error) {
	return a.BridgeAPI.TabContext(tabID)
}

func (a *chromeActionAdapter) ExecuteAction(ctx context.Context, kind string, req ghostchrome.ActionRequest) (map[string]any, error) {
	return a.BridgeAPI.ExecuteAction(ctx, kind, bridge.ActionRequest{
		TabID: req.TabID,
		Kind:  req.Kind,
		Ref:   req.Ref,
		Text:  req.Text,
		Value: req.Value,
	})
}

// BridgeAdapter wraps bridge.BridgeAPI and delegates TabContext,
// ExecuteAction, and AvailableActions to the ghost-chrome BridgeProxy.
// All other methods pass through to the embedded BridgeAPI unchanged.
type BridgeAdapter struct {
	bridge.BridgeAPI // embedded Chrome bridge — all unoverridden methods pass through
	proxy            *ghostchrome.BridgeProxy
	cfg              *config.RuntimeConfig
}

// NewBridgeAdapter creates a BridgeAdapter that encapsulates all
// ghost-chrome static-vs-Chrome routing logic. The returned adapter
// satisfies bridge.BridgeAPI and transparently delegates to the
// underlying Chrome bridge for methods not intercepted by the proxy.
func NewBridgeAdapter(chromeBridge bridge.BridgeAPI, cfg *config.RuntimeConfig) *BridgeAdapter {
	lite := staticfetch.NewBrowser()
	chromeAdapter := &chromeActionAdapter{BridgeAPI: chromeBridge}
	proxy := ghostchrome.NewBridgeProxy(chromeAdapter, lite, func() error {
		return ensureBrowser(chromeBridge, cfg)
	})
	return &BridgeAdapter{
		BridgeAPI: chromeBridge,
		proxy:     proxy,
		cfg:       cfg,
	}
}

func ensureBrowser(b bridge.BridgeAPI, cfg *config.RuntimeConfig) error {
	return b.EnsureBrowser(cfg)
}

func (a *BridgeAdapter) EnsureBrowser(cfg *config.RuntimeConfig) error {
	return ensureBrowser(a.BridgeAPI, cfg)
}

func init() {
	providerhooks.Register("ghost-chrome", providerhooks.Hooks{
		DecorateBridge: func(api bridge.BridgeAPI, cfg *config.RuntimeConfig) bridge.BridgeAPI {
			return NewBridgeAdapter(api, cfg)
		},
		CleanupProfile: bridge.CleanupOrphanedChromeProcesses,
		Shutdown: func() {
			bridge.KillAllPinchtabChrome()
		},
	})
}

func (a *BridgeAdapter) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	ctx, resolvedID, err := a.proxy.TabContext(tabID)
	if err != nil {
		return nil, resolvedID, err
	}
	if resolvedID != tabID {
		a.populateEscalatedRefCache(ctx, resolvedID, tabID)
		// Escalation transfers ownership to Chrome: with the ref cache built,
		// the static tab's only remaining effect would be serving stale-DOM
		// reads/actions, so retire it. No-op for non-lite ID aliasing and on
		// repeat calls (the mapping keeps resolving via tabMap).
		if sb := a.StaticBrowser(); sb != nil {
			sb.CloseTab(tabID)
		}
	}
	// Wrap the context from the proxy in a TabHandle. The proxy's
	// TabContext returns context.Context (from ChromeBridge interface),
	// which is already a *bridge.TabHandle at runtime (set by
	// Bridge.TabContext). We re-wrap to satisfy the return type.
	if th, ok := ctx.(*bridge.TabHandle); ok {
		return th, resolvedID, nil
	}
	return bridge.NewTabHandle(ctx), resolvedID, nil
}

func (a *BridgeAdapter) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	return a.proxy.ExecuteAction(ctx, kind, ghostchrome.ActionRequest{
		TabID: req.TabID,
		Kind:  req.Kind,
		Ref:   req.Ref,
		Text:  req.Text,
		Value: req.Value,
	}, func(ctx context.Context) (map[string]any, error) {
		return a.BridgeAPI.ExecuteAction(ctx, kind, req)
	})
}

// StaticBrowser returns the underlying static fetch browser.
// Returns nil if no lite browser is configured.
func (a *BridgeAdapter) StaticBrowser() *staticfetch.Browser {
	rt := a.proxy.StaticBrowser()
	if rt == nil {
		return nil
	}
	// The proxy wraps the *staticfetch.Browser as browserops.BrowserRuntime;
	// assert it back to the concrete type.
	if b, ok := rt.(*staticfetch.Browser); ok {
		return b
	}
	return nil
}
