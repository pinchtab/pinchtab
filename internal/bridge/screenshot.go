package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

type ScreenshotFormat = page.CaptureScreenshotFormat
type ScreenshotClip = page.Viewport

const (
	ScreenshotFormatJpeg = page.CaptureScreenshotFormatJpeg
	ScreenshotFormatPng  = page.CaptureScreenshotFormatPng
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

// fetchDocumentSize returns the scrollable document dimensions in CSS pixels.
// Used when beyond-viewport capture also needs clip.scale; the synthesized clip
// must cover the document, not just the current viewport.
func fetchDocumentSize(ctx context.Context) (float64, float64) {
	const expression = `JSON.stringify((() => {
		const d = document;
		const de = d.documentElement;
		const b = d.body || de;
		return {
			w: Math.max(de.scrollWidth, b.scrollWidth, de.clientWidth, de.offsetWidth),
			h: Math.max(de.scrollHeight, b.scrollHeight, de.clientHeight, de.offsetHeight)
		};
	})())`
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
	var dims struct {
		W float64 `json:"w"`
		H float64 `json:"h"`
	}
	if err := json.Unmarshal([]byte(result.Result.Value), &dims); err != nil {
		return 0, 0
	}
	return dims.W, dims.H
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

func scaledScreenshotClip(opts ScreenshotOpts, viewportWidth, viewportHeight, documentWidth, documentHeight float64) *page.Viewport {
	if opts.Scale <= 0 || opts.Scale == 1 {
		return opts.Clip
	}
	if opts.Clip != nil {
		clip := *opts.Clip
		if clip.Scale == 0 {
			clip.Scale = 1
		}
		clip.Scale *= opts.Scale
		return &clip
	}

	width, height := viewportWidth, viewportHeight
	if opts.BeyondViewport {
		width, height = documentWidth, documentHeight
	}
	if width <= 0 || height <= 0 {
		return nil
	}
	return &page.Viewport{
		X: 0, Y: 0,
		Width:  width,
		Height: height,
		Scale:  opts.Scale,
	}
}

// captureFromSurface decides the Page.captureScreenshot fromSurface flag.
//
// fromSurface=false reads the renderer's current view directly instead of
// waiting for a fresh compositor surface frame. On idle pages in headed browsers
// (e.g. Cloak) the surface stops swapping frames, so the default fromSurface=true
// blocks until the action deadline (~30s); reading from the view avoids that for
// plain captures. But capture-beyond-viewport and any render-time rescale
// (clip.Scale != 1) both need the page recomposited at a new size, which only
// happens with fromSurface=true — forcing it off there silently drops the
// scale/beyond-viewport effect. So keep it off only for the plain/native-scale
// path.
func captureFromSurface(beyondViewport bool, clip *page.Viewport) bool {
	if beyondViewport {
		return true
	}
	return clip != nil && clip.Scale != 0 && clip.Scale != 1
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
		// Known issue: two back-to-back /capture?scale=<n!=1> on the same
		// tab without nav between can hang on the second call. Workaround:
		// nav between captures (see e2e cli/capture-basic.sh).
		viewportWidth, viewportHeight := opts.ViewportWidth, opts.ViewportHeight
		documentWidth, documentHeight := 0.0, 0.0
		if clip == nil {
			if opts.BeyondViewport {
				documentWidth, documentHeight = fetchDocumentSize(ctx)
			} else if viewportWidth == 0 || viewportHeight == 0 {
				viewportWidth, viewportHeight = fetchViewportSize(ctx)
			}
		}
		clip = scaledScreenshotClip(opts, viewportWidth, viewportHeight, documentWidth, documentHeight)
	}

	fromSurface := captureFromSurface(opts.BeyondViewport, clip)

	var buf []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Wake the target's renderer before capturing. Background / non-foreground
		// tabs (common once target-aware orchestration spreads tabs across
		// providers) throttle their compositor and stop painting, so
		// captureScreenshot blocks until the action deadline (~30s). A best-effort
		// BringToFront resumes painting for the target we are about to capture; the
		// error is ignored so providers whose CDP proxy does not implement it
		// (e.g. Cloak) still capture normally.
		_ = page.BringToFront().Do(ctx)

		shot := page.CaptureScreenshot().WithFormat(opts.Format).WithFromSurface(fromSurface)
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

// ScreenshotClipForNode returns a page-coordinate clip for a backend node ID.
// Handlers use this bridge-owned helper so CDP details stay out of the handler
// layer.
func ScreenshotClipForNode(ctx context.Context, nodeID int64) (*ScreenshotClip, error) {
	// Bring target element into view before computing clip coordinates.
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{
			"backendNodeId": nodeID,
		}, nil)
	})); err != nil {
		return nil, fmt.Errorf("scroll into view: %w", err)
	}

	var resolveResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": nodeID,
		}, &resolveResult)
	})); err != nil {
		return nil, fmt.Errorf("resolve node: %w", err)
	}

	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolveResult, &resolved); err != nil {
		return nil, fmt.Errorf("parse resolved node: %w", err)
	}
	if resolved.Object.ObjectID == "" {
		return nil, fmt.Errorf("element not found in DOM (backendNodeId=%d)", nodeID)
	}

	const boxFn = `function() {
		const rect = this.getBoundingClientRect();
		let x = rect.left + (window.scrollX || window.pageXOffset || 0);
		let y = rect.top + (window.scrollY || window.pageYOffset || 0);
		try {
			let current = window;
			while (current && current.parent && current !== current.parent) {
				const frameEl = current.frameElement;
				if (!frameEl) {
					break;
				}
				const parent = current.parent;
				const frameRect = frameEl.getBoundingClientRect();
				x += frameRect.left + (parent.scrollX || parent.pageXOffset || 0);
				y += frameRect.top + (parent.scrollY || parent.pageYOffset || 0);
				current = parent;
			}
		} catch (e) {
			// Cross-origin ancestors can block frame traversal. Keep the deepest
			// reachable page coordinates in that case.
		}
		return { x, y, width: rect.width, height: rect.height };
	}`

	var callResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": boxFn,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &callResult)
	})); err != nil {
		return nil, fmt.Errorf("read element box: %w", err)
	}

	var boxCall struct {
		Result struct {
			Value struct {
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResult, &boxCall); err != nil {
		return nil, fmt.Errorf("parse element box: %w", err)
	}

	box := boxCall.Result.Value
	if box.Width <= 0 || box.Height <= 0 {
		return nil, fmt.Errorf("element box is empty (width=%.2f height=%.2f)", box.Width, box.Height)
	}
	return &ScreenshotClip{
		X:      box.X,
		Y:      box.Y,
		Width:  box.Width,
		Height: box.Height,
		Scale:  1,
	}, nil
}

// CaptureScreenshot is the provider-aware entry point used across the BridgeAPI
// (screencast polling, recorder, annotated capture). It delegates to the shared
// package-level CaptureScreenshot engine so every provider gets the same
// rendering path, including the BringToFront + WithFromSurface(false) fixes that
// keep headed browsers (e.g. Cloak) from blocking on an idle compositor surface.
func (b *Bridge) CaptureScreenshot(ctx context.Context, format string, quality int, clip *cdptk.ScreenshotClip) ([]byte, error) {
	cdpFormat := page.CaptureScreenshotFormatJpeg
	if format == "png" {
		cdpFormat = page.CaptureScreenshotFormatPng
	}
	var vp *page.Viewport
	if clip != nil {
		vp = &page.Viewport{
			X:      clip.X,
			Y:      clip.Y,
			Width:  clip.Width,
			Height: clip.Height,
			Scale:  clip.Scale,
		}
	}
	buf, err := CaptureScreenshot(ctx, ScreenshotOpts{
		Format:  cdpFormat,
		Quality: quality,
		Clip:    vp,
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	return buf, nil
}
