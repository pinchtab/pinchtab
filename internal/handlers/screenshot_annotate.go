package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

// maxAnnotations bounds how many overlay rectangles we render per screenshot.
// Caps payload size and overlay-render cost on dense pages.
const maxAnnotations = 200

// overlayRootID is the stable DOM id for the injected annotation layer. We
// look for it before injection so a stale overlay from a previous failed
// capture is removed first.
const overlayRootID = "__pinchtab_annotations__"

// captureMode selects how returned annotation boxes are projected.
type captureMode int

const (
	modeViewport captureMode = iota
	modeSelectorClip
)

type annotationItem struct {
	Ref  string         `json:"ref"`
	Role string         `json:"role,omitempty"`
	Name string         `json:"name,omitempty"`
	Tag  string         `json:"tag,omitempty"`
	Box  annotationRect `json:"box"`
}

type annotationRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// collectScreenshotAnnotations builds the candidate annotation set from the
// current accessibility tree (interactive filter), records the refs in the
// tab's ref cache so follow-up actions can target them, and returns
// viewport-relative rects for each candidate. If `selector` is non-empty, the
// selector's own viewport rect is returned as `target` so callers can clip
// and project against it later.
func (h *Handlers) collectScreenshotAnnotations(
	ctx context.Context,
	tabID, selector string,
) (items []annotationItem, target *annotationRect, err error) {
	rawNodes, err := bridge.FetchAXTree(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("a11y tree: %w", err)
	}
	rawNodes = h.scopeSnapshotNodesByFrame(rawNodes, h.selectorFrameID(tabID))

	// Selector narrows both the screenshot clip (handled by caller) and the
	// candidate set. Resolve the target node and constrain the AX subtree to
	// it so refs reflect only what the agent will see in the clip.
	if selector != "" {
		scopeID, scopeErr := h.resolveSelectorNodeID(ctx, tabID, selector)
		if scopeErr != nil {
			return nil, nil, fmt.Errorf("annotate selector: %w", scopeErr)
		}
		rawNodes = bridge.FilterSubtree(rawNodes, scopeID)
		// Bring the target into the viewport before computing rects, otherwise
		// an offscreen selector produces a blank/wrong clip and all candidate
		// rects are read in pre-scroll positions.
		if err := scrollNodeIntoView(ctx, scopeID); err != nil {
			return nil, nil, fmt.Errorf("annotate scroll: %w", err)
		}
		rect, rectErr := annotationRectForNode(ctx, scopeID)
		if rectErr != nil {
			return nil, nil, fmt.Errorf("annotate selector rect: %w", rectErr)
		}
		if rect == nil {
			// nil with no error means cross-origin frame projection blocked the
			// rect read. We'd otherwise silently fall back to a full-viewport
			// capture, which is a different scope than the caller asked for.
			return nil, nil, fmt.Errorf("annotate selector %q: target rect unavailable (cross-origin frame?)", selector)
		}
		target = rect
	}

	flat, refs := bridge.BuildSnapshot(rawNodes, "interactive", -1)
	if len(flat) == 0 && selector == "" {
		// Pages with no AX-interactive content (canvas-heavy fixtures, etc.)
		// fall back to the unfiltered tree so the agent still gets a layout.
		flat, refs = bridge.BuildSnapshot(rawNodes, "", -1)
	}

	// Record refs so a subsequent click/fill on `e5` resolves the same node.
	h.Bridge.SetRefCache(tabID, &bridge.RefCache{
		Refs:    refs,
		Targets: bridge.RefTargetsFromNodes(flat),
		Nodes:   flat,
	})

	items = make([]annotationItem, 0, len(flat))
	for _, n := range flat {
		if n.NodeID == 0 || n.Ref == "" {
			continue
		}
		rect, rectErr := annotationRectForNode(ctx, n.NodeID)
		if rectErr != nil || rect == nil {
			continue
		}
		if rect.W <= 0 || rect.H <= 0 {
			continue
		}
		items = append(items, annotationItem{
			Ref:  n.Ref,
			Role: n.Role,
			Name: n.Name,
			Tag:  n.Tag,
			Box:  *rect,
		})
	}

	// Stable order keeps overlay rendering and JSON output deterministic, and
	// makes the post-cap truncation predictable.
	sort.SliceStable(items, func(i, j int) bool { return refLess(items[i].Ref, items[j].Ref) })
	if len(items) > maxAnnotations {
		items = items[:maxAnnotations]
	}
	return items, target, nil
}

// scrollNodeIntoView ensures the target backend node is visible before we
// read its rect. Mirrors what the non-annotated selector path does in
// screenshotClipForNode.
func scrollNodeIntoView(ctx context.Context, nodeID int64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{
			"backendNodeId": nodeID,
		}, nil)
	}))
}

// annotationRectForNode returns the viewport-relative CSS-pixel rect for a
// backend node id. Document scroll is intentionally NOT added; the overlay
// injector adds scrollX/scrollY when placing absolute-positioned boxes.
func annotationRectForNode(ctx context.Context, nodeID int64) (*annotationRect, error) {
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

	// boxFn returns top-level viewport-relative rects so the overlay (which we
	// always inject into the top document) can place boxes correctly even for
	// iframe-owned nodes. Frame ancestors are walked and their iframe-element
	// bounding rects (already in their parent's viewport) are added. Cross-
	// origin frames throw on access; in that case we mark the rect as
	// unprojectable so the caller can drop it.
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
	return &annotationRect{
		X: box.Result.Value.X,
		Y: box.Result.Value.Y,
		W: box.Result.Value.W,
		H: box.Result.Value.H,
	}, nil
}

// filterAnnotationItems keeps only items whose viewport rect overlaps the
// active capture region. For viewport mode `target` is nil and `viewport`
// is used; for selector-clip mode the selector rect is used instead.
func filterAnnotationItems(items []annotationItem, target *annotationRect, viewport annotationRect) []annotationItem {
	region := viewport
	if target != nil {
		region = *target
	}
	out := items[:0]
	for _, it := range items {
		if rectsOverlap(it.Box, region) {
			out = append(out, it)
		}
	}
	return out
}

func rectsOverlap(a, b annotationRect) bool {
	if a.W <= 0 || a.H <= 0 || b.W <= 0 || b.H <= 0 {
		return false
	}
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}

// projectAnnotationBoxes returns boxes in the coordinate space of the
// returned screenshot. Viewport mode passes the rects through unchanged;
// selector-clip mode subtracts the target origin so boxes are relative to
// the clipped image.
func projectAnnotationBoxes(items []annotationItem, target *annotationRect, mode captureMode) []annotationItem {
	out := make([]annotationItem, len(items))
	for i, it := range items {
		out[i] = it
		if mode == modeSelectorClip && target != nil {
			out[i].Box = annotationRect{
				X: roundFloat(it.Box.X - target.X),
				Y: roundFloat(it.Box.Y - target.Y),
				W: roundFloat(it.Box.W),
				H: roundFloat(it.Box.H),
			}
			continue
		}
		out[i].Box = annotationRect{
			X: roundFloat(it.Box.X),
			Y: roundFloat(it.Box.Y),
			W: roundFloat(it.Box.W),
			H: roundFloat(it.Box.H),
		}
	}
	return out
}

// roundFloat returns the float rounded to the nearest integer (still as
// float64). Spec asks for integer values in the public box payload.
func roundFloat(f float64) float64 {
	if f >= 0 {
		return float64(int64(f + 0.5))
	}
	return float64(int64(f - 0.5))
}

// injectAnnotationOverlay paints an absolute-positioned overlay div on top
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
func injectAnnotationOverlay(ctx context.Context, items []annotationItem, clipTopY float64) error {
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
	})(%s, %q, %v)`, string(encoded), overlayRootID, clipTopY)

	return chromedp.Run(ctx, chromedp.Evaluate(script, nil))
}

// removeAnnotationOverlay removes the overlay div if present. Safe to call
// multiple times and after a failed inject.
func removeAnnotationOverlay(ctx context.Context) error {
	const script = `(function(rootId) {
		var el = document.getElementById(rootId);
		if (el) el.remove();
		return true;
	})("` + overlayRootID + `")`
	return chromedp.Run(ctx, chromedp.Evaluate(script, nil))
}

// viewportRect reports the current CSS-pixel viewport size. Used to filter
// out annotations that fall entirely outside the visible window.
func viewportRect(ctx context.Context) (annotationRect, error) {
	const fn = `(() => ({ x: 0, y: 0, w: window.innerWidth, h: window.innerHeight }))()`
	var raw json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    fn,
			"returnByValue": true,
		}, &raw)
	})); err != nil {
		return annotationRect{}, err
	}
	var resp struct {
		Result struct {
			Value annotationRect `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return annotationRect{}, err
	}
	return resp.Result.Value, nil
}

// captureAnnotatedScreenshot orchestrates the annotated-screenshot pipeline.
// It collects refs, injects a DOM overlay, calls the existing CDP capture
// path, removes the overlay, and projects the returned boxes into the
// coordinate space of the resulting image.
func (h *Handlers) captureAnnotatedScreenshot(
	ctx context.Context,
	tabID, selector, format string,
	quality int,
) (img []byte, projected []annotationItem, outFormat string, err error) {
	items, target, err := h.collectScreenshotAnnotations(ctx, tabID, selector)
	if err != nil {
		return nil, nil, "", err
	}

	mode := modeViewport
	if target != nil {
		mode = modeSelectorClip
	}

	region := annotationRect{}
	if mode == modeViewport {
		region, err = viewportRect(ctx)
		if err != nil {
			return nil, nil, "", err
		}
	}
	items = filterAnnotationItems(items, target, region)

	// Always clear stale overlay state before capture, regardless of whether
	// we have items to draw — protects against leftovers from a prior
	// crashed run.
	_ = removeAnnotationOverlay(ctx)
	overlayInjected := false
	if len(items) > 0 {
		clipTopY := 0.0
		if mode == modeSelectorClip && target != nil {
			clipTopY = target.Y
		}
		if err := injectAnnotationOverlay(ctx, items, clipTopY); err != nil {
			return nil, nil, "", fmt.Errorf("inject overlay: %w", err)
		}
		overlayInjected = true
	}
	defer func() {
		if !overlayInjected {
			return
		}
		// Capture-time deadlines or client cancellation cancel `ctx`, which
		// would skip the cleanup CDP call and leave the overlay in the live
		// page. Use a detached context with its own short deadline so cleanup
		// runs even after the parent is cancelled.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = removeAnnotationOverlay(cleanupCtx)
	}()

	cdpFormat := page.CaptureScreenshotFormatJpeg
	outFormat = "jpeg"
	if format == "png" {
		cdpFormat = page.CaptureScreenshotFormatPng
		outFormat = "png"
	}

	var clip *page.Viewport
	if mode == modeSelectorClip && target != nil {
		// Translate viewport rect into page coords for CDP clip.
		dpr, _ := devicePixelRatioForAnnotation(ctx)
		_ = dpr // CDP clip uses CSS pixels with `scale: 1`.
		offsetX, offsetY, scrollErr := pageScroll(ctx)
		if scrollErr != nil {
			offsetX, offsetY = 0, 0
		}
		clip = &page.Viewport{
			X:      target.X + offsetX,
			Y:      target.Y + offsetY,
			Width:  target.W,
			Height: target.H,
			Scale:  1,
		}
	}

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		shot := page.CaptureScreenshot().WithFormat(cdpFormat)
		if clip != nil {
			shot = shot.WithClip(clip)
		}
		if cdpFormat == page.CaptureScreenshotFormatJpeg {
			shot = shot.WithQuality(int64(quality))
		}
		var captureErr error
		img, captureErr = shot.Do(ctx)
		return captureErr
	})); err != nil {
		return nil, nil, "", fmt.Errorf("capture: %w", err)
	}

	projected = projectAnnotationBoxes(items, target, mode)
	return img, projected, outFormat, nil
}

// pageScroll returns the document's current scroll offsets in CSS pixels.
func pageScroll(ctx context.Context) (float64, float64, error) {
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

// devicePixelRatioForAnnotation kept as a small helper in case callers want
// to scale to image pixels. Annotation boxes themselves stay in CSS pixels.
func devicePixelRatioForAnnotation(ctx context.Context) (float64, error) {
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

// refLess sorts refs like e1 < e2 < e10 by their numeric suffix.
func refLess(a, b string) bool {
	an := refNumber(a)
	bn := refNumber(b)
	if an != bn {
		return an < bn
	}
	return a < b
}

func refNumber(ref string) int {
	n := 0
	started := false
	for _, c := range ref {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
			started = true
		} else if started {
			break
		}
	}
	return n
}
