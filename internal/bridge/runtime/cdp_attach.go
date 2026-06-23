package runtime

import (
	"context"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

// normalizeAttachURL applies the attach allowlist (loopback always allowed;
// non-loopback hosts must be in security.attach.allowHosts) and host-pins the
// URL to the resolved IP so the dial can't be re-routed by a second DNS answer.
func normalizeAttachURL(rawURL string, cfg *config.RuntimeConfig) (string, error) {
	return NormalizeCDPURLWithAllowlist(rawURL, cfg.AttachAllowHosts, cfg.AttachAllowSchemes)
}

// newRemoteCDPContexts creates a chromedp remote allocator + browser context for
// an already-normalized (pinned) CDP URL. ctx==nil defaults to Background.
func newRemoteCDPContexts(ctx context.Context, normalizedURL string) (
	allocCtx context.Context, allocCancel context.CancelFunc,
	browserCtx context.Context, browserCancel context.CancelFunc,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	allocCtx, allocCancel = chromedp.NewRemoteAllocator(ctx, normalizedURL)
	browserCtx, browserCancel = chromedp.NewContext(allocCtx)
	return
}
