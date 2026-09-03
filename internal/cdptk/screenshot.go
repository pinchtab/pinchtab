package cdptk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func ClipForNode(ctx context.Context, nodeID int64, css1x bool) (*ScreenshotClip, error) {
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

	// Translate the element box into top-level page coordinates. captureScreenshot
	// clip coordinates are page-relative, so viewport-relative rects need scroll
	// offsets from the current document and each ancestor frame.
	// The frame walk starts from the NODE's own view for the same reason as the
	// bridge clip builder: the isolated handle runs in the top frame's world, so a
	// bare `window` there loses every frame offset.
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
		return {
			x,
			y,
			width: rect.width,
			height: rect.height,
			dpr: view.devicePixelRatio || 1
		};
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
				DPR    float64 `json:"dpr"`
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
	scale := 1.0
	if css1x {
		if box.DPR <= 0 {
			box.DPR = 1
		}
		scale = 1 / box.DPR
	}

	return &ScreenshotClip{
		X:      box.X,
		Y:      box.Y,
		Width:  box.Width,
		Height: box.Height,
		Scale:  scale,
	}, nil
}

// CaptureFromSurface decides the Page.captureScreenshot fromSurface flag. It is the one
// owner of that rule: it once had two implementations that could not import each other,
// and they drifted — one path was fixed while the other kept fromSurface=false and went
// on applying a clip CDP discarded, so scoped and beyond-viewport captures returned the
// full viewport with their annotation boxes mapped into the clip's coordinate space.
//
// fromSurface=false reads the renderer's current view directly instead of waiting for a
// fresh compositor surface frame. On idle pages in headed browsers (e.g. Cloak) the
// surface stops swapping frames, so the default fromSurface=true blocks until the action
// deadline (~30s) — stalling one-shot screenshots and polling screencast alike. That is
// why the nil-clip fast path matters and must be preserved outside Windows. Windows
// headless Chrome with software rendering can return an unpainted white view through
// that path, so it always requests a freshly composited surface.
//
// Anything that changes the captured region needs the page recomposited, which only
// happens with fromSurface=true: capture-beyond-viewport, a render-time rescale, and the
// clip itself. Reading the view ignores the clip outright and returns the whole viewport,
// so every non-nil clip takes the surface path — keeping it off for a native-scale clip
// is what made a selector capture come back byte-identical to an unclipped one.
//
// Measured in headless Chrome on a 120x60 clip, so the flag is not a no-op there and a
// clipped capture is exactly what makes it observable:
//
//	fromSurface=true  clip -> 120x60      fromSurface=false clip -> 756x413
//	fromSurface=true  nil  -> 756x413     fromSurface=false nil  -> 756x413
//
// The nil-clip pair matches on dimensions, which is why an unclipped browser test cannot
// pin this choice; hence the table test beside this function.
func CaptureFromSurface(beyondViewport bool, clip *page.Viewport) bool {
	return captureFromSurface(runtime.GOOS, beyondViewport, clip)
}

func captureFromSurface(goos string, beyondViewport bool, clip *page.Viewport) bool {
	return goos == "windows" || beyondViewport || clip != nil
}

// ViewportClip is the clip that makes a DEFAULT capture come back in the space its
// own metadata reports. It is the answer to a contract, not an optimisation: /capture
// publishes coordinateSpace, viewport and devicePixelRatio so a client can scale a
// node's boundingBox onto image pixels without guessing, and the read-the-view fast
// path cannot keep that promise. That path returns the real window surface, which
// Emulation.setDeviceMetricsOverride does not touch and whose scale factor is the
// screen's, not the page's — measured in headless Chrome on a HiDPI screen, a page
// reporting 756x413 at devicePixelRatio 1 came back 1512x826, and the same page
// emulated to 400x300 came back 800x600. Both are a caller's overlay landing on the
// wrong pixels, and no reported dpr rescues the emulated case, where the surface can
// carry a different aspect ratio entirely.
//
// A clip covering the viewport composites the page and lets CDP apply the page's own
// device pixel ratio, which is what the beyond-viewport and selector paths already do
// and why those two were correct while the default one was not. Scale stays 1: CDP
// multiplies a clipped capture by the page dpr itself, so a scale of dpr would square
// it. Coordinates are page-relative, hence the scroll offsets.
//
// The cost is stated rather than hidden: a clipped capture takes fromSurface=true, so
// the default /capture gives up the read-the-view fast path this file documents and an
// idle headed browser can stall it until the action deadline. /screenshot and the
// screencast keep the fast path — they publish no coordinate contract to break — so the
// latency trade is paid only by the endpoint whose whole purpose is mapping refs onto
// pixels.
func ViewportClip(scrollX, scrollY, width, height float64) *page.Viewport {
	if width <= 0 || height <= 0 {
		return nil
	}
	return &page.Viewport{X: scrollX, Y: scrollY, Width: width, Height: height, Scale: 1}
}

// captureRefusal is the CDP error a renderer returns when it will not serve the
// read-the-view fast path. It carries no code the client can match on, so the message is
// the only discriminator available.
const captureRefusal = "Unable to capture screenshot"

// CaptureWithSurfaceFallback runs a capture and, when the renderer REFUSES the fast path,
// retries once forcing a fresh composite.
//
// fromSurface=false reads the renderer's current view rather than waiting for a surface
// frame, which is why CaptureFromSurface prefers it — but that view is a resource that can
// be unavailable (no screen-recording permission, another client holding the surface), and
// when it is, CDP answers -32000 instead of a frame. Failing the whole capture there throws
// away a picture the browser can still produce the slower way.
//
// Retried ONLY on that refusal, and only when the fast path was actually taken: any other
// error is a real failure and must surface, and a fromSurface=true capture that fails has
// no faster path left to fall back from. The retry costs one extra round trip in the case
// that was going to fail outright, and nothing at all on a healthy browser.
//
// The fallback is deliberate about what it does NOT change: it re-runs the SAME params, so
// a clip still travels and a clipped capture cannot silently widen to the whole viewport.
func CaptureWithSurfaceFallback(fromSurface bool, capture func(fromSurface bool) ([]byte, error)) ([]byte, error) {
	buf, err := capture(fromSurface)
	if err == nil || fromSurface || !strings.Contains(err.Error(), captureRefusal) {
		return buf, err
	}
	slog.Warn("screenshot: renderer refused the read-the-view capture, retrying from surface", "err", err)
	return capture(true)
}

// NormalizedViewport owns the non-zero-scale rule for a clip already in CDP's own type.
// Scale 0 means native. CDP does not agree: it discards a scale-0 clip and returns the
// whole viewport, no error, exactly as it discards a clip passed with fromSurface=false.
// Same symptom, same silence, so the rule is applied where the value meets CDP rather
// than at whichever producer remembered. A nil viewport stays nil, keeping the nil-clip
// fast path CaptureFromSurface describes.
func NormalizedViewport(vp *page.Viewport) *page.Viewport {
	if vp == nil || vp.Scale != 0 {
		return vp
	}
	out := *vp
	out.Scale = 1
	return &out
}

// ClipViewport converts a ScreenshotClip into the page.Viewport CDP takes, routed
// through NormalizedViewport so no conversion path can hand CDP a scale-0 clip.
func ClipViewport(clip *ScreenshotClip) *page.Viewport {
	if clip == nil {
		return nil
	}
	return NormalizedViewport(&page.Viewport{
		X:      clip.X,
		Y:      clip.Y,
		Width:  clip.Width,
		Height: clip.Height,
		Scale:  clip.Scale,
	})
}
