package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

// InitRemoteCDP attaches to an external browser via CDP URL. The browser
// process is not owned by PinchTab; returned cancels release only chromedp
// contexts.
func InitRemoteCDP(ctx context.Context, cfg *config.RuntimeConfig, cdpURL string) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	if cfg == nil {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("runtime config is required")
	}

	normalized, err := normalizeAttachURL(cdpURL, cfg)
	if err != nil {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("normalize cdpUrl: %w", err)
	}
	if err := requireRemoteProxyAuthOptIn(normalized, cfg); err != nil {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, err
	}

	slog.Info("attaching to remote CDP endpoint", "provider", cfg.DefaultBrowser)

	if ctx == nil {
		ctx = context.Background()
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, normalized)
	probeCtx, probeCancel := context.WithTimeout(ctx, 10*time.Second)
	// Probe the ORIGINAL URL, not the normalized one: normalization pins the
	// host by rewriting it to the resolved IP, and re-validating that bare IP
	// against a hostname allowlist can never match — every allowlisted DNS
	// hostname would fail here. The probe validates and pins the original
	// hostname itself, so the dial stays SSRF-safe; the allocator above still
	// attaches via the normalized pinned URL.
	_, err = ProbeCDPVersion(probeCtx, strings.TrimSpace(cdpURL), cfg.AttachAllowHosts)
	probeCancel()
	if err != nil {
		allocCancel()
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("remote CDP connectivity check failed: %w", err)
	}

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	if ProxyAuthEnabled(cfg.Proxy) {
		auditRemoteProxyAuthForward(normalized, cfg)
		if err := EnableProxyAuth(browserCtx, cfg.Proxy, nil); err != nil {
			browserCancel()
			allocCancel()
			return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("enable proxy auth on remote CDP: %w", err)
		}
		slog.Info("proxy authentication enabled via CDP", "proxy", cfg.Proxy.Redacted())
	}

	return allocCtx, allocCancel, browserCtx, func() {
		// Cancel PinchTab contexts only; do not terminate the external browser.
		browserCancel()
		allocCancel()
	}, stealth.LaunchModeRemoteCDP, nil
}

func requireRemoteProxyAuthOptIn(browserWSURL string, cfg *config.RuntimeConfig) error {
	if cfg == nil || !ProxyAuthEnabled(cfg.Proxy) {
		return nil
	}
	targetHost := remoteCDPTargetHost(browserWSURL)
	if cfg.AttachForwardProxyAuth {
		return nil
	}
	slog.Warn("audit",
		"event", "remote_cdp.proxy_auth_blocked",
		"targetHost", targetHost,
		"proxy", cfg.Proxy.Redacted(),
	)
	return fmt.Errorf("remote CDP proxy authentication is disabled: refusing to send proxy credentials to attached browser %q; set security.attach.forwardProxyAuth=true to allow this trust-boundary crossing", targetHost)
}

func auditRemoteProxyAuthForward(browserWSURL string, cfg *config.RuntimeConfig) {
	if cfg == nil || !ProxyAuthEnabled(cfg.Proxy) || !cfg.AttachForwardProxyAuth {
		return
	}
	slog.Warn("audit",
		"event", "remote_cdp.proxy_auth_forwarded",
		"targetHost", remoteCDPTargetHost(browserWSURL),
		"proxy", cfg.Proxy.Redacted(),
	)
}

func remoteCDPTargetHost(browserWSURL string) string {
	parsed, err := url.Parse(browserWSURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
