package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pinchtab/pinchtab/internal/readiness"
	"time"

	"github.com/chromedp/chromedp"
)

const checkableStateJS = `function() {
	var tag = this.tagName ? this.tagName.toLowerCase() : "";
	var type = (this.type || "").toLowerCase();
	if (tag === "input" && (type === "checkbox" || type === "radio")) {
		return {kind: type, state: this.checked ? "true" : "false"};
	}
	var role = (this.getAttribute && this.getAttribute("role") || "").toLowerCase();
	if (role === "checkbox") {
		var aria = (this.getAttribute("aria-checked") || "").toLowerCase();
		if (aria === "true" || aria === "false" || aria === "mixed") {
			return {kind: "aria-checkbox", state: aria};
		}
		return {error: "role=checkbox requires aria-checked=true, false, or mixed"};
	}
	return {error: "element is not a checkbox, radio, or role=checkbox control (got " + tag + "[type=" + type + "][role=" + role + "])"};
}`

type checkableState struct {
	Error string `json:"error"`
	Kind  string `json:"kind"`
	State string `json:"state"`
}

func readCheckableState(ctx context.Context, objectID string) (checkableState, error) {
	var callResult json.RawMessage
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": checkableStateJS,
			"objectId":            objectID,
			"returnByValue":       true,
		}, &callResult)
	}))
	if err != nil {
		return checkableState{}, err
	}
	var response struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResult, &response); err != nil {
		return checkableState{}, err
	}
	var state checkableState
	if err := json.Unmarshal(response.Result.Value, &state); err != nil {
		return checkableState{}, err
	}
	if state.Error != "" {
		return checkableState{}, fmt.Errorf("%s", state.Error)
	}
	return state, nil
}

func (b *Bridge) actionFocus(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Selector != "" {
		return map[string]any{"focused": true}, chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery))
	}
	if req.NodeID > 0 {
		return map[string]any{"focused": true}, focusBackendNode(ctx, req.NodeID)
	}
	return map[string]any{"focused": true}, nil
}

func (b *Bridge) actionSelect(ctx context.Context, req ActionRequest) (map[string]any, error) {
	val, supplied := SelectValue(req)
	if !supplied {
		return nil, fmt.Errorf(`select requires "value" (or "text"); send "value": "" to select an empty-valued option`)
	}
	// Both paths route through SelectByNodeID so they share its
	// value-then-text match fallback. This lets callers pass either the
	// `<option value="...">` attribute or the option's visible text.
	if req.NodeID > 0 {
		return map[string]any{"selected": val}, SelectByNodeID(ctx, req.NodeID, val)
	}
	if req.Selector != "" {
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		return map[string]any{"selected": val}, SelectByNodeID(ctx, int64(node.BackendNodeID), val)
	}
	return nil, NewInvalidActionRequestError("need selector or ref")
}

func (b *Bridge) actionCheck(ctx context.Context, req ActionRequest) (map[string]any, error) {
	return checkUncheck(ctx, req, true)
}

func (b *Bridge) actionUncheck(ctx context.Context, req ActionRequest) (map[string]any, error) {
	return checkUncheck(ctx, req, false)
}

// checkUncheck implements both check and uncheck actions.
// If wantChecked is true, it ensures the element is checked; if false, unchecked.
// It only clicks if the current state differs from the desired state.
func checkUncheck(ctx context.Context, req ActionRequest, wantChecked bool) (map[string]any, error) {
	// Resolve the element to an objectId via backendNodeId or CSS selector.
	var resolveJS string
	if req.NodeID > 0 {
		// Resolve via backendNodeId using DOM.resolveNode (done below).
	} else if req.Selector != "" {
		resolveJS = req.Selector
	} else {
		return nil, NewInvalidActionRequestError("need selector, ref, or nodeId")
	}

	var objectID string
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if req.NodeID > 0 {
			var result json.RawMessage
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
				"backendNodeId": req.NodeID,
			}, &result); err != nil {
				return err
			}
			var resolved struct {
				Object struct {
					ObjectID string `json:"objectId"`
				} `json:"object"`
			}
			if err := json.Unmarshal(result, &resolved); err != nil {
				return err
			}
			objectID = resolved.Object.ObjectID
			return nil
		}
		// CSS selector path
		var evalResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    fmt.Sprintf(`document.querySelector(%q)`, resolveJS),
			"returnByValue": false,
		}, &evalResult); err != nil {
			return err
		}
		var er struct {
			Result struct {
				ObjectID string `json:"objectId"`
				Type     string `json:"type"`
				Subtype  string `json:"subtype"`
			} `json:"result"`
		}
		if err := json.Unmarshal(evalResult, &er); err != nil {
			return err
		}
		if er.Result.ObjectID == "" || er.Result.Subtype == "null" {
			return fmt.Errorf("element not found: %s", resolveJS)
		}
		objectID = er.Result.ObjectID
		return nil
	}))
	if err != nil {
		return nil, err
	}

	state, err := readCheckableState(ctx, objectID)
	if err != nil {
		return nil, err
	}
	desired := "false"
	if wantChecked {
		desired = "true"
	}

	// Click only if state needs to change. Use the JS-dispatch path so the
	// toggle isn't gated on the headless=new CDP renderer ack chain.
	if state.State != desired {
		if req.NodeID > 0 {
			if err := JSClickByBackendNode(ctx, req.NodeID); err != nil {
				return nil, err
			}
		} else {
			node, err := firstNodeBySelector(ctx, req.Selector)
			if err != nil {
				return nil, err
			}
			if err := JSClickByBackendNode(ctx, int64(node.BackendNodeID)); err != nil {
				return nil, err
			}
		}
	}

	// Framework-backed ARIA controls often update state on the next task. Poll
	// briefly and report the observed value instead of claiming the requested
	// state without proof.
	var observed checkableState
	var observeErr error
	_, waitErr := readiness.WaitUntil(ctx, 500*time.Millisecond, 25*time.Millisecond, func() (struct{}, bool, error) {
		observed, observeErr = readCheckableState(ctx, objectID)
		return struct{}{}, observeErr == nil && observed.State == desired, nil
	})
	switch {
	case waitErr == nil:
		return map[string]any{
			"checked":     wantChecked,
			"controlType": observed.Kind,
			"verified":    true,
		}, nil
	case !errors.Is(waitErr, readiness.ErrNotReady):
		return nil, waitErr
	case observeErr != nil:
		return nil, fmt.Errorf("verify checked state: %w", observeErr)
	default:
		return nil, fmt.Errorf("checked state remained %q after requesting %q", observed.State, desired)
	}
}
