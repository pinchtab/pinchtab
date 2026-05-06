package cdpops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// Headless Chromium can hold synthetic mouseMoved dispatches for about five
// seconds waiting for renderer/compositor ack. Bound the real CDP move and
// fall back to DOM mouse events so hover/move tests and simple automation
// stay responsive.
const mouseMoveDispatchTimeout = 50 * time.Millisecond

func normalizeMouseButton(button string) string {
	switch strings.ToLower(strings.TrimSpace(button)) {
	case "right":
		return "right"
	case "middle":
		return "middle"
	default:
		return "left"
	}
}

func validatePointerCoordinates(x, y float64) error {
	if x < 0 || y < 0 {
		return fmt.Errorf("x/y coordinates must be >= 0")
	}
	return nil
}

func dispatchMouseEvent(ctx context.Context, payload map[string]any) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", payload, nil)
	}))
}

func dispatchRealMouseMove(ctx context.Context, x, y float64, button input.MouseButton, buttons int64) error {
	stepCtx, cancel := context.WithTimeout(ctx, mouseMoveDispatchTimeout)
	defer cancel()
	return chromedp.Run(stepCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, x, y).
			WithButton(button).
			WithButtons(buttons).
			Do(ctx)
	}))
}

var (
	dispatchRealMouseMoveFunc            = dispatchRealMouseMove
	dispatchSyntheticMouseMoveFunc       = dispatchSyntheticMouseMove
	dispatchSyntheticMouseMoveOnNodeFunc = dispatchSyntheticMouseMoveOnNode
)

func dispatchMouseMove(ctx context.Context, x, y float64, button input.MouseButton, buttons int64) error {
	err := dispatchRealMouseMoveFunc(ctx, x, y, button, buttons)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return dispatchSyntheticMouseMoveFunc(ctx, x, y, button, buttons)
}

func dispatchMouseMoveToNode(ctx context.Context, nodeID int64, x, y float64, button input.MouseButton, buttons int64) error {
	err := dispatchRealMouseMoveFunc(ctx, x, y, button, buttons)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return dispatchSyntheticMouseMoveOnNodeFunc(ctx, nodeID, button, buttons)
}

func dispatchSyntheticMouseMove(ctx context.Context, x, y float64, button input.MouseButton, buttons int64) error {
	buttonCode := mouseButtonCode(button)
	expr := fmt.Sprintf(`(function() {
		var cx = %f, cy = %f, button = %d, buttons = %d;
		var target = document.elementFromPoint(cx, cy) || document.documentElement;
		var init = {
			clientX: cx, clientY: cy, screenX: cx, screenY: cy,
			button: button, buttons: buttons,
			bubbles: true, cancelable: true, view: window
		};
		target.dispatchEvent(new MouseEvent('mouseover', init));
		target.dispatchEvent(new MouseEvent('mouseenter', Object.assign({}, init, { bubbles: false })));
		target.dispatchEvent(new MouseEvent('mousemove', init));
	})()`, x, y, buttonCode, buttons)
	return chromedp.Run(ctx, chromedp.Evaluate(expr, nil))
}

func dispatchSyntheticMouseMoveOnNode(ctx context.Context, nodeID int64, button input.MouseButton, buttons int64) error {
	var resolveResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": nodeID,
		}, &resolveResult)
	})); err != nil {
		return fmt.Errorf("DOM.resolveNode: %w", err)
	}

	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolveResult, &resolved); err != nil {
		return err
	}
	if strings.TrimSpace(resolved.Object.ObjectID) == "" {
		return fmt.Errorf("element not found in DOM (backendNodeId=%d)", nodeID)
	}

	const fn = `function(button, buttons) {
		var r = this.getBoundingClientRect();
		var cx = r.left + r.width / 2;
		var cy = r.top + r.height / 2;
		var init = {
			clientX: cx, clientY: cy, screenX: cx, screenY: cy,
			button: button, buttons: buttons,
			bubbles: true, cancelable: true, view: window
		};
		this.dispatchEvent(new MouseEvent('mouseover', init));
		this.dispatchEvent(new MouseEvent('mouseenter', Object.assign({}, init, { bubbles: false })));
		this.dispatchEvent(new MouseEvent('mousemove', init));
	}`

	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": fn,
			"objectId":            resolved.Object.ObjectID,
			"arguments": []map[string]any{
				{"value": mouseButtonCode(button)},
				{"value": buttons},
			},
		}, nil)
	}))
}

func mouseButtonCode(button input.MouseButton) int {
	switch button {
	case input.Middle:
		return 1
	case input.Right:
		return 2
	default:
		return 0
	}
}

func MouseMoveByCoordinate(ctx context.Context, x, y float64) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	return dispatchMouseMove(ctx, x, y, input.None, 0)
}

func MouseDownByCoordinate(ctx context.Context, x, y float64, button string) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	return dispatchMouseEvent(ctx, map[string]any{
		"type":       "mousePressed",
		"button":     normalizeMouseButton(button),
		"clickCount": 1,
		"x":          x,
		"y":          y,
	})
}

func MouseUpByCoordinate(ctx context.Context, x, y float64, button string) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	return dispatchMouseEvent(ctx, map[string]any{
		"type":       "mouseReleased",
		"button":     normalizeMouseButton(button),
		"clickCount": 1,
		"x":          x,
		"y":          y,
	})
}

func MouseWheelByCoordinate(ctx context.Context, x, y float64, deltaX, deltaY int) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}

	// Synthetic Input.dispatchMouseEvent(mouseWheel) in --headless=new no
	// longer reliably fires `wheel` JS listeners and can stall on the
	// compositor ack chain. Dispatch a real WheelEvent at the point under
	// the cursor so listeners run, then scroll the window if no listener
	// called preventDefault().
	expr := fmt.Sprintf(`(function() {
		var dx = %d, dy = %d, cx = %f, cy = %f;
		var target = document.elementFromPoint(cx, cy) || document.documentElement;
		var ev = new WheelEvent('wheel', {
			deltaX: dx, deltaY: dy,
			clientX: cx, clientY: cy,
			bubbles: true, cancelable: true
		});
		if (target.dispatchEvent(ev)) {
			window.scrollBy(dx, dy);
		}
	})()`, deltaX, deltaY, x, y)
	return chromedp.Run(ctx, chromedp.Evaluate(expr, nil))
}

func ClickByCoordinate(ctx context.Context, x, y float64) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	if err := MouseDownByCoordinate(ctx, x, y, "left"); err != nil {
		return err
	}
	return MouseUpByCoordinate(ctx, x, y, "left")
}

func ClickByNodeID(ctx context.Context, nodeID int64) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved",
				"x":    x, "y": y,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"button":     "left",
				"clickCount": 1,
				"x":          x, "y": y,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"button":     "left",
				"clickCount": 1,
				"x":          x, "y": y,
			}, nil)
		}),
		// CDP mouse events don't trigger default browser navigation on <a>
		// elements. For links, fire a JS-level .click() so the browser
		// follows the href.
		chromedp.ActionFunc(func(ctx context.Context) error {
			return jsClickIfLink(ctx, nodeID)
		}),
	)
}

// jsClickIfLink fires element.click() via JS if the node is an <a> with an
// href. CDP Input.dispatchMouseEvent doesn't trigger the browser's default
// link-navigation behavior, so this ensures anchor clicks actually navigate.
func jsClickIfLink(ctx context.Context, nodeID int64) error {
	// Resolve backend node to a remote object so we can call functions on it.
	var resolved json.RawMessage
	if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
		"backendNodeId": nodeID,
	}, &resolved); err != nil {
		return nil
	}
	var obj struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if json.Unmarshal(resolved, &obj) != nil || obj.Object.ObjectID == "" {
		return nil
	}

	const js = `function() {
		var el = this;
		while (el && el.nodeType === 1) {
			if (el.tagName === 'A' && el.hasAttribute('href')) {
				el.click();
				return;
			}
			el = el.parentElement;
		}
	}`
	return chromedp.FromContext(ctx).Target.Execute(ctx,
		"Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": js,
			"objectId":            obj.Object.ObjectID,
		}, nil)
}

func DoubleClickByCoordinate(ctx context.Context, x, y float64) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"button":     "left",
				"clickCount": 2,
				"x":          x,
				"y":          y,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"button":     "left",
				"clickCount": 2,
				"x":          x,
				"y":          y,
			}, nil)
		}),
	)
}

func DoubleClickByNodeID(ctx context.Context, nodeID int64) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"button":     "left",
				"clickCount": 2,
				"x":          x, "y": y,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"button":     "left",
				"clickCount": 2,
				"x":          x, "y": y,
			}, nil)
		}),
	)
}

// DragByNodeID drags an element by (dx, dy) pixels using mousePressed → mouseMoved → mouseReleased.
func DragByNodeID(ctx context.Context, nodeID int64, dx, dy int) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}

	endX := x + float64(dx)
	endY := y + float64(dy)
	dist := math.Sqrt(float64(dx*dx + dy*dy))
	steps := int(dist / 20)
	if steps < 3 {
		steps = 3
	}
	if steps > 20 {
		steps = 20
	}

	if err := dispatchMouseMove(ctx, x, y, input.None, 0); err != nil {
		return err
	}
	if err := dispatchMouseEvent(ctx, map[string]any{
		"type":       "mousePressed",
		"button":     "left",
		"clickCount": 1,
		"x":          x, "y": y,
	}); err != nil {
		return err
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		mx := x + t*float64(dx)
		my := y + t*float64(dy)
		if err := dispatchMouseMove(ctx, mx, my, input.Left, 1); err != nil {
			return err
		}
	}
	return dispatchMouseEvent(ctx, map[string]any{
		"type":       "mouseReleased",
		"button":     "left",
		"clickCount": 1,
		"x":          endX, "y": endY,
	})
}

func HoverByCoordinate(ctx context.Context, x, y float64) error {
	return MouseMoveByCoordinate(ctx, x, y)
}

func ScrollByCoordinate(ctx context.Context, x, y float64, deltaX, deltaY int) error {
	return MouseWheelByCoordinate(ctx, x, y, deltaX, deltaY)
}

func HoverByNodeID(ctx context.Context, nodeID int64) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}

	return dispatchMouseMoveToNode(ctx, nodeID, x, y, input.None, 0)
}

// jsClickFn is invoked via Runtime.callFunctionOn against the resolved
// backend node. It dispatches synthetic mousedown/mouseup MouseEvents and
// then calls el.click() so the browser runs default actions (link nav,
// checkbox toggle, button form submit) AND fires a 'click' event that
// bubbles to listeners. Walks up to find an anchor ancestor so clicking an
// inner span inside an <a> still follows the href.
const jsClickFn = `function() {
	var el = this;
	var r = el.getBoundingClientRect();
	var cx = r.left + r.width / 2;
	var cy = r.top + r.height / 2;
	var init = {
		clientX: cx, clientY: cy, screenX: cx, screenY: cy,
		button: 0, buttons: 1,
		bubbles: true, cancelable: true, view: window
	};
	if (typeof el.focus === 'function') {
		try { el.focus({preventScroll: true}); } catch (e) {}
	}
	el.dispatchEvent(new MouseEvent('mousedown', init));
	el.dispatchEvent(new MouseEvent('mouseup', Object.assign({}, init, {buttons: 0})));
	var target = el;
	while (target && target.nodeType === 1) {
		if (target.tagName === 'A' && target.hasAttribute('href')) break;
		target = target.parentElement;
	}
	if (!target) target = el;
	if (typeof target.click === 'function') {
		target.click();
	} else {
		target.dispatchEvent(new MouseEvent('click', init));
	}
}`

const jsDoubleClickFn = `function() {
	var el = this;
	var r = el.getBoundingClientRect();
	var cx = r.left + r.width / 2;
	var cy = r.top + r.height / 2;
	var base = {
		clientX: cx, clientY: cy, screenX: cx, screenY: cy,
		button: 0, buttons: 1,
		bubbles: true, cancelable: true, view: window
	};
	if (typeof el.focus === 'function') {
		try { el.focus({preventScroll: true}); } catch (e) {}
	}
	for (var i = 1; i <= 2; i++) {
		el.dispatchEvent(new MouseEvent('mousedown', Object.assign({}, base, {detail: i})));
		el.dispatchEvent(new MouseEvent('mouseup', Object.assign({}, base, {detail: i, buttons: 0})));
		el.dispatchEvent(new MouseEvent('click', Object.assign({}, base, {detail: i})));
	}
	el.dispatchEvent(new MouseEvent('dblclick', Object.assign({}, base, {detail: 2})));
}`

func resolveBackendNodeObjectID(ctx context.Context, backendNodeID int64) (string, error) {
	var resolved json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &resolved)
	})); err != nil {
		return "", fmt.Errorf("DOM.resolveNode: %w", err)
	}
	var out struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolved, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.Object.ObjectID) == "" {
		return "", fmt.Errorf("element not found in DOM (backendNodeId=%d)", backendNodeID)
	}
	return out.Object.ObjectID, nil
}

// JSClickByBackendNode performs a click via Runtime.callFunctionOn rather
// than synthesized CDP Input.dispatchMouseEvent. Headless Chromium's CDP
// path can stall ~5s waiting on the renderer ack chain for press/release;
// the JS path runs the same handler chain (mousedown, mouseup, click) plus
// the browser's default action (el.click()) without the ack tax.
func JSClickByBackendNode(ctx context.Context, backendNodeID int64) error {
	if _, _, err := PointerPointForNode(ctx, backendNodeID, true); err != nil {
		return err
	}
	objectID, err := resolveBackendNodeObjectID(ctx, backendNodeID)
	if err != nil {
		return err
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": jsClickFn,
			"objectId":            objectID,
		}, nil)
	}))
}

// JSDoubleClickByBackendNode is the dblclick counterpart of JSClickByBackendNode.
func JSDoubleClickByBackendNode(ctx context.Context, backendNodeID int64) error {
	if _, _, err := PointerPointForNode(ctx, backendNodeID, true); err != nil {
		return err
	}
	objectID, err := resolveBackendNodeObjectID(ctx, backendNodeID)
	if err != nil {
		return err
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": jsDoubleClickFn,
			"objectId":            objectID,
		}, nil)
	}))
}
