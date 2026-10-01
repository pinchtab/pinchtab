// Package instance tracks the browser instances the orchestrator runs and which
// instance owns each tab: Repository holds the instances, Locator maps tab IDs to
// them, and Manager pairs the two.
package instance

import "github.com/pinchtab/pinchtab/internal/bridge"

// TabFetcher abstracts fetching tabs from a bridge instance via HTTP.
type TabFetcher interface {
	FetchTabs(instanceURL string) ([]bridge.InstanceTab, error)
}
