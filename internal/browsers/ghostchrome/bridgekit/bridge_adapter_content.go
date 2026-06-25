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

// staticEscalateError builds the typed signal for NoEscalate mode, carrying the
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
		return a.BridgeAPI.Navigate(ctx, url, params)
	}
	staticBrowser := a.StaticBrowser()
	if staticBrowser != nil {
		// Attach network policy so the static browser enforces SSRF/redirect protections.
		ctx = staticfetch.WithNavigateNetworkPolicy(ctx, navigateParamsToPolicy(&params))
		navResult, err := staticBrowser.Navigate(ctx, url)
		if err == nil {
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

	chromeResult, chromeErr := a.BridgeAPI.Navigate(ctx, url, params)
	if chromeErr != nil {
		return nil, chromeErr
	}
	chromeResult.Route = escalatedRoute(0, "static browser unavailable or failed")
	return chromeResult, nil
}

// resolveEscalationTabID maps a (possibly static "lite-N") tabID to the Chrome
// tab that backs it, lazily escalating through the proxy when no mapping exists
// yet. The shared resolution site for the Snapshot/Text escalation tails.
//
// It routes through proxy.TabContext rather than the read-only ChromeTabID: on a
// cache miss ChromeTabID returned the lite id unchanged, so the escalated read
// then called Chrome with a tab it had never created (tab-not-found, which
// stalls the read to the action timeout). proxy.TabContext re-checks the map and,
// for a known lite tab, creates the Chrome tab from the lite tab's URL and
// records the mapping — so escalation targets a real Chrome tab. A genuinely
// unresolvable tab now surfaces a clear error instead of hanging.
func (a *BridgeAdapter) resolveEscalationTabID(tabID string) (string, error) {
	_, resolved, err := a.proxy.TabContext(tabID)
	if err != nil {
		return "", fmt.Errorf("resolve escalation tab %q: %w", tabID, err)
	}
	return resolved, nil
}

// snapshotIDPIScan runs the IDPI content guard over the snapshot's node text
// (scan-only, no redaction). It returns a non-empty warning when the scanner
// flags the content and an error when it blocks it.
func snapshotIDPIScan(params bridge.ContentParams, flat []bridge.A11yNode) (string, error) {
	if params.ContentGuard == nil {
		return "", nil
	}
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
		return "", fmt.Errorf("snapshot blocked by IDPI scanner: %s", scanResult.BlockReason)
	}
	return scanResult.Warning, nil
}

// textIDPIScan runs the IDPI content guard over extracted text, returning the
// (possibly redacted) text, a warning when flagged, and an error when blocked.
func textIDPIScan(params bridge.ContentParams, text, url string) (string, string, error) {
	if params.ContentGuard == nil {
		return text, "", nil
	}
	scanResult := params.ContentGuard.Scan(text, url)
	if scanResult.Blocked {
		return "", "", fmt.Errorf("content blocked by IDPI scanner: %s", scanResult.BlockReason)
	}
	return scanResult.Text, scanResult.Warning, nil
}

// Snapshot tries the static browser's accessibility tree; if it passes the
// quality gate the nodes are returned, otherwise it falls through to Chrome.
func (a *BridgeAdapter) Snapshot(ctx context.Context, tabID string, filter string, params bridge.ContentParams) (*bridge.SnapshotResult, error) {
	staticBrowser := a.proxy.StaticBrowser()
	if staticBrowser != nil {
		snapResult, err := staticBrowser.Snapshot(ctx, tabID, filter)
		if err == nil && len(snapResult.Nodes) > 0 {
			assessNodes := make([]ghostchrome.SnapshotNode, len(snapResult.Nodes))
			for i, n := range snapResult.Nodes {
				assessNodes[i] = ghostchrome.SnapshotNode{Role: n.Role, Name: n.Name}
			}
			if ghostchrome.AssessSnapshot(assessNodes) {
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

				idpiWarning, err := snapshotIDPIScan(params, flat)
				if err != nil {
					return nil, err
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
			slog.Debug("ghost-chrome snapshot: quality too low, escalating", "tabID", tabID)
		}
	}

	escalatedTabID, err := a.resolveEscalationTabID(tabID)
	if err != nil {
		return nil, err
	}
	chromeSnap, err := a.BridgeAPI.Snapshot(ctx, escalatedTabID, filter, params)
	if err != nil {
		return nil, err
	}
	chromeSnap.Route = escalatedRoute(0, "quality too low or static unavailable")
	return chromeSnap, nil
}

// Text tries the static browser first; if the text passes the quality gate it
// returns immediately, otherwise it falls through to Chrome.
func (a *BridgeAdapter) Text(ctx context.Context, tabID string, params bridge.ContentParams) (*bridge.TextResult, error) {
	staticBrowser := a.proxy.StaticBrowser()
	if staticBrowser != nil {
		textResult, err := staticBrowser.Text(ctx, tabID)
		if err == nil && textResult.Text != "" {
			gr := ghostchrome.AssessContent(textResult.Text)
			if gr.ShouldAccept() {
				text, idpiWarning, err := textIDPIScan(params, textResult.Text, textResult.URL)
				if err != nil {
					return nil, err
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
			slog.Debug("ghost-chrome text: quality too low, escalating",
				"tabID", tabID, "quality", gr.Quality, "reason", gr.FormatReason())
		}
	}

	escalatedTabID, err := a.resolveEscalationTabID(tabID)
	if err != nil {
		return nil, err
	}
	chromeText, err := a.BridgeAPI.Text(ctx, escalatedTabID, params)
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

	if len(p.TrustedProxyCIDRs) > 0 {
		policy.TrustedProxyCIDRs = make([]*net.IPNet, len(p.TrustedProxyCIDRs))
		for i := range p.TrustedProxyCIDRs {
			policy.TrustedProxyCIDRs[i] = &p.TrustedProxyCIDRs[i]
		}
	}

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
