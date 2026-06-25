package cloak

import (
	"context"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/browsers/chrome"
)

// Instance implements browsers.RuntimeInstance for CloakBrowser by
// embedding a Chrome Instance. Screencast always uses polling because
// Cloak's CDP proxy does not support Page.startScreencast.
type Instance struct {
	*chrome.Instance
}

// NewInstance creates a Cloak RuntimeInstance. The embedded Chrome
// Instance is always constructed with headless=true to force
// polling-based screencast (the only screencast strategy that works
// through Cloak's CDP proxy).
func NewInstance(browserCtx context.Context) *Instance {
	return &Instance{
		Instance: chrome.NewInstance(browserCtx, true),
	}
}

var _ browsers.RuntimeInstance = (*Instance)(nil)
