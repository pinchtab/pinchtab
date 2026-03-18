package bridge

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

func (b *Bridge) actionClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	var err error
	if req.Selector != "" {
		err = chromedp.Run(ctx, chromedp.Click(req.Selector, chromedp.ByQuery))
	} else if req.NodeID > 0 {
		err = ClickByNodeID(ctx, req.NodeID)
	} else if req.HasXY {
		err = ClickByCoordinate(ctx, req.X, req.Y)
	} else {
		return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
	}
	if err != nil {
		return nil, err
	}
	if req.WaitNav {
		_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
	}
	return map[string]any{"clicked": true}, nil
}

func (b *Bridge) actionDoubleClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	var err error
	if req.Selector != "" {
		err = chromedp.Run(ctx, chromedp.DoubleClick(req.Selector, chromedp.ByQuery))
	} else if req.NodeID > 0 {
		err = DoubleClickByNodeID(ctx, req.NodeID)
	} else if req.HasXY {
		err = DoubleClickByCoordinate(ctx, req.X, req.Y)
	} else {
		return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"doubleclicked": true}, nil
}

func (b *Bridge) actionHover(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.NodeID > 0 {
		return map[string]any{"hovered": true}, HoverByNodeID(ctx, req.NodeID)
	}
	if req.Selector != "" {
		return map[string]any{"hovered": true}, chromedp.Run(ctx,
			chromedp.Evaluate(fmt.Sprintf(`document.querySelector(%q)?.dispatchEvent(new MouseEvent('mouseover', {bubbles:true}))`, req.Selector), nil),
		)
	}
	if req.HasXY {
		return map[string]any{"hovered": true}, HoverByCoordinate(ctx, req.X, req.Y)
	}
	return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
}

func (b *Bridge) actionScroll(ctx context.Context, req ActionRequest) (map[string]any, error) {
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
}

func (b *Bridge) actionDrag(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.DragX == 0 && req.DragY == 0 {
		return nil, fmt.Errorf("dragX or dragY required for drag")
	}
	if req.NodeID > 0 {
		err := DragByNodeID(ctx, req.NodeID, req.DragX, req.DragY)
		if err != nil {
			return nil, err
		}
		return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY}, nil
	}
	if req.Selector != "" {
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		err = DragByNodeID(ctx, int64(node.BackendNodeID), req.DragX, req.DragY)
		if err != nil {
			return nil, err
		}
		return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY}, nil
	}
	return nil, fmt.Errorf("need selector, ref, or nodeId")
}

func (b *Bridge) actionHumanClick(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.NodeID > 0 {
		// req.NodeID is a backendDOMNodeId from the accessibility tree
		if err := ClickElement(ctx, cdp.BackendNodeID(req.NodeID)); err != nil {
			return nil, err
		}
		return map[string]any{"clicked": true, "human": true}, nil
	}
	if req.Selector != "" {
		node, err := firstNodeBySelector(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		// Use BackendNodeID from the DOM node
		if err := ClickElement(ctx, node.BackendNodeID); err != nil {
			return nil, err
		}
		return map[string]any{"clicked": true, "human": true}, nil
	}
	return nil, fmt.Errorf("need selector, ref, or nodeId")
}

func (b *Bridge) actionScrollIntoView(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.NodeID > 0 {
		return ScrollIntoViewAndGetBox(ctx, req.NodeID)
	}
	if req.Selector != "" {
		nid, err := ResolveCSSToNodeID(ctx, req.Selector)
		if err != nil {
			return nil, err
		}
		return ScrollIntoViewAndGetBox(ctx, nid)
	}
	return nil, fmt.Errorf("need selector or ref")
}
