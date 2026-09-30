package cdpops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/runtimetypes"
)

// IsolatedNodeObjectID converts a backend node id to a JS object handle in the
// top frame's isolated world. The rule and the world both have one owner in
// internal/cdptk, the lowest CDP layer: DOM.resolveNode without an
// executionContextId hands back a main-world object, so every
// Runtime.callFunctionOn against it runs where page script can redefine the DOM
// methods it calls.
//
// JS invoked on the returned handle must not read ambient globals. The handle
// lives in the world it was resolved into, not necessarily the node's frame, so
// `window` and `document` there are the top frame's. Derive them from the node
// via ownerDocument/defaultView instead.
func IsolatedNodeObjectID(ctx context.Context, backendNodeID int64) (string, error) {
	return cdptk.IsolatedNodeObjectID(ctx, backendNodeID)
}

// FrameExecutionContextID returns a Runtime.executionContextId that evaluates
// in the given frame's document, minted by the one owner in internal/cdptk.
//
// Returns (0, nil) when frameID is empty, and that check stays HERE rather than
// being handed to the owner: callers branch on the zero to fall back to the
// context they already have, whereas an empty frame means the top frame's
// isolated world one layer down. Collapsing the two spellings of "empty" would
// silently move every unscoped caller into a freshly minted world.
func FrameExecutionContextID(ctx context.Context, frameID string) (int64, error) {
	if frameID == "" {
		return 0, nil
	}
	return cdptk.IsolatedContextID(ctx, frameID)
}

var resolveNodeAction = func(ctx context.Context, backendNodeID int64) (json.RawMessage, error) {
	var resolved json.RawMessage
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &resolved)
	}))
	return resolved, err
}

// ResolveNodeObjectID converts a backend node id to a main-world JS object handle,
// refusing a node that no longer resolves to an object.
func ResolveNodeObjectID(ctx context.Context, backendNodeID int64) (string, error) {
	raw, err := resolveNodeAction(ctx, backendNodeID)
	if err != nil {
		return "", fmt.Errorf("resolve node: %w", err)
	}
	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(raw, &resolved); err != nil {
		return "", fmt.Errorf("parse resolved node: %w", err)
	}
	if strings.TrimSpace(resolved.Object.ObjectID) == "" {
		return "", fmt.Errorf("element not found in DOM (backendNodeId=%d)", backendNodeID)
	}
	return resolved.Object.ObjectID, nil
}

// CallFunctionOnNode resolves a backend node to a JS object and invokes the
// given function declaration on it, decoding the (by-value) result into result
// when non-nil.
func CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	objectID, err := ResolveNodeObjectID(ctx, backendNodeID)
	if err != nil {
		return err
	}
	return callFunctionOnObject(ctx, objectID, functionDecl, args, result)
}

func callFunctionOnObject(ctx context.Context, objectID, functionDecl string, args []map[string]any, result any) error {
	params := map[string]any{
		"functionDeclaration": functionDecl,
		"objectId":            objectID,
		"returnByValue":       true,
	}
	if len(args) > 0 {
		params["arguments"] = args
	}

	var callResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", params, &callResult)
	})); err != nil {
		return fmt.Errorf("call function on node: %w", err)
	}

	var callParsed struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(callResult, &callParsed); err != nil {
		return fmt.Errorf("parse call result: %w", err)
	}
	if callParsed.ExceptionDetails != nil && callParsed.ExceptionDetails.Text != "" {
		return fmt.Errorf("call function on node: %s", callParsed.ExceptionDetails.Text)
	}

	if result == nil || len(callParsed.Result.Value) == 0 {
		return nil
	}
	return json.Unmarshal(callParsed.Result.Value, result)
}

// DescribeNode fetches DOM.describeNode metadata for a backend node, falling
// back to the node name when localName is empty.
func DescribeNode(ctx context.Context, backendNodeID int64) (*runtimetypes.NodeInfo, error) {
	var info runtimetypes.NodeInfo
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var result json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &result); err != nil {
			return fmt.Errorf("describe node: %w", err)
		}

		var parsed struct {
			Node struct {
				LocalName      string   `json:"localName"`
				NodeName       string   `json:"nodeName"`
				Attributes     []string `json:"attributes"`
				ChildNodeCount int      `json:"childNodeCount"`
			} `json:"node"`
		}
		if err := json.Unmarshal(result, &parsed); err != nil {
			return fmt.Errorf("parse describe node: %w", err)
		}

		info.LocalName = parsed.Node.LocalName
		if info.LocalName == "" {
			info.LocalName = parsed.Node.NodeName
		}
		info.Attributes = parsed.Node.Attributes
		info.ChildNodeCount = parsed.Node.ChildNodeCount
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return &info, nil
}
