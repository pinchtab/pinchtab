package instance

import "github.com/pinchtab/pinchtab/internal/bridge"

// Manager is the orchestrator's single entry point to the instance store and the
// tab index that reads it.
type Manager struct {
	repo    *Repository
	locator *Locator
}

func NewManager(fetcher TabFetcher) *Manager {
	repo := newRepository()
	return &Manager{
		repo:    repo,
		locator: newLocator(repo, fetcher),
	}
}

func (m *Manager) TrackInstance(inst *bridge.Instance) {
	m.repo.add(inst)
}

func (m *Manager) ForgetInstance(id string) {
	m.locator.invalidateInstance(id)
	m.repo.remove(id)
}

func (m *Manager) RememberTab(tabID, instanceID string) {
	m.locator.register(tabID, instanceID)
}

func (m *Manager) ForgetTab(tabID string) {
	m.locator.invalidate(tabID)
}

func (m *Manager) FindInstanceByTabID(tabID string) (*bridge.Instance, error) {
	return m.locator.findInstanceByTabID(tabID)
}

// Reconcile rebuilds the tab index from the running instances, dropping tabs
// closed outside the proxy that were never invalidated.
func (m *Manager) Reconcile() {
	m.locator.refreshAll()
}

// CachedTabCount reports the size of the tab index; tests in other packages use
// it to observe Reconcile, which FindInstanceByTabID's self-healing would hide.
func (m *Manager) CachedTabCount() int {
	return m.locator.cacheSize()
}
