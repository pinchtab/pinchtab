package cdpops

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

// Headless Chromium can hold synthetic mouseMoved dispatches for about five
// seconds waiting for renderer/compositor ack. Bound the real CDP move and
// fall back to DOM mouse events so hover/move tests and simple automation
// stay responsive.
const mouseMoveDispatchTimeout = 50 * time.Millisecond

// DefaultMouseButton is what an unspecified button means. An unspecified button is a
// default; an unrecognised NAME is a caller error, and the two used to share this answer.
const DefaultMouseButton = "left"

// mouseButton is one row of the button vocabulary: the name a caller writes, the CDP enum,
// the "buttons" bitmask that says it is held, and MouseEvent.button for the synthetic DOM
// fallback. Every fact about a button lives on its row, so a name that validates dispatches
// correctly BY CONSTRUCTION — the vocabulary used to have one owner for which names exist
// and hand-written switches for what each name means, so a fourth entry would have passed
// validation and then been dispatched as left, with a drag pressing one button and moving
// under another.
type mouseButton struct {
	name   string
	enum   input.MouseButton
	held   int64
	jsCode int
}

var mouseButtonTable = []mouseButton{
	{name: DefaultMouseButton, enum: input.Left, held: 1, jsCode: 0},
	{name: "right", enum: input.Right, held: 2, jsCode: 2},
	{name: "middle", enum: input.Middle, held: 4, jsCode: 1},
}

// MouseButtons is the button vocabulary, derived from the table rather than maintained
// beside it. The normalizer, the refusal message and the CLI flag help all read this, so a
// fourth button cannot be accepted in one place and refused in another.
func MouseButtons() []string {
	names := make([]string, 0, len(mouseButtonTable))
	for _, b := range mouseButtonTable {
		names = append(names, b.name)
	}
	return names
}

func mouseButtonNamed(name string) (mouseButton, bool) {
	for _, b := range mouseButtonTable {
		if b.name == name {
			return b, true
		}
	}
	return mouseButton{}, false
}

func mouseButtonForEnum(enum input.MouseButton) (mouseButton, bool) {
	for _, b := range mouseButtonTable {
		if b.enum == enum {
			return b, true
		}
	}
	return mouseButton{}, false
}

// ValidateMouseButton refuses a name that is not a button, naming the ones that are. Empty
// and whitespace-only stay valid: that is the default, not forgiveness. The DOM's own
// vocabulary is refused rather than mapped — "primary" happens to mean left, so mapping it
// would look right while "secondary" silently became left, and PinchTab documents neither.
func ValidateMouseButton(button string) error {
	normalized := strings.ToLower(strings.TrimSpace(button))
	if normalized == "" {
		return nil
	}
	if _, ok := mouseButtonNamed(normalized); ok {
		return nil
	}
	return fmt.Errorf("button %q is not a mouse button; use one of %s", button, strings.Join(MouseButtons(), ", "))
}

// normalizeMouseButton keeps its permissive default and its plain-string return: by the time
// a value reaches it, ValidateMouseButton has already refused every name that is not here.
func normalizeMouseButton(button string) string {
	normalized := strings.ToLower(strings.TrimSpace(button))
	if _, ok := mouseButtonNamed(normalized); ok {
		return normalized
	}
	return DefaultMouseButton
}

func validatePointerCoordinates(x, y float64) error {
	if x < 0 || y < 0 {
		return fmt.Errorf("x/y coordinates must be >= 0")
	}
	return nil
}

// Chromium never delivers trusted Input.* events to an unfocused tab, which every
// Target.createTarget(focus=false) tab is, and a screenshot switches emulation back off.
type inputFocusKey struct{}

type inputFocus struct {
	mu      sync.Mutex
	enabled bool
}

func WithTrustedInput(ctx context.Context) context.Context {
	if _, ok := ctx.Value(inputFocusKey{}).(*inputFocus); ok {
		return ctx
	}
	return context.WithValue(ctx, inputFocusKey{}, &inputFocus{})
}

func enableInputFocus(ctx context.Context) error {
	focus, ok := ctx.Value(inputFocusKey{}).(*inputFocus)
	if !ok {
		return emulation.SetFocusEmulationEnabled(true).Do(ctx)
	}
	focus.mu.Lock()
	defer focus.mu.Unlock()
	if focus.enabled {
		return nil
	}
	if err := emulation.SetFocusEmulationEnabled(true).Do(ctx); err != nil {
		return err
	}
	focus.enabled = true
	return nil
}

var enableInputFocusAction = chromedp.ActionFunc(enableInputFocus)

func trustedInput(method string, params map[string]any) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if err := enableInputFocus(ctx); err != nil {
			return err
		}
		return cdp.Execute(ctx, method, params, nil)
	})
}

func TrustedInputAction(action chromedp.Action) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if err := enableInputFocus(ctx); err != nil {
			return err
		}
		return action.Do(ctx)
	})
}

func MouseEventAction(payload map[string]any) chromedp.Action {
	return trustedInput("Input.dispatchMouseEvent", payload)
}

func KeyEventAction(params map[string]any) chromedp.Action {
	return trustedInput("Input.dispatchKeyEvent", params)
}

func InsertTextAction(text string) chromedp.Action {
	return trustedInput("Input.insertText", map[string]any{"text": text})
}

func EnableTrustedInput(ctx context.Context) (context.Context, error) {
	ctx = WithTrustedInput(ctx)
	return ctx, chromedp.Run(ctx, enableInputFocusAction)
}

// mousePressReleaseActions returns the left-button press/release pair at (x,y)
// for the given clickCount (1 = single click, 2 = double click), the part
// shared verbatim by ClickByNodeID and DoubleClickByNodeID.
func mousePressReleaseActions(x, y float64, clickCount int) []chromedp.Action {
	return []chromedp.Action{
		MouseEventAction(map[string]any{
			"type":       "mousePressed",
			"button":     DefaultMouseButton,
			"clickCount": clickCount,
			"x":          x, "y": y,
		}),
		MouseEventAction(map[string]any{
			"type":       "mouseReleased",
			"button":     DefaultMouseButton,
			"clickCount": clickCount,
			"x":          x, "y": y,
		}),
	}
}

func dispatchMouseEvent(ctx context.Context, payload map[string]any) error {
	return chromedp.Run(ctx, MouseEventAction(payload))
}

func dispatchRealMouseMove(ctx context.Context, x, y float64, button input.MouseButton, buttons int64) error {
	ctx, err := EnableTrustedInput(ctx)
	if err != nil {
		return err
	}
	stepCtx, cancel := context.WithTimeout(ctx, mouseMoveDispatchTimeout)
	defer cancel()
	return chromedp.Run(stepCtx, MouseEventAction(map[string]any{
		"type":    "mouseMoved",
		"x":       x,
		"y":       y,
		"button":  button,
		"buttons": buttons,
	}))
}

var (
	dispatchRealMouseMoveFunc            = dispatchRealMouseMove
	dispatchSyntheticMouseMoveFunc       = dispatchSyntheticMouseMove
	dispatchSyntheticMouseMoveOnNodeFunc = dispatchSyntheticMouseMoveOnNode
	// dispatchMouseEventFunc joins them so a drag's press payload is observable without a
	// browser: the button a drag PRESSES and the button its moves HOLD have to be compared
	// to each other, and only the call site can show they agree.
	dispatchMouseEventFunc = dispatchMouseEvent
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

	return CallFunctionOnNode(ctx, nodeID, fn, []map[string]any{
		{"value": mouseButtonCode(button)},
		{"value": buttons},
	}, nil)
}

// mouseButtonCode is MouseEvent.button for the synthetic DOM fallback, read off the table.
// Unlike heldButton, its no-row case has a legitimate caller: a plain move carries
// input.None, which the DOM encodes as button 0 beside buttons 0. So the zero here is the
// answer for "nothing pressed", not a stand-in for left.
func mouseButtonCode(button input.MouseButton) int {
	if row, ok := mouseButtonForEnum(button); ok {
		return row.jsCode
	}
	return noButtonJSCode
}

// noButtonJSCode is MouseEvent.button when no button is pressed. It equals left's code
// because the DOM says so, which is why the two cases must be told apart by their reason
// rather than by their value.
const noButtonJSCode = 0

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
	return dispatchMouseEventFunc(ctx, map[string]any{
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
	return dispatchMouseEventFunc(ctx, map[string]any{
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
		MouseEventAction(map[string]any{"type": "mouseMoved", "x": x, "y": y}),
	}
	actions = append(actions, mousePressReleaseActions(x, y, 1)...)
	// CDP mouse events don't trigger default browser navigation on <a>
	// elements. For links, fire a JS-level .click() so the browser
	// follows the href.
	actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
		return jsClickIfLink(ctx, nodeID)
	}))
	return chromedp.Run(WithTrustedInput(ctx), actions...)
}

func jsClickIfLink(ctx context.Context, nodeID int64) error {
	objectID, err := ResolveNodeObjectID(ctx, nodeID)
	if err != nil {
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
	err = cdptk.CallFunctionOn(ctx, objectID, js, nil, nil)
	if linkContextGone(err) {
		// The trusted press already followed the link and the old document's
		// context is gone: the click landed, so there is nothing left to do.
		return nil
	}
	return err
}

func linkContextGone(err error) bool {
	var protocolErr *cdproto.Error
	return errors.As(err, &protocolErr) && protocolErr.Code == -32000 &&
		(protocolErr.Message == "Cannot find context with specified id" ||
			protocolErr.Message == "Execution context was destroyed.")
}

func DoubleClickByCoordinate(ctx context.Context, x, y float64) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}

	return chromedp.Run(WithTrustedInput(ctx), mousePressReleaseActions(x, y, 2)...)
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
	return chromedp.Run(WithTrustedInput(ctx), actions...)
}

func DragByNodeID(ctx context.Context, nodeID int64, dx, dy int, button string) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}
	return DragBetweenPoints(ctx, x, y, x+float64(dx), y+float64(dy), button)
}

// heldButton resolves a name to its whole row, so the press payload and the held moves of
// one drag cannot describe different buttons. It resolves empty-means-default itself rather
// than borrowing normalizeMouseButton, whose permissive contract answers left for an
// unrecognised name too — routing through it would have moved the silent fallback rather
// than removing it. Unspecified is a default; unrecognised is a caller error.
func heldButton(button string) (mouseButton, error) {
	name := strings.ToLower(strings.TrimSpace(button))
	if name == "" {
		name = DefaultMouseButton
	}
	row, ok := mouseButtonNamed(name)
	if !ok {
		return mouseButton{}, fmt.Errorf("button %q is not a mouse button; use one of %s", button, strings.Join(MouseButtons(), ", "))
	}
	return row, nil
}

// Chrome enters the HTML5 drag pipeline only on movement that reports the pressed button, so
// the held mask on these moves is what fires dragstart, not how many of them there are.
func DragBetweenPoints(ctx context.Context, x, y, endX, endY float64, button string) error {
	ctx = WithTrustedInput(ctx)
	dx := endX - x
	dy := endY - y
	dist := math.Sqrt(dx*dx + dy*dy)
	steps := min(max(int(dist/20), 3), 20)
	held, err := heldButton(button)
	if err != nil {
		return err
	}

	if err := dispatchMouseMove(ctx, x, y, input.None, 0); err != nil {
		return err
	}
	if err := dispatchMouseEventFunc(ctx, map[string]any{
		"type":       "mousePressed",
		"button":     held.name,
		"clickCount": 1,
		"x":          x, "y": y,
	}); err != nil {
		return err
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		if err := dispatchMouseMove(ctx, x+t*dx, y+t*dy, held.enum, held.held); err != nil {
			return err
		}
	}
	return dispatchMouseEventFunc(ctx, map[string]any{
		"type":       "mouseReleased",
		"button":     held.name,
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

// JSClickByBackendNode performs a click via Runtime.callFunctionOn rather
// than synthesized CDP Input.dispatchMouseEvent. Headless Chromium's CDP
// path can stall ~5s waiting on the renderer ack chain for press/release;
// the JS path runs the same handler chain (mousedown, mouseup, click) plus
// the browser's default action (el.click()) without the ack tax.
func JSClickByBackendNode(ctx context.Context, backendNodeID int64) error {
	return CallFunctionOnNode(ctx, backendNodeID, jsClickFn, nil, nil)
}

// JSDispatchClickByBackendNode dispatches synthetic pointer/mouse events on the
// target element without invoking element.click(). This bypasses occlusion while
// staying closer to the browser event sequence than a DOM click.
func JSDispatchClickByBackendNode(ctx context.Context, backendNodeID int64) error {
	return CallFunctionOnNode(ctx, backendNodeID, jsDispatchClickFn, nil, nil)
}

func JSDoubleClickByBackendNode(ctx context.Context, backendNodeID int64) error {
	if _, _, err := PointerPointForNode(ctx, backendNodeID, true); err != nil {
		return err
	}
	return CallFunctionOnNode(ctx, backendNodeID, jsDoubleClickFn, nil, nil)
}
