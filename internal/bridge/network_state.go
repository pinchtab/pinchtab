package bridge

import (
	"context"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func (b *Bridge) SetNetworkConditions(ctx context.Context, params NetworkConditions) error {
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
