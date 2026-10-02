package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
	"github.com/pinchtab/pinchtab/internal/config"
)

var keyInputRequests = map[string]ActionRequest{
	"type":                {Kind: ActionType, NodeID: 42, Text: "hi"},
	"humanized type":      {Kind: ActionType, NodeID: 42, Text: "hi", Humanize: boolPtr(true)},
	"fill with submit":    {Kind: ActionFill, NodeID: 42, Text: "hi", Submit: true},
	"press":               {Kind: ActionPress, Key: "Enter"},
	"keyboard-type":       {Kind: ActionKeyboardType, Text: "hi"},
	"keyboard-inserttext": {Kind: ActionKeyboardInsert, Text: "hi"},
	"keydown":             {Kind: ActionKeyDown, Key: "Enter"},
	"keyup":               {Kind: ActionKeyUp, Key: "Enter"},
}

func TestKeyInputIsRefusedUndispatchedBehindAnOpenDialog(t *testing.T) {
	t.Parallel()
	for name, req := range keyInputRequests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if !DispatchesInput(req) {
				t.Fatalf("%s is not classed as dispatched input", name)
			}
			b := New(context.TODO(), nil, &config.RuntimeConfig{})
			b.Dialogs.SetPending("tab-1", &bridgetabs.DialogState{Type: "confirm", Message: "Submit?"})
			req.TabID = "tab-1"
			_, err := b.Actions[req.Kind](context.Background(), req)
			var blocking *ErrDialogBlocking
			if !errors.As(err, &blocking) || blocking.DialogMessage != "Submit?" {
				t.Fatalf("err = %v, want ErrDialogBlocking naming the open dialog", err)
			}
		})
	}
}

func TestFillWithoutSubmitIsNotDispatchedInput(t *testing.T) {
	t.Parallel()
	for _, req := range []ActionRequest{
		{Kind: ActionFill, NodeID: 42, Text: "hi"},
		{Kind: ActionHover, NodeID: 42},
		{Kind: ActionFocus, NodeID: 42},
	} {
		if DispatchesInput(req) {
			t.Fatalf("%+v classed as dispatched input", req)
		}
	}
}

func TestInputThatOpensADialogReturnsItAsTheResult(t *testing.T) {
	t.Parallel()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	pressThatConfirms := func(ctx context.Context) (map[string]any, error) {
		b.Dialogs.SetPending("tab-1", &bridgetabs.DialogState{Type: "confirm", Message: "Delete order 1?"})
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := b.runInputReportingDialog(ctx, ActionRequest{Kind: ActionPress, TabID: "tab-1", Key: "Enter"}, pressThatConfirms)
	if err != nil {
		t.Fatalf("press that opened a dialog failed: %v", err)
	}
	dialog := OpenedDialog(res)
	if dialog == nil || dialog.Type != "confirm" || dialog.Message != "Delete order 1?" {
		t.Fatalf("opened dialog = %+v, want confirm/Delete order 1?", dialog)
	}
	if ctx.Err() != nil {
		t.Fatal("waited out the action timeout behind the dialog")
	}
}

func TestInputWithoutADialogKeepsItsOwnResult(t *testing.T) {
	t.Parallel()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	pressed := func(context.Context) (map[string]any, error) {
		return map[string]any{"pressed": "Enter"}, nil
	}
	res, err := b.runInputReportingDialog(context.Background(), ActionRequest{Kind: ActionPress, TabID: "tab-1", Key: "Enter"}, pressed)
	if err != nil || res["pressed"] != "Enter" || OpenedDialog(res) != nil {
		t.Fatalf("res = %v, err = %v; want the press result untouched", res, err)
	}
}
