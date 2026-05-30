// Package bridgekit provides BridgeAdapter, a bridge.BridgeAPI wrapper
// that encapsulates all ghost-chrome static-vs-Chrome routing logic.
//
// It lives in a sub-package of ghostchrome to break the import cycle
// that would occur if ghostchrome imported bridge directly
// (bridge -> config -> browsers/all -> ghostchrome -> bridge).
package bridgekit

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome/staticfetch"
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
		return chromeBridge.EnsureChrome(cfg)
	})
	return &BridgeAdapter{
		BridgeAPI: chromeBridge,
		proxy:     proxy,
		cfg:       cfg,
	}
}

func (a *BridgeAdapter) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	ctx, resolvedID, err := a.proxy.TabContext(tabID)
	if err != nil {
		return nil, resolvedID, err
	}
	if resolvedID != tabID {
		a.populateEscalatedRefCache(ctx, resolvedID, tabID)
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

// populateEscalatedRefCache builds a ref cache for a newly escalated Chrome
// tab that maps the STATIC browser's ref names to Chrome's BackendNodeIDs.
// This is necessary because Chrome and the static browser assign different
// sequential ref numbers (e0, e1, ...) to the same page elements. We match
// by (role, name) to bridge the two numbering schemes.
func (a *BridgeAdapter) populateEscalatedRefCache(ctx context.Context, chromeTabID, liteTabID string) {
	if a.BridgeAPI.GetRefCache(chromeTabID) != nil {
		return
	}

	staticBrowser := a.proxy.StaticBrowser()
	if staticBrowser == nil {
		return
	}
	staticSnap, err := staticBrowser.Snapshot(context.Background(), liteTabID, "interactive")
	if err != nil || len(staticSnap.Nodes) == 0 {
		return
	}

	chromeNodes, err := bridge.FetchAXTree(ctx)
	if err != nil {
		return
	}
	flat, _ := bridge.BuildSnapshot(chromeNodes, bridge.FilterInteractive, -1)
	_ = bridge.EnrichA11yNodesWithDOMMetadata(ctx, flat)

	type chromeEntry struct {
		node bridge.A11yNode
		used bool
	}
	chromeByKey := map[string][]*chromeEntry{}
	for i := range flat {
		key := flat[i].Role + "\x00" + flat[i].Name
		chromeByKey[key] = append(chromeByKey[key], &chromeEntry{node: flat[i]})
	}

	refs := make(map[string]int64, len(staticSnap.Nodes))
	targets := make(map[string]bridge.RefTarget, len(staticSnap.Nodes))
	for _, sn := range staticSnap.Nodes {
		if sn.Ref == "" {
			continue
		}
		key := sn.Role + "\x00" + sn.Name
		for _, entry := range chromeByKey[key] {
			if !entry.used && entry.node.NodeID != 0 {
				entry.used = true
				refs[sn.Ref] = entry.node.NodeID
				targets[sn.Ref] = bridge.RefTarget{
					BackendNodeID:  entry.node.NodeID,
					FrameID:        entry.node.FrameID,
					FrameURL:       entry.node.FrameURL,
					FrameName:      entry.node.FrameName,
					ChildFrameID:   entry.node.ChildFrameID,
					ChildFrameURL:  entry.node.ChildFrameURL,
					ChildFrameName: entry.node.ChildFrameName,
				}
				break
			}
		}
	}

	slog.Debug("populated escalated ref cache",
		"chromeTab", chromeTabID, "liteTab", liteTabID,
		"staticRefs", len(staticSnap.Nodes), "mapped", len(refs))

	a.SetRefCache(chromeTabID, &bridge.RefCache{
		Refs:    refs,
		Targets: targets,
		Nodes:   flat,
	})
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

func (a *BridgeAdapter) FocusTab(tabID string) error {
	if chromeID, ok := a.proxy.ChromeTabID(tabID); ok {
		return a.BridgeAPI.FocusTab(chromeID)
	}
	return a.BridgeAPI.FocusTab(tabID)
}

func (a *BridgeAdapter) CloseTab(tabID string) error {
	if chromeID, ok := a.proxy.ChromeTabID(tabID); ok {
		return a.BridgeAPI.CloseTab(chromeID)
	}
	return a.BridgeAPI.CloseTab(tabID)
}

func (a *BridgeAdapter) GetRefCache(tabID string) *bridge.RefCache {
	if cache := a.BridgeAPI.GetRefCache(tabID); cache != nil {
		return cache
	}
	if chromeID, ok := a.proxy.ChromeTabID(tabID); ok {
		return a.BridgeAPI.GetRefCache(chromeID)
	}
	return nil
}

func (a *BridgeAdapter) AvailableActions() []string {
	return a.proxy.AvailableActions()
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

// Navigate implements the ghost-chrome "static first, assess quality, escalate
// to Chrome" routing pattern for navigation. It tries the static browser first;
// if the fetched content passes the quality gate it returns immediately.
// Otherwise it falls through to the Chrome bridge.
func (a *BridgeAdapter) Navigate(ctx context.Context, url string, params bridge.NavigateParams) (*bridge.NavigateResult, error) {
	staticBrowser := a.proxy.StaticBrowser()
	if staticBrowser != nil {
		// Attach network policy from NavigateParams so the static
		// browser enforces SSRF/redirect protections.
		ctx = staticfetch.WithNavigateNetworkPolicy(ctx, navigateParamsToPolicy(&params))
		navResult, err := staticBrowser.Navigate(ctx, url)
		if err == nil {
			// Assess content quality by extracting text from the static page.
			textResult, textErr := staticBrowser.Text(ctx, navResult.TabID)
			if textErr == nil {
				gr := ghostchrome.AssessContent(textResult.Text)
				if gr.ShouldAccept() {
					slog.Debug("ghost-chrome navigate: static accepted",
						"url", url, "quality", gr.Quality)
					return &bridge.NavigateResult{
						TabID: navResult.TabID,
						URL:   navResult.URL,
						Title: navResult.Title,
						Route: &browserops.RouteMetadata{
							RequestedBrowser: "ghost-chrome",
							UsedBrowser:      "ghost-chrome",
							Quality:          gr.Quality,
							Attempts: []browserops.RouteAttempt{
								{Browser: "ghost-chrome", Accepted: true, Reason: gr.FormatReason()},
							},
						},
					}, nil
				}
				// Quality too low — escalate to Chrome.
				slog.Debug("ghost-chrome navigate: quality too low, escalating",
					"url", url, "quality", gr.Quality, "reason", gr.FormatReason())
				chromeResult, chromeErr := a.BridgeAPI.Navigate(ctx, url, params)
				if chromeErr != nil {
					return nil, chromeErr
				}
				chromeResult.Route = &browserops.RouteMetadata{
					RequestedBrowser: "ghost-chrome",
					UsedBrowser:      "ghost-chrome",
					Escalated:        true,
					Quality:          gr.Quality,
					Attempts: []browserops.RouteAttempt{
						{Browser: "ghost-chrome", Accepted: false, Reason: gr.FormatReason()},
						{Browser: "chrome", Accepted: true, Reason: "escalated"},
					},
				}
				return chromeResult, nil
			}
			// Text extraction failed — escalate with zero quality.
			slog.Debug("ghost-chrome navigate: text extraction failed, escalating",
				"url", url, "err", textErr)
		} else {
			slog.Debug("ghost-chrome navigate: static navigate failed, escalating",
				"url", url, "err", err)
		}
	}

	// Static browser unavailable or failed — go straight to Chrome.
	chromeResult, chromeErr := a.BridgeAPI.Navigate(ctx, url, params)
	if chromeErr != nil {
		return nil, chromeErr
	}
	chromeResult.Route = &browserops.RouteMetadata{
		RequestedBrowser: "ghost-chrome",
		UsedBrowser:      "ghost-chrome",
		Escalated:        true,
		Quality:          0,
		Attempts: []browserops.RouteAttempt{
			{Browser: "ghost-chrome", Accepted: false, Reason: "static browser unavailable or failed"},
			{Browser: "chrome", Accepted: true, Reason: "escalated"},
		},
	}
	return chromeResult, nil
}

// Snapshot implements the ghost-chrome "static first, assess quality, escalate
// to Chrome" routing pattern for snapshots. It tries reading the accessibility
// tree from the static browser; if the snapshot passes the quality gate it
// converts the nodes to bridge.A11yNode and returns. Otherwise it falls
// through to Chrome.
func (a *BridgeAdapter) Snapshot(ctx context.Context, tabID string, filter string, params bridge.ContentParams) (*bridge.SnapshotResult, error) {
	staticBrowser := a.proxy.StaticBrowser()
	if staticBrowser != nil {
		snapResult, err := staticBrowser.Snapshot(context.Background(), tabID, filter)
		if err == nil && len(snapResult.Nodes) > 0 {
			// Convert to ghostchrome.SnapshotNode for quality assessment.
			assessNodes := make([]ghostchrome.SnapshotNode, len(snapResult.Nodes))
			for i, n := range snapResult.Nodes {
				assessNodes[i] = ghostchrome.SnapshotNode{Role: n.Role, Name: n.Name}
			}
			if ghostchrome.AssessSnapshot(assessNodes) {
				// Convert browserops.SnapshotNode → bridge.A11yNode.
				flat := make([]bridge.A11yNode, len(snapResult.Nodes))
				refs := make(map[string]int64, len(snapResult.Nodes))
				targets := make(map[string]bridge.RefTarget, len(snapResult.Nodes))
				for i, n := range snapResult.Nodes {
					flat[i] = bridge.A11yNode{
						Ref:   n.Ref,
						Role:  n.Role,
						Name:  n.Name,
						Tag:   n.Tag,
						Value: n.Value,
						Depth: n.Depth,
					}
					if n.Ref != "" {
						refs[n.Ref] = 0
						targets[n.Ref] = bridge.RefTarget{}
					}
				}

				// IDPI: scan snapshot node names/values when ContentGuard is set.
				var idpiWarning string
				if params.ContentGuard != nil {
					var sb strings.Builder
					for _, n := range flat {
						if n.Name != "" || n.Value != "" {
							sb.WriteString(n.Name)
							if n.Name != "" && n.Value != "" {
								sb.WriteByte(' ')
							}
							sb.WriteString(n.Value)
							sb.WriteByte('\n')
						}
					}
					scanResult := params.ContentGuard.ScanOnly(sb.String())
					if scanResult.Blocked {
						return nil, fmt.Errorf("snapshot blocked by IDPI scanner: %s", scanResult.BlockReason)
					}
					if scanResult.Warning != "" {
						idpiWarning = scanResult.Warning
					}
				}

				slog.Debug("ghost-chrome snapshot: static accepted",
					"tabID", tabID, "nodes", len(flat))
				return &bridge.SnapshotResult{
					Nodes:       flat,
					Refs:        refs,
					Targets:     targets,
					IDPIWarning: idpiWarning,
					Route: &browserops.RouteMetadata{
						RequestedBrowser: "ghost-chrome",
						UsedBrowser:      "ghost-chrome",
						Attempts: []browserops.RouteAttempt{
							{Browser: "ghost-chrome", Accepted: true},
						},
					},
				}, nil
			}
			// Quality too low — fall through to Chrome.
			slog.Debug("ghost-chrome snapshot: quality too low, escalating", "tabID", tabID)
		}
	}

	// Escalate to Chrome. Resolve the tab ID for escalated tabs.
	resolvedTabID := tabID
	if chromeID, ok := a.proxy.ChromeTabID(tabID); ok {
		resolvedTabID = chromeID
	}
	chromeSnap, err := a.BridgeAPI.Snapshot(ctx, resolvedTabID, filter, params)
	if err != nil {
		return nil, err
	}
	chromeSnap.Route = &browserops.RouteMetadata{
		RequestedBrowser: "ghost-chrome",
		UsedBrowser:      "ghost-chrome",
		Escalated:        true,
		Attempts: []browserops.RouteAttempt{
			{Browser: "ghost-chrome", Accepted: false, Reason: "quality too low or static unavailable"},
			{Browser: "chrome", Accepted: true, Reason: "escalated"},
		},
	}
	return chromeSnap, nil
}

// Text implements the ghost-chrome "static first, assess quality, escalate to
// Chrome" routing pattern for text extraction. It tries the static browser
// first; if the text passes the quality gate it returns immediately. Otherwise
// it falls through to Chrome.
func (a *BridgeAdapter) Text(ctx context.Context, tabID string, params bridge.ContentParams) (*bridge.TextResult, error) {
	staticBrowser := a.proxy.StaticBrowser()
	if staticBrowser != nil {
		textResult, err := staticBrowser.Text(context.Background(), tabID)
		if err == nil && textResult.Text != "" {
			gr := ghostchrome.AssessContent(textResult.Text)
			if gr.ShouldAccept() {
				// IDPI: scan extracted text when ContentGuard is set.
				text := textResult.Text
				var idpiWarning string
				if params.ContentGuard != nil {
					scanResult := params.ContentGuard.Scan(text, textResult.URL)
					if scanResult.Blocked {
						return nil, fmt.Errorf("content blocked by IDPI scanner: %s", scanResult.BlockReason)
					}
					text = scanResult.Text
					if scanResult.Warning != "" {
						idpiWarning = scanResult.Warning
					}
				}

				slog.Debug("ghost-chrome text: static accepted",
					"tabID", tabID, "quality", gr.Quality)
				return &bridge.TextResult{
					Text:        text,
					URL:         textResult.URL,
					Title:       textResult.Title,
					IDPIWarning: idpiWarning,
					Route: &browserops.RouteMetadata{
						RequestedBrowser: "ghost-chrome",
						UsedBrowser:      "ghost-chrome",
						Quality:          gr.Quality,
						Attempts: []browserops.RouteAttempt{
							{Browser: "ghost-chrome", Accepted: true, Reason: gr.FormatReason()},
						},
					},
				}, nil
			}
			// Quality too low — fall through to Chrome.
			slog.Debug("ghost-chrome text: quality too low, escalating",
				"tabID", tabID, "quality", gr.Quality, "reason", gr.FormatReason())
		}
	}

	// Escalate to Chrome. Resolve the tab ID for escalated tabs.
	resolvedTabID := tabID
	if chromeID, ok := a.proxy.ChromeTabID(tabID); ok {
		resolvedTabID = chromeID
	}
	chromeText, err := a.BridgeAPI.Text(ctx, resolvedTabID, params)
	if err != nil {
		return nil, err
	}
	chromeText.Route = &browserops.RouteMetadata{
		RequestedBrowser: "ghost-chrome",
		UsedBrowser:      "ghost-chrome",
		Escalated:        true,
		Attempts: []browserops.RouteAttempt{
			{Browser: "ghost-chrome", Accepted: false, Reason: "quality too low or static unavailable"},
			{Browser: "chrome", Accepted: true, Reason: "escalated"},
		},
	}
	return chromeText, nil
}

func (a *BridgeAdapter) GetDocumentReadyState(tabID string) (string, error) {
	type readyStater interface {
		GetDocumentReadyState(string) (string, error)
	}
	if rs, ok := a.BridgeAPI.(readyStater); ok {
		return rs.GetDocumentReadyState(tabID)
	}
	return "", nil
}

func (a *BridgeAdapter) IsNetworkIdle(tabID string) (bool, bool) {
	type idleChecker interface {
		IsNetworkIdle(string) (bool, bool)
	}
	if ic, ok := a.BridgeAPI.(idleChecker); ok {
		return ic.IsNetworkIdle(tabID)
	}
	return false, false
}

func (a *BridgeAdapter) SetFingerprintRotateActive(tabID string, active bool) {
	type setter interface {
		SetFingerprintRotateActive(string, bool)
	}
	if s, ok := a.BridgeAPI.(setter); ok {
		s.SetFingerprintRotateActive(tabID, active)
	}
}

func (a *BridgeAdapter) FingerprintRotateActive(tabID string) bool {
	type getter interface {
		FingerprintRotateActive(string) bool
	}
	g, ok := a.BridgeAPI.(getter)
	if !ok {
		return false
	}
	if g.FingerprintRotateActive(tabID) {
		return true
	}
	// The rotate handler stores the flag under the resolved Chrome tab ID,
	// but callers may query with the original lite tab ID.
	if chromeID, mapped := a.proxy.ChromeTabID(tabID); mapped {
		return g.FingerprintRotateActive(chromeID)
	}
	return false
}

func (a *BridgeAdapter) GetFrameScope(tabID string) (bridge.FrameScope, bool) {
	type frameScopeAPI interface {
		GetFrameScope(string) (bridge.FrameScope, bool)
	}
	if fs, ok := a.BridgeAPI.(frameScopeAPI); ok {
		return fs.GetFrameScope(tabID)
	}
	return bridge.FrameScope{}, false
}

func (a *BridgeAdapter) SetFrameScope(tabID string, scope bridge.FrameScope) {
	type frameScopeAPI interface {
		SetFrameScope(string, bridge.FrameScope)
	}
	if fs, ok := a.BridgeAPI.(frameScopeAPI); ok {
		fs.SetFrameScope(tabID, scope)
	}
}

func (a *BridgeAdapter) ClearFrameScope(tabID string) {
	type frameScopeAPI interface {
		ClearFrameScope(string)
	}
	if fs, ok := a.BridgeAPI.(frameScopeAPI); ok {
		fs.ClearFrameScope(tabID)
	}
}

func (a *BridgeAdapter) SetTabHandoff(tabID, reason string, timeout time.Duration) error {
	type handoffAPI interface {
		SetTabHandoff(string, string, time.Duration) error
	}
	if h, ok := a.BridgeAPI.(handoffAPI); ok {
		return h.SetTabHandoff(tabID, reason, timeout)
	}
	return fmt.Errorf("bridge does not support handoff state")
}

func (a *BridgeAdapter) ResumeTabHandoff(tabID string) error {
	type handoffAPI interface {
		ResumeTabHandoff(string) error
	}
	if h, ok := a.BridgeAPI.(handoffAPI); ok {
		return h.ResumeTabHandoff(tabID)
	}
	return fmt.Errorf("bridge does not support handoff state")
}

func (a *BridgeAdapter) TabHandoffState(tabID string) (bridge.TabHandoffState, bool) {
	type handoffAPI interface {
		TabHandoffState(string) (bridge.TabHandoffState, bool)
	}
	if h, ok := a.BridgeAPI.(handoffAPI); ok {
		return h.TabHandoffState(tabID)
	}
	return bridge.TabHandoffState{}, false
}

// navigateParamsToPolicy converts bridge.NavigateParams to a
// staticfetch.NavigateNetworkPolicy so the static browser enforces the
// same SSRF/redirect protections as the Chrome path.
func navigateParamsToPolicy(p *bridge.NavigateParams) *staticfetch.NavigateNetworkPolicy {
	if p == nil {
		return nil
	}

	policy := &staticfetch.NavigateNetworkPolicy{
		AllowInternal: p.AllowInternal,
		MaxRedirects:  p.MaxRedirects,
	}

	// []net.IPNet → []*net.IPNet
	if len(p.TrustedProxyCIDRs) > 0 {
		policy.TrustedProxyCIDRs = make([]*net.IPNet, len(p.TrustedProxyCIDRs))
		for i := range p.TrustedProxyCIDRs {
			policy.TrustedProxyCIDRs[i] = &p.TrustedProxyCIDRs[i]
		}
	}

	// []net.IP → []netip.Addr
	if len(p.TrustedResolvedIPs) > 0 {
		addrs := make([]netip.Addr, 0, len(p.TrustedResolvedIPs))
		for _, ip := range p.TrustedResolvedIPs {
			if addr, ok := netip.AddrFromSlice(ip); ok {
				addrs = append(addrs, addr.Unmap())
			}
		}
		policy.TrustedResolvedIP = addrs
	}

	return policy
}
