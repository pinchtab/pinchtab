package tabs

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// TabExecutor provides safe parallel execution across tabs.
type TabExecutor struct {
	semaphore   chan struct{}
	tabLocks    map[string]*sync.Mutex
	mu          sync.Mutex
	maxParallel int
}

func NewTabExecutor(maxParallel int) *TabExecutor {
	if maxParallel <= 0 {
		maxParallel = DefaultMaxParallel()
	}
	return &TabExecutor{
		semaphore:   make(chan struct{}, maxParallel),
		tabLocks:    make(map[string]*sync.Mutex),
		maxParallel: maxParallel,
	}
}

func DefaultMaxParallel() int {
	n := runtime.NumCPU() * 2
	if n > 8 {
		n = 8
	}
	if n < 1 {
		n = 1
	}
	return n
}

func (te *TabExecutor) MaxParallel() int {
	return te.maxParallel
}

func (te *TabExecutor) tabMutex(tabID string) *sync.Mutex {
	te.mu.Lock()
	defer te.mu.Unlock()
	m, ok := te.tabLocks[tabID]
	if !ok {
		m = &sync.Mutex{}
		te.tabLocks[tabID] = m
	}
	return m
}

func (te *TabExecutor) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	if tabID == "" {
		return fmt.Errorf("tabID must not be empty")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	select {
	case te.semaphore <- struct{}{}:
		defer func() { <-te.semaphore }()
	case <-ctx.Done():
		return fmt.Errorf("tab %s: waiting for execution slot: %w", tabID, ctx.Err())
	}

	tabMu := te.tabMutex(tabID)
	locked := make(chan struct{})
	go func() {
		tabMu.Lock()
		close(locked)
	}()

	select {
	case <-locked:
		defer tabMu.Unlock()
	case <-ctx.Done():
		go func() {
			<-locked
			tabMu.Unlock()
		}()
		return fmt.Errorf("tab %s: waiting for tab lock: %w", tabID, ctx.Err())
	}

	return te.safeRun(ctx, tabID, task)
}

func (te *TabExecutor) safeRun(ctx context.Context, tabID string, task func(ctx context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered in tab execution",
				"tabId", tabID,
				"panic", fmt.Sprintf("%v", r),
			)
			err = fmt.Errorf("tab %s: panic: %v", tabID, r)
		}
	}()
	return task(ctx)
}

func (te *TabExecutor) RemoveTab(tabID string) {
	te.mu.Lock()
	m, ok := te.tabLocks[tabID]
	if !ok {
		te.mu.Unlock()
		return
	}
	delete(te.tabLocks, tabID)
	te.mu.Unlock()

	m.Lock()
	defer m.Unlock() //nolint:staticcheck
}

func (te *TabExecutor) ActiveTabs() int {
	te.mu.Lock()
	defer te.mu.Unlock()
	return len(te.tabLocks)
}

type ExecutorStats struct {
	MaxParallel   int `json:"maxParallel"`
	ActiveTabs    int `json:"activeTabs"`
	SemaphoreUsed int `json:"semaphoreUsed"`
	SemaphoreFree int `json:"semaphoreFree"`
}

func (te *TabExecutor) Stats() ExecutorStats {
	used := len(te.semaphore)
	return ExecutorStats{
		MaxParallel:   te.maxParallel,
		ActiveTabs:    te.ActiveTabs(),
		SemaphoreUsed: used,
		SemaphoreFree: te.maxParallel - used,
	}
}

func (te *TabExecutor) ExecuteWithTimeout(ctx context.Context, tabID string, timeout time.Duration, task func(ctx context.Context) error) error {
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return te.Execute(tCtx, tabID, task)
}

// AcquireExecutionSlotForTest fills one semaphore slot for package-external tests.
func (te *TabExecutor) AcquireExecutionSlotForTest() {
	te.semaphore <- struct{}{}
}

// ReleaseExecutionSlotForTest releases one semaphore slot for package-external tests.
func (te *TabExecutor) ReleaseExecutionSlotForTest() {
	<-te.semaphore
}
