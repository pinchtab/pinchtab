package instance

import (
	"fmt"
	"testing"

	bridgepkg "github.com/pinchtab/pinchtab/internal/bridge"
)

func addRunning(repo *Repository, name, port string) *bridgepkg.Instance {
	inst := &bridgepkg.Instance{
		ID:          "inst_" + port,
		ProfileName: name,
		Port:        port,
		URL:         "http://localhost:" + port,
		Headless:    true,
		Status:      "running",
	}
	repo.add(inst)
	return inst
}

type mockFetcher struct {
	// tabsByURL maps instance URL → tabs
	tabsByURL map[string][]bridgepkg.InstanceTab
}

func newMockFetcher() *mockFetcher {
	return &mockFetcher{tabsByURL: make(map[string][]bridgepkg.InstanceTab)}
}

func (f *mockFetcher) FetchTabs(instanceURL string) ([]bridgepkg.InstanceTab, error) {
	tabs, ok := f.tabsByURL[instanceURL]
	if !ok {
		return nil, fmt.Errorf("instance %s not reachable", instanceURL)
	}
	return tabs, nil
}

func (f *mockFetcher) AddTab(instancePort, tabID, url string) {
	key := "http://localhost:" + instancePort
	f.AddTabForURL(key, tabID, url)
}

func (f *mockFetcher) AddTabForURL(instanceURL, tabID, url string) {
	key := instanceURL
	f.tabsByURL[key] = append(f.tabsByURL[key], bridgepkg.InstanceTab{
		ID:  tabID,
		URL: url,
	})
}

func TestRepository_AddGetAndRemove(t *testing.T) {
	repo := newRepository()
	inst := addRunning(repo, "default", "9868")

	got, ok := repo.get(inst.ID)
	if !ok {
		t.Fatal("instance not found after add")
	}
	if got.ProfileName != "default" {
		t.Errorf("expected profile default, got %s", got.ProfileName)
	}

	repo.remove(inst.ID)
	if _, ok := repo.get(inst.ID); ok {
		t.Error("instance should be gone after remove")
	}
}

func TestRepository_Running_FiltersNonRunning(t *testing.T) {
	repo := newRepository()

	inst1 := addRunning(repo, "prof1", "9868")
	addRunning(repo, "prof2", "9869")

	stopped := *inst1
	stopped.Status = "stopped"
	repo.add(&stopped)

	running := repo.running()
	if len(running) != 1 {
		t.Errorf("expected 1 running, got %d", len(running))
	}
}

func TestLocator_CacheHit(t *testing.T) {
	fetcher := newMockFetcher()
	repo := newRepository()
	locator := newLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")

	locator.register("tab_abc", inst.ID)

	found, err := locator.findInstanceByTabID("tab_abc")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Errorf("expected %s, got %s", inst.ID, found.ID)
	}
}

func TestLocator_CacheMiss_QueriesBridges(t *testing.T) {
	fetcher := newMockFetcher()
	repo := newRepository()
	locator := newLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")

	fetcher.AddTab("9868", "tab_xyz", "https://pinchtab.com")

	found, err := locator.findInstanceByTabID("tab_xyz")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Errorf("expected %s, got %s", inst.ID, found.ID)
	}

	if locator.cacheSize() != 1 {
		t.Errorf("expected cache size 1, got %d", locator.cacheSize())
	}
}

func TestLocator_TabNotFound(t *testing.T) {
	fetcher := newMockFetcher()
	repo := newRepository()
	locator := newLocator(repo, fetcher)

	addRunning(repo, "default", "9868")
	fetcher.AddTab("9868", "tab_abc", "https://pinchtab.com")

	_, err := locator.findInstanceByTabID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent tab")
	}
}

func TestLocator_UsesInstanceURLWhenPresent(t *testing.T) {
	fetcher := newMockFetcher()
	repo := newRepository()
	locator := newLocator(repo, fetcher)

	inst := addRunning(repo, "remote", "9868")
	inst.URL = "https://bridge.example.com:9868"
	repo.add(inst)
	fetcher.AddTabForURL("https://bridge.example.com:9868", "tab_remote", "https://pinchtab.com")

	found, err := locator.findInstanceByTabID("tab_remote")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Fatalf("expected %s, got %s", inst.ID, found.ID)
	}
}

func TestLocator_InvalidateRemovesCacheEntry(t *testing.T) {
	fetcher := newMockFetcher()
	repo := newRepository()
	locator := newLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")
	locator.register("tab_abc", inst.ID)

	if locator.cacheSize() != 1 {
		t.Fatal("expected cache size 1")
	}

	locator.invalidate("tab_abc")
	if locator.cacheSize() != 0 {
		t.Error("expected cache size 0 after invalidate")
	}
}

func TestLocator_InvalidateInstance_RemovesAllTabs(t *testing.T) {
	fetcher := newMockFetcher()
	repo := newRepository()
	locator := newLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")
	locator.register("tab_1", inst.ID)
	locator.register("tab_2", inst.ID)
	locator.register("tab_3", inst.ID)

	locator.invalidateInstance(inst.ID)
	if locator.cacheSize() != 0 {
		t.Errorf("expected cache size 0, got %d", locator.cacheSize())
	}
}

func TestLocator_StaleCache_InstanceGone(t *testing.T) {
	fetcher := newMockFetcher()
	repo := newRepository()
	locator := newLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")
	locator.register("tab_abc", inst.ID)

	// Remove instance from repo (simulates crash/stop).
	repo.remove(inst.ID)

	// Cache hit returns stale entry, but instance is gone → should fallback.
	_, err := locator.findInstanceByTabID("tab_abc")
	if err == nil {
		t.Error("expected error when instance is gone")
	}
}

func TestManager_FindsAndForgetsTabs(t *testing.T) {
	mgr := NewManager(newMockFetcher())

	inst := addRunning(mgr.repo, "default", "9868")
	mgr.RememberTab("tab_abc", inst.ID)

	found, err := mgr.FindInstanceByTabID("tab_abc")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Errorf("found instance %q, want %q", found.ID, inst.ID)
	}

	mgr.ForgetTab("tab_abc")
	if got := mgr.CachedTabCount(); got != 0 {
		t.Errorf("cache size after ForgetTab = %d, want 0", got)
	}
}

func TestManager_ForgetInstanceDropsItAndItsTabs(t *testing.T) {
	mgr := NewManager(newMockFetcher())
	inst := &bridgepkg.Instance{ID: "inst_1", Port: "9001", Status: "running"}
	mgr.TrackInstance(inst)
	mgr.RememberTab("tab_1", inst.ID)
	mgr.RememberTab("tab_2", inst.ID)

	if _, err := mgr.FindInstanceByTabID("tab_1"); err != nil {
		t.Fatalf("tracked instance not found by its tab: %v", err)
	}

	mgr.ForgetInstance(inst.ID)
	if got := mgr.CachedTabCount(); got != 0 {
		t.Errorf("cache size after ForgetInstance = %d, want 0", got)
	}
	if _, ok := mgr.repo.get(inst.ID); ok {
		t.Error("forgotten instance is still tracked")
	}
}

func TestManager_ReconcileDropsTabsOfStoppedInstances(t *testing.T) {
	mgr := NewManager(newMockFetcher())
	mgr.RememberTab("tab-gone", "inst-gone")

	mgr.Reconcile()

	if got := mgr.CachedTabCount(); got != 0 {
		t.Errorf("cache size after Reconcile = %d, want 0", got)
	}
}
