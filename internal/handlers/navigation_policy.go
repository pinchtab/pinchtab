package handlers

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
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

func installNavigateRuntimeGuard(tCtx context.Context, tCancel context.CancelFunc, target *navguard.ValidatedTarget, trustedCIDRs []*net.IPNet) (*navigateRuntimeGuard, error) {
	if target == nil || target.AllowInternal {
		return nil, nil
	}
	if err := chromedp.Run(tCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.Enable().Do(ctx)
	})); err != nil {
		return nil, fmt.Errorf("network enable: %w", err)
	}

	guard := &navigateRuntimeGuard{}
	chromedp.ListenTarget(tCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			if e.Type != network.ResourceTypeDocument {
				return
			}
			guard.noteMainDocumentRequest(string(e.FrameID), string(e.RequestID))
		case *network.EventResponseReceived:
			if !guard.isMainDocumentResponse(string(e.RequestID)) {
				return
			}
			if err := navguard.ValidateRemoteIP(e.Response.RemoteIPAddress, trustedCIDRs, target.TrustedResolvedIP); err != nil {
				guard.setBlocked(err)
				tCancel()
			}
		}
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
