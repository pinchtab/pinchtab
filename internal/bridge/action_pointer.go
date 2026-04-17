package bridge

import (
	"context"
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

func (b *Bridge) actionClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	// Arm a one-shot dialog auto-handler if the caller expects the click
	// to open a native JS dialog. Without this, the click would hang
	// waiting for the dialog to be handled from a separate request.
	dm := b.GetDialogManager()
	armedDialog := false
	if req.DialogAction != "" && req.TabID != "" && dm != nil {
		dm.ArmAutoHandler(req.TabID, req.DialogAction, req.DialogText)
		armedDialog = true
	}

	var err error
	if req.Selector != "" {
		// For submit buttons, use requestSubmit() to fire constraint validation,
		// JS submit handlers, and actual submission in one shot (issue #411).
		submitted, subErr := submitFormIfButton(ctx, req.Selector)
		if subErr != nil {
			slog.Debug("submitFormIfButton failed, falling back to CDP click",
				"selector", req.Selector, "error", subErr)
		} else if submitted {
			if req.WaitNav {
				_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
			}
			return map[string]any{"clicked": true, "submitted": true}, nil
		}
		node, nodeErr := firstNodeBySelector(ctx, req.Selector)
		if nodeErr != nil {
			return nil, nodeErr
		}
		err = ClickByNodeID(ctx, int64(node.BackendNodeID))
	} else if req.NodeID > 0 {
		err = ClickByNodeID(ctx, req.NodeID)
	} else if req.HasXY {
		err = ClickByCoordinate(ctx, req.X, req.Y)
	} else {
		return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
	}
	if err != nil {
		return nil, err
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

func (b *Bridge) actionDoubleClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	var err error
	if req.Selector != "" {
		node, nodeErr := firstNodeBySelector(ctx, req.Selector)
		if nodeErr != nil {
			return nil, nodeErr
		}
		err = DoubleClickByNodeID(ctx, int64(node.BackendNodeID))
	} else if req.NodeID > 0 {
		err = DoubleClickByNodeID(ctx, req.NodeID)
	} else if req.HasXY {
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

func (b *Bridge) actionHumanClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.NodeID > 0 {
		// req.NodeID is a backendDOMNodeId from the accessibility tree
		if err := ClickElement(ctx, cdp.BackendNodeID(req.NodeID)); err != nil {
			return nil, err
		}
		return map[string]any{"clicked": true, "human": true}, nil
	}
	if req.Selector != "" {
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		// Use BackendNodeID from the DOM node
		if err := ClickElement(ctx, node.BackendNodeID); err != nil {
			return nil, err
		}
		return map[string]any{"clicked": true, "human": true}, nil
	}
	return nil, fmt.Errorf("need selector, ref, or nodeId")
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
