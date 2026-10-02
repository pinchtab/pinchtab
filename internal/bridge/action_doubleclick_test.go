package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestDoubleClickAction_ReturnsDialogBlockingInsteadOfHanging(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops
	b.Dialogs.SetPending("tab-1", &bridgetabs.DialogState{Type: "alert", Message: "hello"})
	ops.doubleClickByCoordinate = func(ctx context.Context, x, y float64) error {
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	_, err := b.Actions[ActionDoubleClick](ctx, ActionRequest{TabID: "tab-1", HasXY: true, X: 10, Y: 20})

	var dialogErr *ErrDialogBlocking
	if !errors.As(err, &dialogErr) {
		t.Fatalf("double-click error = %v, want ErrDialogBlocking", err)
	}
	if dialogErr.DialogType != "alert" || dialogErr.DialogMessage != "hello" {
		t.Fatalf("dialog = %+v, want alert/hello", dialogErr)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("double-click took %v to surface the dialog", elapsed)
	}
}

func TestDoubleClickAction_ArmsDialogAutoHandlerAndWaitsForNav(t *testing.T) {
	t.Parallel()
	ops := defaultPointerOps()
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	b.pointerOps = ops
	armedDuringDispatch := false
	ops.doubleClickByCoordinate = func(ctx context.Context, x, y float64) error {
		armedDuringDispatch = b.Dialogs.HasAutoHandler("tab-1")
		b.Dialogs.TakeAutoHandler("tab-1")
		return nil
	}
	waitedForNav := false
	ops.waitNav = func(context.Context, time.Duration) { waitedForNav = true }

	res, err := b.Actions[ActionDoubleClick](context.Background(), ActionRequest{
		TabID:        "tab-1",
		HasXY:        true,
		X:            10,
		Y:            20,
		DialogAction: "accept",
		WaitNav:      true,
	})
	if err != nil {
		t.Fatalf("double-click returned error: %v", err)
	}
	if !armedDuringDispatch {
		t.Fatal("dialog auto-handler was not armed during the double-click dispatch")
	}
	if !waitedForNav {
		t.Fatal("double-click ignored waitNav")
	}
	if res["doubleclicked"] != true {
		t.Fatalf("result = %#v, want doubleclicked=true", res)
	}
}

func TestClickThatOpensADialogReturnsItAsTheClickResult(t *testing.T) {
	t.Parallel()
	humanize := true
	for _, tc := range []struct {
		name     string
		kind     string
		humanize *bool
		flag     string
	}{
		{"click", ActionClick, nil, "clicked"},
		{"double-click", ActionDoubleClick, nil, "doubleclicked"},
		{"humanized click", ActionClick, &humanize, "human"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ops := defaultPointerOps()
			b := New(context.TODO(), nil, &config.RuntimeConfig{})
			b.pointerOps = ops
			openDialog := func(ctx context.Context) error {
				b.Dialogs.SetPending("tab-1", &bridgetabs.DialogState{Type: "confirm", Message: "Delete order 1?"})
				<-ctx.Done()
				return ctx.Err()
			}
			ops.clickByCoordinate = func(ctx context.Context, _, _ float64, _ int) error { return openDialog(ctx) }
			ops.doubleClickByCoordinate = func(ctx context.Context, _, _ float64) error { return openDialog(ctx) }
			ops.humanClickCoordinate = func(ctx context.Context, _, _ float64) error { return openDialog(ctx) }
			waitedForNav := false
			ops.waitNav = func(context.Context, time.Duration) { waitedForNav = true }

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			res, err := b.Actions[tc.kind](ctx, ActionRequest{TabID: "tab-1", HasXY: true, X: 10, Y: 20, Humanize: tc.humanize, WaitNav: true})
			if err != nil {
				t.Fatalf("click that opened a dialog failed: %v", err)
			}
			if res[tc.flag] != true {
				t.Fatalf("result = %#v, want %s=true", res, tc.flag)
			}
			dialog := OpenedDialog(res)
			if dialog == nil || dialog.Type != "confirm" || dialog.Message != "Delete order 1?" {
				t.Fatalf("opened dialog = %+v, want confirm/Delete order 1?", dialog)
			}
			if waitedForNav {
				t.Fatal("waited for navigation behind an open dialog")
			}
		})
	}
}
