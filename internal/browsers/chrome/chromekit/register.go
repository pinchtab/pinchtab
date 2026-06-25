package chromekit

import (
	"github.com/pinchtab/pinchtab/internal/bridge"
	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	"github.com/pinchtab/pinchtab/internal/browsers/providerhooks"
)

func init() {
	providerhooks.Register("chrome", providerhooks.Hooks{
		CleanupProfile: bridge.CleanupOrphanedChromeProcesses,
		Shutdown: func() {
			bridge.KillAllPinchtabChrome()
		},
	})
}
