package bridgekit

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome/staticfetch"
)

// StaticFirstNavigate reports whether this bridge can attempt a navigate on
// its static browser without Chrome. Handlers probe it to defer the Chrome
// launch (NavigateParams.NoEscalate / SkipStatic two-phase protocol).
func (a *BridgeAdapter) StaticFirstNavigate() bool {
	return a.StaticBrowser() != nil
}

// escalateError builds the typed signal for NoEscalate mode, carrying the
// static attempt's route metadata for the caller to merge.
func staticEscalateError(quality int, reason string) *bridge.StaticEscalateError {
	return &bridge.StaticEscalateError{
		Quality: quality,
		Reason:  reason,
		Route:   staticAcceptedRoute(quality, reason, false),
	}
}

// staticAcceptedRoute builds the RouteMetadata for a static attempt the
// quality gate accepted (accepted=true) or rejected without a Chrome
// fallthrough (accepted=false, used by staticEscalateError).
func staticAcceptedRoute(quality int, reason string, accepted bool) *browserops.RouteMetadata {
	return &browserops.RouteMetadata{
		RequestedBrowser: "ghost-chrome",
		UsedBrowser:      "ghost-chrome",
		Quality:          quality,
		Attempts: []browserops.RouteAttempt{
			{Browser: "ghost-chrome", Accepted: accepted, Reason: reason},
		},
	}
}

// escalatedRoute builds the RouteMetadata for a static attempt that fell
// through to Chrome.
func escalatedRoute(quality int, reason string) *browserops.RouteMetadata {
	return &browserops.RouteMetadata{
		RequestedBrowser: "ghost-chrome",
		UsedBrowser:      "ghost-chrome",
		Escalated:        true,
		Quality:          quality,
		Attempts: []browserops.RouteAttempt{
			{Browser: "ghost-chrome", Accepted: false, Reason: reason},
			{Browser: "chrome", Accepted: true, Reason: "escalated"},
		},
	}
}

// Navigate implements the ghost-chrome "static first, assess quality, escalate
// to Chrome" routing pattern for navigation. It tries the static browser first;
// if the fetched content passes the quality gate it returns immediately.
// Otherwise it falls through to the Chrome bridge — or, in NoEscalate mode,
// returns *bridge.StaticEscalateError so the handler can launch Chrome itself.
func (a *BridgeAdapter) Navigate(ctx context.Context, url string, params bridge.NavigateParams) (*bridge.NavigateResult, error) {
	if params.SkipStatic {
		// The handler already ran the static phase and is escalating.
		return a.BridgeAPI.Navigate(ctx, url, params)
	}
	// Concrete type needed: CloseTab is not part of browserops.BrowserRuntime.
	staticBrowser := a.StaticBrowser()
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
						Route: staticAcceptedRoute(gr.Quality, gr.FormatReason(), true),
					}, nil
				}
				// Quality too low — escalate to Chrome. The static tab was
				// never exposed to the caller; release it.
				slog.Debug("ghost-chrome navigate: quality too low, escalating",
					"url", url, "quality", gr.Quality, "reason", gr.FormatReason())
				staticBrowser.CloseTab(navResult.TabID)
				if params.NoEscalate {
					return nil, staticEscalateError(gr.Quality, gr.FormatReason())
				}
				chromeResult, chromeErr := a.BridgeAPI.Navigate(ctx, url, params)
				if chromeErr != nil {
					return nil, chromeErr
				}
				chromeResult.Route = escalatedRoute(gr.Quality, gr.FormatReason())
				return chromeResult, nil
			}
			// Text extraction failed — escalate with zero quality. The static
			// tab was never exposed to the caller; release it.
			slog.Debug("ghost-chrome navigate: text extraction failed, escalating",
				"url", url, "err", textErr)
			staticBrowser.CloseTab(navResult.TabID)
			if params.NoEscalate {
				return nil, staticEscalateError(0, "text extraction failed: "+textErr.Error())
			}
		} else {
			slog.Debug("ghost-chrome navigate: static navigate failed, escalating",
				"url", url, "err", err)
			if params.NoEscalate {
				return nil, staticEscalateError(0, "static navigate failed: "+err.Error())
			}
		}
	}
	if params.NoEscalate {
		return nil, staticEscalateError(0, "static browser unavailable")
	}

	// Static browser unavailable or failed — go straight to Chrome.
	chromeResult, chromeErr := a.BridgeAPI.Navigate(ctx, url, params)
	if chromeErr != nil {
		return nil, chromeErr
	}
	chromeResult.Route = escalatedRoute(0, "static browser unavailable or failed")
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
		snapResult, err := staticBrowser.Snapshot(ctx, tabID, filter)
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
					Route:       staticAcceptedRoute(0, "", true),
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
	chromeSnap.Route = escalatedRoute(0, "quality too low or static unavailable")
	return chromeSnap, nil
}

// Text implements the ghost-chrome "static first, assess quality, escalate to
// Chrome" routing pattern for text extraction. It tries the static browser
// first; if the text passes the quality gate it returns immediately. Otherwise
// it falls through to Chrome.
func (a *BridgeAdapter) Text(ctx context.Context, tabID string, params bridge.ContentParams) (*bridge.TextResult, error) {
	staticBrowser := a.proxy.StaticBrowser()
	if staticBrowser != nil {
		textResult, err := staticBrowser.Text(ctx, tabID)
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
					Route:       staticAcceptedRoute(gr.Quality, gr.FormatReason(), true),
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
	chromeText.Route = escalatedRoute(0, "quality too low or static unavailable")
	return chromeText, nil
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
