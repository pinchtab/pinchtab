package bridge

import (
	"context"
	"errors"
	"fmt"
	"time"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/readiness"
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
	var state checkableState
	if err := cdptk.CallFunctionOn(ctx, objectID, checkableStateJS, nil, &state); err != nil {
		return checkableState{}, err
	}
	if state.Error != "" {
		return checkableState{}, NewInvalidActionRequestError(state.Error)
	}
	return state, nil
}

func (b *Bridge) actionFocus(ctx context.Context, req ActionRequest) (map[string]any, error) {
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return nil, err
	}
	if nodeID > 0 {
		err = bridgecdpops.FocusBackendNode(ctx, nodeID)
	}
	return map[string]any{"focused": true}, err
}

func (b *Bridge) actionSelect(ctx context.Context, req ActionRequest) (map[string]any, error) {
	val, supplied := SelectValue(req)
	if !supplied {
		return nil, fmt.Errorf(`select requires "value" (or "text"); send "value": "" to select an empty-valued option`)
	}
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return nil, err
	}
	if nodeID <= 0 {
		return nil, NewInvalidActionRequestError("need selector or ref")
	}
	err = bridgecdpops.SelectByNodeID(ctx, nodeID, val)
	var notSelect *bridgecdpops.NotSelectError
	if errors.As(err, &notSelect) {
		return nil, fmt.Errorf("%w: %w", ErrInvalidActionRequest, notSelect)
	}
	return map[string]any{"selected": val}, err
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
	nodeID, err := targetNodeID(ctx, req.NodeID, req.Selector)
	if err != nil {
		return nil, err
	}
	if nodeID <= 0 {
		return nil, NewInvalidActionRequestError("need selector, ref, or nodeId")
	}

	objectID, err := bridgecdpops.ResolveNodeObjectID(ctx, nodeID)
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
		if err := bridgecdpops.JSClickByBackendNode(ctx, nodeID); err != nil {
			return nil, err
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
