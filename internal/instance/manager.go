package instance

import (
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/instance/allocation"
)

// Manager is the InstanceManager facade.
// It composes Repository, Locator, and Allocator —
// delegating all calls, owning zero business logic.
type Manager struct {
	Repo      *Repository
	Locator   *Locator
	Allocator *Allocator
}

// NewManager creates a fully wired Manager.
func NewManager(launcher InstanceLauncher, fetcher TabFetcher) *Manager {
	repo := NewRepository(launcher)
	locator := NewLocator(repo, fetcher)
	allocator := NewAllocator(repo, &allocation.FCFS{})

	return &Manager{
		Repo:      repo,
		Locator:   locator,
		Allocator: allocator,
	}
}

// --- Lifecycle (delegates to Repository) ---

// Launch starts a new instance.
func (m *Manager) Launch(name, port string, headless bool) (*bridge.Instance, error) {
	return m.Repo.Launch(name, port, headless)
}

// Stop terminates an instance and invalidates its tab cache.
func (m *Manager) Stop(id string) error {
	m.Locator.InvalidateInstance(id)
	return m.Repo.Stop(id)
}

// List returns all tracked instances.
func (m *Manager) List() []bridge.Instance {
	return m.Repo.List()
}

// Get returns an instance by ID.
func (m *Manager) Get(id string) (*bridge.Instance, bool) {
	return m.Repo.Get(id)
}

// Running returns only running instances.
func (m *Manager) Running() []bridge.Instance {
	return m.Repo.Running()
}

// --- Discovery (delegates to Locator) ---

// FindInstanceByTabID returns the instance that owns a tab.
func (m *Manager) FindInstanceByTabID(tabID string) (*bridge.Instance, error) {
	return m.Locator.FindInstanceByTabID(tabID)
}

// RegisterTab adds a tab→instance mapping to the cache.
func (m *Manager) RegisterTab(tabID, instanceID string) {
	m.Locator.Register(tabID, instanceID)
}

// InvalidateTab removes a tab from the cache.
func (m *Manager) InvalidateTab(tabID string) {
	m.Locator.Invalidate(tabID)
}

// --- Allocation (delegates to Allocator) ---

// Allocate selects a running instance using the configured policy.
func (m *Manager) Allocate() (*bridge.Instance, error) {
	return m.Allocator.Allocate()
}

// SetAllocationPolicy swaps the allocation policy at runtime by name.
func (m *Manager) SetAllocationPolicy(name string) error {
	policy, err := allocation.New(name)
	if err != nil {
		return err
	}
	m.Allocator.SetPolicy(policy)
	return nil
}
