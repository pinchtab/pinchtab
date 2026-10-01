package tabs

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// tabGate is one tab's turnstile: the mutex that serializes work on it, plus
// the count of callers currently inside or waiting.
//
// The count exists so the gate cannot be removed from the map while anyone is
// still using it. Without it a caller arriving mid-removal finds no entry,
// makes a second mutex for the same tab, and runs beside the work the first
// mutex was serializing — which is the one thing this type exists to prevent.
type tabGate struct {
	mu sync.Mutex
	// users and removing are guarded by TabExecutor.mu, not by gate.mu: they
	// describe who may delete the map entry, which is a decision about the map.
	users    int
	removing bool
}

// TabExecutor provides safe parallel execution across tabs.
type TabExecutor struct {
	semaphore   chan struct{}
	tabLocks    map[string]*tabGate
	mu          sync.Mutex
	maxParallel int
}

func NewTabExecutor(maxParallel int) *TabExecutor {
	if maxParallel <= 0 {
		maxParallel = DefaultMaxParallel()
	}
	return &TabExecutor{
		semaphore:   make(chan struct{}, maxParallel),
		tabLocks:    make(map[string]*tabGate),
		maxParallel: maxParallel,
	}
}

func DefaultMaxParallel() int {
	return max(min(runtime.NumCPU()*2, 8), 1)
}

func (te *TabExecutor) MaxParallel() int {
	return te.maxParallel
}

// enterGate returns this tab's gate and registers the caller against it, so the
// entry cannot be deleted while the caller is still inside.
func (te *TabExecutor) enterGate(tabID string) *tabGate {
	te.mu.Lock()
	defer te.mu.Unlock()
	g, ok := te.tabLocks[tabID]
	if !ok {
		g = &tabGate{}
		te.tabLocks[tabID] = g
	}
	g.users++
	return g
}

// leaveGate deregisters the caller, and drops the entry once the last user of a
// gate a RemoveTab asked for is gone.
//
// A gate no removal has asked for stays in the map after its last user leaves:
// ActiveTabs counts tabs this executor has run and not been told to forget, and
// reclaiming on idle would turn it into a count of tabs running right now.
func (te *TabExecutor) leaveGate(tabID string, g *tabGate) {
	te.mu.Lock()
	defer te.mu.Unlock()
	g.users--
	if g.users == 0 && g.removing {
		if cur, ok := te.tabLocks[tabID]; ok && cur == g {
			delete(te.tabLocks, tabID)
		}
	}
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

	gate := te.enterGate(tabID)
	tabMu := &gate.mu
	locked := make(chan struct{})
	go func() {
		tabMu.Lock()
		close(locked)
	}()

	select {
	case <-locked:
		defer te.leaveGate(tabID, gate)
		defer tabMu.Unlock()
	case <-ctx.Done():
		// The acquire is already in flight and cannot be cancelled, so the
		// abandoning caller stays registered until it lands and releases —
		// leaving earlier would let the entry be deleted while this goroutine
		// still holds the mutex behind it.
		go func() {
			<-locked
			tabMu.Unlock()
			te.leaveGate(tabID, gate)
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

// RemoveTab forgets a tab, once the work already running on it has finished.
//
// The entry used to be deleted first and drained second. In that order a caller
// arriving during the drain found nothing in the map, created a second mutex for
// the same tab, and ran beside the task still holding the first — two CDP
// operations interleaved on one tab, which is exactly what this executor exists
// to prevent. It is reachable whenever a tab is closed while an action on it is
// in flight and another is still resolving.
//
// So the entry now stays until the last user of it is gone: the drain registers
// as a user itself, and whoever leaves last does the deleting.
func (te *TabExecutor) RemoveTab(tabID string) {
	te.mu.Lock()
	g, ok := te.tabLocks[tabID]
	if !ok {
		te.mu.Unlock()
		return
	}
	g.users++
	g.removing = true
	te.mu.Unlock()

	g.mu.Lock()
	g.mu.Unlock() //nolint:staticcheck // taken solely to wait out the work in flight
	te.leaveGate(tabID, g)
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
