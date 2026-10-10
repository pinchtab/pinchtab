package cdptk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/chromedp/chromedp"
)

// isolatedWorldName is the ONE name every PinchTab isolated world that yields an
// object handle is created under. Page.createIsolatedWorld keys on frame and
// name, so repeat calls for a frame return the same context rather than minting
// one per resolution — and one name means a frame has ONE PinchTab world instead
// of two, which makes handles resolved there interchangeable in a single
// Runtime.callFunctionOn by construction rather than by a rule nobody can check.
//
// "Every isolated world" would be the stronger claim and it is not the true one:
// the screencast repaint loop mints its own world in the top frame. It is exempt
// because the hazard is a callFunctionOn given handles from two worlds, and its
// context id is only ever an argument to runtime.Evaluate — it never becomes an
// object handle, so it cannot reach the hazard whatever frame it sits in. The
// exemption and the condition it rests on are both checked, in
// worldNameExemptions and TestAnExemptWorldCannotProduceObjectHandles.
//
// There were two names for this one rule: a node scope here and a frame scope in
// the bridge. Neither frame policy was wrong. A bare backend node id does not
// carry its frame, so node resolution must use the top frame; evaluating
// `document` for a named frame must use that frame. Those are two needs of one
// mechanism, which is why the frame is a parameter and the mechanism is shared.
//
// No isolation is weakened by sharing it. The world exists so page script cannot
// hide or redirect targets by replacing DOM methods in the main world, and page
// script can reach neither name. Fewer worlds is not a looser boundary.
//
// CROSS-FRAME RESIDUAL, recorded rather than built: handles from two DIFFERENT
// frames' worlds are still not interchangeable, and nothing in the types says
// which frame a handle came from. That hazard has no call site — the only place
// passing an object handle as a callFunctionOn ARGUMENT resolves both handles in
// one IsolatedNodeObjectIDs call, so they share a world — and a handle type
// carrying its context id is real machinery against a shape nothing writes. The
// same-frame case, the only one reachable today, is closed above. Reopen it when
// a caller genuinely needs handles from two frames in one call.
const isolatedWorldName = "pinchtab-scope"

// IsolatedNodeObjectID converts a backend node id to a JS object handle in the
// top frame's isolated world.
//
// DOM.resolveNode without an executionContextId hands back a main-world object,
// and every Runtime.callFunctionOn against it then runs where page script can
// redefine the DOM methods it uses — so geometry read through such a handle is
// whatever the page says it is, not where the element actually sits.
//
// The isolated world is per-frame, but a handle from any frame's world reaches a
// node in another same-process frame, so the top frame's world serves all of
// them: a bare backend node id does not carry its frame, and DOM.describeNode
// reports frameId only for frame owner elements.
//
// It never returns a usable zero. A caller that cannot obtain an isolated
// context gets an error rather than a main-world handle, so the boundary fails
// closed.
func IsolatedNodeObjectID(ctx context.Context, backendNodeID int64) (string, error) {
	objectIDs, err := IsolatedNodeObjectIDs(ctx, backendNodeID)
	if err != nil {
		return "", err
	}
	return objectIDs[0], nil
}

// IsolatedNodeObjectIDs resolves several backend node ids against ONE isolated
// context, so an operation comparing two nodes pays for the frame tree read and
// the world creation once instead of once per node. Handles it returns are
// therefore usable in the same Runtime.callFunctionOn, which a per-node resolve
// only guaranteed by accident.
func IsolatedNodeObjectIDs(ctx context.Context, backendNodeIDs ...int64) ([]string, error) {
	if len(backendNodeIDs) == 0 {
		return nil, fmt.Errorf("no backend node ids to resolve")
	}

	execID, err := IsolatedContextID(ctx, "")
	if err != nil {
		return nil, err
	}

	objectIDs := make([]string, 0, len(backendNodeIDs))
	for _, backendNodeID := range backendNodeIDs {
		objectID, err := resolveNodeInContext(ctx, backendNodeID, execID)
		if err != nil {
			return nil, err
		}
		objectIDs = append(objectIDs, objectID)
	}
	return objectIDs, nil
}

func resolveNodeInContext(ctx context.Context, backendNodeID, execID int64) (string, error) {
	var raw json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId":      backendNodeID,
			"executionContextId": execID,
		}, &raw)
	})); err != nil {
		return "", err
	}

	var parsed struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Object.ObjectID == "" {
		return "", fmt.Errorf("backend node %d is no longer attached", backendNodeID)
	}
	return parsed.Object.ObjectID, nil
}

// IsolatedContextID returns the execution context of the isolated world for
// frameID, or for the top frame when frameID is empty. It is the only place a
// PinchTab isolated world is created — see isolatedWorldName for why one name and
// a frame parameter replaced two worlds, and TestOnlyOneIsolatedWorldNameExists
// for the census that keeps it that way.
//
// It never returns a usable zero: a caller that cannot obtain an isolated context
// gets an error rather than a main-world context, so the boundary fails closed.
// Callers that want a NO-OP for an unnamed frame — "use whatever context the
// caller already has" — must keep that check themselves, because empty here means
// the top frame's isolated world, not the default context.
func IsolatedContextID(ctx context.Context, frameID string) (int64, error) {
	if frameID == "" {
		resolved, err := topFrameID(ctx)
		if err != nil {
			return 0, err
		}
		frameID = resolved
	}

	var raw json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Page.createIsolatedWorld", map[string]any{
			"frameId":   frameID,
			"worldName": isolatedWorldName,
		}, &raw)
	})); err != nil {
		return 0, fmt.Errorf("create isolated world for frame %q: %w", frameID, err)
	}

	var resp struct {
		ExecutionContextID int64 `json:"executionContextId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, err
	}
	if resp.ExecutionContextID == 0 {
		return 0, fmt.Errorf("frame %q has no isolated execution context", frameID)
	}
	return resp.ExecutionContextID, nil
}

// EvaluateInIsolatedWorld runs expression in the isolated world of frameID (the
// top frame when frameID is empty) and unmarshals the returned value into
// result. Unlike the bridge's EvaluateInFrame — which falls back to the MAIN
// world for the top frame — this always runs in the PinchTab isolated world, so
// page script that redefines globals or DOM prototypes cannot alter the run. It
// awaits a returned promise, so a script whose last expression is a thenable
// (axe.run) resolves before the value is read.
func EvaluateInIsolatedWorld(ctx context.Context, frameID, expression string, result any) error {
	execID, err := IsolatedContextID(ctx, frameID)
	if err != nil {
		return err
	}
	return EvaluateInContext(ctx, execID, expression, true, result)
}

// EvaluateInContext runs expression in the execution context execID and decodes
// the by-value result into result when non-nil. A thrown script is an error
// carrying the exception's own text, not a zero value.
func EvaluateInContext(ctx context.Context, execID int64, expression string, awaitPromise bool, result any) error {
	obj, err := runScript(ctx, "Runtime.evaluate", evaluateParams(execID, expression, awaitPromise, true))
	if err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}
	return obj.decode(result)
}

// EvaluateForHandle runs expression in the execution context execID and returns
// the object handle it yields, or "" when it yields null, undefined or a
// primitive. A thrown script is an error, never the Error object's handle.
func EvaluateForHandle(ctx context.Context, execID int64, expression string) (string, error) {
	obj, err := runScript(ctx, "Runtime.evaluate", evaluateParams(execID, expression, false, false))
	if err != nil {
		return "", fmt.Errorf("evaluate: %w", err)
	}
	return obj.ObjectID, nil
}

// CallFunctionOn runs functionDecl with objectID as `this` and decodes the by-value
// result into result when non-nil. A thrown script is an error carrying the
// exception's own text, so a probe that threw never reads as a zero value.
func CallFunctionOn(ctx context.Context, objectID, functionDecl string, args []map[string]any, result any) error {
	obj, err := runScript(ctx, "Runtime.callFunctionOn", callParams(objectID, functionDecl, args, true))
	if err != nil {
		return fmt.Errorf("call function on node: %w", err)
	}
	return obj.decode(result)
}

// CallFunctionOnForHandle runs functionDecl with objectID as `this` and returns
// the object handle it yields, or "" when it yields null, undefined or a
// primitive. A thrown script is an error, never the Error object's handle.
func CallFunctionOnForHandle(ctx context.Context, objectID, functionDecl string, args []map[string]any) (string, error) {
	return CallFunctionOnForGroupedHandle(ctx, objectID, functionDecl, args, "")
}

// CallFunctionOnForGroupedHandle is CallFunctionOnForHandle with the yielded
// handle filed under objectGroup, so ReleaseObjectGroup can free it together
// with every handle later derived from it instead of leaving them alive until
// the page navigates.
func CallFunctionOnForGroupedHandle(ctx context.Context, objectID, functionDecl string, args []map[string]any, objectGroup string) (string, error) {
	params := callParams(objectID, functionDecl, args, false)
	if objectGroup != "" {
		params["objectGroup"] = objectGroup
	}
	obj, err := runScript(ctx, "Runtime.callFunctionOn", params)
	if err != nil {
		return "", fmt.Errorf("call function on node: %w", err)
	}
	return obj.ObjectID, nil
}

// ArrayElementHandles returns one handle per element of the array behind
// arrayObjectID, in index order, in one Runtime.getProperties call. The handles
// share the array's object group.
func ArrayElementHandles(ctx context.Context, arrayObjectID string) ([]string, error) {
	var props struct {
		Result []struct {
			Name  string       `json:"name"`
			Value remoteObject `json:"value"`
		} `json:"result"`
	}
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.getProperties",
			map[string]any{"objectId": arrayObjectID, "ownProperties": true}, &props)
	}))
	if err != nil {
		return nil, fmt.Errorf("array elements: %w", err)
	}
	byIndex := map[int]string{}
	for _, p := range props.Result {
		if i, err := strconv.Atoi(p.Name); err == nil && p.Value.ObjectID != "" {
			byIndex[i] = p.Value.ObjectID
		}
	}
	handles := make([]string, 0, len(byIndex))
	for i := 0; i < len(byIndex); i++ {
		h, ok := byIndex[i]
		if !ok {
			break
		}
		handles = append(handles, h)
	}
	return handles, nil
}

// ReleaseObjectGroup frees every handle filed under objectGroup.
func ReleaseObjectGroup(ctx context.Context, objectGroup string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.releaseObjectGroup",
			map[string]any{"objectGroup": objectGroup}, nil)
	}))
}

func evaluateParams(execID int64, expression string, awaitPromise, byValue bool) map[string]any {
	return map[string]any{
		"expression":    expression,
		"contextId":     execID,
		"returnByValue": byValue,
		"awaitPromise":  awaitPromise,
	}
}

func callParams(objectID, functionDecl string, args []map[string]any, byValue bool) map[string]any {
	params := map[string]any{
		"functionDeclaration": functionDecl,
		"objectId":            objectID,
		"returnByValue":       byValue,
	}
	if len(args) > 0 {
		params["arguments"] = args
	}
	return params
}

type remoteObject struct {
	ObjectID string          `json:"objectId"`
	Value    json.RawMessage `json:"value"`
}

func (o remoteObject) decode(result any) error {
	if result == nil || len(o.Value) == 0 {
		return nil
	}
	return json.Unmarshal(o.Value, result)
}

func runScript(ctx context.Context, method string, params map[string]any) (remoteObject, error) {
	var raw json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, method, params, &raw)
	})); err != nil {
		return remoteObject{}, err
	}

	var parsed struct {
		Result           remoteObject      `json:"result"`
		ExceptionDetails *exceptionDetails `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return remoteObject{}, fmt.Errorf("parse result: %w", err)
	}
	if parsed.ExceptionDetails != nil {
		return remoteObject{}, errors.New(parsed.ExceptionDetails.message())
	}
	return parsed.Result, nil
}

// exceptionDetails is the part of CDP's Runtime.ExceptionDetails a caller can act
// on. Text alone is usually just "Uncaught"; the thrown error's message lives in
// the exception's description.
type exceptionDetails struct {
	Text      string `json:"text"`
	Exception *struct {
		Description string `json:"description"`
	} `json:"exception,omitempty"`
}

func (d *exceptionDetails) message() string {
	desc := ""
	if d.Exception != nil {
		desc = d.Exception.Description
	}
	switch {
	case d.Text != "" && desc != "":
		return d.Text + ": " + desc
	case desc != "":
		return desc
	case d.Text != "":
		return d.Text
	default:
		return "script threw an exception"
	}
}

// topFrameID reads the top frame's id, the frame IsolatedContextID falls back to
// when the caller names none.
func topFrameID(ctx context.Context) (string, error) {
	// GetFrameTree issues a CDP call, so it needs an executor context; calling it
	// with the caller's bare ctx fails with "invalid context".
	var frameID string
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		tree, err := GetFrameTree(ctx)
		if err != nil {
			return err
		}
		if tree != nil && tree.Frame != nil {
			frameID = tree.Frame.ID.String()
		}
		return nil
	})); err != nil {
		return "", fmt.Errorf("resolve top frame: %w", err)
	}
	if frameID == "" {
		return "", fmt.Errorf("resolve top frame: frame id is empty")
	}
	return frameID, nil
}

// BackendNodeIDForHandle maps an object handle back to the backend node id the
// ref cache and every DOM.* call key on.
func BackendNodeIDForHandle(ctx context.Context, objectID string) (int64, error) {
	var desc struct {
		Node struct {
			BackendNodeID int64 `json:"backendNodeId"`
		} `json:"node"`
	}
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{"objectId": objectID}, &desc)
	}))
	if err != nil {
		return 0, fmt.Errorf("describe node: %w", err)
	}
	return desc.Node.BackendNodeID, nil
}
