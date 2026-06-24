package bridge

import (
	"context"

	"github.com/chromedp/chromedp"
)

// namedKeyDefs maps friendly key names to CDP Input.dispatchKeyEvent parameters.
// Keys absent from this table fall through to chromedp.KeyEvent.
// insertText non-empty → use "keyDown" (fires keypress + default action); empty → "rawKeyDown".
var namedKeyDefs = map[string]struct {
	code       string
	virtualKey int64
	insertText string
}{
	"Enter":      {"Enter", 13, "\r"},
	"Return":     {"Enter", 13, "\r"},
	"Tab":        {"Tab", 9, "\t"},
	"Escape":     {"Escape", 27, ""},
	"Backspace":  {"Backspace", 8, ""},
	"Delete":     {"Delete", 46, ""},
	"ArrowLeft":  {"ArrowLeft", 37, ""},
	"ArrowRight": {"ArrowRight", 39, ""},
	"ArrowUp":    {"ArrowUp", 38, ""},
	"ArrowDown":  {"ArrowDown", 40, ""},
	"Home":       {"Home", 36, ""},
	"End":        {"End", 35, ""},
	"PageUp":     {"PageUp", 33, ""},
	"PageDown":   {"PageDown", 34, ""},
	"Insert":     {"Insert", 45, ""},
	"Shift":      {"ShiftLeft", 16, ""},
	"Control":    {"ControlLeft", 17, ""},
	"Alt":        {"AltLeft", 18, ""},
	"Meta":       {"MetaLeft", 91, ""},
	"F1":         {"F1", 112, ""},
	"F2":         {"F2", 113, ""},
	"F3":         {"F3", 114, ""},
	"F4":         {"F4", 115, ""},
	"F5":         {"F5", 116, ""},
	"F6":         {"F6", 117, ""},
	"F7":         {"F7", 118, ""},
	"F8":         {"F8", 119, ""},
	"F9":         {"F9", 120, ""},
	"F10":        {"F10", 121, ""},
	"F11":        {"F11", 122, ""},
	"F12":        {"F12", 123, ""},
}

// Unrecognised keys fall back to chromedp.KeyEvent.
func DispatchNamedKey(ctx context.Context, key string) error {
	def, ok := namedKeyDefs[key]
	if !ok {
		return chromedp.Run(ctx, chromedp.KeyEvent(key))
	}

	w3cKey := key
	if key == "Return" {
		w3cKey = "Enter"
	}

	dispatchEvent := func(evType string) chromedp.ActionFunc {
		return chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchKeyEvent", map[string]any{
				"type":                  evType,
				"key":                   w3cKey,
				"code":                  def.code,
				"windowsVirtualKeyCode": def.virtualKey,
				"nativeVirtualKeyCode":  def.virtualKey,
			}, nil)
		})
	}

	// "keyDown" + text field fires keypress and triggers default actions (form submit, tab advance).
	// "rawKeyDown" for non-character keys — no keypress needed.
	downType := "rawKeyDown"
	var downText string
	if def.insertText != "" {
		downType = "keyDown"
		downText = def.insertText
	}
	dispatchKeyDown := chromedp.ActionFunc(func(ctx context.Context) error {
		params := map[string]any{
			"type":                  downType,
			"key":                   w3cKey,
			"code":                  def.code,
			"windowsVirtualKeyCode": def.virtualKey,
			"nativeVirtualKeyCode":  def.virtualKey,
		}
		if downText != "" {
			params["text"] = downText
			params["unmodifiedText"] = downText
		}
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchKeyEvent", params, nil)
	})
	actions := chromedp.Tasks{dispatchKeyDown}
	actions = append(actions, dispatchEvent("keyUp"))

	return chromedp.Run(ctx, actions...)
}

func TypeByNodeID(ctx context.Context, nodeID int64, text string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.KeyEvent(text),
	)
}
