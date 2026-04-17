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
		if err := FillByNodeID(ctx, req.NodeID, req.Text); err != nil {
			return nil, err
		}
		result := map[string]any{"filled": req.Text}
		if actual, err := ReadInputValue(ctx, req.NodeID); err == nil && req.Text != "" && actual != req.Text {
			result["warning"] = "fill may not have been picked up by the page (e.g. React controlled input); try 'type' instead"
		}
		return result, nil
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

// keyboardTypeThreshold is the character count above which we switch from
// per-character key events to batched insertText for performance. Per-char
// events cause timeouts on long strings (issue #413).
const keyboardTypeThreshold = 20

func (b *Bridge) actionKeyboardType(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text required for keyboard-type")
	}

	// For long strings, use insertText to avoid timeout (issue #413).
	// We still fire a keydown at the start and keyup at the end to trigger
	// any key-event listeners that apps might depend on.
	// Use rune count (not byte length) since we're counting keystrokes.
	if len([]rune(req.Text)) > keyboardTypeThreshold {
		return b.keyboardTypeBatched(ctx, req.Text)
	}

	return b.keyboardTypePerChar(ctx, req.Text)
}

// keyboardTypePerChar dispatches individual keyDown/keyUp events for each character.
// Used for short strings where per-character events are acceptable.
func (b *Bridge) keyboardTypePerChar(ctx context.Context, text string) (map[string]any, error) {
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		for _, ch := range text {
			s := string(ch)
			params := map[string]any{
				"type":           "keyDown",
				"text":           s,
				"key":            s,
				"unmodifiedText": s,
			}
			// Only set virtualKeyCode for alphanumeric characters (A-Z, 0-9).
			// Using ASCII values for punctuation like '.' (46) conflicts with
			// special key codes (Delete=46), causing characters to be swallowed.
			// See issue #412.
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
				vk := int(ch)
				if ch >= 'a' && ch <= 'z' {
					vk = int(ch - 32) // Convert to uppercase for VK code
				}
				params["windowsVirtualKeyCode"] = vk
				params["nativeVirtualKeyCode"] = vk
			}
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchKeyEvent", params, nil); err != nil {
				return err
			}
			paramsUp := map[string]any{
				"type": "keyUp",
				"key":  s,
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
	return map[string]any{"typed": text}, nil
}

// keyboardTypeBatchedEdgeChars is how many characters to type with real
// key events at the start and end of a batched string.
const keyboardTypeBatchedEdgeChars = 5

// keyboardTypeBatched types the first and last few characters with real key
// events, and uses Input.insertText for the middle portion. This provides
// realistic keystroke simulation at boundaries while avoiding CDP timeouts
// on long strings (issue #413).
func (b *Bridge) keyboardTypeBatched(ctx context.Context, text string) (map[string]any, error) {
	runes := []rune(text)
	edgeChars := keyboardTypeBatchedEdgeChars

	// If string is short enough, just type the whole thing
	if len(runes) <= edgeChars*2 {
		return b.keyboardTypePerChar(ctx, text)
	}

	head := string(runes[:edgeChars])
	middle := string(runes[edgeChars : len(runes)-edgeChars])
	tail := string(runes[len(runes)-edgeChars:])

	// Type first 5 characters with key events
	if _, err := b.keyboardTypePerChar(ctx, head); err != nil {
		return nil, err
	}

	// Insert middle portion
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.insertText", map[string]any{
			"text": middle,
		}, nil)
	}))
	if err != nil {
		return nil, err
	}

	// Type last 5 characters with key events
	if _, err := b.keyboardTypePerChar(ctx, tail); err != nil {
		return nil, err
	}

	return map[string]any{"typed": text, "batched": true}, nil
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
