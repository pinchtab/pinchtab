package cdpops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"
)

var (
	ErrElementOccluded  = errors.New("element is occluded")
	ErrElementHidden    = errors.New("element is hidden")
	ErrElementBlocked   = errors.New("element is blocked from pointer interaction")
	ErrElementOffscreen = errors.New("element center is outside viewport")
)

type pointerProbe struct {
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	InViewport   bool    `json:"inViewport"`
	Visible      bool    `json:"visible"`
	PointerEvent string  `json:"pointerEvent"`
	Occluded     bool    `json:"occluded"`
	TopTag       string  `json:"topTag"`
}

// PointerPointForNode validates clickability assumptions and returns a
// stable pointer coordinate for the backend node.
func PointerPointForNode(ctx context.Context, backendNodeID int64, requireTopMost bool) (float64, float64, error) {
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": backendNodeID}, nil)
	})); err != nil {
		return 0, 0, fmt.Errorf("scroll into view: %w", err)
	}

	var resolveResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &resolveResult)
	})); err != nil {
		return 0, 0, fmt.Errorf("resolve node: %w", err)
	}

	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolveResult, &resolved); err != nil {
		return 0, 0, err
	}
	if strings.TrimSpace(resolved.Object.ObjectID) == "" {
		return 0, 0, fmt.Errorf("resolve node: backend node %d not found", backendNodeID)
	}

	const probeJS = `function() {
		const r = this.getBoundingClientRect();
		const style = window.getComputedStyle(this);
		const x = r.left + (r.width / 2);
		const y = r.top + (r.height / 2);
		const inViewport = x >= 0 && y >= 0 && x <= window.innerWidth && y <= window.innerHeight;
		const visible = !!style && style.display !== 'none' && style.visibility !== 'hidden' && Number(style.opacity || '1') > 0;
		const pointerEvent = style ? String(style.pointerEvents || '') : '';
		let occluded = false;
		let topTag = '';
		if (inViewport) {
			const top = document.elementFromPoint(x, y);
			if (top) {
				topTag = String(top.tagName || '').toLowerCase();
				const related = top === this || this.contains(top) || top.contains(this);
				occluded = !related;
			}
		}
		return {
			x,
			y,
			width: r.width,
			height: r.height,
			inViewport,
			visible,
			pointerEvent,
			occluded,
			topTag
		};
	}`

	var probeRaw json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": probeJS,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &probeRaw)
	})); err != nil {
		return 0, 0, fmt.Errorf("pointer probe: %w", err)
	}

	var callRes struct {
		Result struct {
			Value pointerProbe `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(probeRaw, &callRes); err != nil {
		return 0, 0, err
	}
	probe := callRes.Result.Value

	if probe.Width <= 0 || probe.Height <= 0 || !probe.Visible {
		return 0, 0, fmt.Errorf("%w: width=%.2f height=%.2f", ErrElementHidden, probe.Width, probe.Height)
	}
	if !probe.InViewport {
		return 0, 0, fmt.Errorf("%w: x=%.2f y=%.2f", ErrElementOffscreen, probe.X, probe.Y)
	}
	if strings.EqualFold(strings.TrimSpace(probe.PointerEvent), "none") {
		return 0, 0, fmt.Errorf("%w: pointer-events=none", ErrElementBlocked)
	}
	if requireTopMost && probe.Occluded {
		return 0, 0, fmt.Errorf("%w: top=%s", ErrElementOccluded, probe.TopTag)
	}

	return probe.X, probe.Y, nil
}
