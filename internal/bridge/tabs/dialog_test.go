package tabs

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
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
		return
	}
	if pending.Type != "confirm" {
		t.Fatalf("pending.Type = %q, want confirm", pending.Type)
	}
}

func TestDialogAccept_PromptDefaultIsTheSameOnArmedAndPendingPaths(t *testing.T) {
	orig := handleDialogAction
	t.Cleanup(func() { handleDialogAction = orig })

	submitted := make(chan string, 1)
	handleDialogAction = func(ctx context.Context, accept bool, promptText string) error {
		submitted <- promptText
		return nil
	}

	for _, defaultPrompt := range []string{"anon", strings.Repeat("x", maxDialogTextBytes+100)} {
		opening := &page.EventJavascriptDialogOpening{Type: page.DialogTypePrompt, Message: "Your name?", DefaultPrompt: defaultPrompt}

		armedDM := NewDialogManager()
		armedDM.ArmAutoHandler("tab1", "accept", "")
		onDialogOpening(context.Background(), "tab1", armedDM, false, opening)
		var armedText string
		select {
		case armedText = <-submitted:
		case <-time.After(5 * time.Second):
			t.Fatal("armed handler never answered the dialog")
		}

		pendingDM := NewDialogManager()
		onDialogOpening(context.Background(), "tab1", pendingDM, false, opening)
		result, err := HandlePendingDialog(context.Background(), "tab1", pendingDM, true, "")
		if err != nil {
			t.Fatalf("HandlePendingDialog() error = %v", err)
		}
		pendingText := <-submitted

		if armedText != defaultPrompt || pendingText != armedText {
			t.Fatalf("armed submitted %d bytes, pending submitted %d bytes, want both the %d-byte default", len(armedText), len(pendingText), len(defaultPrompt))
		}
		if result.DefaultPrompt == "" || len(result.DefaultPrompt) > maxDialogTextBytes {
			t.Fatalf("result.DefaultPrompt is %d bytes, want a non-empty display value capped at %d", len(result.DefaultPrompt), maxDialogTextBytes)
		}
	}
}

func TestResolvePromptText(t *testing.T) {
	cases := []struct {
		name   string
		accept bool
		text   string
		want   string
	}{
		{"accept without text submits the default", true, "", "anon"},
		{"explicit text wins", true, "bob", "bob"},
		{"dismiss ignores the default", false, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolvePromptText(tc.accept, tc.text, "anon"); got != tc.want {
				t.Fatalf("resolvePromptText() = %q, want %q", got, tc.want)
			}
		})
	}
}
