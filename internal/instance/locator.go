package instance

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// Locator discovers which instance owns a given tab.
// Uses an in-memory cache for O(1) lookups, falling back to
// querying bridge instances on cache miss.
type Locator struct {
	mu    sync.RWMutex
	cache map[string]string // tabID → instanceID

	repo    *Repository
	fetcher TabFetcher
}

func newLocator(repo *Repository, fetcher TabFetcher) *Locator {
	return &Locator{
		cache:   make(map[string]string),
		repo:    repo,
		fetcher: fetcher,
	}
}

// Fast path: cache hit (O(1)). Slow path: queries all bridges.
func (l *Locator) findInstanceByTabID(tabID string) (*bridge.Instance, error) {
	l.mu.RLock()
	instID, ok := l.cache[tabID]
	l.mu.RUnlock()

	if ok {
		inst, found := l.repo.get(instID)
		if found && inst.Status == bridge.InstanceRunning {
			return inst, nil
		}
		// Stale cache entry — instance gone or not running.
		l.invalidate(tabID)
	}

	running := l.repo.running()
	for _, inst := range running {
		url := inst.URL
		if url == "" {
			url = fmt.Sprintf("http://localhost:%s", inst.Port)
		}
		tabs, err := l.fetcher.FetchTabs(url)
		if err != nil {
			slog.Debug("locator: failed to fetch tabs", "instance", inst.ID, "err", err)
			continue
		}
		// Cache the tabs scanned so far in one critical section rather than
		// taking the write lock per tab, matching refreshAll.
		l.mu.Lock()
		var match bool
		for _, tab := range tabs {
			l.cache[tab.ID] = inst.ID
			if tab.ID == tabID {
				match = true
				break
			}
		}
		l.mu.Unlock()

		if match {
			result := inst // copy
			return &result, nil
		}
	}

	return nil, fmt.Errorf("tab %q not found in any running instance", tabID)
}

func (l *Locator) register(tabID, instanceID string) {
	l.mu.Lock()
	l.cache[tabID] = instanceID
	l.mu.Unlock()
}

func (l *Locator) invalidate(tabID string) {
	l.mu.Lock()
	delete(l.cache, tabID)
	l.mu.Unlock()
}

func (l *Locator) invalidateInstance(instanceID string) {
	l.mu.Lock()
	for tabID, instID := range l.cache {
		if instID == instanceID {
			delete(l.cache, tabID)
		}
	}
	l.mu.Unlock()
}

// refreshAll clears the cache and re-discovers all tabs from running instances.
func (l *Locator) refreshAll() {
	l.mu.Lock()
	l.cache = make(map[string]string)
	l.mu.Unlock()

	running := l.repo.running()
	for _, inst := range running {
		url := inst.URL
		if url == "" {
			url = fmt.Sprintf("http://localhost:%s", inst.Port)
		}
		tabs, err := l.fetcher.FetchTabs(url)
		if err != nil {
			slog.Debug("locator: refresh failed", "instance", inst.ID, "err", err)
			continue
		}
		l.mu.Lock()
		for _, tab := range tabs {
			l.cache[tab.ID] = inst.ID
		}
		l.mu.Unlock()
	}
}

func (l *Locator) cacheSize() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.cache)
}
