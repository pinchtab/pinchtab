package bridge

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

func firstNodeBySelector(ctx context.Context, selector string) (*cdp.Node, error) {
	var nodes []*cdp.Node
	if err := chromedp.Run(ctx,
		chromedp.Nodes(selector, &nodes, chromedp.ByQuery),
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
