package bridge

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTabExecutor_MultiTabSimulation(t *testing.T) {
	// Simulate 3 tabs executing independently
	te := NewTabExecutor(4)
	results := make(map[string][]int)
	var mu sync.Mutex

	var wg sync.WaitGroup
	tabs := []string{"tab1", "tab2", "tab3"}

	for _, tab := range tabs {
		for step := 0; step < 5; step++ {
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
	te := NewTabExecutor(4)

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
	te := NewTabExecutor(4)

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
	te := NewTabExecutor(4)
	var completed int32
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		tabID := fmt.Sprintf("tab%d", i%10) // 10 unique tabs
		go func() {
			defer wg.Done()
			err := te.Execute(context.Background(), tabID, func(ctx context.Context) error {
				time.Sleep(time.Millisecond)
				atomic.AddInt32(&completed, 1)
				return nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if n := atomic.LoadInt32(&completed); n != 50 {
		t.Errorf("expected 50 completions, got %d", n)
	}
}

func TestTabExecutor_StressRapidCreateRemove(t *testing.T) {
	te := NewTabExecutor(4)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
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
	te := NewTabExecutor(8)
	var counter int32
	var wg sync.WaitGroup

	// 30 goroutines all targeting the same tab
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = te.Execute(context.Background(), "single_tab", func(ctx context.Context) error {
				atomic.AddInt32(&counter, 1)
				return nil
			})
		}()
	}
	wg.Wait()

	if n := atomic.LoadInt32(&counter); n != 30 {
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
		executor:  NewTabExecutor(2),
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
	te := NewTabExecutor(3)
	tm := &TabManager{
		tabs:      make(map[string]*TabEntry),
		snapshots: make(map[string]*RefCache),
		executor:  te,
	}
	if tm.Executor() != te {
		t.Error("Executor() should return the configured TabExecutor")
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
	te := NewTabExecutor(4)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
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
	te := NewTabExecutor(2)
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
	te := NewTabExecutor(2)
	started := make(chan struct{}, 2)

	// Fill both semaphore slots
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
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
