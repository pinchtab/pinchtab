package bridge

import (
	"context"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

// ViewportParams holds parameters for setting the browser viewport.
type ViewportParams struct {
	Width             int64
	Height            int64
	DeviceScaleFactor float64
	Mobile            bool
}

// SetViewport sets the browser viewport dimensions via CDP emulation.
func (b *Bridge) SetViewport(ctx context.Context, params ViewportParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetDeviceMetricsOverride(params.Width, params.Height, params.DeviceScaleFactor, params.Mobile).
			WithScreenWidth(params.Width).
			WithScreenHeight(params.Height).
			Do(ctx)
	}))
}

// SetGeolocation overrides the browser geolocation via CDP emulation.
func (b *Bridge) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetGeolocationOverride().
			WithLatitude(lat).
			WithLongitude(lng).
			WithAccuracy(accuracy).
			Do(ctx)
	}))
}

// SetEmulatedMedia emulates a CSS media feature via CDP.
func (b *Bridge) SetEmulatedMedia(ctx context.Context, feature, value string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetEmulatedMedia().
			WithFeatures([]*emulation.MediaFeature{{Name: feature, Value: value}}).
			Do(ctx)
	}))
}
