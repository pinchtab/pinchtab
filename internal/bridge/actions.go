package bridge

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/human"
)

const (
	ActionClick      = "click"
	ActionType       = "type"
	ActionFill       = "fill"
	ActionPress      = "press"
	ActionFocus      = "focus"
	ActionHover      = "hover"
	ActionSelect     = "select"
	ActionScroll     = "scroll"
	ActionHumanClick = "humanClick"
	ActionHumanType  = "humanType"
)

func (b *Bridge) InitActionRegistry() {
	b.Actions = map[string]ActionFunc{
		ActionClick: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			var err error
			if req.Selector != "" {
				err = chromedp.Run(ctx, chromedp.Click(req.Selector, chromedp.ByQuery))
			} else if req.NodeID > 0 {
				err = ClickByNodeID(ctx, req.NodeID)
			} else {
				return nil, fmt.Errorf("need selector, ref, or nodeId")
			}
			if err != nil {
				return nil, err
			}
			if req.WaitNav {
				_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
			}
			return map[string]any{"clicked": true}, nil
		},
		ActionType: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Text == "" {
				return nil, fmt.Errorf("text required for type")
			}
			if req.Selector != "" {
				return map[string]any{"typed": req.Text}, chromedp.Run(ctx,
					chromedp.Click(req.Selector, chromedp.ByQuery),
					chromedp.SendKeys(req.Selector, req.Text, chromedp.ByQuery),
				)
			}
			if req.NodeID > 0 {
				return map[string]any{"typed": req.Text}, TypeByNodeID(ctx, req.NodeID, req.Text)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		ActionFill: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Selector != "" {
				return map[string]any{"filled": req.Text}, chromedp.Run(ctx, chromedp.SetValue(req.Selector, req.Text, chromedp.ByQuery))
			}
			return map[string]any{"filled": req.Text}, nil
		},
		ActionPress: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Key == "" {
				return nil, fmt.Errorf("key required for press")
			}
			return map[string]any{"pressed": req.Key}, chromedp.Run(ctx, chromedp.KeyEvent(req.Key))
		},
		ActionFocus: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Selector != "" {
				return map[string]any{"focused": true}, chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery))
			}
			if req.NodeID > 0 {
				return map[string]any{"focused": true}, chromedp.Run(ctx,
					chromedp.ActionFunc(func(ctx context.Context) error {
						p := map[string]any{"backendNodeId": req.NodeID}
						return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", p, nil)
					}),
				)
			}
			return map[string]any{"focused": true}, nil
		},
		ActionHover: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				return map[string]any{"hovered": true}, HoverByNodeID(ctx, req.NodeID)
			}
			if req.Selector != "" {
				return map[string]any{"hovered": true}, chromedp.Run(ctx,
					chromedp.Evaluate(fmt.Sprintf(`document.querySelector(%q)?.dispatchEvent(new MouseEvent('mouseover', {bubbles:true}))`, req.Selector), nil),
				)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		ActionSelect: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			val := req.Value
			if val == "" {
				val = req.Text
			}
			if val == "" {
				return nil, fmt.Errorf("value required for select")
			}
			if req.NodeID > 0 {
				return map[string]any{"selected": val}, SelectByNodeID(ctx, req.NodeID, val)
			}
			if req.Selector != "" {
				return map[string]any{"selected": val}, chromedp.Run(ctx,
					chromedp.SetValue(req.Selector, val, chromedp.ByQuery),
				)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		ActionScroll: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				return map[string]any{"scrolled": true}, ScrollByNodeID(ctx, req.NodeID)
			}
			if req.ScrollX != 0 || req.ScrollY != 0 {
				js := fmt.Sprintf("window.scrollBy(%d, %d)", req.ScrollX, req.ScrollY)
				return map[string]any{"scrolled": true, "x": req.ScrollX, "y": req.ScrollY},
					chromedp.Run(ctx, chromedp.Evaluate(js, nil))
			}
			return map[string]any{"scrolled": true, "y": 800},
				chromedp.Run(ctx, chromedp.Evaluate("window.scrollBy(0, 800)", nil))
		},
		ActionHumanClick: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				// req.NodeID is a backendDOMNodeId from the accessibility tree
				if err := human.ClickElement(ctx, cdp.BackendNodeID(req.NodeID)); err != nil {
					return nil, err
				}
				return map[string]any{"clicked": true, "human": true}, nil
			}
			if req.Selector != "" {
				var nodes []*cdp.Node
				if err := chromedp.Run(ctx,
					chromedp.Nodes(req.Selector, &nodes, chromedp.ByQuery),
				); err != nil {
					return nil, err
				}
				if len(nodes) == 0 {
					return nil, fmt.Errorf("element not found: %s", req.Selector)
				}
				// Use BackendNodeID from the DOM node
				if err := human.ClickElement(ctx, nodes[0].BackendNodeID); err != nil {
					return nil, err
				}
				return map[string]any{"clicked": true, "human": true}, nil
			}
			return nil, fmt.Errorf("need selector, ref, or nodeId")
		},
		ActionHumanType: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Text == "" {
				return nil, fmt.Errorf("text required for humanType")
			}

			if req.Selector != "" {
				if err := chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery)); err != nil {
					return nil, err
				}
			} else if req.NodeID > 0 {
				if err := chromedp.Run(ctx,
					chromedp.ActionFunc(func(ctx context.Context) error {
						return dom.Focus().WithNodeID(cdp.NodeID(req.NodeID)).Do(ctx)
					}),
				); err != nil {
					return nil, err
				}
			} else {
				return nil, fmt.Errorf("need selector, ref, or nodeId")
			}

			actions := human.Type(req.Text, req.Fast)
			if err := chromedp.Run(ctx, actions...); err != nil {
				return nil, err
			}

			return map[string]any{"typed": req.Text, "human": true}, nil
		},
	}
}
