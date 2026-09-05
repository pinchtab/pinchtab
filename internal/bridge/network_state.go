package bridge

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// SetNetworkConditions blocks the tab's traffic through the route manager's
// Fetch interception when offline, then flips navigator.onLine and the
// throttle through Network.emulateNetworkConditions. The interception is what
// makes the tab offline; the emulation alone is cosmetic and is re-applied by
// the TabManager after every navigation because Chrome drops it.
func (b *Bridge) SetNetworkConditions(ctx context.Context, tabID string, params NetworkConditions) error {
	if b.routeMgr == nil {
		return fmt.Errorf("route manager not initialized")
	}
	tabHandle, resolvedID, err := b.TabContext(tabID)
	if err != nil {
		return err
	}
	if err := b.routeMgr.SetOffline(tabHandle, resolvedID, params.Offline); err != nil {
		return fmt.Errorf("offline interception: %w", err)
	}
	if err := applyNetworkConditions(ctx, params); err != nil {
		if params.Offline {
			_ = b.routeMgr.SetOffline(tabHandle, resolvedID, false)
		}
		return err
	}
	b.TabManager.SetNetworkConditions(resolvedID, params)
	return nil
}

func applyNetworkConditions(ctx context.Context, params NetworkConditions) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.OverrideNetworkState(params.Offline, params.Latency, params.DownloadThroughput, params.UploadThroughput).
			Do(ctx)
	}))
}

func (b *Bridge) SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error {
	hdrs := make(network.Headers, len(headers))
	for k, v := range headers {
		hdrs[k] = v
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetExtraHTTPHeaders(hdrs).Do(ctx)
	}))
}
