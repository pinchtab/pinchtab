package observe

import (
	"context"
	"encoding/json"

	"github.com/chromedp/chromedp"
)

// ViewportInfo describes the layout viewport at capture time. Populated by
// FetchLayout and used by AnnotateBounds for coordinate translation and the
// visibility heuristic.
type ViewportInfo struct {
	Width            float64
	Height           float64
	ScrollX          float64
	ScrollY          float64
	DevicePixelRatio float64
}

// FetchLayout returns the page's current layout viewport in CSS pixels. One
// CDP round trip via Runtime.evaluate so this composes inside PairedCapture
// without an extra Page.getLayoutMetrics call.
func FetchLayout(ctx context.Context) (ViewportInfo, error) {
	var out ViewportInfo
	const expression = `JSON.stringify({
		w: window.innerWidth,
		h: window.innerHeight,
		sx: window.scrollX || window.pageXOffset || 0,
		sy: window.scrollY || window.pageYOffset || 0,
		dpr: window.devicePixelRatio || 1
	})`
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
		return out, err
	}
	var parsed struct {
		W   float64 `json:"w"`
		H   float64 `json:"h"`
		SX  float64 `json:"sx"`
		SY  float64 `json:"sy"`
		DPR float64 `json:"dpr"`
	}
	if err := json.Unmarshal([]byte(result.Result.Value), &parsed); err != nil {
		return out, err
	}
	out.Width = parsed.W
	out.Height = parsed.H
	out.ScrollX = parsed.SX
	out.ScrollY = parsed.SY
	out.DevicePixelRatio = parsed.DPR
	return out, nil
}

// AnnotateBounds fills BoundingBox + Visible on each node whose NodeID is
// non-zero. Each node costs one DOM.getBoxModel round trip; for the typical
// FilterInteractive snapshot of <50 nodes the total budget is ~250ms.
//
// DOM.getBoxModel returns document-relative CSS coordinates. pageCoords=true
// leaves boxes in that space for beyondViewport/clip captures. pageCoords=false
// projects boxes into viewport coordinates by subtracting the current scroll
// offset, matching the default viewport-only screenshot.
//
// Visibility heuristic: a node is Visible if its rect has non-zero area and
// intersects the viewport. The check is intentionally cheap — strict
// occlusion (document.elementFromPoint) is deferred.
func AnnotateBounds(ctx context.Context, nodes []A11yNode, pageCoords bool, vp ViewportInfo) error {
	for i := range nodes {
		if nodes[i].NodeID == 0 {
			continue
		}
		box, ok := getBoxAABB(ctx, nodes[i].NodeID)
		if !ok {
			continue
		}
		visible := isVisible(box, true, vp)
		if !pageCoords {
			box.X -= vp.ScrollX
			box.Y -= vp.ScrollY
		}
		nodes[i].BoundingBox = &box
		nodes[i].Visible = visible
	}
	return nil
}

func getBoxAABB(ctx context.Context, backendNodeID int64) (BoundingBox, bool) {
	var result json.RawMessage
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getBoxModel", map[string]any{
			"backendNodeId": backendNodeID,
		}, &result)
	}))
	if err != nil {
		return BoundingBox{}, false
	}
	var box struct {
		Model struct {
			Content []float64 `json:"content"`
		} `json:"model"`
	}
	if err := json.Unmarshal(result, &box); err != nil {
		return BoundingBox{}, false
	}
	q := box.Model.Content
	if len(q) < 8 {
		return BoundingBox{}, false
	}
	// AABB across the 4 corners — robust against transformed elements.
	minX, maxX := q[0], q[0]
	minY, maxY := q[1], q[1]
	for k := 2; k < 8; k += 2 {
		if q[k] < minX {
			minX = q[k]
		}
		if q[k] > maxX {
			maxX = q[k]
		}
		if q[k+1] < minY {
			minY = q[k+1]
		}
		if q[k+1] > maxY {
			maxY = q[k+1]
		}
	}
	return BoundingBox{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}, true
}

func isVisible(b BoundingBox, pageCoords bool, vp ViewportInfo) bool {
	if b.W <= 0 || b.H <= 0 {
		return false
	}
	if vp.Width <= 0 || vp.Height <= 0 {
		// No viewport info to compare against; fall back to area test only.
		return true
	}
	// Compute the viewport rect in the same coordinate space as the box.
	var vx, vy float64
	if pageCoords {
		vx, vy = vp.ScrollX, vp.ScrollY
	}
	// Rect-rect intersection.
	return b.X+b.W > vx &&
		b.Y+b.H > vy &&
		b.X < vx+vp.Width &&
		b.Y < vy+vp.Height
}
