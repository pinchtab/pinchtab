package bridge

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultMaxParallel(t *testing.T) {
	n := DefaultMaxParallel()
	if n < 1 {
		t.Errorf("DefaultMaxParallel must be >= 1, got %d", n)
	}
	expected := runtime.NumCPU() * 2
	if expected > 8 {
		expected = 8
	}
	if n != expected {
		t.Errorf("DefaultMaxParallel = %d, want %d (NumCPU=%d)", n, expected, runtime.NumCPU())
	}
}

func TestNewTabExecutor_DefaultLimit(t *testing.T) {
	te := NewTabExecutor(0)
	if te.MaxParallel() != DefaultMaxParallel() {
		t.Errorf("expected default max parallel %d, got %d", DefaultMaxParallel(), te.MaxParallel())
	}
}

func TestNewTabExecutor_CustomLimit(t *testing.T) {
	te := NewTabExecutor(3)
	if te.MaxParallel() != 3 {
		t.Errorf("expected max parallel 3, got %d", te.MaxParallel())
	}
}

func TestTabExecutor_SingleTask(t *testing.T) {
	te := NewTabExecutor(2)
	var executed bool
	err := te.Execute(context.Background(), "tab1", func(ctx context.Context) error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !executed {
		t.Error("task was not executed")
	}
}

func TestTabExecutor_PropagatesError(t *testing.T) {
	te := NewTabExecutor(2)
	wantErr := errors.New("task failed")
	err := te.Execute(context.Background(), "tab1", func(ctx context.Context) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}

func TestTabExecutor_PanicRecovery(t *testing.T) {
	te := NewTabExecutor(2)
	err := te.Execute(context.Background(), "tab1", func(ctx context.Context) error {
		panic("boom")
	})
	if err == nil {
		t.Fatal("expected error from panic, got nil")
	}
	if err.Error() != "tab tab1: panic: boom" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestTabExecutor_ContextCancellation(t *testing.T) {
	te := NewTabExecutor(1)

	// Fill semaphore
	te.AcquireExecutionSlotForTest()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := te.Execute(ctx, "tab1", func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}

	// Release semaphore
	te.ReleaseExecutionSlotForTest()
}

func TestTabExecutor_CancelledContextBeforeExecute(t *testing.T) {
	te := NewTabExecutor(2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancel

	err := te.Execute(ctx, "tab1", func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestTabExecutor_PerTabSequential(t *testing.T) {
	te := NewTabExecutor(4)
	var counter int32
	var maxConcurrent int32

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = te.Execute(context.Background(), "tab1", func(ctx context.Context) error {
				cur := atomic.AddInt32(&counter, 1)
				// Track max concurrent executions for same tab
				for {
					old := atomic.LoadInt32(&maxConcurrent)
					if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt32(&counter, -1)
				return nil
			})
		}()
	}
	wg.Wait()

	if max := atomic.LoadInt32(&maxConcurrent); max > 1 {
		t.Errorf("per-tab execution should be sequential, but max concurrent was %d", max)
	}
}

func TestTabExecutor_CrossTabParallel(t *testing.T) {
	te := NewTabExecutor(4)
	var maxConcurrent int32
	var current int32

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		tabID := fmt.Sprintf("tab%d", i)
		go func() {
			defer wg.Done()
			_ = te.Execute(context.Background(), tabID, func(ctx context.Context) error {
				cur := atomic.AddInt32(&current, 1)
				for {
					old := atomic.LoadInt32(&maxConcurrent)
					if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				atomic.AddInt32(&current, -1)
				return nil
			})
		}()
	}
	wg.Wait()

	if max := atomic.LoadInt32(&maxConcurrent); max < 2 {
		t.Errorf("cross-tab execution should be parallel, but max concurrent was %d", max)
	}
}

func TestTabExecutor_SemaphoreLimit(t *testing.T) {
	maxParallel := 2
	te := NewTabExecutor(maxParallel)
	var maxConcurrent int32
	var current int32

	var wg sync.WaitGroup
	// Launch more tasks than the semaphore allows
	for i := 0; i < 8; i++ {
		wg.Add(1)
		tabID := fmt.Sprintf("tab%d", i)
		go func() {
			defer wg.Done()
			_ = te.Execute(context.Background(), tabID, func(ctx context.Context) error {
				cur := atomic.AddInt32(&current, 1)
				for {
					old := atomic.LoadInt32(&maxConcurrent)
					if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
						break
					}
				}
				time.Sleep(30 * time.Millisecond)
				atomic.AddInt32(&current, -1)
				return nil
			})
		}()
	}
	wg.Wait()

	if max := atomic.LoadInt32(&maxConcurrent); int(max) > maxParallel {
		t.Errorf("semaphore should limit to %d, but max concurrent was %d", maxParallel, max)
	}
}

func TestTabExecutor_RemoveTab(t *testing.T) {
	te := NewTabExecutor(2)
	// Execute a task to create the per-tab mutex
	_ = te.Execute(context.Background(), "tab1", func(ctx context.Context) error { return nil })

	if te.ActiveTabs() != 1 {
		t.Errorf("expected 1 active tab, got %d", te.ActiveTabs())
	}

	te.RemoveTab("tab1")
	if te.ActiveTabs() != 0 {
		t.Errorf("expected 0 active tabs after remove, got %d", te.ActiveTabs())
	}
}

func TestTabExecutor_RemoveTab_Nonexistent(t *testing.T) {
	te := NewTabExecutor(2)
	// Should not panic
	te.RemoveTab("nonexistent")
}

func TestTabExecutor_Stats(t *testing.T) {
	te := NewTabExecutor(4)
	stats := te.Stats()
	if stats.MaxParallel != 4 {
		t.Errorf("expected MaxParallel 4, got %d", stats.MaxParallel)
	}
	if stats.ActiveTabs != 0 {
		t.Errorf("expected 0 active tabs, got %d", stats.ActiveTabs)
	}
	if stats.SemaphoreUsed != 0 {
		t.Errorf("expected 0 semaphore used, got %d", stats.SemaphoreUsed)
	}
	if stats.SemaphoreFree != 4 {
		t.Errorf("expected 4 semaphore free, got %d", stats.SemaphoreFree)
	}
}

func TestTabExecutor_ExecuteWithTimeout(t *testing.T) {
	te := NewTabExecutor(2)
	err := te.ExecuteWithTimeout(context.Background(), "tab1", 100*time.Millisecond, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTabExecutor_ExecuteWithTimeout_Exceeded(t *testing.T) {
	te := NewTabExecutor(2)
	err := te.ExecuteWithTimeout(context.Background(), "tab1", 20*time.Millisecond, func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestTabExecutor_EmptyTabID(t *testing.T) {
	te := NewTabExecutor(2)
	err := te.Execute(context.Background(), "", func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for empty tabID")
	}
	if err.Error() != "tabID must not be empty" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestTabExecutor_NilTask(t *testing.T) {
	te := NewTabExecutor(2)
	// Passing a nil task should panic and be recovered
	err := te.Execute(context.Background(), "tab1", nil)
	if err == nil {
		t.Fatal("expected error from nil task panic")
	}
}

func TestTabExecutor_MaxParallelOne(t *testing.T) {
	// With maxParallel=1, ALL tabs are fully serialized (no parallelism)
	te := NewTabExecutor(1)
	var maxConcurrent int32
	var current int32

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		tabID := fmt.Sprintf("tab%d", i)
		go func() {
			defer wg.Done()
			_ = te.Execute(context.Background(), tabID, func(ctx context.Context) error {
				cur := atomic.AddInt32(&current, 1)
				for {
					old := atomic.LoadInt32(&maxConcurrent)
					if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt32(&current, -1)
				return nil
			})
		}()
	}
	wg.Wait()

	if max := atomic.LoadInt32(&maxConcurrent); max > 1 {
		t.Errorf("maxParallel=1 should serialize all tabs, but max concurrent was %d", max)
	}
}

func TestTabExecutor_NegativeMaxParallel(t *testing.T) {
	te := NewTabExecutor(-5)
	if te.MaxParallel() != DefaultMaxParallel() {
		t.Errorf("negative maxParallel should use default, got %d", te.MaxParallel())
	}
}

func TestTabExecutor_MultiplePanicsAcrossTabs(t *testing.T) {
	te := NewTabExecutor(4)
	var wg sync.WaitGroup
	errs := make([]error, 4)

	for i := 0; i < 4; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			errs[i] = te.Execute(context.Background(), fmt.Sprintf("tab%d", i), func(ctx context.Context) error {
				panic(fmt.Sprintf("panic in tab%d", i))
			})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err == nil {
			t.Errorf("tab%d: expected panic error, got nil", i)
		}
	}

	// Executor should still work after multiple panics
	err := te.Execute(context.Background(), "healthy_tab", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("executor broken after panics: %v", err)
	}
}

func TestTabExecutor_ReusedTabIDAfterRemove(t *testing.T) {
	te := NewTabExecutor(2)
	// Execute on tab, then remove, then reuse the same ID
	var firstExecuted, secondExecuted bool

	err := te.Execute(context.Background(), "reuse_tab", func(ctx context.Context) error {
		firstExecuted = true
		return nil
	})
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}

	te.RemoveTab("reuse_tab")
	if te.ActiveTabs() != 0 {
		t.Fatalf("expected 0 active tabs after remove, got %d", te.ActiveTabs())
	}

	// Reuse same tab ID — should get a fresh mutex and work fine
	err = te.Execute(context.Background(), "reuse_tab", func(ctx context.Context) error {
		secondExecuted = true
		return nil
	})
	if err != nil {
		t.Fatalf("second execute after reuse: %v", err)
	}

	if !firstExecuted || !secondExecuted {
		t.Error("both executions should have run")
	}
}

func TestTabExecutor_ContextTimeoutOnPerTabLock(t *testing.T) {
	te := NewTabExecutor(4)

	// Hold the per-tab lock for tab1 via a long-running task
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = te.Execute(context.Background(), "tab1", func(ctx context.Context) error {
			close(started)
			<-done // Block until test releases
			return nil
		})
	}()
	<-started

	// Try to execute on the same tab with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := te.Execute(ctx, "tab1", func(ctx context.Context) error {
		return nil
	})
	close(done) // Release the blocking task

	if err == nil {
		t.Fatal("expected timeout error waiting for per-tab lock")
	}
}

func TestTabExecutor_SequentialVsParallelTiming(t *testing.T) {
	// Measures that parallel execution across different tabs is genuinely
	// faster than sequential execution on a single tab.
	taskDuration := 20 * time.Millisecond
	numTasks := 4
	te := NewTabExecutor(numTasks)

	// Sequential: all tasks on one tab
	seqStart := time.Now()
	for i := 0; i < numTasks; i++ {
		_ = te.Execute(context.Background(), "single_tab", func(ctx context.Context) error {
			time.Sleep(taskDuration)
			return nil
		})
	}
	seqDuration := time.Since(seqStart)

	// Parallel: each task on a different tab
	te2 := NewTabExecutor(numTasks)
	parStart := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		tabID := fmt.Sprintf("par_tab_%d", i)
		go func() {
			defer wg.Done()
			_ = te2.Execute(context.Background(), tabID, func(ctx context.Context) error {
				time.Sleep(taskDuration)
				return nil
			})
		}()
	}
	wg.Wait()
	parDuration := time.Since(parStart)

	// Sequential should take ~numTasks * taskDuration
	// Parallel should take ~taskDuration (all run concurrently)
	expectedSeqMin := time.Duration(numTasks) * taskDuration
	if seqDuration < expectedSeqMin/2 {
		t.Errorf("sequential took %v, expected at least ~%v", seqDuration, expectedSeqMin)
	}

	// Parallel should be at least 2x faster than sequential
	if parDuration >= seqDuration/2 {
		t.Errorf("parallel (%v) should be significantly faster than sequential (%v)", parDuration, seqDuration)
	}

	t.Logf("Sequential: %v, Parallel: %v, Speedup: %.2fx", seqDuration, parDuration, float64(seqDuration)/float64(parDuration))
}

func TestTabExecutor_SemaphoreFairnessUnderContention(t *testing.T) {
	// With maxParallel=2 and 10 tasks on different tabs, all tasks should complete.
	// No task should starve.
	te := NewTabExecutor(2)
	completed := make([]bool, 10)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			err := te.Execute(context.Background(), fmt.Sprintf("fair_tab_%d", i), func(ctx context.Context) error {
				time.Sleep(5 * time.Millisecond)
				mu.Lock()
				completed[i] = true
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Errorf("task %d failed: %v", i, err)
			}
		}()
	}
	wg.Wait()

	for i, c := range completed {
		if !c {
			t.Errorf("task %d was starved (never completed)", i)
		}
	}
}

func TestTabExecutor_ErrorDoesNotCorruptState(t *testing.T) {
	// After a task returns an error, the same tab should still accept new tasks.
	te := NewTabExecutor(2)

	err := te.Execute(context.Background(), "err_tab", func(ctx context.Context) error {
		return fmt.Errorf("deliberate error")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	// Tab should still work
	var executed bool
	err = te.Execute(context.Background(), "err_tab", func(ctx context.Context) error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("tab broken after error: %v", err)
	}
	if !executed {
		t.Error("task did not execute after prior error")
	}
}

func TestTabExecutor_ManyUniqueTabsCreation(t *testing.T) {
	// Verify that creating many unique tabs doesn't cause issues.
	te := NewTabExecutor(4)
	var wg sync.WaitGroup
	numTabs := 100

	for i := 0; i < numTabs; i++ {
		wg.Add(1)
		tabID := fmt.Sprintf("unique_tab_%d", i)
		go func() {
			defer wg.Done()
			_ = te.Execute(context.Background(), tabID, func(ctx context.Context) error {
				return nil
			})
		}()
	}
	wg.Wait()

	if te.ActiveTabs() != numTabs {
		t.Errorf("expected %d active tabs, got %d", numTabs, te.ActiveTabs())
	}

	// Clean up all
	for i := 0; i < numTabs; i++ {
		te.RemoveTab(fmt.Sprintf("unique_tab_%d", i))
	}
	if te.ActiveTabs() != 0 {
		t.Errorf("expected 0 active tabs after cleanup, got %d", te.ActiveTabs())
	}
}

func TestTabExecutor_SlowAndFastTabsConcurrent(t *testing.T) {
	// One slow tab shouldn't block a fast tab from completing.
	te := NewTabExecutor(4)
	fastDone := make(chan struct{})
	slowDone := make(chan struct{})

	// Slow tab
	go func() {
		_ = te.Execute(context.Background(), "slow_tab", func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})
		close(slowDone)
	}()

	// Short delay to ensure slow task starts first
	time.Sleep(5 * time.Millisecond)

	// Fast tab
	go func() {
		_ = te.Execute(context.Background(), "fast_tab", func(ctx context.Context) error {
			return nil // instant
		})
		close(fastDone)
	}()

	// Fast tab should finish before slow tab
	select {
	case <-fastDone:
		// expected
	case <-slowDone:
		t.Error("slow tab finished before fast tab — fast tab was blocked")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tasks")
	}

	<-slowDone
}
