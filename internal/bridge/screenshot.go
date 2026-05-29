package bridge

import (
	"context"
	"encoding/json"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// MinScale / MaxScale bound the bitmap rescale factor. Anything outside is
// almost certainly a mistake or a DoS — 50× would ask the renderer for a
// multi-gigapixel image.
const (
	MinScale = 0.05
	MaxScale = 4.0
)

func ClampScale(scale float64) float64 {
	if scale <= 0 {
		return 1
	}
	if scale < MinScale {
		return MinScale
	}
	if scale > MaxScale {
		return MaxScale
	}
	return scale
}

// fetchViewportSize returns window.innerWidth/innerHeight via one
// Runtime.evaluate round trip. Falls back to (0, 0) on any failure so the
// caller can decide whether to bail or skip the rescale.
func fetchViewportSize(ctx context.Context) (float64, float64) {
	const expression = `JSON.stringify([window.innerWidth, window.innerHeight])`
	var result struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    expression,
			"returnByValue": true,
		}, &result)
	})); err != nil {
		return 0, 0
	}
	var dims [2]float64
	if err := json.Unmarshal([]byte(result.Result.Value), &dims); err != nil {
		return 0, 0
	}
	return dims[0], dims[1]
}

// ScreenshotOpts mirrors the subset of page.CaptureScreenshot parameters the
// callers (HandleScreenshot, PairedCapture) need to coordinate on.
type ScreenshotOpts struct {
	Format         page.CaptureScreenshotFormat
	Quality        int
	Clip           *page.Viewport
	BeyondViewport bool

	// Scale rescales the rendered output bitmap. 1 (or 0) = native. 0.5
	// halves the image in each axis (quarter of the pixels). Applied via
	// CDP's clip.scale; when no Clip is otherwise set we synthesize one
	// covering the viewport so the parameter takes effect.
	Scale float64

	// ViewportWidth/ViewportHeight are used only when Scale != 1 and Clip is
	// nil — to build a viewport-covering clip. Callers fetch these from
	// observe.FetchLayout (or similar) before invoking CaptureScreenshot.
	ViewportWidth  float64
	ViewportHeight float64
}

// CaptureScreenshot runs Page.captureScreenshot with the supplied options.
// Quality is applied only for JPEG; clip and beyondViewport are mutually
// exclusive (clip wins) — the same rule the handler enforces on input.
//
// When Scale is non-default (not 0 and not 1), the function ensures a clip is
// passed to CDP — either by reusing the supplied one with Scale multiplied in,
// or by synthesizing a viewport-covering clip from ViewportWidth/Height. CDP's
// top-level capture call has no scale parameter outside of clip, so this is
// the only way to apply a render-time rescale.
func CaptureScreenshot(ctx context.Context, opts ScreenshotOpts) ([]byte, error) {
	clip := opts.Clip
	if opts.Scale > 0 && opts.Scale != 1 {
		if clip != nil {
			c := *clip
			if c.Scale == 0 {
				c.Scale = 1
			}
			c.Scale *= opts.Scale
			clip = &c
		} else {
			// Synthesize a viewport-covering clip so CDP's clip.scale applies.
			//
			// Known issue: two back-to-back /capture?scale=<n!=1> on the same
			// tab without nav between can hang on the second call. Workaround:
			// nav between captures (see e2e cli/capture-basic.sh).
			w, h := opts.ViewportWidth, opts.ViewportHeight
			if w == 0 || h == 0 {
				w, h = fetchViewportSize(ctx)
			}
			if w > 0 && h > 0 {
				clip = &page.Viewport{
					X: 0, Y: 0,
					Width:  w,
					Height: h,
					Scale:  opts.Scale,
				}
			}
		}
	}

	var buf []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		shot := page.CaptureScreenshot().WithFormat(opts.Format)
		if clip != nil {
			shot = shot.WithClip(clip)
		}
		if opts.BeyondViewport && clip == nil {
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
