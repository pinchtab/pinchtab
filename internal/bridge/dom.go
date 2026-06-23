package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

func (b *Bridge) CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return bridgecdpops.CallFunctionOnNode(ctx, backendNodeID, functionDecl, args, result)
}

// If frameID is empty, behaves like Evaluate.
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

func (b *Bridge) DescribeNode(ctx context.Context, backendNodeID int64) (*NodeInfo, error) {
	return bridgecdpops.DescribeNode(ctx, backendNodeID)
}
