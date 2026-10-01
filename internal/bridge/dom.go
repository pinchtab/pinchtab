package bridge

import (
	"context"
	"fmt"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

func (b *Bridge) CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return bridgecdpops.CallFunctionOnNode(ctx, backendNodeID, functionDecl, args, result)
}

// If frameID is empty, behaves like Evaluate.
func (b *Bridge) EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts EvalOpts) error {
	if frameID == "" {
		return b.Evaluate(ctx, expression, result, opts)
	}

	execID, err := bridgecdpops.FrameExecutionContextID(ctx, frameID)
	if err != nil {
		return fmt.Errorf("resolve frame context: %w", err)
	}
	if execID == 0 {
		// Top frame — use the simpler bridge path.
		return b.Evaluate(ctx, expression, result, opts)
	}

	return cdptk.EvaluateInContext(ctx, execID, expression, opts.AwaitPromise, result)
}

func (b *Bridge) DescribeNode(ctx context.Context, backendNodeID int64) (*NodeInfo, error) {
	return bridgecdpops.DescribeNode(ctx, backendNodeID)
}
