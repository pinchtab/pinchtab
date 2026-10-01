package instance_test

import (
	"fmt"
	"testing"

	bridgepkg "github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/instance"
)

func addRunning(repo *instance.Repository, name, port string) *bridgepkg.Instance {
	inst := &bridgepkg.Instance{
		ID:          "inst_" + port,
		ProfileName: name,
		Port:        port,
		URL:         "http://localhost:" + port,
		Headless:    true,
		Status:      "running",
	}
	repo.Add(inst)
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
	repo := instance.NewRepository()
	inst := addRunning(repo, "default", "9868")

	got, ok := repo.Get(inst.ID)
	if !ok {
		t.Fatal("instance not found after add")
	}
	if got.ProfileName != "default" {
		t.Errorf("expected profile default, got %s", got.ProfileName)
	}

	repo.Remove(inst.ID)
	if _, ok := repo.Get(inst.ID); ok {
		t.Error("instance should be gone after remove")
	}
}

func TestRepository_Running_FiltersNonRunning(t *testing.T) {
	repo := instance.NewRepository()

	inst1 := addRunning(repo, "prof1", "9868")
	addRunning(repo, "prof2", "9869")

	stopped := *inst1
	stopped.Status = "stopped"
	repo.Add(&stopped)

	running := repo.Running()
	if len(running) != 1 {
		t.Errorf("expected 1 running, got %d", len(running))
	}
}

func TestLocator_CacheHit(t *testing.T) {
	fetcher := newMockFetcher()
	repo := instance.NewRepository()
	locator := instance.NewLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")

	locator.Register("tab_abc", inst.ID)

	found, err := locator.FindInstanceByTabID("tab_abc")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Errorf("expected %s, got %s", inst.ID, found.ID)
	}
}

func TestLocator_CacheMiss_QueriesBridges(t *testing.T) {
	fetcher := newMockFetcher()
	repo := instance.NewRepository()
	locator := instance.NewLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")

	fetcher.AddTab("9868", "tab_xyz", "https://pinchtab.com")

	found, err := locator.FindInstanceByTabID("tab_xyz")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Errorf("expected %s, got %s", inst.ID, found.ID)
	}

	if locator.CacheSize() != 1 {
		t.Errorf("expected cache size 1, got %d", locator.CacheSize())
	}
}

func TestLocator_TabNotFound(t *testing.T) {
	fetcher := newMockFetcher()
	repo := instance.NewRepository()
	locator := instance.NewLocator(repo, fetcher)

	addRunning(repo, "default", "9868")
	fetcher.AddTab("9868", "tab_abc", "https://pinchtab.com")

	_, err := locator.FindInstanceByTabID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent tab")
	}
}

func TestLocator_UsesInstanceURLWhenPresent(t *testing.T) {
	fetcher := newMockFetcher()
	repo := instance.NewRepository()
	locator := instance.NewLocator(repo, fetcher)

	inst := addRunning(repo, "remote", "9868")
	inst.URL = "https://bridge.example.com:9868"
	repo.Add(inst)
	fetcher.AddTabForURL("https://bridge.example.com:9868", "tab_remote", "https://pinchtab.com")

	found, err := locator.FindInstanceByTabID("tab_remote")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Fatalf("expected %s, got %s", inst.ID, found.ID)
	}
}

func TestLocator_InvalidateRemovesCacheEntry(t *testing.T) {
	fetcher := newMockFetcher()
	repo := instance.NewRepository()
	locator := instance.NewLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")
	locator.Register("tab_abc", inst.ID)

	if locator.CacheSize() != 1 {
		t.Fatal("expected cache size 1")
	}

	locator.Invalidate("tab_abc")
	if locator.CacheSize() != 0 {
		t.Error("expected cache size 0 after invalidate")
	}
}

func TestLocator_InvalidateInstance_RemovesAllTabs(t *testing.T) {
	fetcher := newMockFetcher()
	repo := instance.NewRepository()
	locator := instance.NewLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")
	locator.Register("tab_1", inst.ID)
	locator.Register("tab_2", inst.ID)
	locator.Register("tab_3", inst.ID)

	locator.InvalidateInstance(inst.ID)
	if locator.CacheSize() != 0 {
		t.Errorf("expected cache size 0, got %d", locator.CacheSize())
	}
}

func TestLocator_StaleCache_InstanceGone(t *testing.T) {
	fetcher := newMockFetcher()
	repo := instance.NewRepository()
	locator := instance.NewLocator(repo, fetcher)

	inst := addRunning(repo, "default", "9868")
	locator.Register("tab_abc", inst.ID)

	// Remove instance from repo (simulates crash/stop).
	repo.Remove(inst.ID)

	// Cache hit returns stale entry, but instance is gone → should fallback.
	_, err := locator.FindInstanceByTabID("tab_abc")
	if err == nil {
		t.Error("expected error when instance is gone")
	}
}

func TestManager_FindsAndInvalidatesTabsThroughTheLocator(t *testing.T) {
	mgr := instance.NewManager(newMockFetcher())

	inst := addRunning(mgr.Repo, "default", "9868")
	mgr.Locator.Register("tab_abc", inst.ID)

	found, err := mgr.FindInstanceByTabID("tab_abc")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Errorf("found instance %q, want %q", found.ID, inst.ID)
	}

	mgr.InvalidateTab("tab_abc")
	if got := mgr.Locator.CacheSize(); got != 0 {
		t.Errorf("cache size after invalidate = %d, want 0", got)
	}
}
