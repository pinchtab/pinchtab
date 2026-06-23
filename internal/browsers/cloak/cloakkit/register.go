package cloakkit

import (
	"github.com/pinchtab/pinchtab/internal/bridge"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	"github.com/pinchtab/pinchtab/internal/browsers/providerhooks"
)

func init() {
	providerhooks.Register("cloak", providerhooks.Hooks{
		CleanupProfile: bridge.CleanupOrphanedChromeProcesses,
		Shutdown: func() {
			bridge.KillAllPinchtabChrome()
		},
	})
}
