// Package instance implements the decomposed InstanceManager (Facade Pattern).
//
// Components:
//   - Repository: instance lifecycle (Launch, Stop, List, Get)
//   - Locator: tab discovery + cache (FindInstanceByTabID)
//   - Allocator: policy application (AllocateInstance)
//   - TabService: tab operations (CreateTab, CloseTab, ListTabs)
//   - Router: HTTP proxying (ProxyTabRequest)
//   - Manager: facade composing all 5 components
package instance

import "github.com/pinchtab/pinchtab/internal/bridge"

// TabEntry represents a tab in a bridge's registry.
type TabEntry struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// InstanceLauncher abstracts the process of launching and stopping browser instances.
// The Orchestrator implements this — the instance package delegates to it
// for process management while owning the higher-level abstractions.
type InstanceLauncher interface {
	Launch(name, port string, headless bool) (*bridge.Instance, error)
	Stop(id string) error
}

// TabFetcher abstracts fetching tabs from a bridge instance via HTTP.
type TabFetcher interface {
	FetchTabs(instanceURL string) ([]bridge.InstanceTab, error)
}
