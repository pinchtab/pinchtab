package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gobwas/ws"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
	internalurls "github.com/pinchtab/pinchtab/internal/urls"
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
	slog.Info("attaching to existing browser via CDP", "cdpUrl", internalurls.RedactForLog(wsURL))

	remoteAllocCtx, remoteAllocCancel, browserCtx, browserCancel, err := attachExistingCDP(wsURL)
	if err == nil {
		slog.Info("attached to existing browser via CDP", "cdpUrl", internalurls.RedactForLog(wsURL))
		return remoteAllocCtx, remoteAllocCancel, browserCtx, browserCancel, stealth.LaunchModeAttached, nil
	}

	refreshedURL, refreshErr := refreshStaleCDPURL(cfg.CDPAttachURL, wsURL, err, cfg)
	if refreshErr != nil {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized,
			fmt.Errorf("%w; stale CDP endpoint refresh failed: %v", err, refreshErr)
	}
	if refreshedURL == "" {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, err
	}

	slog.Info("retrying CDP attach after browser endpoint refresh", "cdpUrl", internalurls.RedactForLog(refreshedURL))
	remoteAllocCtx, remoteAllocCancel, browserCtx, browserCancel, err = attachExistingCDP(refreshedURL)
	if err != nil {
		return nil, nil, nil, nil, stealth.LaunchModeUninitialized, err
	}
	slog.Info("attached to existing browser via refreshed CDP endpoint", "cdpUrl", internalurls.RedactForLog(refreshedURL))
	return remoteAllocCtx, remoteAllocCancel, browserCtx, browserCancel, stealth.LaunchModeAttached, nil
}

func attachExistingCDP(wsURL string) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, error) {
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
		return nil, nil, nil, nil, fmt.Errorf("failed to attach to CDP at %s: %w", internalurls.RedactForLog(wsURL), err)
	}
	return remoteAllocCtx, remoteAllocCancel, browserCtx, browserCancel, nil
}

// refreshStaleCDPURL returns a fresh browser endpoint only when the dial got a
// typed 404 and /json/version proves the browser GUID changed.
func refreshStaleCDPURL(rawURL, staleURL string, attachErr error, cfg *config.RuntimeConfig) (string, error) {
	var status ws.StatusError
	if !errors.As(attachErr, &status) || status != ws.StatusError(http.StatusNotFound) {
		return "", nil
	}

	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe, pinned, err := probeCDPVersionParsed(ctx, parsed, cfg.AttachAllowHosts, cfg.AttachAllowSchemes)
	if err != nil {
		return "", err
	}
	refreshedURL, err := normalizeResolvedDevToolsURL(probe.WebSocketDebuggerURL, parsed, pinned, cfg.AttachAllowHosts)
	if err != nil {
		return "", err
	}
	stale, err := url.Parse(staleURL)
	if err != nil {
		return "", err
	}
	refreshed, err := url.Parse(refreshedURL)
	if err != nil {
		return "", err
	}
	if stale.EscapedPath() == refreshed.EscapedPath() {
		return "", nil
	}
	return refreshedURL, nil
}
