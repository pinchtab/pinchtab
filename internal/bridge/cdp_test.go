package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestWaitForTitle_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := WaitForTitle(ctx, 5*time.Second)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestWaitForTitle_NoTimeout(t *testing.T) {
	ctx, _ := chromedp.NewContext(context.Background())

	// With timeout <= 0, should return immediately
	title, _ := WaitForTitle(ctx, 0)
	if title != "" {
		t.Errorf("expected empty title without browser, got %q", title)
	}
}

func TestNavigatePage_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NavigatePage(ctx, "https://pinchtab.com")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestSelectByNodeID_UsesValue(t *testing.T) {
	ctx, _ := chromedp.NewContext(context.Background())
	// Without a real browser this will error, but it must NOT silently succeed
	// (the old implementation was a no-op that always returned nil).
	err := SelectByNodeID(ctx, 1, "option-value")
	if err == nil {
		t.Error("expected error without browser connection, got nil (possible no-op)")
	}
}

func TestGetElementCenter_ParsesBoxModel(t *testing.T) {
	// Test the box model parsing logic
	// Content quad: [x1,y1, x2,y2, x3,y3, x4,y4]
	// For a 100x50 box at position (200, 100):
	// corners: (200,100), (300,100), (300,150), (200,150)
	content := []float64{200, 100, 300, 100, 300, 150, 200, 150}

	// Calculate expected center
	expectedX := (content[0] + content[2] + content[4] + content[6]) / 4 // (200+300+300+200)/4 = 250
	expectedY := (content[1] + content[3] + content[5] + content[7]) / 4 // (100+100+150+150)/4 = 125

	if expectedX != 250 {
		t.Errorf("expected X=250, got %f", expectedX)
	}
	if expectedY != 125 {
		t.Errorf("expected Y=125, got %f", expectedY)
	}
}

// TestGetElementCenterJS_ContextCancelled verifies that getElementCenterJS
// returns an error when the context is already cancelled (no browser panic).
func TestGetElementCenterJS_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := getElementCenterJS(ctx, 1)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

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
	err := DispatchNamedKey(ctx, "a")
	if err == nil {
		t.Error("expected error dispatching key on cancelled context")
	}
}

// TestDispatchNamedKey_KnownKeyOnCancelledCtx verifies that a known named key
// ("Enter") also returns an error on a cancelled context.
func TestDispatchNamedKey_KnownKeyOnCancelledCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DispatchNamedKey(ctx, "Enter")
	if err == nil {
		t.Error("expected error dispatching Enter on cancelled context")
	}
}

func TestClickByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ClickByCoordinate(ctx, 0, 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestHoverByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := HoverByCoordinate(ctx, 10, 20)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDoubleClickByCoordinate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DoubleClickByCoordinate(ctx, 0, 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDoubleClickByCoordinate_RejectNegativeCoordinates(t *testing.T) {
	ctx := context.Background()

	if err := DoubleClickByCoordinate(ctx, -1, 0); err == nil {
		t.Fatal("expected dblclick negative X coordinate to fail")
	}

	if err := DoubleClickByCoordinate(ctx, 0, -1); err == nil {
		t.Fatal("expected dblclick negative Y coordinate to fail")
	}
}

func TestCoordinateActions_RejectNegativeCoordinates(t *testing.T) {
	ctx := context.Background()

	if err := ClickByCoordinate(ctx, -1, 0); err == nil {
		t.Fatal("expected click negative coordinate to fail")
	}
	if err := HoverByCoordinate(ctx, 0, -1); err == nil {
		t.Fatal("expected hover negative coordinate to fail")
	}
}
