package bridge

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

type debuggerCommand struct {
	method string
	params any
}

type debuggerRecorder struct {
	commands []debuggerCommand
	failAt   string
	failErr  error
}

func (r *debuggerRecorder) Execute(_ context.Context, method string, params, _ any) error {
	r.commands = append(r.commands, debuggerCommand{method: method, params: params})
	if method == r.failAt {
		if r.failErr != nil {
			return r.failErr
		}
		return errors.New("injected CDP failure")
	}
	return nil
}

func TestWireTabManagerReappliesDebuggerGuardAfterReconnect(t *testing.T) {
	recorder := &debuggerRecorder{}
	ctx := cdp.WithExecutor(context.Background(), recorder)
	b := &Bridge{}

	for _, tabID := range []string{"attached-tab", "reconnected-tab"} {
		b.TabManager = nil
		b.wireTabManager(context.Background())
		if b.TabManager.onTabSetup == nil {
			t.Fatalf("wireTabManager(%q) omitted target setup", tabID)
		}
		if err := b.TabManager.onTabSetup(ctx, tabID); err != nil {
			t.Fatalf("wired tab setup(%q): %v", tabID, err)
		}
	}

	want := []string{
		debugger.CommandEnable,
		debugger.CommandSetSkipAllPauses,
		debugger.CommandResume,
		debugger.CommandEnable,
		debugger.CommandSetSkipAllPauses,
		debugger.CommandResume,
	}
	if len(recorder.commands) != len(want) {
		t.Fatalf("CDP commands = %v, want %v", recorder.commands, want)
	}
	for i, method := range want {
		if recorder.commands[i].method != method {
			t.Fatalf("CDP command %d = %q, want %q", i, recorder.commands[i].method, method)
		}
		if method == debugger.CommandSetSkipAllPauses {
			params, ok := recorder.commands[i].params.(*debugger.SetSkipAllPausesParams)
			if !ok || !params.Skip {
				t.Fatalf("CDP command %d must set skip=true, got %#v", i, recorder.commands[i].params)
			}
		}
	}
}

func TestTabSetupAcceptsChromeAlreadyRunningResponse(t *testing.T) {
	recorder := &debuggerRecorder{
		failAt: debugger.CommandResume,
		failErr: &cdproto.Error{
			Code:    -32000,
			Message: "Can only perform operation while paused.",
		},
	}
	ctx := cdp.WithExecutor(context.Background(), recorder)

	if err := (&Bridge{}).tabSetup(ctx, "running-tab"); err != nil {
		t.Fatalf("tabSetup rejected an already-running target: %v", err)
	}
}

func TestTabSetupFailsClosedWhenSkipAllPausesFails(t *testing.T) {
	recorder := &debuggerRecorder{failAt: debugger.CommandSetSkipAllPauses}
	ctx := cdp.WithExecutor(context.Background(), recorder)

	err := (&Bridge{}).tabSetup(ctx, "attached-tab")
	if err == nil || !strings.Contains(err.Error(), "skip debugger pauses") {
		t.Fatalf("tabSetup error = %v, want skip-debugger-pauses failure", err)
	}

	want := []string{
		debugger.CommandEnable,
		debugger.CommandSetSkipAllPauses,
		debugger.CommandDisable,
	}
	if len(recorder.commands) != len(want) {
		t.Fatalf("CDP commands = %v, want %v", recorder.commands, want)
	}
	for i, method := range want {
		if recorder.commands[i].method != method {
			t.Fatalf("CDP command %d = %q, want %q", i, recorder.commands[i].method, method)
		}
	}
}

func TestTabSetupFailsClosedWhenPausedTargetCannotResume(t *testing.T) {
	recorder := &debuggerRecorder{failAt: debugger.CommandResume}
	ctx := cdp.WithExecutor(context.Background(), recorder)

	err := (&Bridge{}).tabSetup(ctx, "paused-tab")
	if err == nil || !strings.Contains(err.Error(), "resume paused debugger") {
		t.Fatalf("tabSetup error = %v, want resume-paused-debugger failure", err)
	}

	want := []string{
		debugger.CommandEnable,
		debugger.CommandSetSkipAllPauses,
		debugger.CommandResume,
		debugger.CommandDisable,
	}
	if len(recorder.commands) != len(want) {
		t.Fatalf("CDP commands = %v, want %v", recorder.commands, want)
	}
	for i, method := range want {
		if recorder.commands[i].method != method {
			t.Fatalf("CDP command %d = %q, want %q", i, recorder.commands[i].method, method)
		}
	}
}

func TestManagedTargetSetupPropagatesDebuggerGuardFailure(t *testing.T) {
	want := errors.New("guard unavailable")
	tm := NewTabManager(nil, nil, nil, nil, func(context.Context, string) error { return want })

	if _, err := tm.setupManagedTarget(context.Background(), "tab", "target"); !errors.Is(err, want) {
		t.Fatalf("setupManagedTarget error = %v, want %v", err, want)
	}
}

func TestAttachedTabSetupSkipsLaunchOnlyTargetMutations(t *testing.T) {
	for _, mode := range []stealth.LaunchMode{stealth.LaunchModeAttached, stealth.LaunchModeRemoteCDP} {
		t.Run(string(mode), func(t *testing.T) {
			recorder := &debuggerRecorder{}
			ctx := cdp.WithExecutor(context.Background(), recorder)
			b := &Bridge{
				Config: &config.RuntimeConfig{
					NoAnimations: true,
					StealthLevel: "full",
					Cloak:        config.CloakBrowserRuntimeConfig{Locale: "en-GB"},
				},
				stealthLaunchMode: mode,
			}
			b.ensureStealthBundle()

			if err := b.tabSetup(ctx, "external-tab"); err != nil {
				t.Fatal(err)
			}
			want := []string{debugger.CommandEnable, debugger.CommandSetSkipAllPauses, debugger.CommandResume}
			if len(recorder.commands) != len(want) {
				t.Fatalf("attach commands = %v, want debugger guard only %v", recorder.commands, want)
			}
			for i, command := range recorder.commands {
				if command.method != want[i] {
					t.Fatalf("attach command %d = %q, want %q", i, command.method, want[i])
				}
			}
			status := b.StealthStatus()
			if status == nil || !status.AttachMutationsSkipped {
				t.Fatalf("attach status = %+v, want explicit skipped-mutations signal", status)
			}
		})
	}
}

// This controlled local-Chromium test first reproduces the freeze boundary,
// releases it, then proves the guard prevents the same statement from pausing.
// It is opt-in so ordinary unit and race suites do not require a browser.
func TestDebuggerPauseGuardLive(t *testing.T) {
	if os.Getenv("PINCHTAB_DEBUGGER_GUARD_LIVE") != "1" {
		t.Skip("set PINCHTAB_DEBUGGER_GUARD_LIVE=1 for the local Chromium proof")
	}

	chromePath, err := exec.LookPath("chromium")
	if err != nil {
		t.Fatal("chromium is required for the live debugger guard proof")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chromePath))
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	if err := chromedp.Run(tabCtx); err != nil {
		t.Fatalf("start local Chromium: %v", err)
	}
	paused := make(chan struct{}, 2)
	chromedp.ListenTarget(tabCtx, func(ev any) {
		if _, ok := ev.(*debugger.EventPaused); ok {
			select {
			case paused <- struct{}{}:
			default:
			}
		}
	})

	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		if _, err := debugger.Enable().Do(ctx); err != nil {
			return err
		}
		return debugger.SetSkipAllPauses(false).Do(ctx)
	})); err != nil {
		t.Fatalf("arm baseline debugger pause: %v", err)
	}

	baselineDone := make(chan error, 1)
	go func() {
		var result string
		baselineDone <- chromedp.Run(tabCtx, chromedp.Evaluate(`(() => { debugger; return "baseline released"; })()`, &result))
	}()

	select {
	case <-paused:
		t.Log("baseline reproduced: debugger statement paused the attached target")
	case err := <-baselineDone:
		t.Fatalf("baseline did not pause; evaluation returned %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("baseline pause was not observed within 3s")
	}

	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(preventDebuggerPauses)); err != nil {
		t.Fatalf("install guard and release baseline pause: %v", err)
	}
	select {
	case err := <-baselineDone:
		if err != nil {
			t.Fatalf("baseline evaluation after release: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("baseline evaluation stayed frozen after release")
	}
	var guardedResult string
	guardedCtx, guardedCancel := context.WithTimeout(tabCtx, 3*time.Second)
	defer guardedCancel()
	if err := chromedp.Run(guardedCtx, chromedp.Evaluate(`(() => { debugger; return "guarded"; })()`, &guardedResult)); err != nil {
		t.Fatalf("guarded debugger statement: %v", err)
	}
	if guardedResult != "guarded" {
		t.Fatalf("guarded result = %q, want guarded", guardedResult)
	}
	select {
	case <-paused:
		t.Fatal("guarded debugger statement still emitted a pause")
	default:
	}
	t.Log("guarded pass: the same debugger statement completed without a pause")

	newTargetCtx, newTargetCancel := chromedp.NewContext(allocCtx)
	defer newTargetCancel()
	newTargetPaused := make(chan struct{}, 1)
	chromedp.ListenTarget(newTargetCtx, func(ev any) {
		if _, ok := ev.(*debugger.EventPaused); ok {
			select {
			case newTargetPaused <- struct{}{}:
			default:
			}
		}
	})
	var newTargetResult string
	newTargetRunCtx, newTargetRunCancel := context.WithTimeout(newTargetCtx, 3*time.Second)
	defer newTargetRunCancel()
	if err := chromedp.Run(newTargetRunCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return (&Bridge{}).tabSetup(ctx, "new-target")
		}),
		chromedp.Evaluate(`(() => { debugger; return "new target guarded"; })()`, &newTargetResult),
	); err != nil {
		t.Fatalf("new target debugger statement: %v", err)
	}
	if newTargetResult != "new target guarded" {
		t.Fatalf("new target result = %q, want new target guarded", newTargetResult)
	}
	select {
	case <-newTargetPaused:
		t.Fatal("new target debugger statement emitted a pause")
	default:
	}
	t.Log("new-target pass: per-target setup prevented the first debugger statement from pausing")
}
