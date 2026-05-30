package ghostchrome

import (
	"context"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

// Instance implements browsers.RuntimeInstance for Ghost-chrome by
// embedding a Chrome Instance. All RuntimeInstance methods delegate to
// Chrome — Ghost-chrome's staticfetch optimization operates at the
// Bridge/BridgeAdapter layer, not here.
type Instance struct {
	*chrome.Instance
}

// NewInstance creates a Ghost-chrome RuntimeInstance backed by a Chrome
// Instance.
func NewInstance(browserCtx context.Context, headless bool) *Instance {
	return &Instance{
		Instance: chrome.NewInstance(browserCtx, headless),
	}
}

var _ browsers.RuntimeInstance = (*Instance)(nil)
