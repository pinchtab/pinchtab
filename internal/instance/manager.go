package instance

import "github.com/pinchtab/pinchtab/internal/bridge"

// Manager pairs the instance Repository with the tab Locator that reads it.
type Manager struct {
	Repo    *Repository
	Locator *Locator
}

// NewManager creates a fully wired Manager.
func NewManager(fetcher TabFetcher) *Manager {
	repo := NewRepository()
	return &Manager{
		Repo:    repo,
		Locator: NewLocator(repo, fetcher),
	}
}

// FindInstanceByTabID returns the instance that owns a tab.
func (m *Manager) FindInstanceByTabID(tabID string) (*bridge.Instance, error) {
	return m.Locator.FindInstanceByTabID(tabID)
}

// InvalidateTab removes a tab from the cache.
func (m *Manager) InvalidateTab(tabID string) {
	m.Locator.Invalidate(tabID)
}
