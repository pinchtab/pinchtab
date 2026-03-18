package bridge

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

func (b *Bridge) actionType(ctx context.Context, req ActionRequest) (map[string]any, error) {
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
}

func (b *Bridge) actionFill(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Selector != "" {
		return map[string]any{"filled": req.Text}, chromedp.Run(ctx, chromedp.SetValue(req.Selector, req.Text, chromedp.ByQuery))
	}
	if req.NodeID > 0 {
		return map[string]any{"filled": req.Text}, FillByNodeID(ctx, req.NodeID, req.Text)
	}
	return nil, fmt.Errorf("need selector or ref")
}

func (b *Bridge) actionPress(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Key == "" {
		return nil, fmt.Errorf("key required for press")
	}
	return map[string]any{"pressed": req.Key}, DispatchNamedKey(ctx, req.Key)
}

func (b *Bridge) actionHumanType(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text required for humanType")
	}

	if req.Selector != "" {
		if err := chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery)); err != nil {
			return nil, err
		}
	} else if req.NodeID > 0 {
		// req.NodeID is a BackendNodeID from the accessibility tree (same as humanClick).
		// Must use DOM.focus with backendNodeId, not dom.Focus().WithNodeID() which
		// expects a DOM NodeID — a different ID space. Using the wrong type causes
		// "Could not find node with given id (-32000)". See issue #226.
		if err := focusBackendNode(ctx, req.NodeID); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("need selector, ref, or nodeId")
	}

	actions := Type(req.Text, req.Fast)
	if err := chromedp.Run(ctx, actions...); err != nil {
		return nil, err
	}

	return map[string]any{"typed": req.Text, "human": true}, nil
}

func (b *Bridge) actionKeyboardType(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text required for keyboard-type")
	}
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		for _, ch := range req.Text {
			s := string(ch)
			params := map[string]any{
				"type":                  "keyDown",
				"text":                  s,
				"key":                   s,
				"unmodifiedText":        s,
				"windowsVirtualKeyCode": int(ch),
				"nativeVirtualKeyCode":  int(ch),
			}
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchKeyEvent", params, nil); err != nil {
				return err
			}
			paramsUp := map[string]any{
				"type":                  "keyUp",
				"key":                   s,
				"windowsVirtualKeyCode": int(ch),
				"nativeVirtualKeyCode":  int(ch),
			}
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchKeyEvent", paramsUp, nil); err != nil {
				return err
			}
		}
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return map[string]any{"typed": req.Text}, nil
}

func (b *Bridge) actionKeyboardInsert(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text required for keyboard-inserttext")
	}
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.insertText", map[string]any{
			"text": req.Text,
		}, nil)
	}))
	if err != nil {
		return nil, err
	}
	return map[string]any{"inserted": req.Text}, nil
}

func (b *Bridge) actionKeyDown(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Key == "" {
		return nil, fmt.Errorf("key required for keydown")
	}
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		params := map[string]any{"type": "keyDown", "key": req.Key}
		if def, ok := namedKeyDefs[req.Key]; ok {
			params["code"] = def.code
			params["windowsVirtualKeyCode"] = def.virtualKey
			params["nativeVirtualKeyCode"] = def.virtualKey
		}
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchKeyEvent", params, nil)
	}))
	if err != nil {
		return nil, err
	}
	return map[string]any{"keydown": req.Key}, nil
}

func (b *Bridge) actionKeyUp(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Key == "" {
		return nil, fmt.Errorf("key required for keyup")
	}
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		params := map[string]any{"type": "keyUp", "key": req.Key}
		if def, ok := namedKeyDefs[req.Key]; ok {
			params["code"] = def.code
			params["windowsVirtualKeyCode"] = def.virtualKey
			params["nativeVirtualKeyCode"] = def.virtualKey
		}
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchKeyEvent", params, nil)
	}))
	if err != nil {
		return nil, err
	}
	return map[string]any{"keyup": req.Key}, nil
}
