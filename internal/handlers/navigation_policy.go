package handlers

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/navguard"
)

type navigateRuntimeGuard struct {
	mu          sync.Mutex
	mainFrameID string
	requestID   string
	blockedErr  error
}

func (g *navigateRuntimeGuard) noteMainDocumentRequest(frameID, requestID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.mainFrameID == "" {
		g.mainFrameID = frameID
	}
	if frameID == g.mainFrameID {
		g.requestID = requestID
	}
}

func (g *navigateRuntimeGuard) isMainDocumentResponse(requestID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.requestID != "" && requestID == g.requestID
}

func (g *navigateRuntimeGuard) setBlocked(err error) {
	if err == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.blockedErr == nil {
		g.blockedErr = err
	}
}

func (g *navigateRuntimeGuard) blocked() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.blockedErr
}

func installNavigateRuntimeGuardWithBridge(b bridge.BridgeAPI, tCtx context.Context, tCancel context.CancelFunc, target *navguard.ValidatedTarget, trustedCIDRs []*net.IPNet) (*navigateRuntimeGuard, error) {
	if target == nil || target.AllowInternal {
		return nil, nil
	}
	if err := b.EnableNetwork(tCtx); err != nil {
		return nil, fmt.Errorf("network enable: %w", err)
	}

	guard := &navigateRuntimeGuard{}
	b.ListenNetworkEvents(tCtx, bridge.NetworkEventHandler{
		OnRequestWillBeSent: func(frameID, requestID, resourceType string) {
			if resourceType != "Document" {
				return
			}
			guard.noteMainDocumentRequest(frameID, requestID)
		},
		OnResponseReceived: func(requestID, remoteIPAddress string) {
			if !guard.isMainDocumentResponse(requestID) {
				return
			}
			if err := navguard.ValidateRemoteIP(remoteIPAddress, trustedCIDRs, target.TrustedResolvedIP); err != nil {
				guard.setBlocked(err)
				tCancel()
			}
		},
	})
	return guard, nil
}

// validatedNavigateTarget is an alias kept for backward compatibility within
// the handlers package during migration. New code should use navguard.ValidatedTarget.
type validatedNavigateTarget = navguard.ValidatedTarget

// validateNavigateURL delegates to navguard.ValidateURL.
func validateNavigateURL(raw string) error {
	return navguard.ValidateURL(raw)
}

// validateNavigateTarget delegates to navguard's target validation logic.
// Per-call Validator construction is intentional: trusted CIDRs come from the
// per-request (target-scoped) effective config, not a static startup value.
func validateNavigateTarget(raw string, allowExplicitInternal bool, trustedResolveCIDRs []*net.IPNet) (*navguard.ValidatedTarget, error) {
	v := &navguard.Validator{TrustedResolveCIDRs: trustedResolveCIDRs}
	return v.ValidateTarget(context.Background(), raw, allowExplicitInternal)
}

// validateNavigateRemoteIPAddress delegates to navguard.ValidateRemoteIP.
func validateNavigateRemoteIPAddress(raw string, trustedCIDRs []*net.IPNet, trustedIPs []netip.Addr) error {
	return navguard.ValidateRemoteIP(raw, trustedCIDRs, trustedIPs)
}

// parseCIDRs delegates to navguard.ParseCIDRs.
func parseCIDRs(raw []string) []*net.IPNet {
	return navguard.ParseCIDRs(raw)
}

// buildNavigateTrustedProxyCIDRs delegates to navguard.BuildTrustedProxyCIDRs
// using the runtime config fields.
func buildNavigateTrustedProxyCIDRs(cfg *config.RuntimeConfig) []*net.IPNet {
	if cfg == nil {
		return nil
	}
	return navguard.BuildTrustedProxyCIDRs(cfg.TrustLoopbackProxy, cfg.TrustedProxyCIDRs)
}
