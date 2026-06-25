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
	"Enter":  {"Enter", 13, "\r"},
	"Return": {"Enter", 13, "\r"},
	"Tab":    {"Tab", 9, "\t"},
	"Escape": {"Escape", 27, ""},
	// Modifier keys must dispatch keyDown/keyUp events, never text. Without these
	// entries they fall through to chromedp.KeyEvent and the literal name ("Shift")
	// is typed into the focused field (issue #588). insertText is empty so the
	// browser receives a real modifier key event with no character.
	"Shift":      {"ShiftLeft", 16, ""},
	"Control":    {"ControlLeft", 17, ""},
	"Alt":        {"AltLeft", 18, ""},
	"Meta":       {"MetaLeft", 91, ""},
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

// printableShortcutKey returns the CDP code and Windows virtual-key code for a
// single printable ASCII key. It is used to dispatch keyboard shortcuts such as
// Ctrl+C / Cmd+A where the key is not in namedKeyDefs but must still carry a real
// code/virtualKey so the page recognises the chord. ok is false for anything we
// cannot map (the caller then falls back to typing the key).
func printableShortcutKey(key string) (code string, vk int64, ok bool) {
	if len(key) != 1 {
		return "", 0, false
	}
	c := key[0]
	switch {
	case c >= 'a' && c <= 'z':
		u := c - ('a' - 'A')
		return "Key" + string(rune(u)), int64(u), true
	case c >= 'A' && c <= 'Z':
		return "Key" + string(rune(c)), int64(c), true
	case c >= '0' && c <= '9':
		return "Digit" + string(rune(c)), int64(c), true
	}
	return "", 0, false
}

// editingCommand maps a Ctrl/Cmd shortcut to the editing command CDP applies via
// the keyDown "commands" field. On macOS the built-in editor only performs these
// actions (select-all, copy, …) when the command name is supplied; Linux/Windows
// derive them from the key event directly, where the empty default is harmless.
func editingCommand(key string, modifiers int) string {
	const ctrl, meta, shift = 2, 4, 8
	if modifiers&(ctrl|meta) == 0 {
		return ""
	}
	switch key {
	case "a", "A":
		return "selectAll"
	case "c", "C":
		return "copy"
	case "v", "V":
		return "paste"
	case "x", "X":
		return "cut"
	case "z", "Z":
		if modifiers&shift != 0 {
			return "redo"
		}
		return "undo"
	case "y", "Y":
		return "redo"
	}
	return ""
}

// DispatchNamedKey dispatches key with the given CDP modifier bitmask
// (Alt=1, Ctrl=2, Meta=4, Shift=8). Unrecognised keys with no modifiers fall
// back to chromedp.KeyEvent (types the key); with modifiers they are dispatched
// as a shortcut so chords like Ctrl+C / Cmd+A reach the page.
func DispatchNamedKey(ctx context.Context, key string, modifiers int) error {
	def, ok := namedKeyDefs[key]
	if !ok {
		if modifiers != 0 {
			if code, vk, mapped := printableShortcutKey(key); mapped {
				return dispatchKeyChord(ctx, key, code, vk, "", modifiers, editingCommand(key, modifiers))
			}
		}
		return chromedp.Run(ctx, chromedp.KeyEvent(key))
	}

	w3cKey := key
	if key == "Return" {
		w3cKey = "Enter"
	}

	// A named key with modifiers is a shortcut (Shift+ArrowRight selects,
	// Ctrl+Backspace deletes a word): suppress its default text so it isn't
	// also inserted as a character.
	text := def.insertText
	if modifiers != 0 {
		text = ""
	}
	return dispatchKeyChord(ctx, w3cKey, def.code, def.virtualKey, text, modifiers, "")
}

// dispatchKeyChord sends a keyDown+keyUp pair via CDP, applying the modifier
// bitmask. When text is non-empty the keyDown uses type "keyDown" and carries the
// text (fires keypress + default actions like form submit); otherwise it uses
// "rawKeyDown" for non-character / shortcut keys.
func dispatchKeyChord(ctx context.Context, key, code string, vk int64, text string, modifiers int, command string) error {
	params := func(evType string, isKeyDown bool) map[string]any {
		p := map[string]any{
			"type":                  evType,
			"key":                   key,
			"code":                  code,
			"windowsVirtualKeyCode": vk,
			"nativeVirtualKeyCode":  vk,
		}
		if modifiers != 0 {
			p["modifiers"] = modifiers
		}
		if isKeyDown && text != "" {
			p["text"] = text
			p["unmodifiedText"] = text
		}
		// The editing command rides on the keyDown so macOS performs the action.
		if isKeyDown && command != "" {
			p["commands"] = []string{command}
		}
		return p
	}

	downType := "rawKeyDown"
	if text != "" {
		downType = "keyDown"
	}
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			return chromedp.FromContext(c).Target.Execute(c, "Input.dispatchKeyEvent", params(downType, true), nil)
		}),
		chromedp.ActionFunc(func(c context.Context) error {
			return chromedp.FromContext(c).Target.Execute(c, "Input.dispatchKeyEvent", params("keyUp", false), nil)
		}),
	)
}

func TypeByNodeID(ctx context.Context, nodeID int64, text string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.KeyEvent(text),
	)
}
