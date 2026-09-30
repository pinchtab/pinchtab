package bridge

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/config"
)

type frozenRecorder struct {
	mu    sync.Mutex
	calls []bool
}

func (r *frozenRecorder) set(_ context.Context, frozen bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, frozen)
	return nil
}

func (r *frozenRecorder) snapshot() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bool(nil), r.calls...)
}

func newFreezeTM(t *testing.T, policy string, delay time.Duration) (*TabManager, *frozenRecorder) {
	t.Helper()
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{
		TabLifecyclePolicy: policy,
		TabCloseDelay:      delay,
	}, nil, nil)

	rec := &frozenRecorder{}
	tm.setFrozen = rec.set
	tm.tabs["tab1"] = &TabEntry{Ctx: context.Background(), CDPID: "tab1", CreatedAt: time.Now(), LastUsed: time.Now()}
	return tm, rec
}

func frozenOrTimeout(tm *TabManager, want bool, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if tm.TabFrozen("tab1") == want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return tm.TabFrozen("tab1") == want
}

func (r *frozenRecorder) awaitCalls(n int) []bool {
	deadline := time.Now().Add(time.Second)
	for len(r.snapshot()) < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	return r.snapshot()
}

func freezeNow(t *testing.T, tm *TabManager) {
	t.Helper()
	tm.mu.RLock()
	gen := tm.tabs["tab1"].idleGen
	tm.mu.RUnlock()
	tm.freezeIdleTab("tab1", gen)
	if !tm.TabFrozen("tab1") {
		t.Fatal("tab should be frozen before the access under test")
	}
}

func TestFreezeIdleFreezesTheTabAfterTheDelay(t *testing.T) {
	tm, rec := newFreezeTM(t, "freeze_idle", 20*time.Millisecond)

	tm.ScheduleIdleLifecycle("tab1")

	if got := rec.awaitCalls(1); len(got) != 1 || !got[0] || !tm.TabFrozen("tab1") {
		t.Fatalf("lifecycle calls = %v, want [frozen]", got)
	}
	if _, ok := tm.tabs["tab1"]; !ok {
		t.Fatal("freeze_idle must keep the tab open")
	}
}

func TestCloseIdleStillClosesInsteadOfFreezing(t *testing.T) {
	tm, rec := newFreezeTM(t, "close_idle", time.Hour)
	tm.ScheduleIdleLifecycle("tab1")
	tm.mu.RLock()
	gen := tm.tabs["tab1"].idleGen
	tm.mu.RUnlock()

	tm.idleFire("tab1", gen)

	if got := rec.snapshot(); len(got) != 0 {
		t.Fatalf("close_idle froze the tab: %v", got)
	}
}

func TestEveryAccessPathUnfreezesAFrozenTab(t *testing.T) {
	cases := []struct {
		name   string
		access func(tm *TabManager) error
	}{
		{"tab lookup", func(tm *TabManager) error { _, _, err := tm.TabContext("tab1"); return err }},
		{"focus", func(tm *TabManager) error { _ = tm.FocusTab("tab1"); return nil }},
		{"adopt tracked target", func(tm *TabManager) error {
			_, err := tm.adoptExistingTarget(target.ID("tab1"), false)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tm, rec := newFreezeTM(t, "freeze_idle", time.Hour)
			freezeNow(t, tm)

			if err := tc.access(tm); err != nil {
				t.Fatalf("access: %v", err)
			}

			if tm.TabFrozen("tab1") {
				t.Fatal("tab still frozen after access")
			}
			if got := rec.snapshot(); len(got) != 2 || got[1] {
				t.Fatalf("lifecycle calls = %v, want [frozen active]", got)
			}
		})
	}
}

func TestAnAccessRearmsTheFreeze(t *testing.T) {
	tm, rec := newFreezeTM(t, "freeze_idle", 30*time.Millisecond)
	freezeNow(t, tm)

	if _, _, err := tm.TabContext("tab1"); err != nil {
		t.Fatalf("access: %v", err)
	}

	if got := rec.awaitCalls(3); !reflect.DeepEqual(got, []bool{true, false, true}) || !tm.TabFrozen("tab1") {
		t.Fatalf("lifecycle calls = %v, want [frozen active frozen]", got)
	}
}

func TestALoneAccessArmsTheFreeze(t *testing.T) {
	tm, _ := newFreezeTM(t, "freeze_idle", 20*time.Millisecond)

	if _, _, err := tm.TabContext("tab1"); err != nil {
		t.Fatalf("access: %v", err)
	}

	if !frozenOrTimeout(tm, true, time.Second) {
		t.Fatal("an accessed tab never froze")
	}
}

func TestARequestHoldKeepsTheTabAwakeUntilTheRequestEnds(t *testing.T) {
	tm, rec := newFreezeTM(t, "freeze_idle", 20*time.Millisecond)
	req, endRequest := context.WithCancel(context.Background())
	tm.HoldAwakeUntil(req, "tab1")
	tm.ScheduleIdleLifecycle("tab1")

	time.Sleep(80 * time.Millisecond)
	if got := rec.snapshot(); len(got) != 0 {
		t.Fatalf("tab frozen while a request held it: %v", got)
	}

	endRequest()
	if !frozenOrTimeout(tm, true, time.Second) {
		t.Fatal("tab never froze after the request ended")
	}
}

func TestReleasingAHoldTwiceDoesNotDropAnotherHold(t *testing.T) {
	tm, rec := newFreezeTM(t, "freeze_idle", 10*time.Millisecond)
	first := tm.HoldAwake("tab1")
	tm.HoldAwake("tab1")

	first()
	first()
	tm.ScheduleIdleLifecycle("tab1")
	time.Sleep(60 * time.Millisecond)

	if got := rec.snapshot(); len(got) != 0 {
		t.Fatalf("tab frozen under a live hold: %v", got)
	}
}

type tabKey struct{}

func TestAWedgedTabDoesNotBlockAnotherTabsThaw(t *testing.T) {
	tm, _ := newFreezeTM(t, "freeze_idle", time.Hour)
	wedged := make(chan struct{})
	entered := make(chan bool, 1)
	defer close(wedged)
	tm.setFrozen = func(ctx context.Context, frozen bool) error {
		if ctx.Value(tabKey{}) == "A" {
			_, bounded := ctx.Deadline()
			entered <- bounded
			<-wedged
		}
		return nil
	}
	for _, id := range []string{"tabA", "tabB"} {
		tm.tabs[id] = &TabEntry{Ctx: context.WithValue(context.Background(), tabKey{}, id[3:]), frozen: true}
	}

	go func() { _, _, _ = tm.TabContext("tabA") }()
	if bounded := <-entered; !bounded {
		t.Fatal("the lifecycle call runs without a deadline")
	}

	done := make(chan error, 1)
	go func() { _, _, err := tm.TabContext("tabB"); done <- err }()
	select {
	case err := <-done:
		if err != nil || tm.TabFrozen("tabB") {
			t.Fatalf("thaw of tabB: err=%v frozen=%v", err, tm.TabFrozen("tabB"))
		}
	case <-time.After(time.Second):
		t.Fatal("a wedged tabA blocked the thaw of tabB")
	}
}

func TestAFailedThawFailsTheRequestAndTheNextAccessRetries(t *testing.T) {
	tm, rec := newFreezeTM(t, "freeze_idle", time.Hour)
	freezeNow(t, tm)
	fail := true
	tm.setFrozen = func(ctx context.Context, frozen bool) error {
		_ = rec.set(ctx, frozen)
		if fail {
			fail = false
			return errors.New("renderer busy")
		}
		return nil
	}

	_, _, err := tm.TabContext("tab1")
	var unfreeze *TabUnfreezeError
	if !errors.As(err, &unfreeze) || unfreeze.TabID != "tab1" || !strings.Contains(err.Error(), "could not be unfrozen") {
		t.Fatalf("first access err = %v, want a TabUnfreezeError for tab1", err)
	}
	if !tm.TabFrozen("tab1") {
		t.Fatal("a failed thaw was recorded as awake")
	}

	if _, _, err := tm.TabContext("tab1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if tm.TabFrozen("tab1") {
		t.Fatal("tab still frozen after a successful retry")
	}
	if got := rec.snapshot(); len(got) != 3 || got[1] || got[2] {
		t.Fatalf("lifecycle calls = %v, want [frozen active active]", got)
	}
}

func TestTabsDoingUnpolledWorkAreNeverFrozen(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, b *Bridge)
	}{
		{"handoff paused", func(t *testing.T, b *Bridge) {
			if err := b.SetTabHandoff("tab1", "", 0); err != nil {
				t.Fatal(err)
			}
		}},
		{"request in flight, such as a screencast stream", func(t *testing.T, b *Bridge) {
			b.HoldAwakeUntil(t.Context(), "tab1")
		}},
		{"network interception rules", func(t *testing.T, b *Bridge) {
			b.routeMgr.mu.Lock()
			b.routeMgr.perTab["tab1"] = &tabRouteState{rules: []RouteRule{{Pattern: "*", Action: RouteActionAbort}}}
			b.routeMgr.mu.Unlock()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := New(context.TODO(), nil, &config.RuntimeConfig{TabLifecyclePolicy: "freeze_idle", TabCloseDelay: 10 * time.Millisecond})
			b.wireTabManager(context.Background())
			rec := &frozenRecorder{}
			b.setFrozen = rec.set
			b.tabs["tab1"] = &TabEntry{Ctx: context.Background(), CDPID: "tab1"}
			tc.setup(t, b)

			b.ScheduleIdleLifecycle("tab1")
			time.Sleep(60 * time.Millisecond)

			if got := rec.snapshot(); len(got) != 0 {
				t.Fatalf("tab was frozen: %v", got)
			}
		})
	}
}
