package cdpops

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
)

type backgroundNavigationExecutor struct {
	methods      []string
	focusFlags   []bool
	emulationErr error
	lifecycleErr error
}

func (e *backgroundNavigationExecutor) Execute(_ context.Context, method string, params, _ any) error {
	e.methods = append(e.methods, method)
	if method == emulation.CommandSetFocusEmulationEnabled {
		e.focusFlags = append(e.focusFlags, params.(*emulation.SetFocusEmulationEnabledParams).Enabled)
		return e.emulationErr
	}
	if method == page.CommandSetWebLifecycleState {
		return e.lifecycleErr
	}
	return nil
}

func TestDispatchBackgroundNavigationEmulatesFocusWithoutActivatingTab(t *testing.T) {
	exec := &backgroundNavigationExecutor{}
	ctx := cdp.WithExecutor(context.Background(), exec)

	if err := dispatchBackgroundNavigation(ctx, "https://example.test/", false); err != nil {
		t.Fatalf("dispatch background navigation: %v", err)
	}

	wantMethods := []string{
		emulation.CommandSetFocusEmulationEnabled,
		page.CommandSetWebLifecycleState,
		page.CommandNavigate,
	}
	if !reflect.DeepEqual(exec.methods, wantMethods) {
		t.Fatalf("CDP methods = %v, want %v", exec.methods, wantMethods)
	}
	if !reflect.DeepEqual(exec.focusFlags, []bool{true}) {
		t.Fatalf("focus emulation flags = %v, want [true]", exec.focusFlags)
	}
	for _, method := range exec.methods {
		if method == page.CommandBringToFront || method == target.CommandActivateTarget {
			t.Fatalf("background navigation activated the tab via %s", method)
		}
	}
}

func TestDispatchBackgroundNavigationFailsClosedWhenActiveLifecycleIsUnavailable(t *testing.T) {
	exec := &backgroundNavigationExecutor{lifecycleErr: errors.New("unsupported")}
	ctx := cdp.WithExecutor(context.Background(), exec)

	err := dispatchBackgroundNavigation(ctx, "https://example.test/", false)
	if err == nil {
		t.Fatal("expected lifecycle error")
	}
	wantMethods := []string{
		emulation.CommandSetFocusEmulationEnabled,
		page.CommandSetWebLifecycleState,
	}
	if !reflect.DeepEqual(exec.methods, wantMethods) {
		t.Fatalf("CDP methods after lifecycle failure = %v, want no navigation or activation", exec.methods)
	}
}

func TestDispatchBackgroundNavigationFailsClosedWhenFocusEmulationIsUnavailable(t *testing.T) {
	exec := &backgroundNavigationExecutor{emulationErr: errors.New("unsupported")}
	ctx := cdp.WithExecutor(context.Background(), exec)

	err := dispatchBackgroundNavigation(ctx, "https://example.test/", false)
	if err == nil {
		t.Fatal("expected focus-emulation error")
	}
	if !reflect.DeepEqual(exec.methods, []string{emulation.CommandSetFocusEmulationEnabled}) {
		t.Fatalf("CDP methods after focus-emulation failure = %v, want no navigation or activation", exec.methods)
	}
}
