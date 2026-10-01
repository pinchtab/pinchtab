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
