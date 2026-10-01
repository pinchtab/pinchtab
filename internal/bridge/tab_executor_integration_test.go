package bridge

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bridgetabs "github.com/pinchtab/pinchtab/internal/bridge/tabs"
)

func TestTabExecutor_MultiTabSimulation(t *testing.T) {
	// Simulate 3 tabs executing independently
	te := bridgetabs.NewTabExecutor(4)
	results := make(map[string][]int)
	var mu sync.Mutex

	var wg sync.WaitGroup
	tabs := []string{"tab1", "tab2", "tab3"}

	for _, tab := range tabs {
		for step := range 5 {
			wg.Add(1)
			tab, step := tab, step
			go func() {
				defer wg.Done()
				err := te.Execute(context.Background(), tab, func(ctx context.Context) error {
					time.Sleep(time.Duration(step) * time.Millisecond)
					mu.Lock()
					results[tab] = append(results[tab], step)
					mu.Unlock()
					return nil
				})
				if err != nil {
					t.Errorf("tab %s step %d: %v", tab, step, err)
				}
			}()
		}
	}
	wg.Wait()

	for _, tab := range tabs {
		mu.Lock()
		steps := results[tab]
		mu.Unlock()
		if len(steps) != 5 {
			t.Errorf("tab %s: expected 5 steps, got %d", tab, len(steps))
		}
	}
}

func TestTabExecutor_ErrorIsolation(t *testing.T) {
	te := bridgetabs.NewTabExecutor(4)

	// Tab1 fails
	err1 := te.Execute(context.Background(), "tab1", func(ctx context.Context) error {
		return fmt.Errorf("tab1 error")
	})

	// Tab2 should still work
	err2 := te.Execute(context.Background(), "tab2", func(ctx context.Context) error {
		return nil
	})

	if err1 == nil {
		t.Error("expected error from tab1")
	}
	if err2 != nil {
		t.Errorf("tab2 should succeed regardless of tab1: %v", err2)
	}
}

func TestTabExecutor_PanicIsolation(t *testing.T) {
	te := bridgetabs.NewTabExecutor(4)

	// Tab1 panics
	err1 := te.Execute(context.Background(), "tab1", func(ctx context.Context) error {
		panic("tab1 crashed")
	})

	// Tab2 should still work
	err2 := te.Execute(context.Background(), "tab2", func(ctx context.Context) error {
		return nil
	})

	if err1 == nil {
		t.Error("expected error from tab1 panic")
	}
	if err2 != nil {
		t.Errorf("tab2 should succeed regardless of tab1 panic: %v", err2)
	}
}

func TestTabExecutor_StressHighConcurrency(t *testing.T) {
	te := bridgetabs.NewTabExecutor(4)
	var completed atomic.Int32
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(1)
		tabID := fmt.Sprintf("tab%d", i%10) // 10 unique tabs
		go func() {
			defer wg.Done()
			err := te.Execute(context.Background(), tabID, func(ctx context.Context) error {
				time.Sleep(time.Millisecond)
				completed.Add(1)
				return nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if n := completed.Load(); n != 50 {
		t.Errorf("expected 50 completions, got %d", n)
	}
}

func TestTabExecutor_StressRapidCreateRemove(t *testing.T) {
	te := bridgetabs.NewTabExecutor(4)
	var wg sync.WaitGroup

	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tabID := fmt.Sprintf("tab_%d", i)
			_ = te.Execute(context.Background(), tabID, func(ctx context.Context) error {
				time.Sleep(time.Millisecond)
				return nil
			})
			te.RemoveTab(tabID)
		}(i)
	}
	wg.Wait()
}

func TestTabExecutor_StressSameTabConcurrent(t *testing.T) {
	te := bridgetabs.NewTabExecutor(8)
	var counter atomic.Int32
	var wg sync.WaitGroup

	// 30 goroutines all targeting the same tab
	for range 30 {
		wg.Go(func() {
			_ = te.Execute(context.Background(), "single_tab", func(ctx context.Context) error {
				counter.Add(1)
				return nil
			})
		})
	}
	wg.Wait()

	if n := counter.Load(); n != 30 {
		t.Errorf("expected 30 executions, got %d", n)
	}
}

func TestTabManager_ExecuteWithoutExecutor(t *testing.T) {
	tm := &TabManager{
		tabs:      make(map[string]*TabEntry),
		snapshots: make(map[string]*RefCache),
		executor:  nil, // No executor
	}
	var executed bool
	err := tm.Execute(context.Background(), "tab1", func(ctx context.Context) error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !executed {
		t.Error("task should execute directly when executor is nil")
	}
}

func TestTabManager_ExecuteWithExecutor(t *testing.T) {
	tm := &TabManager{
		tabs:      make(map[string]*TabEntry),
		snapshots: make(map[string]*RefCache),
		executor:  bridgetabs.NewTabExecutor(2),
	}
	var executed bool
	err := tm.Execute(context.Background(), "tab1", func(ctx context.Context) error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !executed {
		t.Error("task should be executed via executor")
	}
}

func TestTabManager_ExecutorAccessor(t *testing.T) {
	te := bridgetabs.NewTabExecutor(3)
	tm := &TabManager{
		tabs:      make(map[string]*TabEntry),
		snapshots: make(map[string]*RefCache),
		executor:  te,
	}
	if tm.Executor() != te {
		t.Error("Executor() should return the configured bridgetabs.TabExecutor")
	}
}

func TestTabManager_ExecutorNilAccessor(t *testing.T) {
	tm := &TabManager{
		tabs:      make(map[string]*TabEntry),
		snapshots: make(map[string]*RefCache),
	}
	if tm.Executor() != nil {
		t.Error("Executor() should return nil when not configured")
	}
}

func TestTabExecutor_ConcurrentRemoveAndExecute(t *testing.T) {
	// Verify that concurrent RemoveTab + Execute for the same tab doesn't
	// cause a race condition or deadlock.
	te := bridgetabs.NewTabExecutor(4)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		tabID := fmt.Sprintf("race_tab_%d", i)

		go func() {
			defer wg.Done()
			_ = te.Execute(context.Background(), tabID, func(ctx context.Context) error {
				time.Sleep(time.Millisecond)
				return nil
			})
		}()

		go func() {
			defer wg.Done()
			time.Sleep(500 * time.Microsecond)
			te.RemoveTab(tabID)
		}()
	}
	wg.Wait()
}

func TestTabExecutor_RemoveTabDuringActiveExecution(t *testing.T) {
	// Verify that RemoveTab waits for an active task to finish before removing.
	te := bridgetabs.NewTabExecutor(2)
	taskStarted := make(chan struct{})
	taskDone := make(chan struct{})
	var taskCompleted bool

	go func() {
		_ = te.Execute(context.Background(), "active_tab", func(ctx context.Context) error {
			close(taskStarted)
			time.Sleep(50 * time.Millisecond)
			taskCompleted = true
			return nil
		})
		close(taskDone)
	}()

	<-taskStarted
	// RemoveTab should block until the active task finishes
	te.RemoveTab("active_tab")

	// After RemoveTab returns, the task should have completed
	if !taskCompleted {
		t.Error("RemoveTab returned before active task completed")
	}

	<-taskDone // Wait for Execute goroutine to finish
}

func TestTabExecutor_StatsUnderLoad(t *testing.T) {
	te := bridgetabs.NewTabExecutor(2)
	started := make(chan struct{}, 2)

	// Fill both semaphore slots
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = te.Execute(context.Background(), fmt.Sprintf("stats_tab_%d", i), func(ctx context.Context) error {
				started <- struct{}{}
				time.Sleep(100 * time.Millisecond)
				return nil
			})
		}(i)
	}

	// Wait for both tasks to start
	<-started
	<-started

	stats := te.Stats()
	if stats.SemaphoreUsed != 2 {
		t.Errorf("expected 2 semaphore slots used, got %d", stats.SemaphoreUsed)
	}
	if stats.SemaphoreFree != 0 {
		t.Errorf("expected 0 semaphore slots free, got %d", stats.SemaphoreFree)
	}
	if stats.ActiveTabs != 2 {
		t.Errorf("expected 2 active tabs, got %d", stats.ActiveTabs)
	}

	wg.Wait()
}

// peakTracker records the highest number of tasks seen inside a tab at once.
// One task at a time is the whole promise of a per-tab gate, so the peak is the
// property worth measuring; a count of completions is not.
type peakTracker struct {
	inFlight int64
	peak     int64
}

func (p *peakTracker) enter() {
	n := atomic.AddInt64(&p.inFlight, 1)
	for {
		old := atomic.LoadInt64(&p.peak)
		if n <= old || atomic.CompareAndSwapInt64(&p.peak, old, n) {
			return
		}
	}
}

func (p *peakTracker) leave() { atomic.AddInt64(&p.inFlight, -1) }

func (p *peakTracker) Peak() int64 { return atomic.LoadInt64(&p.peak) }

// RemoveTab must not let a second task onto a tab that is still busy.
//
// The entry used to be deleted before the drain, so a caller arriving during it
// found nothing in the map, made a second mutex for the same tab, and ran
// alongside the task the first mutex was holding. Measured before the fix: two
// tasks inside one tab at once.
//
// TestTabExecutor_ConcurrentRemoveAndExecute already covers this shape and
// passed throughout, because it asks whether the executor deadlocks or races
// the map — liveness — and runs a single Execute per tab, so mutual exclusion
// is not observable in it. This asks the safety question instead: how many
// tasks were inside at the same time.
func TestRemoveTabDoesNotAdmitASecondTaskToABusyTab(t *testing.T) {
	te := bridgetabs.NewTabExecutor(8)
	var peak peakTracker

	blocking := func(hold <-chan struct{}) func(context.Context) error {
		return func(context.Context) error {
			peak.enter()
			defer peak.leave()
			<-hold
			return nil
		}
	}

	holdFirst := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		_ = te.Execute(context.Background(), "tab1", blocking(holdFirst))
		close(firstDone)
	}()
	waitFor(t, func() bool { return atomic.LoadInt64(&peak.inFlight) == 1 }, "first task to start")

	// RemoveTab blocks draining the first task; the second arrives mid-drain.
	removed := make(chan struct{})
	go func() { te.RemoveTab("tab1"); close(removed) }()
	// The second caller has to arrive AFTER the removal has done its map work
	// and settled into the drain — that is the window the defect lived in, and
	// racing the two starts hides it about as often as it shows it.
	time.Sleep(25 * time.Millisecond)

	holdSecond := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		_ = te.Execute(context.Background(), "tab1", blocking(holdSecond))
		close(secondDone)
	}()

	// Give the second caller every chance to slip in beside the first.
	time.Sleep(100 * time.Millisecond)
	got := peak.Peak()

	close(holdFirst)
	close(holdSecond)
	<-firstDone
	<-secondDone
	<-removed

	if got > 1 {
		t.Errorf("%d tasks were inside tab1 at once; RemoveTab admitted a second caller while the tab was still busy", got)
	}
}

// waitFor polls a condition instead of sleeping a guessed interval, so the test
// is not slower than it needs to be nor flaky on a loaded machine.
func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
