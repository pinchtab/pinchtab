package bridge

import (
	"context"
	"strings"
	"testing"
)

const testMaxDialogTextBytes = 8 * 1024

func TestDialogManager_SetAndGetPending(t *testing.T) {
	dm := NewDialogManager()

	// No pending dialog initially
	if got := dm.GetPending("tab1"); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}

	// Set a pending dialog
	state := &DialogState{Type: "alert", Message: "Hello"}
	dm.SetPending("tab1", state)

	got := dm.GetPending("tab1")
	if got == nil {
		t.Fatal("expected pending dialog, got nil")
	}
	if got.Type != "alert" {
		t.Errorf("expected type 'alert', got %q", got.Type)
	}
	if got.Message != "Hello" {
		t.Errorf("expected message 'Hello', got %q", got.Message)
	}
}

func TestDialogManager_ClearPending(t *testing.T) {
	dm := NewDialogManager()
	dm.SetPending("tab1", &DialogState{Type: "confirm", Message: "Are you sure?"})

	dm.ClearPending("tab1")

	if got := dm.GetPending("tab1"); got != nil {
		t.Fatalf("expected nil after clear, got %+v", got)
	}
}

func TestDialogManager_ClearPending_NoOp(t *testing.T) {
	dm := NewDialogManager()
	// Should not panic
	dm.ClearPending("nonexistent")
}

func TestDialogManager_GetAndClear(t *testing.T) {
	dm := NewDialogManager()
	dm.SetPending("tab1", &DialogState{Type: "prompt", Message: "Enter name", DefaultPrompt: "default"})

	got := dm.GetAndClear("tab1")
	if got == nil {
		t.Fatal("expected pending dialog, got nil")
	}
	if got.Type != "prompt" {
		t.Errorf("expected type 'prompt', got %q", got.Type)
	}
	if got.DefaultPrompt != "default" {
		t.Errorf("expected defaultPrompt 'default', got %q", got.DefaultPrompt)
	}

	// Should be cleared now
	if got2 := dm.GetPending("tab1"); got2 != nil {
		t.Fatalf("expected nil after GetAndClear, got %+v", got2)
	}
}

func TestDialogManager_GetAndClear_NoPending(t *testing.T) {
	dm := NewDialogManager()
	got := dm.GetAndClear("tab1")
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestDialogManager_MultipleTabsIndependent(t *testing.T) {
	dm := NewDialogManager()
	dm.SetPending("tab1", &DialogState{Type: "alert", Message: "Tab 1"})
	dm.SetPending("tab2", &DialogState{Type: "confirm", Message: "Tab 2"})

	got1 := dm.GetPending("tab1")
	got2 := dm.GetPending("tab2")

	if got1 == nil || got1.Message != "Tab 1" {
		t.Errorf("tab1: expected 'Tab 1', got %+v", got1)
	}
	if got2 == nil || got2.Message != "Tab 2" {
		t.Errorf("tab2: expected 'Tab 2', got %+v", got2)
	}

	// Clear tab1, tab2 should remain
	dm.ClearPending("tab1")
	if dm.GetPending("tab1") != nil {
		t.Error("tab1 should be cleared")
	}
	if dm.GetPending("tab2") == nil {
		t.Error("tab2 should still be pending")
	}
}

func TestDialogManager_OverwritePending(t *testing.T) {
	dm := NewDialogManager()
	dm.SetPending("tab1", &DialogState{Type: "alert", Message: "First"})
	dm.SetPending("tab1", &DialogState{Type: "confirm", Message: "Second"})

	got := dm.GetPending("tab1")
	if got == nil {
		t.Fatal("expected pending dialog")
	}
	if got.Type != "confirm" || got.Message != "Second" {
		t.Errorf("expected overwritten dialog, got %+v", got)
	}
}

func TestDialogState_Fields(t *testing.T) {
	state := &DialogState{
		Type:              "prompt",
		Message:           "Enter value",
		DefaultPrompt:     "default",
		HasBrowserHandler: true,
	}
	if state.Type != "prompt" {
		t.Errorf("expected type 'prompt', got %q", state.Type)
	}
	if state.Message != "Enter value" {
		t.Errorf("expected message 'Enter value', got %q", state.Message)
	}
	if state.DefaultPrompt != "default" {
		t.Errorf("expected defaultPrompt 'default', got %q", state.DefaultPrompt)
	}
	if !state.HasBrowserHandler {
		t.Error("expected HasBrowserHandler to be true")
	}
}

func TestDialogManager_TruncatesOversizedDialogText(t *testing.T) {
	dm := NewDialogManager()
	dm.SetPending("tab1", &DialogState{
		Type:          "prompt",
		Message:       strings.Repeat("m", testMaxDialogTextBytes+256),
		DefaultPrompt: strings.Repeat("p", testMaxDialogTextBytes+256),
	})

	got := dm.GetPending("tab1")
	if got == nil {
		t.Fatal("expected pending dialog")
	}
	if len(got.Message) > testMaxDialogTextBytes {
		t.Fatalf("message length = %d, want <= %d", len(got.Message), testMaxDialogTextBytes)
	}
	if len(got.DefaultPrompt) > testMaxDialogTextBytes {
		t.Fatalf("default prompt length = %d, want <= %d", len(got.DefaultPrompt), testMaxDialogTextBytes)
	}
}

func TestHandlePendingDialog_NoPending(t *testing.T) {
	dm := NewDialogManager()
	// No real CDP context needed — should fail before reaching CDP
	_, err := HandlePendingDialog(context.TODO(), "tab1", dm, true, "")
	if err == nil {
		t.Fatal("expected error for no pending dialog")
	}
	if got := err.Error(); got != "no dialog open on tab tab1" {
		t.Errorf("unexpected error: %s", got)
	}
}
