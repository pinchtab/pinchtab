package bridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

func textEntryResult(kind, text string) map[string]any {
	result := map[string]any{
		kind:  true,
		"len": utf8.RuneCountInString(text),
	}
	return result
}

func (b *Bridge) actionType(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text required for type")
	}
	if b.effectiveHumanize(req) {
		return b.actionHumanizedType(ctx, req)
	}
	if req.Selector != "" {
		return textEntryResult("typed", req.Text), chromedp.Run(ctx,
			bridgecdpops.TrustedInputAction(chromedp.Click(req.Selector, chromedp.ByQuery)),
			bridgecdpops.TrustedInputAction(chromedp.SendKeys(req.Selector, req.Text, chromedp.ByQuery)),
		)
	}
	if req.NodeID > 0 {
		return textEntryResult("typed", req.Text), TypeByNodeID(ctx, req.NodeID, req.Text)
	}
	return nil, NewInvalidActionRequestError("need selector or ref")
}

func (b *Bridge) actionFill(ctx context.Context, req ActionRequest) (map[string]any, error) {
	text, _ := FillText(req)
	result := textEntryResult("filled", text)
	if req.Selector != "" {
		var nodes []*cdp.Node
		if err := chromedp.Run(ctx,
			chromedp.Focus(req.Selector, chromedp.ByQuery),
			chromedp.Nodes(req.Selector, &nodes, chromedp.ByQuery),
		); err != nil {
			return nil, err
		}
		setErr := chromedp.Run(ctx, chromedp.SetValue(req.Selector, text, chromedp.ByQuery))
		if err := checkFillLanded(ctx, int64(nodes[0].BackendNodeID), text, result); err != nil {
			return nil, err
		}
		if _, dropped := result["warning"]; setErr != nil && !dropped {
			return nil, setErr
		}
		return finishFill(ctx, result, req.Submit)
	}
	if req.NodeID > 0 {
		if err := bridgecdpops.FillByNodeID(ctx, req.NodeID, text); err != nil {
			var notFillable *bridgecdpops.NotFillableError
			if errors.As(err, &notFillable) {
				return nil, fmt.Errorf("%w: %w", ErrInvalidActionRequest, err)
			}
			return nil, err
		}
		if err := checkFillLanded(ctx, req.NodeID, text, result); err != nil {
			return nil, err
		}
		return finishFill(ctx, result, req.Submit)
	}
	return nil, NewInvalidActionRequestError("need selector or ref")
}

// Compared against what was asked for rather than against "", so a clear that
// did not land is reported too.
func checkFillLanded(ctx context.Context, nodeID int64, text string, result map[string]any) error {
	actual, err := bridgecdpops.ReadInputValue(ctx, nodeID)
	if err != nil || actual == text {
		return nil
	}
	if rejected, err := bridgecdpops.RejectedISOValue(ctx, nodeID, text); err == nil && rejected != nil {
		return fmt.Errorf("%w: %w", ErrInvalidActionRequest, rejected)
	}
	result["warning"] = "fill may not have been picked up by the page (e.g. React controlled input); try 'type' instead"
	return nil
}

func finishFill(ctx context.Context, result map[string]any, submit bool) (map[string]any, error) {
	if !submit {
		return result, nil
	}
	if err := DispatchNamedKey(ctx, "Enter", 0); err != nil {
		return nil, fmt.Errorf("submit filled field: %w", err)
	}
	result["submitted"] = true
	return result, nil
}

func (b *Bridge) actionPress(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Key == "" {
		return nil, fmt.Errorf("key required for press")
	}
	key, chordModifiers, chord, err := parsePressChord(req.Key)
	if err != nil {
		return nil, err
	}
	if chord {
		if req.Modifiers != 0 {
			return nil, fmt.Errorf("press chord %q also supplied modifiers; use one chord form", req.Key)
		}
		req.Key, req.Modifiers = key, chordModifiers
	}
	if req.NodeID > 0 {
		if err := bridgecdpops.FocusBackendNode(ctx, req.NodeID); err != nil {
			return nil, err
		}
	} else if req.Selector != "" {
		if err := chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery)); err != nil {
			return nil, err
		}
	}
	return map[string]any{"pressed": req.Key}, DispatchNamedKey(ctx, req.Key, req.Modifiers)
}

func parsePressChord(value string) (key string, modifiers int, chord bool, err error) {
	if !strings.Contains(value, "+") || value == "+" {
		return value, 0, false, nil
	}
	parts := strings.Split(value, "+")
	if len(parts) < 2 || strings.TrimSpace(parts[len(parts)-1]) == "" {
		return "", 0, true, fmt.Errorf(
			"invalid press chord %q; use modifiers such as Ctrl+A or Shift+ArrowLeft", value,
		)
	}
	seen := 0
	for _, raw := range parts[:len(parts)-1] {
		var bit int
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "alt", "option":
			bit = 1
		case "ctrl", "control":
			bit = 2
		case "meta", "cmd", "command", "super", "win":
			bit = 4
		case "shift":
			bit = 8
		default:
			return "", 0, true, fmt.Errorf(
				"invalid press chord modifier %q; use Ctrl, Alt, Shift, or Meta", strings.TrimSpace(raw),
			)
		}
		if seen&bit != 0 {
			return "", 0, true, fmt.Errorf("duplicate press chord modifier %q", strings.TrimSpace(raw))
		}
		seen |= bit
	}
	return strings.TrimSpace(parts[len(parts)-1]), seen, true, nil
}

func (b *Bridge) actionHumanizedType(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text required for type")
	}

	if req.Selector != "" {
		if err := chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery)); err != nil {
			return nil, err
		}
	} else if req.NodeID > 0 {
		if err := bridgecdpops.FocusBackendNode(ctx, req.NodeID); err != nil {
			return nil, err
		}
	} else {
		return nil, NewInvalidActionRequestError("need selector, ref, or nodeId")
	}

	return b.humanizedTypeFocused(ctx, req)
}

func (b *Bridge) humanizedTypeFocused(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if err := chromedp.Run(ctx, Type(req.Text, req.Fast)...); err != nil {
		return nil, err
	}
	result := textEntryResult("typed", req.Text)
	result["human"] = true
	return result, nil
}

// keyboardTypeThreshold is the character count above which we switch from
// per-character key events to batched insertText for performance. Per-char
// events cause timeouts on long strings (issue #413).
const keyboardTypeThreshold = 20

func (b *Bridge) actionKeyboardType(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text required for keyboard-type")
	}

	if b.effectiveHumanize(req) {
		if req.Selector == "" && req.NodeID <= 0 {
			return b.humanizedTypeFocused(ctx, req)
		}
		return b.actionHumanizedType(ctx, req)
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
			if err := bridgecdpops.KeyEventAction(params).Do(ctx); err != nil {
				return err
			}
			paramsUp := map[string]any{
				"type": "keyUp",
				"key":  s,
			}
			if err := bridgecdpops.KeyEventAction(paramsUp).Do(ctx); err != nil {
				return err
			}
		}
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return textEntryResult("typed", text), nil
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

	if len(runes) <= edgeChars*2 {
		return b.keyboardTypePerChar(ctx, text)
	}

	head := string(runes[:edgeChars])
	middle := string(runes[edgeChars : len(runes)-edgeChars])
	tail := string(runes[len(runes)-edgeChars:])

	if _, err := b.keyboardTypePerChar(ctx, head); err != nil {
		return nil, err
	}

	err := chromedp.Run(ctx, bridgecdpops.InsertTextAction(middle))
	if err != nil {
		return nil, err
	}

	if _, err := b.keyboardTypePerChar(ctx, tail); err != nil {
		return nil, err
	}

	result := textEntryResult("typed", text)
	result["batched"] = true
	return result, nil
}

func (b *Bridge) actionKeyboardInsert(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Text == "" {
		return nil, fmt.Errorf("text required for keyboard-inserttext")
	}
	err := chromedp.Run(ctx, bridgecdpops.InsertTextAction(req.Text))
	if err != nil {
		return nil, err
	}
	return textEntryResult("inserted", req.Text), nil
}

func (b *Bridge) actionKeyDown(ctx context.Context, req ActionRequest) (map[string]any, error) {
	return b.dispatchSingleKeyEvent(ctx, req, "keyDown", ActionKeyDown)
}

func (b *Bridge) actionKeyUp(ctx context.Context, req ActionRequest) (map[string]any, error) {
	return b.dispatchSingleKeyEvent(ctx, req, "keyUp", ActionKeyUp)
}

func (b *Bridge) dispatchSingleKeyEvent(ctx context.Context, req ActionRequest, eventType, resultKey string) (map[string]any, error) {
	if req.Key == "" {
		return nil, fmt.Errorf("key required for %s", resultKey)
	}
	if err := dispatchNamedKeyEvent(ctx, req.Key, eventType); err != nil {
		return nil, err
	}
	return map[string]any{resultKey: req.Key}, nil
}
