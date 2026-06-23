package runtime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

// initBrowserFromExistingCDP attaches the bridge to a browser that is already
// running outside pinchtab (e.g. the user's everyday browser launched with
// --remote-debugging-port=NNNN). No process is spawned and no profile lock
// is taken. The allocator is a chromedp remote allocator; the returned
// cancel funcs release only the chromedp side, never the external browser.
func initBrowserFromExistingCDP(cfg *config.RuntimeConfig, bundle *stealth.Bundle) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	browserID, _ := config.ParseBrowser(cfg.DefaultBrowser, cfg.BrowsersAvailable)
	if browserID == "" {
		browserID = config.NormalizeBrowser(cfg.DefaultBrowser)
	}
	if b, ok := browsers.Get(browserID); ok && !b.SupportsRemoteCDP() {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized,
			fmt.Errorf("provider %q does not support remote CDP attach", browserID)
	}

	// Same SSRF/attach guard as the RemoteCDPURL path (init_remote.go):
	// loopback endpoints always pass; non-loopback hosts must be listed in
	// security.attach.allowHosts. The returned URL is host-pinned to the
	// resolved IP so the dial can't be re-routed by a second DNS answer.
	wsURL, err := normalizeAttachURL(cfg.CDPAttachURL, cfg)
	if err != nil {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("normalize cdpAttachUrl: %w", err)
	}
	slog.Info("attaching to existing browser via CDP", "cdpUrl", wsURL)

	remoteAllocCtx, remoteAllocCancel, browserCtx, browserCancel := newRemoteCDPContexts(context.Background(), wsURL)

	// Touch the browser so we fail fast if the CDP URL is unreachable. We
	// intentionally do NOT inject the stealth/UA script here — the user's
	// browser is theirs, and rewriting its launch contract would be both
	// surprising and likely break extensions, profile features, and
	// already-open tabs.
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return nil
	})); err != nil {
		browserCancel()
		remoteAllocCancel()
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, fmt.Errorf("failed to attach to CDP at %s: %w", wsURL, err)
	}

	slog.Info("attached to existing browser via CDP", "cdpUrl", wsURL)
	return remoteAllocCtx, remoteAllocCancel, browserCtx, browserCancel, stealth.LaunchModeAttached, nil
}
