package handlers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/netguard"
)

const maxNavigateURLLen = 8 << 10

type validatedNavigateTarget struct {
	allowInternal bool
}

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

func validateNavigateURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("url required")
	}
	if len(raw) > maxNavigateURLLen {
		return fmt.Errorf("url too long")
	}
	if strings.EqualFold(raw, "about:blank") {
		return nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if parsed.Scheme == "" {
		return nil
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return fmt.Errorf("invalid URL scheme: %s", parsed.Scheme)
	}
}

func validateNavigateTarget(raw string, allowExplicitInternal bool) (*validatedNavigateTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "about:blank") {
		return &validatedNavigateTarget{allowInternal: true}, nil
	}

	host, hasHost := extractNavigateHost(raw)
	if !hasHost {
		return &validatedNavigateTarget{}, nil
	}
	if netguard.IsLocalHost(host) {
		return &validatedNavigateTarget{allowInternal: true}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if _, err := netguard.ResolveAndValidatePublicIPs(ctx, host); err != nil {
		if errors.Is(err, netguard.ErrResolveHost) {
			return nil, fmt.Errorf("could not resolve navigation host")
		}
		if errors.Is(err, netguard.ErrPrivateInternalIP) {
			if allowExplicitInternal {
				return &validatedNavigateTarget{allowInternal: true}, nil
			}
			return nil, fmt.Errorf("navigation target resolves to blocked private/internal IP")
		}
		return nil, fmt.Errorf("could not resolve navigation host")
	}
	return &validatedNavigateTarget{}, nil
}

func extractNavigateHost(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	if host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), "."); host != "" {
		return host, true
	}
	if parsed.Scheme != "" {
		return "", false
	}

	bare := parsed.Path
	bare = strings.SplitN(bare, "/", 2)[0]
	bare = strings.SplitN(bare, "?", 2)[0]
	bare = strings.SplitN(bare, "#", 2)[0]
	bare = strings.TrimSpace(bare)
	if bare == "" || strings.HasPrefix(bare, "/") || strings.HasPrefix(bare, ".") {
		return "", false
	}
	if h, _, err := net.SplitHostPort(bare); err == nil {
		bare = h
	}
	host := strings.TrimSuffix(strings.ToLower(bare), ".")
	if host == "" {
		return "", false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil || strings.Contains(host, ".") || strings.Contains(host, ":") {
		return host, true
	}
	return "", false
}

func validateNavigateRemoteIPAddress(raw string, trustedCIDRs []*net.IPNet) error {
	normalized := netguard.NormalizeRemoteIP(raw)
	if err := netguard.ValidateRemoteIPAddress(raw); err != nil {
		if ip := net.ParseIP(normalized); ip != nil {
			for _, cidr := range trustedCIDRs {
				if cidr.Contains(ip) {
					return nil
				}
			}
		}
		return fmt.Errorf("navigation connected to blocked remote IP %s", normalized)
	}
	return nil
}

func parseCIDRs(raw []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			s += "/32"
		}
		if _, cidr, err := net.ParseCIDR(s); err == nil {
			nets = append(nets, cidr)
		}
	}
	return nets
}

func installNavigateRuntimeGuard(tCtx context.Context, tCancel context.CancelFunc, target *validatedNavigateTarget, trustedCIDRs []*net.IPNet) (*navigateRuntimeGuard, error) {
	if target == nil || target.allowInternal {
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
			if err := validateNavigateRemoteIPAddress(e.Response.RemoteIPAddress, trustedCIDRs); err != nil {
				guard.setBlocked(err)
				tCancel()
			}
		}
	})
	return guard, nil
}
