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

// mouseEventAction returns a chromedp.Action that dispatches one
// Input.dispatchMouseEvent with the given payload. Node-targeted click
// sequences assemble several of these into a single chromedp.Run batch so the
// trusted-CDP move/press/release steps stop being hand-rolled per call site.
func mouseEventAction(payload map[string]any) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", payload, nil)
	})
}

// mousePressReleaseActions returns the left-button press/release pair at (x,y)
// for the given clickCount (1 = single click, 2 = double click), the part
// shared verbatim by ClickByNodeID and DoubleClickByNodeID.
func mousePressReleaseActions(x, y float64, clickCount int) []chromedp.Action {
	return []chromedp.Action{
		mouseEventAction(map[string]any{
			"type":       "mousePressed",
			"button":     "left",
			"clickCount": clickCount,
			"x":          x, "y": y,
		}),
		mouseEventAction(map[string]any{
			"type":       "mouseReleased",
			"button":     "left",
			"clickCount": clickCount,
			"x":          x, "y": y,
		}),
	}
}

func dispatchMouseEvent(ctx context.Context, payload map[string]any) error {
	return chromedp.Run(ctx, mouseEventAction(payload))
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
	objectID, err := resolveBackendNodeObjectID(ctx, nodeID)
	if err != nil {
		return err
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
			"objectId":            objectID,
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

// Modifiers is the CDP key-modifier bitmask (Alt=1, Ctrl=2, Meta=4, Shift=8)
// held during a pointer dispatch. Input.dispatchMouseEvent accepts this value
// verbatim under "modifiers", enabling gestures like Shift+click and
// Cmd/Ctrl+click. The bits below mirror that encoding for the JS WheelEvent
// path, which needs booleans instead.
const (
	modAlt   = 1
	modCtrl  = 2
	modMeta  = 4
	modShift = 8
)

func MouseDownByCoordinate(ctx context.Context, x, y float64, button string, modifiers int) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	return dispatchMouseEvent(ctx, map[string]any{
		"type":       "mousePressed",
		"button":     normalizeMouseButton(button),
		"clickCount": 1,
		"modifiers":  modifiers,
		"x":          x,
		"y":          y,
	})
}

func MouseUpByCoordinate(ctx context.Context, x, y float64, button string, modifiers int) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	return dispatchMouseEvent(ctx, map[string]any{
		"type":       "mouseReleased",
		"button":     normalizeMouseButton(button),
		"clickCount": 1,
		"modifiers":  modifiers,
		"x":          x,
		"y":          y,
	})
}

func MouseWheelByCoordinate(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}

	// Synthetic Input.dispatchMouseEvent(mouseWheel) in --headless=new no
	// longer reliably fires `wheel` JS listeners and can stall on the
	// compositor ack chain. Dispatch a real WheelEvent at the point under
	// the cursor so listeners run, then scroll the window if no listener
	// called preventDefault(). Held modifiers (Shift for horizontal scroll,
	// Ctrl for zoom intent) are reflected on the event init.
	expr := fmt.Sprintf(`(function() {
		var dx = %d, dy = %d, cx = %f, cy = %f;
		var target = document.elementFromPoint(cx, cy) || document.documentElement;
		var ev = new WheelEvent('wheel', {
			deltaX: dx, deltaY: dy,
			clientX: cx, clientY: cy,
			altKey: %t, ctrlKey: %t, metaKey: %t, shiftKey: %t,
			bubbles: true, cancelable: true
		});
		if (target.dispatchEvent(ev)) {
			window.scrollBy(dx, dy);
		}
	})()`, deltaX, deltaY, x, y,
		modifiers&modAlt != 0, modifiers&modCtrl != 0,
		modifiers&modMeta != 0, modifiers&modShift != 0)
	return chromedp.Run(ctx, chromedp.Evaluate(expr, nil))
}

func ClickByCoordinate(ctx context.Context, x, y float64, modifiers int) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	if err := MouseDownByCoordinate(ctx, x, y, "left", modifiers); err != nil {
		return err
	}
	return MouseUpByCoordinate(ctx, x, y, "left", modifiers)
}

func ClickByNodeID(ctx context.Context, nodeID int64) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}

	actions := []chromedp.Action{
		mouseEventAction(map[string]any{"type": "mouseMoved", "x": x, "y": y}),
	}
	actions = append(actions, mousePressReleaseActions(x, y, 1)...)
	// CDP mouse events don't trigger default browser navigation on <a>
	// elements. For links, fire a JS-level .click() so the browser
	// follows the href.
	actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
		return jsClickIfLink(ctx, nodeID)
	}))
	return chromedp.Run(ctx, actions...)
}

func jsClickIfLink(ctx context.Context, nodeID int64) error {
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

	actions := []chromedp.Action{
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
	}
	actions = append(actions, mousePressReleaseActions(x, y, 2)...)
	return chromedp.Run(ctx, actions...)
}

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

func ScrollByCoordinate(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) error {
	return MouseWheelByCoordinate(ctx, x, y, deltaX, deltaY, modifiers)
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

const jsDispatchClickFn = `function() {
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
	el.dispatchEvent(new PointerEvent('pointerdown', Object.assign({}, init, {pointerId: 1, pointerType: 'mouse', isPrimary: true})));
	el.dispatchEvent(new MouseEvent('mousedown', init));
	el.dispatchEvent(new PointerEvent('pointerup', Object.assign({}, init, {pointerId: 1, pointerType: 'mouse', isPrimary: true, buttons: 0})));
	el.dispatchEvent(new MouseEvent('mouseup', Object.assign({}, init, {buttons: 0})));
	el.dispatchEvent(new MouseEvent('click', Object.assign({}, init, {buttons: 0, detail: 1})));
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

// callFunctionOnBackendNode resolves the backend node to a Runtime object and
// invokes fn against it via Runtime.callFunctionOn. The JS click variants share
// this resolve-then-invoke wrapper and differ only in the fn they pass, so a
// future fix to the JS fallback path lands here once.
func callFunctionOnBackendNode(ctx context.Context, backendNodeID int64, fn string) error {
	objectID, err := resolveBackendNodeObjectID(ctx, backendNodeID)
	if err != nil {
		return err
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": fn,
			"objectId":            objectID,
		}, nil)
	}))
}

// JSClickByBackendNode performs a click via Runtime.callFunctionOn rather
// than synthesized CDP Input.dispatchMouseEvent. Headless Chromium's CDP
// path can stall ~5s waiting on the renderer ack chain for press/release;
// the JS path runs the same handler chain (mousedown, mouseup, click) plus
// the browser's default action (el.click()) without the ack tax.
func JSClickByBackendNode(ctx context.Context, backendNodeID int64) error {
	return callFunctionOnBackendNode(ctx, backendNodeID, jsClickFn)
}

// JSDispatchClickByBackendNode dispatches synthetic pointer/mouse events on the
// target element without invoking element.click(). This bypasses occlusion while
// staying closer to the browser event sequence than a DOM click.
func JSDispatchClickByBackendNode(ctx context.Context, backendNodeID int64) error {
	return callFunctionOnBackendNode(ctx, backendNodeID, jsDispatchClickFn)
}

func JSDoubleClickByBackendNode(ctx context.Context, backendNodeID int64) error {
	if _, _, err := PointerPointForNode(ctx, backendNodeID, true); err != nil {
		return err
	}
	return callFunctionOnBackendNode(ctx, backendNodeID, jsDoubleClickFn)
}
