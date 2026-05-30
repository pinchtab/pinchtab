package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
)

// CallFunctionOnNode resolves a backend node ID to a Runtime object, then
// calls the given JavaScript function on it. args may be nil. The result
// is unmarshaled from the CDP returnByValue response.
func (b *Bridge) CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Step 1: Resolve backend node ID to a remote object.
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

		// Step 2: Call the function on the resolved object.
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

		// Step 3: Parse the result.
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

// EvaluateInFrame evaluates a JavaScript expression in the given frame's
// execution context. If frameID is empty, behaves like Evaluate.
func (b *Bridge) EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts EvalOpts) error {
	if frameID == "" {
		return b.Evaluate(ctx, expression, result, opts)
	}

	execID, err := FrameExecutionContextID(ctx, frameID)
	if err != nil {
		return fmt.Errorf("resolve frame context: %w", err)
	}
	if execID == 0 {
		// Top frame — use the simpler bridge path.
		return b.Evaluate(ctx, expression, result, opts)
	}

	var raw json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    expression,
			"returnByValue": true,
			"contextId":     execID,
		}, &raw)
	}))
	if err != nil {
		return err
	}

	var parsed struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("evaluate in frame parse: %w", err)
	}
	if parsed.ExceptionDetails != nil && parsed.ExceptionDetails.Text != "" {
		return fmt.Errorf("%s", parsed.ExceptionDetails.Text)
	}
	if result == nil || len(parsed.Result.Value) == 0 {
		return nil
	}
	return json.Unmarshal(parsed.Result.Value, result)
}

// DescribeNode returns DOM structural info for a backend node ID.
func (b *Bridge) DescribeNode(ctx context.Context, backendNodeID int64) (*NodeInfo, error) {
	var info NodeInfo
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
