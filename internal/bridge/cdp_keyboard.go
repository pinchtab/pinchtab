package bridge

import (
	"context"

	"github.com/chromedp/chromedp"
)

// namedKeyDefs maps friendly key names (as accepted by the CLI "press" command)
// to their CDP Input.dispatchKeyEvent parameters. Keys not in this table fall
// through to chromedp.KeyEvent so that single printable characters still work.
var namedKeyDefs = map[string]struct {
	code       string
	virtualKey int64
	insertText string // non-empty for keys that produce a character (Enter→\r, Tab→\t)
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

// DispatchNamedKey sends proper CDP keyDown / keyUp events for well-known key
// names (e.g. "Enter", "Tab", "Escape", "ArrowLeft") so that JavaScript event
// handlers receive a KeyboardEvent with the correct key property.
//
// Unlike chromedp.KeyEvent, which treats multi-character strings as text
// sequences and would type "Enter" as five separate characters, this function
// consults namedKeyDefs and emits a single logical keystroke. Unrecognised keys
// fall back to chromedp.KeyEvent so that single printable characters still work.
func DispatchNamedKey(ctx context.Context, key string) error {
	def, ok := namedKeyDefs[key]
	if !ok {
		return chromedp.Run(ctx, chromedp.KeyEvent(key))
	}

	// Normalise "Return" → "Enter" for the W3C key value.
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

	actions := chromedp.Tasks{dispatchEvent("rawKeyDown")}
	if def.insertText != "" {
		insertText := def.insertText
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.insertText", map[string]any{
				"text": insertText,
			}, nil)
		}))
	}
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
