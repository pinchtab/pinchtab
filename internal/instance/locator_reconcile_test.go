package instance

import (
	"testing"

	bridgepkg "github.com/pinchtab/pinchtab/internal/bridge"
)

// findInstanceByTabID caches every tab it discovers, but tabs also close
// without passing through the orchestrator proxy — the bridge evicts idle tabs
// on its own, and pages call window.close(). Those entries are never
// invalidated, so the cache only ever grows. refreshAll is the reconciler.
func TestLocatorRefreshAllDropsTabsClosedOutOfBand(t *testing.T) {
	repo := newRepository()
	fetcher := newMockFetcher()
	addRunning(repo, "p", "9001")

	for _, tabID := range []string{"tab-1", "tab-2", "tab-3"} {
		fetcher.AddTab("9001", tabID, "https://example.com")
	}

	// Looking up the last tab caches the ones scanned before it too; the scan
	// returns early on a match, so everything up to the hit lands in the cache.
	locator := newLocator(repo, fetcher)
	if _, err := locator.findInstanceByTabID("tab-3"); err != nil {
		t.Fatalf("findInstanceByTabID: %v", err)
	}
	if got := locator.cacheSize(); got != 3 {
		t.Fatalf("cache size after discovery = %d, want 3", got)
	}

	// Two tabs go away without the orchestrator ever seeing a /close.
	fetcher.tabsByURL["http://localhost:9001"] = []bridgepkg.InstanceTab{
		{ID: "tab-2", URL: "https://example.com"},
	}

	if got := locator.cacheSize(); got != 3 {
		t.Fatalf("cache size = %d, want 3 — entries linger until reconciled", got)
	}

	locator.refreshAll()

	if got := locator.cacheSize(); got != 1 {
		t.Errorf("cache size after refreshAll = %d, want 1 — closed tabs must be dropped", got)
	}
}
