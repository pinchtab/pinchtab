package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

var scrollByCoordinateAction = ScrollByCoordinate
var mouseMoveByCoordinateAction = MouseMoveByCoordinate
var mouseDownByCoordinateAction = MouseDownByCoordinate
var mouseUpByCoordinateAction = MouseUpByCoordinate
var clickElementAction = ClickElement
var clickByNodeIDAction = ClickByNodeID
var jsClickByBackendNodeAction = JSClickByBackendNode
var dispatchClickByBackendNodeAction = JSDispatchClickByBackendNode
var doubleClickByNodeIDAction = DoubleClickByNodeID
var jsDoubleClickByBackendNodeAction = JSDoubleClickByBackendNode

const trustedNodeClickTimeout = 100 * time.Millisecond

func clickByNodeIDWithJSFallback(ctx context.Context, nodeID int64) error {
	trustedCtx, cancel := context.WithTimeout(ctx, trustedNodeClickTimeout)
	err := clickByNodeIDAction(trustedCtx, nodeID)
	cancel()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		// Re-check: the parent context may have been cancelled (e.g. by the
		// dialog-detection polling loop) while the trusted click was running.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Use a bounded timeout for the JS fallback so it cannot hang for the
		// full action timeout when JS execution is blocked (e.g. by an open
		// dialog).
		jsCtx, jsCancel := context.WithTimeout(ctx, trustedNodeClickTimeout)
		defer jsCancel()
		return jsClickByBackendNodeAction(jsCtx, nodeID)
	}
	return err
}

func clickByNodeIDWithMode(ctx context.Context, nodeID int64, mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "default":
		return clickByNodeIDWithJSFallback(ctx, nodeID)
	case "dom":
		return jsClickByBackendNodeAction(ctx, nodeID)
	case "dispatch":
		return dispatchClickByBackendNodeAction(ctx, nodeID)
	default:
		return fmt.Errorf("invalid click mode: %s", mode)
	}
}

func doubleClickByNodeIDWithJSFallback(ctx context.Context, nodeID int64) error {
	trustedCtx, cancel := context.WithTimeout(ctx, trustedNodeClickTimeout)
	err := doubleClickByNodeIDAction(trustedCtx, nodeID)
	cancel()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		jsCtx, jsCancel := context.WithTimeout(ctx, trustedNodeClickTimeout)
		defer jsCancel()
		return jsDoubleClickByBackendNodeAction(jsCtx, nodeID)
	}
	return err
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

var scrollViewportCenter = func(ctx context.Context) (float64, float64, error) {
	var viewport struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`({
		x: Math.max(1, Math.floor(window.innerWidth / 2)),
		y: Math.max(1, Math.floor(window.innerHeight / 2))
	})`, &viewport)); err != nil {
		return 0, 0, err
	}
	return viewport.X, viewport.Y, nil
}

// submitFormIfButton checks whether the target element is a submit button and,
// if so, uses requestSubmit() for a single-shot submission: constraint
// validation + submit event (so JS handlers run) + actual submission.
// Falls back to CDP click if the element is not a submit button or on error.
func submitFormIfButton(ctx context.Context, selector string) (bool, error) {
	var isSubmit bool
	err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return false;
			var tag = el.tagName.toLowerCase();
			var type = (el.type || '').toLowerCase();
			return (tag === 'button' && (type === 'submit' || type === '')) ||
			       (tag === 'input' && type === 'submit');
		})()
	`, selector), &isSubmit))
	if err != nil || !isSubmit {
		return false, err
	}
	// Fire full event chain via requestSubmit(el):
	// - runs constraint validation
	// - dispatches the submit event (so JS handlers like Odoo's fire)
	// - submits the form if nothing cancels it
	// One call, no double-fire (replaces manual dispatchEvent + form.submit).
	var submitted bool
	err = chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return false;
			el.focus();
			var opts = {bubbles: true, cancelable: true};
			el.dispatchEvent(new MouseEvent('mousedown', opts));
			el.dispatchEvent(new MouseEvent('mouseup', opts));
			el.dispatchEvent(new MouseEvent('click', opts));
			var form = el.closest('form');
			if (form) { form.requestSubmit(el); }
			return true;
		})()
	`, selector), &submitted))
	return submitted, err
}

// settle finalizes the popup auto-switch from a deferred call in the click
// handlers: on success it adopts/focuses any opened tab and augments result; on
// error it cancels — without restore when the error is a blocking dialog, so
// the popup isn't torn down mid-dialog. nil-safe so handlers can defer it
// unconditionally. Centralizes the finish/cancel branching that actionClick,
// actionDoubleClick, and actionHumanizedClick would otherwise each copy.
func (s *autoSwitchSession) settle(ctx context.Context, result map[string]any, err error) map[string]any {
	if s == nil {
		return result
	}
	if err == nil {
		return s.finish(ctx, result)
	}
	var dialogErr *ErrDialogBlocking
	if errors.As(err, &dialogErr) {
		s.cancelWithoutRestore()
	} else {
		s.cancel(ctx)
	}
	return result
}

// armDialogAutoHandler arms a one-shot dialog auto-handler when the request
// names a DialogAction and the tab has a dialog manager. It returns the manager
// (possibly nil) and whether a handler was armed. Shared by actionClick and
// actionHumanizedClick.
func (b *Bridge) armDialogAutoHandler(req ActionRequest) (*DialogManager, bool) {
	dm := b.GetDialogManager()
	if req.DialogAction != "" && req.TabID != "" && dm != nil {
		dm.ArmAutoHandler(req.TabID, req.DialogAction, req.DialogText)
		return dm, true
	}
	return dm, false
}

// dialogBlocking returns a populated *ErrDialogBlocking when a blocking dialog
// is pending on the tab, or nil otherwise. Shared by the click handlers' dialog
// poll loops so the pending-check and error construction stay identical.
func dialogBlocking(dm *DialogManager, tabID string) error {
	pending := dm.GetPending(tabID)
	if pending == nil {
		return nil
	}
	return &ErrDialogBlocking{
		DialogType:    pending.Type,
		DialogMessage: pending.Message,
	}
}

func (b *Bridge) actionClick(ctx context.Context, req ActionRequest) (result map[string]any, err error) {
	if b.effectiveHumanize(req) {
		return b.actionHumanizedClick(ctx, req)
	}

	// Arm popup-aware auto-switch: if this click opens a new tab, we adopt
	// + focus it and surface the new tab ID on the response.
	auto := b.beginAutoSwitch(req)
	defer func() { result = auto.settle(ctx, result, err) }()

	// Arm a one-shot dialog auto-handler if the caller expects the click
	// to open a native JS dialog. Without this, the click would hang
	// waiting for the dialog to be handled from a separate request.
	dm, armedDialog := b.armDialogAutoHandler(req)

	detectDialog := !armedDialog && req.TabID != "" && dm != nil
	var clickCtx context.Context
	var clickCancel context.CancelFunc
	if detectDialog {
		clickCtx, clickCancel = context.WithCancel(ctx)
		defer clickCancel()
	} else {
		clickCtx = ctx
	}

	type clickResult struct {
		err error
	}
	resultCh := make(chan clickResult, 1)

	// Run click in goroutine so we can poll for dialogs
	go func() {
		var err error
		if auto != nil {
			auto.prepareWindowOpenCapture(clickCtx)
		}
		if req.Selector != "" {
			// For submit buttons, use requestSubmit() to fire constraint validation,
			// JS submit handlers, and actual submission in one shot (issue #411).
			submitted, subErr := submitFormIfButton(clickCtx, req.Selector)
			if subErr != nil {
				slog.Debug("submitFormIfButton failed, falling back to JS click",
					"selector", req.Selector, "error", subErr)
			} else if submitted {
				resultCh <- clickResult{err: nil}
				return
			}
			node, nodeErr := firstNodeBySelector(clickCtx, req.Selector)
			if nodeErr != nil {
				resultCh <- clickResult{err: nodeErr}
				return
			}
			if auto != nil {
				auto.prepareNode(clickCtx, int64(node.BackendNodeID))
			}
			err = clickByNodeIDWithMode(clickCtx, int64(node.BackendNodeID), req.Mode)
		} else if req.NodeID > 0 {
			if auto != nil {
				auto.prepareNode(clickCtx, req.NodeID)
			}
			err = clickByNodeIDWithMode(clickCtx, req.NodeID, req.Mode)
		} else if req.HasXY {
			err = ClickByCoordinate(clickCtx, req.X, req.Y)
		} else {
			resultCh <- clickResult{err: fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")}
			return
		}
		resultCh <- clickResult{err: err}
	}()

	if detectDialog {
		ticker := time.NewTicker(dialogAutoHandlePollInterval)
		defer ticker.Stop()
		for {
			select {
			case result := <-resultCh:
				if result.err != nil {
					return nil, result.err
				}
				if req.WaitNav {
					_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
				}
				return map[string]any{"clicked": true}, nil
			case <-ticker.C:
				if e := dialogBlocking(dm, req.TabID); e != nil {
					clickCancel()
					return nil, e
				}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	res := <-resultCh
	if res.err != nil {
		return nil, res.err
	}
	if armedDialog {
		waitForArmedDialogSettle(dm, req.TabID, dialogAutoHandleTimeout)
	}
	if req.WaitNav {
		_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
	}
	return map[string]any{"clicked": true}, nil
}

func waitForArmedDialogSettle(dm *DialogManager, tabID string, timeout time.Duration) {
	if dm == nil || strings.TrimSpace(tabID) == "" {
		return
	}
	if timeout <= 0 {
		timeout = dialogAutoHandleTimeout
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !dm.HasAutoHandler(tabID) {
			// Allow the handler goroutine to finish UI side-effects before
			// immediate follow-up reads (for example get_text assertions).
			time.Sleep(dialogAutoHandleSettleDelay)
			return
		}
		time.Sleep(dialogAutoHandlePollInterval)
	}

	// Prevent stale one-shot handlers from leaking into later clicks.
	_ = dm.TakeAutoHandler(tabID)
}

func (b *Bridge) actionDoubleClick(ctx context.Context, req ActionRequest) (result map[string]any, err error) {
	auto := b.beginAutoSwitch(req)
	defer func() { result = auto.settle(ctx, result, err) }()
	if req.Selector != "" {
		node, nodeErr := firstNodeBySelector(ctx, req.Selector)
		if nodeErr != nil {
			return nil, nodeErr
		}
		if auto != nil {
			auto.prepareNode(ctx, int64(node.BackendNodeID))
		}
		err = doubleClickByNodeIDWithJSFallback(ctx, int64(node.BackendNodeID))
	} else if req.NodeID > 0 {
		if auto != nil {
			auto.prepareNode(ctx, req.NodeID)
		}
		err = doubleClickByNodeIDWithJSFallback(ctx, req.NodeID)
	} else if req.HasXY {
		if auto != nil {
			auto.prepareWindowOpenCapture(ctx)
		}
		err = DoubleClickByCoordinate(ctx, req.X, req.Y)
	} else {
		return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"doubleclicked": true}, nil
}

func (b *Bridge) actionHover(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.NodeID > 0 {
		return map[string]any{"hovered": true}, HoverByNodeID(ctx, req.NodeID)
	}
	if req.Selector != "" {
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		return map[string]any{"hovered": true}, HoverByNodeID(ctx, int64(node.BackendNodeID))
	}
	if req.HasXY {
		return map[string]any{"hovered": true}, HoverByCoordinate(ctx, req.X, req.Y)
	}
	return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
}

func (b *Bridge) rememberPointerPosition(tabID string, x, y float64) {
	if b == nil || tabID == "" {
		return
	}
	b.pointerMu.Lock()
	b.pointerByTab[tabID] = pointerState{X: x, Y: y, Known: true}
	b.pointerMu.Unlock()
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
	return fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
}

func (b *Bridge) pointerCoordinatesFromRequest(ctx context.Context, req ActionRequest, allowCurrent bool) (float64, float64, error) {
	if req.HasXY {
		return req.X, req.Y, nil
	}
	if req.NodeID > 0 {
		return PointerPointForNode(ctx, req.NodeID, false)
	}
	if req.Selector != "" {
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return 0, 0, err
		}
		return PointerPointForNode(ctx, int64(node.BackendNodeID), false)
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
	if err := mouseMoveByCoordinateAction(ctx, x, y); err != nil {
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
	if err := mouseDownByCoordinateAction(ctx, x, y, button); err != nil {
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
	if err := mouseUpByCoordinateAction(ctx, x, y, button); err != nil {
		return nil, err
	}
	b.rememberPointerPosition(req.TabID, x, y)
	return map[string]any{"up": true, "x": x, "y": y, "button": button}, nil
}

func (b *Bridge) actionMouseWheel(ctx context.Context, req ActionRequest) (map[string]any, error) {
	x, y, err := b.pointerCoordinatesFromRequest(ctx, req, true)
	if err != nil {
		if req.HasXY || req.NodeID > 0 || req.Selector != "" || req.TabID == "" {
			return nil, err
		}
		x, y, err = scrollViewportCenter(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve wheel viewport center: %w", err)
		}
	}
	deltaX := req.DeltaX
	deltaY := req.DeltaY
	if deltaX == 0 && deltaY == 0 {
		deltaX = req.ScrollX
		deltaY = req.ScrollY
	}
	if deltaX == 0 && deltaY == 0 {
		deltaY = 120
	}
	if err := scrollByCoordinateAction(ctx, x, y, deltaX, deltaY); err != nil {
		return nil, err
	}
	b.rememberPointerPosition(req.TabID, x, y)
	return map[string]any{"wheel": true, "x": x, "y": y, "deltaX": deltaX, "deltaY": deltaY}, nil
}

func (b *Bridge) actionScroll(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.NodeID > 0 {
		return map[string]any{"scrolled": true}, ScrollByNodeID(ctx, req.NodeID)
	}
	if req.Selector != "" {
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		return map[string]any{"scrolled": true}, ScrollByNodeID(ctx, int64(node.BackendNodeID))
	}

	scrollX := req.ScrollX
	scrollY := req.ScrollY
	if scrollX == 0 && scrollY == 0 {
		scrollY = 120
	}

	scrollTargetX := req.X
	scrollTargetY := req.Y
	if !req.HasXY {
		var err error
		scrollTargetX, scrollTargetY, err = scrollViewportCenter(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve scroll viewport center: %w", err)
		}
	}

	return map[string]any{
			"scrolled": true,
			// Legacy keys retained for compatibility with existing clients.
			"x":       scrollX,
			"y":       scrollY,
			"targetX": scrollTargetX,
			"targetY": scrollTargetY,
			"deltaX":  scrollX,
			"deltaY":  scrollY,
		},
		scrollByCoordinateAction(ctx, scrollTargetX, scrollTargetY, scrollX, scrollY)
}

func (b *Bridge) actionDrag(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.DragX == 0 && req.DragY == 0 {
		return nil, fmt.Errorf("dragX or dragY required for drag")
	}
	if req.NodeID > 0 {
		err := DragByNodeID(ctx, req.NodeID, req.DragX, req.DragY)
		if err != nil {
			return nil, err
		}
		return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY}, nil
	}
	if req.Selector != "" {
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		err = DragByNodeID(ctx, int64(node.BackendNodeID), req.DragX, req.DragY)
		if err != nil {
			return nil, err
		}
		return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY}, nil
	}
	return nil, fmt.Errorf("need selector, ref, or nodeId")
}

func (b *Bridge) actionHumanizedClick(ctx context.Context, req ActionRequest) (result map[string]any, err error) {
	auto := b.beginAutoSwitch(req)
	defer func() { result = auto.settle(ctx, result, err) }()
	var backendNodeID cdp.BackendNodeID
	switch {
	case req.NodeID > 0:
		backendNodeID = cdp.BackendNodeID(req.NodeID)
		if auto != nil {
			auto.prepareNode(ctx, req.NodeID)
		}
	case req.Selector != "":
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		backendNodeID = node.BackendNodeID
		if auto != nil {
			auto.prepareNode(ctx, int64(node.BackendNodeID))
		}
	default:
		return nil, fmt.Errorf("need selector, ref, or nodeId")
	}

	// If the caller expects this click to open a native JS dialog, arm a
	// one-shot auto-handler so it gets accepted/dismissed instead of leaving the
	// renderer blocked. Without this the humanized path can only ever report
	// dialog_blocking. Mirrors actionClick's non-humanized branch.
	dm, armedDialog := b.armDialogAutoHandler(req)

	// Run the multi-step humanized click (bezier mouse-move + press + release) in
	// a goroutine. When no dialog-action was provided, poll for an unexpected
	// blocking dialog so it surfaces as ErrDialogBlocking rather than hanging the
	// renderer for the full action timeout.
	detectDialog := !armedDialog && req.TabID != "" && dm != nil
	clickCtx := ctx
	var clickCancel context.CancelFunc
	if detectDialog {
		clickCtx, clickCancel = context.WithCancel(ctx)
		defer clickCancel()
	}

	resultCh := make(chan error, 1)
	go func() {
		resultCh <- clickElementAction(clickCtx, backendNodeID)
	}()

	if detectDialog {
		ticker := time.NewTicker(dialogAutoHandlePollInterval)
		defer ticker.Stop()
		for {
			select {
			case err := <-resultCh:
				if err != nil {
					return nil, err
				}
				return map[string]any{"clicked": true, "human": true}, nil
			case <-ticker.C:
				if e := dialogBlocking(dm, req.TabID); e != nil {
					clickCancel()
					return nil, e
				}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	if err := <-resultCh; err != nil {
		return nil, err
	}
	if armedDialog {
		waitForArmedDialogSettle(dm, req.TabID, dialogAutoHandleTimeout)
	}
	return map[string]any{"clicked": true, "human": true}, nil
}

func (b *Bridge) actionScrollIntoView(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.NodeID > 0 {
		return ScrollIntoViewAndGetBox(ctx, req.NodeID)
	}
	if req.Selector != "" {
		nid, err := ResolveCSSToNodeID(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		return ScrollIntoViewAndGetBox(ctx, nid)
	}
	return nil, fmt.Errorf("need selector or ref")
}
