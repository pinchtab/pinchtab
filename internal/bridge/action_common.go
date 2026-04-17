package bridge

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

// firstNodeBySelector resolves a CSS selector to the first matching node.
// Uses chromedp.AtLeast(0) so a missing selector returns immediately with a
// clear "element not found" error instead of waiting the full action timeout
// (~30 s) for the element to appear. Callers that need to wait for dynamic
// content should use `/wait --selector ...` explicitly before the action.
func firstNodeBySelector(ctx context.Context, selector string) (*cdp.Node, error) {
	var nodes []*cdp.Node
	if err := chromedp.Run(ctx,
		chromedp.Nodes(selector, &nodes, chromedp.ByQuery, chromedp.AtLeast(0)),
	); err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("element not found: %s", selector)
	}
	return nodes[0], nil
}

func focusBackendNode(ctx context.Context, nodeID int64) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
	)
}
