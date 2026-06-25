package bridge

import (
	"context"
	"testing"
)

// TestDispatchNamedKey_RecognisedKeys checks that namedKeyDefs contains the
// keys most commonly used in automation scripts.
func TestDispatchNamedKey_RecognisedKeys(t *testing.T) {
	mustBeKnown := []string{
		"Enter", "Return", "Tab", "Escape", "Backspace", "Delete",
		"ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown",
		"Home", "End", "PageUp", "PageDown",
		"F1", "F5", "F12",
	}
	for _, k := range mustBeKnown {
		if _, ok := namedKeyDefs[k]; !ok {
			t.Errorf("namedKeyDefs is missing key %q", k)
		}
	}
}

// TestDispatchNamedKey_EnterInsertText verifies that the Enter key definition
// produces a "\r" insertText payload so that form submissions and textareas
// receive a newline rather than the literal string "Enter".
func TestDispatchNamedKey_EnterInsertText(t *testing.T) {
	def := namedKeyDefs["Enter"]
	if def.insertText != "\r" {
		t.Errorf("Enter key should insert \\r, got %q", def.insertText)
	}
	if def.code != "Enter" {
		t.Errorf("Enter key code should be \"Enter\", got %q", def.code)
	}
	if def.virtualKey != 13 {
		t.Errorf("Enter virtual key should be 13, got %d", def.virtualKey)
	}
}

// TestDispatchNamedKey_TabInsertText verifies that Tab produces the "\t"
// insert-text payload so that focus advances in form fields.
func TestDispatchNamedKey_TabInsertText(t *testing.T) {
	def := namedKeyDefs["Tab"]
	if def.insertText != "\t" {
		t.Errorf("Tab key should insert \\t, got %q", def.insertText)
	}
}

// TestDispatchNamedKey_NonPrintableNoInsertText verifies that non-printable
// keys (Escape, ArrowLeft, F5 …) do NOT carry an insertText payload.
func TestDispatchNamedKey_NonPrintableNoInsertText(t *testing.T) {
	nonPrintable := []string{"Escape", "Backspace", "Delete", "ArrowLeft", "F5"}
	for _, k := range nonPrintable {
		def := namedKeyDefs[k]
		if def.insertText != "" {
			t.Errorf("key %q should have empty insertText, got %q", k, def.insertText)
		}
	}
}

// TestDispatchNamedKey_ModifiersNotTyped is the regression test for issue #588:
// modifier keys (Shift/Control/Alt/Meta) must be recognised named keys with an
// empty insertText so they dispatch keyDown/keyUp events instead of falling
// through to chromedp.KeyEvent, which would type the literal name (e.g. "Shift")
// into the focused field.
func TestDispatchNamedKey_ModifiersNotTyped(t *testing.T) {
	for _, k := range []string{"Shift", "Control", "Alt", "Meta"} {
		def, ok := namedKeyDefs[k]
		if !ok {
			t.Errorf("modifier %q must be a recognised named key (else it gets typed as text)", k)
			continue
		}
		if def.insertText != "" {
			t.Errorf("modifier %q must have empty insertText, got %q (would type literal text)", k, def.insertText)
		}
		if def.code == "" || def.virtualKey == 0 {
			t.Errorf("modifier %q must carry a code and virtualKey, got code=%q vk=%d", k, def.code, def.virtualKey)
		}
	}
}

// TestPrintableShortcutKey verifies the code/virtual-key mapping used to dispatch
// keyboard chords (Ctrl+C, Cmd+A, …) on printable keys.
func TestPrintableShortcutKey(t *testing.T) {
	cases := []struct {
		key      string
		wantCode string
		wantVK   int64
		wantOK   bool
	}{
		{"c", "KeyC", 67, true}, // Ctrl+C
		{"a", "KeyA", 65, true}, // Cmd/Ctrl+A
		{"C", "KeyC", 67, true}, // already-uppercase maps the same
		{"5", "Digit5", 53, true},
		{"Enter", "", 0, false}, // multi-char → not a printable shortcut key
		{"-", "", 0, false},     // unmapped punctuation
	}
	for _, tc := range cases {
		code, vk, ok := printableShortcutKey(tc.key)
		if ok != tc.wantOK || code != tc.wantCode || vk != tc.wantVK {
			t.Errorf("printableShortcutKey(%q) = (%q,%d,%v), want (%q,%d,%v)",
				tc.key, code, vk, ok, tc.wantCode, tc.wantVK, tc.wantOK)
		}
	}
}

// TestDispatchNamedKey_ReturnAlias verifies that "Return" is an alias for
// Enter and produces the same CDP parameters.
func TestDispatchNamedKey_ReturnAlias(t *testing.T) {
	enter := namedKeyDefs["Enter"]
	ret := namedKeyDefs["Return"]
	if enter != ret {
		t.Error("\"Return\" keyDef should equal \"Enter\" keyDef")
	}
}

// TestDispatchNamedKey_FallbackOnCancelledCtx verifies that an unrecognised key
// ("a") falls back to chromedp.KeyEvent and returns an error on a cancelled
// context rather than silently succeeding.
func TestDispatchNamedKey_FallbackOnCancelledCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// "a" is not in namedKeyDefs → falls back to chromedp.KeyEvent
	err := DispatchNamedKey(ctx, "a", 0)
	if err == nil {
		t.Error("expected error dispatching key on cancelled context")
	}
}

// TestDispatchNamedKey_KnownKeyOnCancelledCtx verifies that a known named key
// ("Enter") also returns an error on a cancelled context.
func TestDispatchNamedKey_KnownKeyOnCancelledCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DispatchNamedKey(ctx, "Enter", 0)
	if err == nil {
		t.Error("expected error dispatching Enter on cancelled context")
	}
}
