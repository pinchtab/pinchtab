package bridge

import (
	"context"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// ScreenshotOpts mirrors the subset of page.CaptureScreenshot parameters the
// callers (HandleScreenshot, PairedCapture) need to coordinate on.
type ScreenshotOpts struct {
	Format         page.CaptureScreenshotFormat
	Quality        int
	Clip           *page.Viewport
	BeyondViewport bool
}

// CaptureScreenshot runs Page.captureScreenshot with the supplied options.
// Quality is applied only for JPEG; clip and beyondViewport are mutually
// exclusive (clip wins) — the same rule the handler enforces on input.
func CaptureScreenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	var buf []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		shot := page.CaptureScreenshot().WithFormat(opts.Format)
		if opts.Clip != nil {
			shot = shot.WithClip(opts.Clip)
		}
		if opts.BeyondViewport && opts.Clip == nil {
			shot = shot.WithCaptureBeyondViewport(true)
		}
		if opts.Format == page.CaptureScreenshotFormatJpeg {
			shot = shot.WithQuality(int64(opts.Quality))
		}
		var inner error
		buf, inner = shot.Do(ctx)
		return inner
	}))
	return buf, err
}
