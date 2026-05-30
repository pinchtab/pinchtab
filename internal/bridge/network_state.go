package bridge

import (
	"context"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// NetworkConditions holds parameters for network emulation.
type NetworkConditions struct {
	Offline            bool
	Latency            float64
	DownloadThroughput float64
	UploadThroughput   float64
}

// SetNetworkConditions emulates network conditions (offline, latency, throughput) via CDP.
func (b *Bridge) SetNetworkConditions(ctx context.Context, params NetworkConditions) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.OverrideNetworkState(params.Offline, params.Latency, params.DownloadThroughput, params.UploadThroughput).
			Do(ctx)
	}))
}

// SetExtraHTTPHeaders sets extra HTTP headers that will be sent with every request via CDP.
func (b *Bridge) SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error {
	hdrs := make(network.Headers, len(headers))
	for k, v := range headers {
		hdrs[k] = v
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetExtraHTTPHeaders(hdrs).Do(ctx)
	}))
}
