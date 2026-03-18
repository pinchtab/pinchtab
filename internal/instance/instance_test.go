package instance_test

import (
	"fmt"
	"testing"

	bridgepkg "github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/instance"
	"github.com/pinchtab/pinchtab/internal/instance/allocation"
)

// --- Test doubles ---

type mockLauncher struct {
	instances map[string]*bridgepkg.Instance
	nextID    int
	stopErr   error
}

func newMockLauncher() *mockLauncher {
	return &mockLauncher{instances: make(map[string]*bridgepkg.Instance)}
}

func (m *mockLauncher) Launch(name, port string, headless bool) (*bridgepkg.Instance, error) {
	m.nextID++
	inst := &bridgepkg.Instance{
		ID:          fmt.Sprintf("inst_%d", m.nextID),
		ProfileName: name,
		Port:        port,
		URL:         "http://localhost:" + port,
		Headless:    headless,
		Status:      "running",
	}
	m.instances[inst.ID] = inst
	return inst, nil
}

func (m *mockLauncher) Stop(id string) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	delete(m.instances, id)
	return nil
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

// --- Repository tests ---

func TestRepository_LaunchAndGet(t *testing.T) {
	launcher := newMockLauncher()
	repo := instance.NewRepository(launcher)

	inst, err := repo.Launch("default", "9868", true)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := repo.Get(inst.ID)
	if !ok {
		t.Fatal("instance not found after launch")
	}
	if got.ProfileName != "default" {
		t.Errorf("expected profile default, got %s", got.ProfileName)
	}
	if repo.Count() != 1 {
		t.Errorf("expected count 1, got %d", repo.Count())
	}
}

func TestRepository_StopRemovesInstance(t *testing.T) {
	launcher := newMockLauncher()
	repo := instance.NewRepository(launcher)

	inst, _ := repo.Launch("default", "9868", true)
	if err := repo.Stop(inst.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.Get(inst.ID); ok {
		t.Error("instance should be gone after stop")
	}
	if repo.Count() != 0 {
		t.Errorf("expected count 0, got %d", repo.Count())
	}
}

func TestRepository_Running_FiltersNonRunning(t *testing.T) {
	launcher := newMockLauncher()
	repo := instance.NewRepository(launcher)

	inst1, _ := repo.Launch("prof1", "9868", true)
	_, _ = repo.Launch("prof2", "9869", true)

	// Manually mark inst1 as stopped via Add.
	stopped := *inst1
	stopped.Status = "stopped"
	repo.Add(&stopped)

	running := repo.Running()
	if len(running) != 1 {
		t.Errorf("expected 1 running, got %d", len(running))
	}
}

// --- Locator tests ---

func TestLocator_CacheHit(t *testing.T) {
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	repo := instance.NewRepository(launcher)
	locator := instance.NewLocator(repo, fetcher)

	inst, _ := repo.Launch("default", "9868", true)

	// Pre-register in cache.
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
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	repo := instance.NewRepository(launcher)
	locator := instance.NewLocator(repo, fetcher)

	inst, _ := repo.Launch("default", "9868", true)

	// Set up fetcher to return tabs for this instance.
	fetcher.AddTab("9868", "tab_xyz", "https://pinchtab.com")

	found, err := locator.FindInstanceByTabID("tab_xyz")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Errorf("expected %s, got %s", inst.ID, found.ID)
	}

	// Should now be cached.
	if locator.CacheSize() != 1 {
		t.Errorf("expected cache size 1, got %d", locator.CacheSize())
	}
}

func TestLocator_TabNotFound(t *testing.T) {
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	repo := instance.NewRepository(launcher)
	locator := instance.NewLocator(repo, fetcher)

	_, _ = repo.Launch("default", "9868", true)
	fetcher.AddTab("9868", "tab_abc", "https://pinchtab.com")

	_, err := locator.FindInstanceByTabID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent tab")
	}
}

func TestLocator_UsesInstanceURLWhenPresent(t *testing.T) {
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	repo := instance.NewRepository(launcher)
	locator := instance.NewLocator(repo, fetcher)

	inst, _ := repo.Launch("remote", "9868", true)
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
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	repo := instance.NewRepository(launcher)
	locator := instance.NewLocator(repo, fetcher)

	inst, _ := repo.Launch("default", "9868", true)
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
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	repo := instance.NewRepository(launcher)
	locator := instance.NewLocator(repo, fetcher)

	inst, _ := repo.Launch("default", "9868", true)
	locator.Register("tab_1", inst.ID)
	locator.Register("tab_2", inst.ID)
	locator.Register("tab_3", inst.ID)

	locator.InvalidateInstance(inst.ID)
	if locator.CacheSize() != 0 {
		t.Errorf("expected cache size 0, got %d", locator.CacheSize())
	}
}

func TestLocator_StaleCache_InstanceGone(t *testing.T) {
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	repo := instance.NewRepository(launcher)
	locator := instance.NewLocator(repo, fetcher)

	inst, _ := repo.Launch("default", "9868", true)
	locator.Register("tab_abc", inst.ID)

	// Remove instance from repo (simulates crash/stop).
	repo.Remove(inst.ID)

	// Cache hit returns stale entry, but instance is gone → should fallback.
	_, err := locator.FindInstanceByTabID("tab_abc")
	if err == nil {
		t.Error("expected error when instance is gone")
	}
}

// --- Allocator tests ---

func TestAllocator_FCFS(t *testing.T) {
	launcher := newMockLauncher()
	repo := instance.NewRepository(launcher)
	policy := &allocation.FCFS{}
	alloc := instance.NewAllocator(repo, policy)

	_, _ = repo.Launch("prof1", "9868", true)

	got, err := alloc.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if got.ProfileName != "prof1" {
		t.Errorf("expected prof1, got %s", got.ProfileName)
	}
}

func TestAllocator_RoundRobin(t *testing.T) {
	launcher := newMockLauncher()
	repo := instance.NewRepository(launcher)
	policy := allocation.NewRoundRobin()
	alloc := instance.NewAllocator(repo, policy)

	_, _ = repo.Launch("prof1", "9868", true)
	_, _ = repo.Launch("prof2", "9869", true)

	// RoundRobin should cycle. Exact order depends on map iteration,
	// but each allocation should succeed.
	for range 4 {
		_, err := alloc.Allocate()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestAllocator_NoRunningInstances(t *testing.T) {
	launcher := newMockLauncher()
	repo := instance.NewRepository(launcher)
	alloc := instance.NewAllocator(repo, &allocation.FCFS{})

	_, err := alloc.Allocate()
	if err == nil {
		t.Error("expected error with no running instances")
	}
}

// --- Manager facade tests ---

func TestManager_DelegatesToComponents(t *testing.T) {
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	mgr := instance.NewManager(launcher, fetcher)

	// Launch via manager → delegates to repo.
	inst, err := mgr.Launch("default", "9868", true)
	if err != nil {
		t.Fatal(err)
	}

	// Get via manager → delegates to repo.
	got, ok := mgr.Get(inst.ID)
	if !ok || got.ID != inst.ID {
		t.Error("Get should delegate to repo")
	}

	// List via manager → delegates to repo.
	list := mgr.List()
	if len(list) != 1 {
		t.Errorf("expected 1 instance, got %d", len(list))
	}

	// RegisterTab + FindInstanceByTabID → delegates to locator.
	mgr.RegisterTab("tab_abc", inst.ID)
	found, err := mgr.FindInstanceByTabID("tab_abc")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != inst.ID {
		t.Error("FindInstanceByTabID should delegate to locator")
	}

	// Allocate → delegates to allocator.
	alloc, err := mgr.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if alloc.ProfileName != "default" {
		t.Error("Allocate should delegate to allocator")
	}

	// Stop → delegates to repo + invalidates cache.
	if err := mgr.Stop(inst.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := mgr.Get(inst.ID); ok {
		t.Error("instance should be gone after stop")
	}
}

func TestManager_StopInvalidatesTabCache(t *testing.T) {
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	mgr := instance.NewManager(launcher, fetcher)

	inst, _ := mgr.Launch("default", "9868", true)
	mgr.RegisterTab("tab_1", inst.ID)
	mgr.RegisterTab("tab_2", inst.ID)

	_ = mgr.Stop(inst.ID)

	// Tabs should be invalidated.
	_, err := mgr.FindInstanceByTabID("tab_1")
	if err == nil {
		t.Error("expected error: tab cache should be invalidated after stop")
	}
}

func TestManager_SetAllocationPolicy(t *testing.T) {
	launcher := newMockLauncher()
	fetcher := newMockFetcher()
	mgr := instance.NewManager(launcher, fetcher)

	if err := mgr.SetAllocationPolicy("round_robin"); err != nil {
		t.Fatalf("SetAllocationPolicy returned error: %v", err)
	}
	if got := mgr.Allocator.Policy().Name(); got != "round_robin" {
		t.Fatalf("expected round_robin policy, got %q", got)
	}

	if err := mgr.SetAllocationPolicy("does_not_exist"); err == nil {
		t.Fatal("expected error for unknown policy")
	}
}
