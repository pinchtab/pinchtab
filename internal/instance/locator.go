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

// NewLocator creates a Locator backed by the given repository and tab fetcher.
func NewLocator(repo *Repository, fetcher TabFetcher) *Locator {
	return &Locator{
		cache:   make(map[string]string),
		repo:    repo,
		fetcher: fetcher,
	}
}

// FindInstanceByTabID returns the instance that owns the given tab.
// Fast path: cache hit (O(1)). Slow path: queries all bridges.
func (l *Locator) FindInstanceByTabID(tabID string) (*bridge.Instance, error) {
	// Fast path: check cache.
	l.mu.RLock()
	instID, ok := l.cache[tabID]
	l.mu.RUnlock()

	if ok {
		inst, found := l.repo.Get(instID)
		if found && inst.Status == "running" {
			return inst, nil
		}
		// Stale cache entry — instance gone or not running.
		l.Invalidate(tabID)
	}

	// Slow path: query all running bridges.
	running := l.repo.Running()
	for _, inst := range running {
		url := fmt.Sprintf("http://localhost:%s", inst.Port)
		tabs, err := l.fetcher.FetchTabs(url)
		if err != nil {
			slog.Debug("locator: failed to fetch tabs", "instance", inst.ID, "err", err)
			continue
		}
		for _, tab := range tabs {
			// Cache every tab we discover for future lookups.
			l.mu.Lock()
			l.cache[tab.ID] = inst.ID
			l.mu.Unlock()

			if tab.ID == tabID {
				result := inst // copy
				return &result, nil
			}
		}
	}

	return nil, fmt.Errorf("tab %q not found in any running instance", tabID)
}

// Register adds a tab→instance mapping to the cache.
func (l *Locator) Register(tabID, instanceID string) {
	l.mu.Lock()
	l.cache[tabID] = instanceID
	l.mu.Unlock()
}

// Invalidate removes a tab from the cache.
func (l *Locator) Invalidate(tabID string) {
	l.mu.Lock()
	delete(l.cache, tabID)
	l.mu.Unlock()
}

// InvalidateInstance removes all cached tabs for an instance.
func (l *Locator) InvalidateInstance(instanceID string) {
	l.mu.Lock()
	for tabID, instID := range l.cache {
		if instID == instanceID {
			delete(l.cache, tabID)
		}
	}
	l.mu.Unlock()
}

// RefreshAll clears the cache and re-discovers all tabs from running instances.
func (l *Locator) RefreshAll() {
	l.mu.Lock()
	l.cache = make(map[string]string)
	l.mu.Unlock()

	running := l.repo.Running()
	for _, inst := range running {
		url := fmt.Sprintf("http://localhost:%s", inst.Port)
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

// CacheSize returns the number of cached tab→instance mappings.
func (l *Locator) CacheSize() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.cache)
}
