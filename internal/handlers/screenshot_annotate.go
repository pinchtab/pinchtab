package handlers

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

// collectScreenshotAnnotations builds the candidate annotation set from the
// current accessibility tree (interactive filter), records the refs in the
// tab's ref cache so follow-up actions can target them, and returns
// viewport-relative rects for each candidate. If `selector` is non-empty, the
// selector's own viewport rect is returned as `target` so callers can clip
// and project against it later.
func (h *Handlers) collectScreenshotAnnotations(
	ctx context.Context,
	tabID, selector string,
) (items []cdptk.AnnotationItem, target *cdptk.AnnotationRect, err error) {
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
		if err := cdptk.ScrollNodeIntoView(ctx, scopeID); err != nil {
			return nil, nil, fmt.Errorf("annotate scroll: %w", err)
		}
		rect, rectErr := cdptk.AnnotationRectForNode(ctx, scopeID)
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

	items = make([]cdptk.AnnotationItem, 0, len(flat))
	for _, n := range flat {
		if n.NodeID == 0 || n.Ref == "" {
			continue
		}
		rect, rectErr := cdptk.AnnotationRectForNode(ctx, n.NodeID)
		if rectErr != nil || rect == nil {
			continue
		}
		if rect.W <= 0 || rect.H <= 0 {
			continue
		}
		items = append(items, cdptk.AnnotationItem{
			Ref:  n.Ref,
			Role: n.Role,
			Name: n.Name,
			Tag:  n.Tag,
			Box:  *rect,
		})
	}

	// Stable order keeps overlay rendering and JSON output deterministic, and
	// makes the post-cap truncation predictable.
	sort.SliceStable(items, func(i, j int) bool { return cdptk.RefLess(items[i].Ref, items[j].Ref) })
	if len(items) > cdptk.MaxAnnotations {
		items = items[:cdptk.MaxAnnotations]
	}
	return items, target, nil
}

// captureAnnotatedScreenshot orchestrates the annotated-screenshot pipeline.
// It collects refs, injects a DOM overlay, calls the provider-aware capture
// path (so chrome and cloak alike honor the same rendering), removes the
// overlay, and projects the returned boxes into the coordinate space of the
// resulting image.
func (h *Handlers) captureAnnotatedScreenshot(
	ctx context.Context,
	tabID, selector, format string,
	quality int,
	beyondViewport bool,
) (img []byte, projected []cdptk.AnnotationItem, outFormat string, err error) {
	items, target, err := h.collectScreenshotAnnotations(ctx, tabID, selector)
	if err != nil {
		return nil, nil, "", err
	}

	mode := cdptk.ModeViewport
	switch {
	case target != nil:
		mode = cdptk.ModeSelectorClip
	case beyondViewport:
		mode = cdptk.ModeBeyondViewport
	}

	// scrollX/scrollY are read once and reused for region shift, clipTopY, and
	// final box projection so all three stay consistent if the page scrolls
	// between calls.
	var scrollX, scrollY float64
	var docW, docH float64
	region := cdptk.AnnotationRect{}
	switch mode {
	case cdptk.ModeViewport:
		region, err = cdptk.ViewportRect(ctx)
		if err != nil {
			return nil, nil, "", err
		}
	case cdptk.ModeBeyondViewport:
		docW, docH, err = cdptk.DocumentSize(ctx)
		if err != nil {
			return nil, nil, "", fmt.Errorf("document size: %w", err)
		}
		scrollX, scrollY, _ = cdptk.PageScroll(ctx)
		// Item rects are viewport-relative; shift the document rect into the
		// same space so FilterAnnotationItems keeps every element that lives
		// anywhere inside the captured document.
		region = cdptk.AnnotationRect{X: -scrollX, Y: -scrollY, W: docW, H: docH}
	}
	items = cdptk.FilterAnnotationItems(items, target, region)

	// Always clear stale overlay state before capture, regardless of whether we
	// have items to draw — protects against leftovers from a prior crashed run.
	_ = cdptk.RemoveAnnotationOverlay(ctx)
	overlayInjected := false
	if len(items) > 0 {
		clipTopY := 0.0
		switch {
		case mode == cdptk.ModeSelectorClip && target != nil:
			clipTopY = target.Y
		case mode == cdptk.ModeBeyondViewport:
			// Document top in viewport coords. Lets the overlay's "is there room
			// for a label above the box inside the captured image" check reduce
			// to "document y >= labelHeight".
			clipTopY = -scrollY
		}
		if err := cdptk.InjectAnnotationOverlay(ctx, items, clipTopY); err != nil {
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
		_ = cdptk.RemoveAnnotationOverlay(cleanupCtx)
	}()

	outFormat = format
	if format != "png" {
		outFormat = "jpeg"
	}

	var clip *cdptk.ScreenshotClip
	switch {
	case mode == cdptk.ModeSelectorClip && target != nil:
		// Translate the viewport rect into page coords for the CDP clip.
		offsetX, offsetY, scrollErr := cdptk.PageScroll(ctx)
		if scrollErr != nil {
			offsetX, offsetY = 0, 0
		}
		clip = &cdptk.ScreenshotClip{
			X:      target.X + offsetX,
			Y:      target.Y + offsetY,
			Width:  target.W,
			Height: target.H,
			Scale:  1,
		}
	case mode == cdptk.ModeBeyondViewport:
		// A document-covering clip captures the full scrollable page through the
		// provider-aware method, equivalent to captureBeyondViewport on chrome
		// while also working for headed providers like cloak.
		clip = &cdptk.ScreenshotClip{X: 0, Y: 0, Width: docW, Height: docH, Scale: 1}
	}

	img, err = h.Bridge.CaptureScreenshot(ctx, outFormat, quality, clip)
	if err != nil {
		return nil, nil, "", fmt.Errorf("capture: %w", err)
	}

	projected = cdptk.ProjectAnnotationBoxes(items, target, mode, scrollX, scrollY)
	return img, projected, outFormat, nil
}
