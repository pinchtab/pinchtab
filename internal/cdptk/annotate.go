package cdptk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
)

// ScrollNodeIntoView ensures the target backend node is visible before we
// read its rect. Mirrors what the non-annotated selector path does in
// ClipForNode.
func ScrollNodeIntoView(ctx context.Context, nodeID int64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{
			"backendNodeId": nodeID,
		}, nil)
	}))
}

// AnnotationRectForNode returns the viewport-relative CSS-pixel rect for a
// backend node id. Document scroll is intentionally NOT added; the overlay
// injector adds scrollX/scrollY when placing absolute-positioned boxes.
func AnnotationRectForNode(ctx context.Context, nodeID int64) (*AnnotationRect, error) {
	var resolveResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": nodeID,
		}, &resolveResult)
	})); err != nil {
		return nil, err
	}
	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolveResult, &resolved); err != nil {
		return nil, err
	}
	if resolved.Object.ObjectID == "" {
		return nil, fmt.Errorf("element not found (backendNodeId=%d)", nodeID)
	}

	// boxFn returns top-level viewport-relative rects. Walks frame ancestors,
	// adding each iframe-element's bounding rect (already in its parent's
	// viewport). If we hit a null frameElement before reaching the top window
	// — typical cross-origin barrier — we flag the rect unprojectable rather
	// than emitting partial frame-local coordinates.
	const boxFn = `function() {
		const rect = this.getBoundingClientRect();
		let x = rect.x;
		let y = rect.y;
		let frameError = false;
		try {
			let cur = window;
			while (cur && cur.parent && cur !== cur.parent) {
				const frameEl = cur.frameElement;
				if (!frameEl) {
					// We are still inside a nested frame (cur !== cur.parent)
					// but cannot reach the host iframe element. Treat the rect
					// as unprojectable so the caller drops it.
					frameError = true;
					break;
				}
				const frameRect = frameEl.getBoundingClientRect();
				x += frameRect.left;
				y += frameRect.top;
				cur = cur.parent;
			}
		} catch (e) {
			frameError = true;
		}
		return { x: x, y: y, w: rect.width, h: rect.height, frameError: frameError };
	}`

	var callResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": boxFn,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &callResult)
	})); err != nil {
		return nil, err
	}
	var box struct {
		Result struct {
			Value struct {
				X          float64 `json:"x"`
				Y          float64 `json:"y"`
				W          float64 `json:"w"`
				H          float64 `json:"h"`
				FrameError bool    `json:"frameError"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResult, &box); err != nil {
		return nil, err
	}
	if box.Result.Value.FrameError {
		// Cross-origin ancestor frame blocked coordinate projection. Skip the
		// node rather than emitting wrong coordinates.
		return nil, nil
	}
	return &AnnotationRect{
		X: box.Result.Value.X,
		Y: box.Result.Value.Y,
		W: box.Result.Value.W,
		H: box.Result.Value.H,
	}, nil
}

// InjectAnnotationOverlay paints an absolute-positioned overlay div on top
// of the page. Viewport-relative rects are converted to document
// coordinates inside the page so the overlay tracks the document during
// capture, regardless of scroll state.
//
// clipTopY is the y coordinate of the captured region's top edge, in the
// same viewport-relative space as item rects. For viewport mode it is 0;
// for selector clip it is the target rect's y. The overlay JS uses it to
// decide whether the label has room above the box in the *captured image*
// (not just in the live viewport) — selector-clipped boxes near the clip's
// top would otherwise place their label above the cropped image and lose
// the visible label.
func InjectAnnotationOverlay(ctx context.Context, items []AnnotationItem, clipTopY float64) error {
	if len(items) == 0 {
		return nil
	}
	// Strip projection-only fields the page does not need.
	type overlayItem struct {
		Ref string  `json:"ref"`
		X   float64 `json:"x"`
		Y   float64 `json:"y"`
		W   float64 `json:"w"`
		H   float64 `json:"h"`
	}
	payload := make([]overlayItem, len(items))
	for i, it := range items {
		payload[i] = overlayItem{Ref: it.Ref, X: it.Box.X, Y: it.Box.Y, W: it.Box.W, H: it.Box.H}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// JS: removes any prior overlay, builds a new layer with one div per
	// annotation. Labels show the numeric portion of the ref ("e5" -> "5")
	// so dense pages stay readable; the response payload still uses "e5".
	script := fmt.Sprintf(`(function(items, rootId, clipTopY) {
		var prev = document.getElementById(rootId);
		if (prev) prev.remove();
		var root = document.createElement('div');
		root.id = rootId;
		root.style.position = 'absolute';
		root.style.top = '0';
		root.style.left = '0';
		root.style.pointerEvents = 'none';
		root.style.zIndex = '2147483646';
		var sx = window.scrollX || window.pageXOffset || 0;
		var sy = window.scrollY || window.pageYOffset || 0;
		for (var i = 0; i < items.length; i++) {
			var it = items[i];
			var box = document.createElement('div');
			box.style.position = 'absolute';
			box.style.left = (it.x + sx) + 'px';
			box.style.top = (it.y + sy) + 'px';
			box.style.width = it.w + 'px';
			box.style.height = it.h + 'px';
			box.style.boxSizing = 'border-box';
			box.style.border = '2px solid rgba(255, 51, 102, 0.95)';
			box.style.borderRadius = '2px';
			box.style.pointerEvents = 'none';
			var label = document.createElement('div');
			label.textContent = String(it.ref).replace(/^[^0-9]+/, '') || it.ref;
			label.style.position = 'absolute';
			label.style.left = '0';
			// Place label above the box by default; flip inside the box when
			// there isn't enough room above the *captured region's* top edge,
			// not just the viewport's. For viewport capture clipTopY is 0 so
			// this collapses to the simple case; for selector clips it is the
			// target rect's y, so labels near the top of the clip stay
			// visible in the cropped image.
			var labelHeight = 16;
			if ((it.y - clipTopY) < labelHeight) {
				label.style.top = '1px';
			} else {
				label.style.top = '-18px';
			}
			label.style.padding = '1px 4px';
			label.style.background = 'rgba(255, 51, 102, 0.95)';
			label.style.color = '#fff';
			label.style.font = '600 11px/1 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif';
			label.style.borderRadius = '2px';
			label.style.whiteSpace = 'nowrap';
			label.style.pointerEvents = 'none';
			box.appendChild(label);
			root.appendChild(box);
		}
		document.documentElement.appendChild(root);
		return items.length;
	})(%s, %q, %v)`, string(encoded), OverlayRootID, clipTopY)

	return chromedp.Run(ctx, chromedp.Evaluate(script, nil))
}

// RemoveAnnotationOverlay removes the overlay div if present. Safe to call
// multiple times and after a failed inject.
func RemoveAnnotationOverlay(ctx context.Context) error {
	const script = `(function(rootId) {
		var el = document.getElementById(rootId);
		if (el) el.remove();
		return true;
	})("` + OverlayRootID + `")`
	return chromedp.Run(ctx, chromedp.Evaluate(script, nil))
}

// ViewportRect reports the current CSS-pixel viewport size. Used to filter
// out annotations that fall entirely outside the visible window.
func ViewportRect(ctx context.Context) (AnnotationRect, error) {
	const fn = `(() => ({ x: 0, y: 0, w: window.innerWidth, h: window.innerHeight }))()`
	var raw json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    fn,
			"returnByValue": true,
		}, &raw)
	})); err != nil {
		return AnnotationRect{}, err
	}
	var resp struct {
		Result struct {
			Value AnnotationRect `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return AnnotationRect{}, err
	}
	return resp.Result.Value, nil
}

// PageScroll returns the document's current scroll offsets in CSS pixels.
func PageScroll(ctx context.Context) (float64, float64, error) {
	const fn = `(() => ({ x: window.scrollX || window.pageXOffset || 0, y: window.scrollY || window.pageYOffset || 0 }))()`
	var raw json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    fn,
			"returnByValue": true,
		}, &raw)
	})); err != nil {
		return 0, 0, err
	}
	var resp struct {
		Result struct {
			Value struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, 0, err
	}
	return resp.Result.Value.X, resp.Result.Value.Y, nil
}

// DevicePixelRatio kept as a small helper in case callers want
// to scale to image pixels. Annotation boxes themselves stay in CSS pixels.
func DevicePixelRatio(ctx context.Context) (float64, error) {
	const fn = `(() => window.devicePixelRatio || 1)()`
	var raw json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    fn,
			"returnByValue": true,
		}, &raw)
	})); err != nil {
		return 1, err
	}
	var resp struct {
		Result struct {
			Value float64 `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 1, err
	}
	if resp.Result.Value <= 0 {
		return 1, nil
	}
	return resp.Result.Value, nil
}

// DocumentSize returns the CSS-pixel scrollable size of the document. Used to
// clip beyond-viewport captures to the full page content. Mirrors what
// Puppeteer/Playwright measure for fullPage screenshots — max of
// documentElement and body across scrollWidth/scrollHeight.
func DocumentSize(ctx context.Context) (float64, float64, error) {
	const fn = `(() => {
		const d = document;
		const de = d.documentElement;
		const b = d.body || de;
		return {
			w: Math.max(de.scrollWidth, b.scrollWidth, de.clientWidth, de.offsetWidth),
			h: Math.max(de.scrollHeight, b.scrollHeight, de.clientHeight, de.offsetHeight)
		};
	})()`
	var raw json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    fn,
			"returnByValue": true,
		}, &raw)
	})); err != nil {
		return 0, 0, err
	}
	var resp struct {
		Result struct {
			Value struct {
				W float64 `json:"w"`
				H float64 `json:"h"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, 0, err
	}
	if resp.Result.Value.W <= 0 || resp.Result.Value.H <= 0 {
		return 0, 0, fmt.Errorf("invalid document size (%.0fx%.0f)", resp.Result.Value.W, resp.Result.Value.H)
	}
	return resp.Result.Value.W, resp.Result.Value.H, nil
}
