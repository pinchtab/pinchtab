package bridge

import (
	"context"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

func (b *Bridge) SetViewport(ctx context.Context, params ViewportParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetDeviceMetricsOverride(params.Width, params.Height, params.DeviceScaleFactor, params.Mobile).
			WithScreenWidth(params.Width).
			WithScreenHeight(params.Height).
			Do(ctx)
	}))
}

func (b *Bridge) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetGeolocationOverride().
			WithLatitude(lat).
			WithLongitude(lng).
			WithAccuracy(accuracy).
			Do(ctx)
	}))
}

func (b *Bridge) SetEmulatedMedia(ctx context.Context, feature, value string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetEmulatedMedia().
			WithFeatures([]*emulation.MediaFeature{{Name: feature, Value: value}}).
			Do(ctx)
	}))
}
