package bridge

import (
	"context"

	"github.com/chromedp/chromedp"
)

func targetNodeID(ctx context.Context, nodeID int64, css string) (int64, error) {
	if nodeID > 0 || css == "" {
		return nodeID, nil
	}
	return ResolveCSSToNodeID(ctx, css)
}

func focusBackendNode(ctx context.Context, nodeID int64) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
	)
}
