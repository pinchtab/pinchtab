package cdpops

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
)

// GetElementCenter returns the center coordinates of an element using DOM.getBoxModel.
func GetElementCenter(ctx context.Context, backendNodeID int64) (x, y float64, err error) {
	var result json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getBoxModel", map[string]any{
			"backendNodeId": backendNodeID,
		}, &result)
	}))
	if err != nil {
		return 0, 0, err
	}

	var box struct {
		Model struct {
			Content []float64 `json:"content"`
		} `json:"model"`
	}
	if err = json.Unmarshal(result, &box); err != nil {
		return 0, 0, err
	}

	if len(box.Model.Content) < 4 {
		return 0, 0, fmt.Errorf("invalid box model: expected at least 4 coordinates")
	}

	x = (box.Model.Content[0] + box.Model.Content[2] + box.Model.Content[4] + box.Model.Content[6]) / 4
	y = (box.Model.Content[1] + box.Model.Content[3] + box.Model.Content[5] + box.Model.Content[7]) / 4

	if x == 0 && y == 0 {
		return GetElementCenterJS(ctx, backendNodeID)
	}

	return x, y, nil
}

// GetElementCenterJS resolves the DOM node and evaluates getBoundingClientRect().
func GetElementCenterJS(ctx context.Context, backendNodeID int64) (float64, float64, error) {
	var resolveResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &resolveResult)
	})); err != nil {
		return 0, 0, fmt.Errorf("DOM.resolveNode: %w", err)
	}

	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolveResult, &resolved); err != nil {
		return 0, 0, err
	}
	if resolved.Object.ObjectID == "" {
		return 0, 0, fmt.Errorf("element not found in DOM (backendNodeId=%d)", backendNodeID)
	}

	const rectFn = `function() {
		var r = this.getBoundingClientRect();
		return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
	}`

	var callResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": rectFn,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &callResult)
	})); err != nil {
		return 0, 0, fmt.Errorf("getBoundingClientRect: %w", err)
	}

	var callRes struct {
		Result struct {
			Value struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResult, &callRes); err != nil {
		return 0, 0, err
	}

	return callRes.Result.Value.X, callRes.Result.Value.Y, nil
}

// ScrollIntoViewAndGetBox scrolls the element into view and returns its box.
func ScrollIntoViewAndGetBox(ctx context.Context, nodeID int64) (map[string]any, error) {
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": nodeID}, nil)
	})); err != nil {
		return nil, fmt.Errorf("scrollIntoViewIfNeeded: %w", err)
	}

	var resolveResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": nodeID,
		}, &resolveResult)
	})); err != nil {
		return nil, fmt.Errorf("DOM.resolveNode: %w", err)
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
		return nil, fmt.Errorf("element not found in DOM (backendNodeId=%d)", nodeID)
	}

	const boxFn = `function() {
		var r = this.getBoundingClientRect();
		return { x: Math.round(r.x), y: Math.round(r.y), width: Math.round(r.width), height: Math.round(r.height) };
	}`

	var callResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": boxFn,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &callResult)
	})); err != nil {
		return nil, fmt.Errorf("getBoundingClientRect: %w", err)
	}

	var callRes struct {
		Result struct {
			Value struct {
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResult, &callRes); err != nil {
		return nil, err
	}

	box := callRes.Result.Value
	return map[string]any{
		"scrolled": true,
		"box": map[string]any{
			"x":      box.X,
			"y":      box.Y,
			"width":  box.Width,
			"height": box.Height,
		},
	}, nil
}
