package bridge

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func TestReconcileTabBudgetBoundsUntrackedTargets(t *testing.T) {
	chromePath := testbrowser.Path(t)
	profile := testbrowser.ProfileDir(t)
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancel := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancel()
		cancelAlloc()
		_ = os.RemoveAll(profile)
	})

	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		t.Fatalf("start browser: %v", err)
	}

	const maxTabs = 3
	tm := NewTabManager(ctx, &config.RuntimeConfig{
		MaxTabs:           maxTabs,
		TabEvictionPolicy: "close_lru",
	}, nil, nil)

	exec, err := browserExecutorContext(ctx)
	if err != nil {
		t.Fatalf("browser executor: %v", err)
	}

	var firstCreated target.ID
	for i := range 6 {
		id, err := target.CreateTarget("about:blank").Do(exec)
		if err != nil {
			t.Fatalf("create untracked target %d: %v", i, err)
		}
		if i == 0 {
			firstCreated = id
		}
	}

	tm.mu.Lock()
	tm.tabs["managed"] = &TabEntry{CDPID: string(firstCreated)}
	tm.mu.Unlock()

	before, err := tm.ListTargets()
	if err != nil {
		t.Fatalf("list targets: %v", err)
	}
	if len(before) <= maxTabs {
		t.Fatalf("precondition: expected more than %d page targets before reconcile, got %d", maxTabs, len(before))
	}

	tm.reconcileTabBudget()

	after, err := tm.ListTargets()
	if err != nil {
		t.Fatalf("list targets after reconcile: %v", err)
	}
	if len(after) > maxTabs {
		t.Fatalf("reconcile did not bound live page targets to MaxTabs: got %d, want <= %d", len(after), maxTabs)
	}

	survived := false
	for _, tg := range after {
		if tg.TargetID == firstCreated {
			survived = true
		}
	}
	if !survived {
		t.Fatalf("reconcile reaped the tracked/managed target %s", firstCreated)
	}
}
