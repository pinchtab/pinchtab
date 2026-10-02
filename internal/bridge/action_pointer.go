package bridge

import (
	"context"
	"errors"
	"fmt"
	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
	"math"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/readiness"
)

type pointerOps struct {
	scrollByCoordinate      func(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) (bridgecdpops.ScrollPosition, error)
	mouseMoveByCoordinate   func(ctx context.Context, x, y float64) error
	mouseDownByCoordinate   func(ctx context.Context, x, y float64, button string, modifiers int) error
	mouseUpByCoordinate     func(ctx context.Context, x, y float64, button string, modifiers int) error
	clickByCoordinate       func(ctx context.Context, x, y float64, modifiers int) error
	clickElement            func(ctx context.Context, backendNodeID cdp.BackendNodeID) error
	humanClickCoordinate    func(ctx context.Context, x, y float64) error
	hoverElement            func(ctx context.Context, backendNodeID cdp.BackendNodeID) error
	hoverCoordinate         func(ctx context.Context, x, y float64) error
	clickByNodeID           func(ctx context.Context, nodeID int64) error
	jsClickByBackendNode    func(ctx context.Context, nodeID int64) error
	dispatchClickByBackend  func(ctx context.Context, nodeID int64) error
	clickFloatingFlyoutItem func(ctx context.Context, nodeID int64) (bool, error)
	doubleClickByNodeID     func(ctx context.Context, nodeID int64) error
	jsDoubleClickByBackend  func(ctx context.Context, nodeID int64) error
	doubleClickByCoordinate func(ctx context.Context, x, y float64) error
	waitNav                 func(ctx context.Context, delay time.Duration)
	scrollViewportCenter    func(ctx context.Context) (float64, float64, error)
}

func defaultPointerOps() *pointerOps {
	return &pointerOps{
		scrollByCoordinate:      bridgecdpops.ScrollByCoordinate,
		mouseMoveByCoordinate:   bridgecdpops.MouseMoveByCoordinate,
		mouseDownByCoordinate:   bridgecdpops.MouseDownByCoordinate,
		mouseUpByCoordinate:     bridgecdpops.MouseUpByCoordinate,
		clickByCoordinate:       bridgecdpops.ClickByCoordinate,
		clickElement:            ClickElement,
		humanClickCoordinate:    Click,
		hoverElement:            HoverElement,
		hoverCoordinate:         Hover,
		clickByNodeID:           bridgecdpops.ClickByNodeID,
		jsClickByBackendNode:    bridgecdpops.JSClickByBackendNode,
		dispatchClickByBackend:  bridgecdpops.JSDispatchClickByBackendNode,
		clickFloatingFlyoutItem: clickFloatingFlyoutItem,
		doubleClickByNodeID:     bridgecdpops.DoubleClickByNodeID,
		jsDoubleClickByBackend:  bridgecdpops.JSDoubleClickByBackendNode,
		doubleClickByCoordinate: bridgecdpops.DoubleClickByCoordinate,
		waitNav:                 sleepForNav,
		scrollViewportCenter:    viewportCenter,
	}
}

func (b *Bridge) pointer() *pointerOps {
	if b.pointerOps != nil {
		return b.pointerOps
	}
	return defaultPointerOps()
}

func sleepForNav(ctx context.Context, delay time.Duration) {
	_ = chromedp.Run(ctx, chromedp.Sleep(delay))
}

// In headless Chromium the trusted Input.dispatchMouseEvent click often never
// completes, so its step stays short: a longer wait only delays every click
// before the fallback lands. The JS fallback gets a quarter of the action time left,
// clamped: the floor survives a CPU-starved renderer, and the share and ceiling
// keep a dialog-blocked fallback returning well before the action timeout.
const (
	trustedClickStepTimeout = 250 * time.Millisecond
	minClickStepBudget      = 250 * time.Millisecond
	maxClickStepBudget      = 2500 * time.Millisecond
	clickStepBudgetShare    = 4
)

func clickStepBudget(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return maxClickStepBudget
	}
	return min(max(time.Until(deadline)/clickStepBudgetShare, minClickStepBudget), maxClickStepBudget)
}

func (p *pointerOps) clickByNodeIDWithJSFallback(ctx context.Context, nodeID int64) error {
	return withTrustedFallback(ctx, nodeID, p.clickByNodeID, p.jsClickByBackendNode)
}

func withTrustedFallback(ctx context.Context, nodeID int64, trusted, js func(context.Context, int64) error) error {
	trustedCtx, cancel := context.WithTimeout(ctx, trustedClickStepTimeout)
	err := trusted(trustedCtx, nodeID)
	cancel()
	if err == nil {
		return nil
	}
	// The parent may have been cancelled (e.g. by the dialog-detection
	// polling loop) while the trusted click was running.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	jsCtx, jsCancel := context.WithTimeout(ctx, clickStepBudget(ctx))
	defer jsCancel()
	return js(jsCtx, nodeID)
}

func (p *pointerOps) clickByNodeIDWithMode(ctx context.Context, nodeID int64, mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "default":
		handled, err := p.clickFloatingFlyoutItem(ctx, nodeID)
		if err != nil || handled {
			return err
		}
		return p.clickByNodeIDWithJSFallback(ctx, nodeID)
	case "dom":
		return p.jsClickByBackendNode(ctx, nodeID)
	case "dispatch":
		return p.dispatchClickByBackend(ctx, nodeID)
	default:
		return fmt.Errorf("invalid click mode: %s", mode)
	}
}

// clickFloatingFlyoutItem avoids DOM.scrollIntoViewIfNeeded for portal-backed
// menu options: scrolling their floating owner can rerender and detach the node
// before the pointer dispatch. A DOM click is intentional for this narrow role
// and positioning combination; all other nodes retain the trusted pointer path.
func clickFloatingFlyoutItem(ctx context.Context, nodeID int64) (handled bool, err error) {
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		node, err := dom.DescribeNode().WithBackendNodeID(cdp.BackendNodeID(nodeID)).Do(ctx)
		if err != nil {
			return err
		}
		role := strings.ToLower(strings.TrimSpace(node.AttributeValue("role")))
		if role != "menuitem" && role != "option" {
			return nil
		}

		object, err := dom.ResolveNode().WithBackendNodeID(cdp.BackendNodeID(nodeID)).Do(ctx)
		if err != nil {
			return err
		}
		result, exception, err := runtime.CallFunctionOn(`function() {
			if (!this.isConnected) return false;
			for (var el = this; el && el.nodeType === 1; el = el.parentElement) {
				var position = getComputedStyle(el).position;
				if (position === 'fixed' || position === 'absolute') {
					try { this.focus({preventScroll: true}); } catch (e) {}
					this.click();
					return true;
				}
			}
			return false;
		}`).WithObjectID(object.ObjectID).WithReturnByValue(true).Do(ctx)
		if err != nil {
			return err
		}
		if exception != nil {
			return exception
		}
		handled = string(result.Value) == "true"
		return nil
	}))
	return handled, err
}

func (p *pointerOps) doubleClickByNodeIDWithJSFallback(ctx context.Context, nodeID int64) error {
	return withTrustedFallback(ctx, nodeID, p.doubleClickByNodeID, p.jsDoubleClickByBackend)
}

// effectiveHumanize resolves whether an action should use the humanized
// (bezier + per-event jitter + pre-press sleeps) input path. Precedence:
//
//  1. Per-request override: ActionRequest.Humanize (if non-nil)
//  2. Per-instance default: bridge Config.Humanize
//  3. Built-in default: false
//
// The action kind is intentionally NOT consulted. The public API uses
// click/type with humanize=true instead of separate humanized action names.
func (b *Bridge) effectiveHumanize(req ActionRequest) bool {
	if req.Humanize != nil {
		return *req.Humanize
	}
	if b != nil && b.Config != nil {
		return b.Config.Humanize
	}
	return false
}

const (
	dialogAutoHandlePollInterval = 10 * time.Millisecond
	dialogAutoHandleSettleDelay  = 40 * time.Millisecond
	dialogAutoHandleTimeout      = 750 * time.Millisecond
)

type pointerState struct {
	X     float64
	Y     float64
	Known bool
}

func viewportCenter(ctx context.Context) (float64, float64, error) {
	l, err := cdptk.Layout(ctx)
	if err != nil {
		return 0, 0, err
	}
	return max(1, math.Floor(l.Width/2)), max(1, math.Floor(l.Height/2)), nil
}

// settle finalizes the popup auto-switch from runClick's deferred call: on
// success it adopts/focuses any opened tab and augments result; on error it
// cancels. A dialog the click opened blocks page JS, so that case cancels
// without the window.open restore script, which would hang. nil-safe.
func (s *autoSwitchSession) settle(ctx context.Context, result map[string]any, openedDialog *bridgetabs.DialogState, err error) map[string]any {
	if s == nil {
		return result
	}
	switch {
	case openedDialog != nil:
		s.cancelWithoutRestore()
	case err == nil:
		return s.finish(ctx, result)
	default:
		s.cancel(ctx)
	}
	return result
}

const OpenedDialogResultKey = "dialog"

// OpenedDialog returns the JavaScript dialog a click opened, nil when the
// click result names none.
func OpenedDialog(result map[string]any) *bridgetabs.DialogState {
	dialog, _ := result[OpenedDialogResultKey].(*bridgetabs.DialogState)
	return dialog
}

func (b *Bridge) armDialogAutoHandler(req ActionRequest) (*bridgetabs.DialogManager, bool) {
	dm := b.GetDialogManager()
	if req.DialogAction != "" && req.TabID != "" && dm != nil {
		dm.ArmAutoHandler(req.TabID, req.DialogAction, req.DialogText)
		return dm, true
	}
	return dm, false
}

func dialogBlocking(dm *bridgetabs.DialogManager, tabID string) error {
	pending := dm.GetPending(tabID)
	if pending == nil {
		return nil
	}
	return &ErrDialogBlocking{
		DialogType:    pending.Type,
		DialogMessage: pending.Message,
	}
}

// scaleScreencastCoords rescales req.X/Y from the screencast frame pixel space
// (req.FrameW/FrameH) into the live CSS viewport. Dashboard input maps a click on
// the frame to frame-pixel coordinates; on HiDPI the frame is larger than the CSS
// viewport (e.g. 2x), so without this the click would land at the wrong position
// and miss its target. No-op when FrameW/FrameH are unset (coords already CSS px).
func scaleScreencastCoords(ctx context.Context, req *ActionRequest) {
	if !req.HasXY || req.FrameW <= 0 || req.FrameH <= 0 {
		return
	}
	vw, vh := fetchViewportSize(ctx)
	if vw <= 0 || vh <= 0 {
		return
	}
	req.X = req.X * vw / req.FrameW
	req.Y = req.Y * vh / req.FrameH
}

func (b *Bridge) actionClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	scaleScreencastCoords(ctx, &req)
	if !req.Submit && b.effectiveHumanize(req) {
		return b.actionHumanizedClick(ctx, req)
	}
	gesture := func(p *pointerOps) pointerGesture { return p.clickGesture(req) }
	return b.runClick(ctx, req, gesture, map[string]any{"clicked": true})
}

// runClick dispatches one click gesture with popup auto-switch armed: a tab the
// click opens is adopted and named on the result, and a JavaScript dialog the
// click opens is named on the result instead of failing a click that landed.
func (b *Bridge) runClick(ctx context.Context, req ActionRequest, gesture func(*pointerOps) pointerGesture, clicked map[string]any) (result map[string]any, err error) {
	auto := b.beginAutoSwitch(req)
	var openedDialog *bridgetabs.DialogState
	defer func() { result = auto.settle(ctx, result, openedDialog, err) }()

	openedDialog, err = b.runWatchingDialogs(ctx, req, func(clickCtx context.Context) error {
		p := b.pointer()
		return p.clickTarget(clickCtx, req, auto, gesture(p))
	})
	if err != nil {
		return nil, err
	}
	if openedDialog != nil {
		clicked[OpenedDialogResultKey] = openedDialog
		return clicked, nil
	}
	b.waitNavIfRequested(ctx, req)
	return clicked, nil
}

type pointerGesture struct {
	onNode       func(ctx context.Context, nodeID int64) error
	onPoint      func(ctx context.Context, x, y float64) error
	onSubmitNode func(ctx context.Context, nodeID int64) error
}

func (p *pointerOps) clickGesture(req ActionRequest) pointerGesture {
	return pointerGesture{
		onNode: func(ctx context.Context, nodeID int64) error {
			return p.clickByNodeIDWithMode(ctx, nodeID, req.Mode)
		},
		onPoint: func(ctx context.Context, x, y float64) error {
			return p.clickByCoordinate(ctx, x, y, req.Modifiers)
		},
		onSubmitNode: p.jsClickByBackendNode,
	}
}

func (p *pointerOps) doubleClickGesture() pointerGesture {
	return pointerGesture{
		onNode:  p.doubleClickByNodeIDWithJSFallback,
		onPoint: p.doubleClickByCoordinate,
	}
}

func (p *pointerOps) clickTarget(ctx context.Context, req ActionRequest, auto *autoSwitchSession, gesture pointerGesture) error {
	if auto != nil {
		auto.prepareWindowOpenCapture(ctx)
	}
	prepare := func(nodeID int64) {
		if auto != nil {
			auto.prepareNode(ctx, nodeID)
		}
	}
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return err
	}
	switch {
	case req.Submit && gesture.onSubmitNode != nil:
		if nodeID <= 0 {
			return fmt.Errorf("click submit requires a selector, ref, or nodeId")
		}
		prepare(nodeID)
		// One DOM click only. In particular, do not use the trusted-click
		// timeout fallback: its second dispatch can double-submit a slow SPA.
		return gesture.onSubmitNode(ctx, nodeID)
	case nodeID > 0:
		prepare(nodeID)
		return gesture.onNode(ctx, nodeID)
	case req.HasXY:
		return gesture.onPoint(ctx, req.X, req.Y)
	default:
		return NewInvalidActionRequestError("need selector, ref, nodeId, or x/y coordinates")
	}
}

// runWatchingDialogs runs click with a one-shot dialog auto-handler armed when
// the request names a DialogAction, so the click does not hang waiting for the
// dialog to be handled from a separate request. Otherwise a dialog already open
// refuses the click undispatched with ErrDialogBlocking, and a dialog that
// opens once the click is dispatched is returned as the click's outcome rather
// than hanging the renderer for the full action timeout.
func (b *Bridge) runWatchingDialogs(ctx context.Context, req ActionRequest, click func(context.Context) error) (*bridgetabs.DialogState, error) {
	dm, armedDialog := b.armDialogAutoHandler(req)
	if armedDialog || req.TabID == "" || dm == nil {
		if err := click(ctx); err != nil {
			return nil, err
		}
		if armedDialog {
			waitForArmedDialogSettle(ctx, dm, req.TabID)
		}
		return nil, nil
	}
	if err := dialogBlocking(dm, req.TabID); err != nil {
		return nil, err
	}

	clickCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resultCh := make(chan error, 1)
	go func() { resultCh <- click(clickCtx) }()

	ticker := time.NewTicker(dialogAutoHandlePollInterval)
	defer ticker.Stop()
	for {
		select {
		case err := <-resultCh:
			if err != nil {
				return nil, err
			}
			return dm.GetPending(req.TabID), nil
		case <-ticker.C:
			if opened := dm.GetPending(req.TabID); opened != nil {
				return opened, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (b *Bridge) waitNavIfRequested(ctx context.Context, req ActionRequest) {
	if req.WaitNav {
		b.pointer().waitNav(ctx, b.Config.WaitNavDelay)
	}
}

func waitForArmedDialogSettle(ctx context.Context, dm *bridgetabs.DialogManager, tabID string) {
	if dm == nil || strings.TrimSpace(tabID) == "" {
		return
	}

	if _, err := readiness.WaitUntil(ctx, dialogAutoHandleTimeout, dialogAutoHandlePollInterval, func() (struct{}, bool, error) {
		return struct{}{}, !dm.HasAutoHandler(tabID), nil
	}); err == nil {
		settle := time.NewTimer(dialogAutoHandleSettleDelay)
		defer settle.Stop()
		select {
		case <-ctx.Done():
		case <-settle.C:
		}
		return
	}
	_ = dm.TakeAutoHandler(tabID)
}

func (b *Bridge) actionDoubleClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	scaleScreencastCoords(ctx, &req)
	return b.runClick(ctx, req, (*pointerOps).doubleClickGesture, map[string]any{"doubleclicked": true})
}

func (b *Bridge) actionHover(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if b.effectiveHumanize(req) {
		return b.actionHumanizedHover(ctx, req)
	}
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return nil, err
	}
	if nodeID > 0 {
		return map[string]any{"hovered": true}, bridgecdpops.HoverByNodeID(ctx, nodeID)
	}
	if req.HasXY {
		return map[string]any{"hovered": true}, bridgecdpops.HoverByCoordinate(ctx, req.X, req.Y)
	}
	return nil, NewInvalidActionRequestError("need selector, ref, nodeId, or x/y coordinates")
}

func (b *Bridge) actionHumanizedHover(ctx context.Context, req ActionRequest) (map[string]any, error) {
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return nil, err
	}
	switch {
	case nodeID > 0:
		err = b.pointer().hoverElement(ctx, cdp.BackendNodeID(nodeID))
	case req.HasXY:
		err = b.pointer().hoverCoordinate(ctx, req.X, req.Y)
	default:
		return nil, NewInvalidActionRequestError("need selector, ref, nodeId, or x/y coordinates")
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"hovered": true, "human": true}, nil
}

func (b *Bridge) rememberPointerPosition(tabID string, x, y float64) {
	if b == nil || tabID == "" {
		return
	}
	b.pointerMu.Lock()
	b.pointerByTab[tabID] = pointerState{X: x, Y: y, Known: true}
	b.pointerMu.Unlock()
}

func (b *Bridge) dropPointerState(tabID string) {
	b.pointerMu.Lock()
	defer b.pointerMu.Unlock()
	delete(b.pointerByTab, tabID)
}

func (b *Bridge) currentPointerPosition(tabID string) (float64, float64, bool) {
	if b == nil || tabID == "" {
		return 0, 0, false
	}
	b.pointerMu.RLock()
	defer b.pointerMu.RUnlock()
	state, ok := b.pointerByTab[tabID]
	if !ok || !state.Known {
		return 0, 0, false
	}
	return state.X, state.Y, true
}

func pointerTargetRequiredError(req ActionRequest, allowCurrent bool) error {
	if allowCurrent && strings.TrimSpace(req.TabID) != "" {
		return fmt.Errorf("no pointer position known for tab %s; move pointer first or provide selector, ref, nodeId, or x/y coordinates", req.TabID)
	}
	return NewInvalidActionRequestError("need selector, ref, nodeId, or x/y coordinates")
}

func (b *Bridge) pointerCoordinatesFromRequest(ctx context.Context, req ActionRequest, allowCurrent bool) (float64, float64, error) {
	if req.HasXY {
		return req.X, req.Y, nil
	}
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return 0, 0, err
	}
	if nodeID > 0 {
		return bridgecdpops.PointerPointForNode(ctx, nodeID, false)
	}
	if allowCurrent {
		if x, y, ok := b.currentPointerPosition(req.TabID); ok {
			return x, y, nil
		}
	}
	return 0, 0, pointerTargetRequiredError(req, allowCurrent)
}

func (b *Bridge) actionMouseMove(ctx context.Context, req ActionRequest) (map[string]any, error) {
	x, y, err := b.pointerCoordinatesFromRequest(ctx, req, false)
	if err != nil {
		return nil, err
	}
	if err := b.pointer().mouseMoveByCoordinate(ctx, x, y); err != nil {
		return nil, err
	}
	b.rememberPointerPosition(req.TabID, x, y)
	return map[string]any{"moved": true, "x": x, "y": y}, nil
}

func (b *Bridge) actionMouseDown(ctx context.Context, req ActionRequest) (map[string]any, error) {
	x, y, err := b.pointerCoordinatesFromRequest(ctx, req, true)
	if err != nil {
		return nil, err
	}
	button := req.Button
	if button == "" {
		button = "left"
	}
	if err := b.pointer().mouseDownByCoordinate(ctx, x, y, button, req.Modifiers); err != nil {
		return nil, err
	}
	b.rememberPointerPosition(req.TabID, x, y)
	return map[string]any{"down": true, "x": x, "y": y, "button": button}, nil
}

func (b *Bridge) actionMouseUp(ctx context.Context, req ActionRequest) (map[string]any, error) {
	x, y, err := b.pointerCoordinatesFromRequest(ctx, req, true)
	if err != nil {
		return nil, err
	}
	button := req.Button
	if button == "" {
		button = "left"
	}
	if err := b.pointer().mouseUpByCoordinate(ctx, x, y, button, req.Modifiers); err != nil {
		return nil, err
	}
	b.rememberPointerPosition(req.TabID, x, y)
	return map[string]any{"up": true, "x": x, "y": y, "button": button}, nil
}

func (b *Bridge) actionMouseWheel(ctx context.Context, req ActionRequest) (map[string]any, error) {
	// Resolved before the pointer target, so a request that cannot scroll is refused
	// without reaching CDP at all.
	deltaX, deltaY, err := wheelDelta(req)
	if err != nil {
		return nil, err
	}
	x, y, err := b.pointerCoordinatesFromRequest(ctx, req, true)
	if err != nil {
		if req.HasXY || req.NodeID > 0 || req.Selector != "" || req.TabID == "" {
			return nil, err
		}
		x, y, err = b.pointer().scrollViewportCenter(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve wheel viewport center: %w", err)
		}
	}
	if _, err := b.pointer().scrollByCoordinate(ctx, x, y, deltaX, deltaY, req.Modifiers); err != nil {
		return nil, err
	}
	b.rememberPointerPosition(req.TabID, x, y)
	return map[string]any{"wheel": true, "x": x, "y": y, "deltaX": deltaX, "deltaY": deltaY}, nil
}

const defaultScrollNotch = 120

func resolveScrollDelta(x, y int, explicit bool, spelling string) (int, int, error) {
	if x != 0 || y != 0 {
		return x, y, nil
	}
	if explicit {
		return 0, 0, fmt.Errorf("a zero delta is not a scroll: pass a non-zero %s", spelling)
	}
	return 0, defaultScrollNotch, nil
}

func scrollDeltaFromRequest(primaryX, primaryY, fallbackX, fallbackY int, explicit bool, spelling string) (int, int, error) {
	if primaryX == 0 && primaryY == 0 {
		primaryX, primaryY = fallbackX, fallbackY
	}
	return resolveScrollDelta(primaryX, primaryY, explicit, spelling)
}

func wheelDelta(req ActionRequest) (int, int, error) {
	return scrollDeltaFromRequest(req.DeltaX, req.DeltaY, req.ScrollX, req.ScrollY, req.HasDelta || req.HasScroll, "deltaX/deltaY")
}

func scrollDelta(req ActionRequest) (int, int, error) {
	return scrollDeltaFromRequest(req.ScrollX, req.ScrollY, req.DeltaX, req.DeltaY, req.HasScroll || req.HasDelta, "scrollX/scrollY, or a selector to scroll into view")
}

func (b *Bridge) actionScroll(ctx context.Context, req ActionRequest) (map[string]any, error) {
	scaleScreencastCoords(ctx, &req)
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return nil, err
	}
	if nodeID > 0 {
		return map[string]any{"scrolled": true}, cdptk.ScrollNodeIntoView(ctx, nodeID)
	}

	deltaX, deltaY, err := scrollDelta(req)
	if err != nil {
		return nil, err
	}

	targetX := req.X
	targetY := req.Y
	if !req.HasXY {
		targetX, targetY, err = b.pointer().scrollViewportCenter(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve scroll viewport center: %w", err)
		}
	}

	position, err := b.pointer().scrollByCoordinate(ctx, targetX, targetY, deltaX, deltaY, req.Modifiers)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"scrolled": position.Moved,
		"scrollX":  position.X,
		"scrollY":  position.Y,
		"targetX":  targetX,
		"targetY":  targetY,
		"deltaX":   deltaX,
		"deltaY":   deltaY,
	}, nil
}

func (b *Bridge) actionDrag(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.hasDragDestination() {
		if req.DragX != 0 || req.DragY != 0 {
			return nil, NewInvalidActionRequestError("drag takes a destination (toSelector/toNodeId/toX+toY) or an offset (dragX/dragY), not both")
		}
		return b.dragToDestination(ctx, req)
	}
	if req.DragX == 0 && req.DragY == 0 {
		return nil, NewInvalidActionRequestError("dragX or dragY required for drag")
	}
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return nil, err
	}
	if nodeID <= 0 {
		return nil, NewInvalidActionRequestError("need selector, ref, or nodeId")
	}
	if err := bridgecdpops.DragByNodeID(ctx, nodeID, req.DragX, req.DragY, req.Button); err != nil {
		return nil, err
	}
	return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY}, nil
}

func (b *Bridge) dragToDestination(ctx context.Context, req ActionRequest) (map[string]any, error) {
	fromX, fromY, err := dragPointFor(ctx, dragEnd{nodeID: req.NodeID, selector: req.Selector, x: req.X, y: req.Y, hasPoint: req.HasXY})
	if err != nil {
		return nil, fmt.Errorf("drag source: %w", err)
	}
	toX, toY, err := dragPointFor(ctx, dragEnd{nodeID: req.ToNodeID, selector: req.ToSelector, x: req.ToX, y: req.ToY, hasPoint: req.HasToXY})
	if err != nil {
		return nil, fmt.Errorf("drag destination: %w", err)
	}
	if err := bridgecdpops.DragBetweenPoints(ctx, fromX, fromY, toX, toY, req.Button); err != nil {
		return nil, err
	}
	return map[string]any{
		"dragged": true,
		"fromX":   fromX,
		"fromY":   fromY,
		"toX":     toX,
		"toY":     toY,
	}, nil
}

type dragEnd struct {
	nodeID   int64
	selector string
	x, y     float64
	hasPoint bool
}

func dragPointFor(ctx context.Context, end dragEnd) (float64, float64, error) {
	nodeID, err := targetNodeID(ctx, end.nodeID, end.selector)
	if err != nil {
		return 0, 0, err
	}
	switch {
	case nodeID > 0:
		return bridgecdpops.PointerPointForNode(ctx, nodeID, true)
	case end.hasPoint:
		return end.x, end.y, nil
	}
	return 0, 0, NewInvalidActionRequestError("need selector, ref, nodeId, or coordinates")
}

func (r ActionRequest) hasDragDestination() bool {
	return r.ToNodeID > 0 || r.ToSelector != "" || r.HasToXY
}

func (b *Bridge) actionHumanizedClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	return b.runClick(ctx, req, (*pointerOps).humanClickGesture, map[string]any{"clicked": true, "human": true})
}

func (p *pointerOps) humanClickGesture() pointerGesture {
	return pointerGesture{
		onNode: func(ctx context.Context, nodeID int64) error {
			return p.clickElement(ctx, cdp.BackendNodeID(nodeID))
		},
		onPoint: p.humanClickCoordinate,
	}
}

func (b *Bridge) actionScrollIntoView(ctx context.Context, req ActionRequest) (map[string]any, error) {
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return nil, err
	}
	if nodeID <= 0 {
		return nil, NewInvalidActionRequestError("need selector or ref")
	}
	return ScrollIntoViewAndGetBox(ctx, nodeID)
}
