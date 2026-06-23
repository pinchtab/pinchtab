package cdpops

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"

	"github.com/pinchtab/pinchtab/internal/runtimetypes"
)

// FrameExecutionContextID returns a Runtime.executionContextId that evaluates
// in the given frame's document. Returns (0, nil) when frameID is empty so
// callers can fall back to the default top-level context without branching.
func FrameExecutionContextID(ctx context.Context, frameID string) (int64, error) {
	if frameID == "" {
		return 0, nil
	}

	var worldResult json.RawMessage
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Page.createIsolatedWorld", map[string]any{
			"frameId":   frameID,
			"worldName": "pinchtab-frame-scope",
		}, &worldResult)
	}))
	if err != nil {
		return 0, fmt.Errorf("create isolated world for frame %q: %w", frameID, err)
	}

	var resp struct {
		ExecutionContextID int64 `json:"executionContextId"`
	}
	if err := json.Unmarshal(worldResult, &resp); err != nil {
		return 0, err
	}
	if resp.ExecutionContextID == 0 {
		return 0, fmt.Errorf("frame %q has no execution context", frameID)
	}
	return resp.ExecutionContextID, nil
}

// CallFunctionOnNode resolves a backend node to a JS object and invokes the
// given function declaration on it, decoding the (by-value) result into result
// when non-nil.
func CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var resolveResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &resolveResult); err != nil {
			return fmt.Errorf("resolve node: %w", err)
		}

		var resolved struct {
			Object struct {
				ObjectID string `json:"objectId"`
			} `json:"object"`
		}
		if err := json.Unmarshal(resolveResult, &resolved); err != nil {
			return fmt.Errorf("parse resolved node: %w", err)
		}
		if resolved.Object.ObjectID == "" {
			return fmt.Errorf("element not found in DOM (backendNodeId=%d)", backendNodeID)
		}

		params := map[string]any{
			"functionDeclaration": functionDecl,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}
		if len(args) > 0 {
			params["arguments"] = args
		}

		var callResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", params, &callResult); err != nil {
			return fmt.Errorf("call function on node: %w", err)
		}

		var callParsed struct {
			Result struct {
				Type  string          `json:"type"`
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
	}))
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
