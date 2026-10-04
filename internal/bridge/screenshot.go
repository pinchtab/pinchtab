package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/config"
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
	w, h, err := cdptk.DocumentSize(ctx)
	if err != nil {
		return 0, 0
	}
	return w, h
}

// ScreenshotOpts mirrors the subset of page.CaptureScreenshot parameters the
// callers (HandleScreenshot, PairedCapture) need to coordinate on.
type ScreenshotOpts struct {
	Format         page.CaptureScreenshotFormat
	Quality        int
	Clip           *page.Viewport
	BeyondViewport bool

	// ClipToViewport fetches the layout and captures exactly the emulated viewport,
	// including scroll offsets, instead of reading the window surface when neither
	// Clip nor BeyondViewport is set.
	ClipToViewport bool

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

	// DisableActivation suppresses the Page.bringToFront call that wakes a
	// backgrounded tab's compositor before capturing (the inverse of
	// instanceDefaults.captureAllowActivation, default true — so the zero
	// value here is the safe/reliable default even if a caller forgets to
	// set it). When true, capture relies solely on focus emulation and never
	// raises the tab in the operator's browser, accepting that a background
	// tab's capture may then block until the caller's deadline.
	DisableActivation bool
}

func scaledScreenshotClip(opts ScreenshotOpts, viewportWidth, viewportHeight, documentWidth, documentHeight float64) *page.Viewport {
	if opts.Scale <= 0 || opts.Scale == 1 {
		return cdptk.NormalizedViewport(opts.Clip)
	}
	if opts.Clip != nil {
		clip := *cdptk.NormalizedViewport(opts.Clip)
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

// captureScreenshotWithoutActivation wakes a background renderer before
// capturing it. Focus emulation alone does not resume a backgrounded tab's
// compositor — that requires Page.bringToFront — so unless disableActivation
// is set, it always runs as well; focus emulation is layered on top only so
// document.hasFocus()/`:focus` reflect the capture as best-effort. When
// disableActivation is true, the tab is never raised in the operator's
// browser, accepting that its capture may then block until the caller's
// deadline (Chromium never resumes a backgrounded compositor without either
// activation or being brought into view).
func captureScreenshotWithoutActivation(ctx context.Context, shot *page.CaptureScreenshotParams, disableActivation bool) ([]byte, error) {
	executor := cdp.ExecutorFromContext(ctx)
	focusEmulated := emulation.SetFocusEmulationEnabled(true).Do(ctx) == nil
	if focusEmulated {
		defer func() {
			restoreCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			restoreCtx = cdp.WithExecutor(restoreCtx, executor)
			_ = emulation.SetFocusEmulationEnabled(false).Do(restoreCtx)
		}()
	}
	if !disableActivation {
		_ = page.BringToFront().Do(ctx)
	}
	return shot.Do(ctx)
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
	if opts.ClipToViewport && opts.Clip == nil && !opts.BeyondViewport {
		if vp, err := FetchLayout(ctx); err == nil {
			opts.Clip = cdptk.ViewportClip(vp.ScrollX, vp.ScrollY, vp.Width, vp.Height)
		}
	}

	// Known issue: two back-to-back /capture?scale=<n!=1> on the same
	// tab without nav between can hang on the second call. Workaround:
	// nav between captures (see e2e cli/capture-basic.sh).
	viewportWidth, viewportHeight := opts.ViewportWidth, opts.ViewportHeight
	documentWidth, documentHeight := 0.0, 0.0
	if opts.Scale > 0 && opts.Scale != 1 && opts.Clip == nil {
		if opts.BeyondViewport {
			documentWidth, documentHeight = fetchDocumentSize(ctx)
		} else if viewportWidth == 0 || viewportHeight == 0 {
			viewportWidth, viewportHeight = fetchViewportSize(ctx)
		}
	}
	clip := scaledScreenshotClip(opts, viewportWidth, viewportHeight, documentWidth, documentHeight)

	fromSurface := cdptk.CaptureFromSurface(opts.BeyondViewport, clip)

	return cdptk.CaptureWithSurfaceFallback(fromSurface, func(fromSurface bool) ([]byte, error) {
		var buf []byte
		err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
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
			buf, inner = captureScreenshotWithoutActivation(ctx, shot, opts.DisableActivation)
			return inner
		}))
		return buf, err
	})
}

// ClipsToViewport reports whether the configured browser supports idle surface
// captures, allowing screenshots to match the emulated viewport.
func ClipsToViewport(cfg *config.RuntimeConfig) bool {
	if cfg == nil {
		return false
	}
	// Same provider lookup as shouldUsePollingScreencast (screencast.go).
	browser, ok := browsers.Get(config.NormalizeBrowser(cfg.DefaultBrowser))
	return ok && browser.Capabilities().Has(browsers.CapSurfaceCapture)
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

	// Isolated world: boxFn reads getBoundingClientRect, so a main-world handle
	// would let the page steer the clip origin by redefining it.
	objectID, err := IsolatedNodeObjectID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("resolve node: %w", err)
	}

	// The frame walk starts from the NODE's own view, not the ambient `window`.
	// An isolated-world handle runs in the world it was resolved into, which is
	// not necessarily the node's frame, and a bare `window` there is the wrong
	// frame's — its frameElement chain is empty and every frame offset is lost.
	const boxFn = `function() {
		const view = (this.ownerDocument && this.ownerDocument.defaultView) || window;
		const rect = this.getBoundingClientRect();
		let x = rect.left + (view.scrollX || view.pageXOffset || 0);
		let y = rect.top + (view.scrollY || view.pageYOffset || 0);
		try {
			let current = view;
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
			"objectId":            objectID,
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
// rendering path, gated by b.Config.CaptureAllowActivation (default true).
func (b *Bridge) CaptureScreenshot(ctx context.Context, format string, quality int, clip *cdptk.ScreenshotClip) ([]byte, error) {
	cdpFormat := page.CaptureScreenshotFormatJpeg
	if format == "png" {
		cdpFormat = page.CaptureScreenshotFormatPng
	}
	vp := cdptk.ClipViewport(clip)
	disableActivation := b.Config != nil && !b.Config.CaptureAllowActivation
	buf, err := CaptureScreenshot(ctx, ScreenshotOpts{
		Format:            cdpFormat,
		Quality:           quality,
		Clip:              vp,
		ClipToViewport:    ClipsToViewport(b.Config),
		DisableActivation: disableActivation,
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	return buf, nil
}
