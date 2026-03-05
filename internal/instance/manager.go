package instance

import (
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/allocation"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

// Manager is the InstanceManager facade.
// It composes Repository, Locator, Allocator, and Router —
// delegating all calls, owning zero business logic.
type Manager struct {
	Repo      *Repository
	Locator   *Locator
	Allocator *Allocator
	Router    *Router
}

// NewManager creates a fully wired Manager.
func NewManager(launcher InstanceLauncher, fetcher TabFetcher, policy allocation.Policy) *Manager {
	if policy == nil {
		policy = &allocation.FCFS{}
	}

	client := &http.Client{Timeout: 60 * time.Second}
	repo := NewRepository(launcher)
	locator := NewLocator(repo, fetcher)
	allocator := NewAllocator(repo, policy)
	router := NewRouter(locator, client)

	return &Manager{
		Repo:      repo,
		Locator:   locator,
		Allocator: allocator,
		Router:    router,
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

// --- Routing (delegates to Router) ---

// ProxyTabRequest proxies an HTTP request to the bridge that owns the tab.
func (m *Manager) ProxyTabRequest(w http.ResponseWriter, r *http.Request, tabID string) {
	m.Router.ProxyTabRequest(w, r, tabID)
}
