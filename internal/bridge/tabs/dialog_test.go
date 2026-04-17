package tabs

import (
	"context"
	"fmt"
	"testing"
)

func TestDialogManager_AutoHandlerVisibility(t *testing.T) {
	dm := NewDialogManager()
	if dm.HasAutoHandler("tab1") {
		t.Fatal("expected no armed handler initially")
	}
	dm.ArmAutoHandler("tab1", "accept", "hi")
	if !dm.HasAutoHandler("tab1") {
		t.Fatal("expected armed handler after ArmAutoHandler")
	}
	_ = dm.TakeAutoHandler("tab1")
	if dm.HasAutoHandler("tab1") {
		t.Fatal("expected armed handler to be cleared after TakeAutoHandler")
	}
}

func TestHandlePendingDialog_NoPending_FallbackHandlesDialog(t *testing.T) {
	dm := NewDialogManager()
	orig := handleDialogAction
	t.Cleanup(func() { handleDialogAction = orig })

	called := false
	handleDialogAction = func(ctx context.Context, accept bool, promptText string) error {
		called = true
		if !accept {
			t.Fatalf("accept = false, want true")
		}
		if promptText != "hello" {
			t.Fatalf("promptText = %q, want hello", promptText)
		}
		return nil
	}

	result, err := HandlePendingDialog(context.Background(), "tab1", dm, true, "hello")
	if err != nil {
		t.Fatalf("HandlePendingDialog() error = %v", err)
	}
	if !called {
		t.Fatal("expected fallback dialog handler to be called")
	}
	if result == nil || !result.Handled || result.Type != "unknown" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestHandlePendingDialog_NoPending_NoDialogOpen(t *testing.T) {
	dm := NewDialogManager()
	orig := handleDialogAction
	t.Cleanup(func() { handleDialogAction = orig })

	handleDialogAction = func(ctx context.Context, accept bool, promptText string) error {
		return fmt.Errorf("No dialog is showing")
	}

	_, err := HandlePendingDialog(context.Background(), "tab1", dm, true, "")
	if err == nil {
		t.Fatal("expected error for no open dialog")
	}
	if got := err.Error(); got != "no dialog open on tab tab1" {
		t.Fatalf("error = %q, want %q", got, "no dialog open on tab tab1")
	}
}

func TestHandlePendingDialog_Pending_NoDialogOpenTreatedHandled(t *testing.T) {
	dm := NewDialogManager()
	dm.SetPending("tab1", &DialogState{Type: "alert", Message: "hello"})

	orig := handleDialogAction
	t.Cleanup(func() { handleDialogAction = orig })

	handleDialogAction = func(ctx context.Context, accept bool, promptText string) error {
		return fmt.Errorf("No dialog is showing")
	}

	result, err := HandlePendingDialog(context.Background(), "tab1", dm, true, "")
	if err != nil {
		t.Fatalf("HandlePendingDialog() error = %v", err)
	}
	if result == nil || !result.Handled {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Type != "alert" || result.Message != "hello" {
		t.Fatalf("unexpected result payload: %#v", result)
	}
	if pending := dm.GetPending("tab1"); pending != nil {
		t.Fatalf("pending dialog should stay cleared, got %#v", pending)
	}
}

func TestHandlePendingDialog_Pending_ErrorRequeues(t *testing.T) {
	dm := NewDialogManager()
	dm.SetPending("tab1", &DialogState{Type: "confirm", Message: "Are you sure?"})

	orig := handleDialogAction
	t.Cleanup(func() { handleDialogAction = orig })

	handleDialogAction = func(ctx context.Context, accept bool, promptText string) error {
		return fmt.Errorf("transport broken")
	}

	_, err := HandlePendingDialog(context.Background(), "tab1", dm, false, "")
	if err == nil {
		t.Fatal("expected error from dialog handler")
	}
	pending := dm.GetPending("tab1")
	if pending == nil {
		t.Fatal("expected dialog state to be re-queued")
	}
	if pending.Type != "confirm" {
		t.Fatalf("pending.Type = %q, want confirm", pending.Type)
	}
}
