package bridge

import (
	"context"

	bridgeruntime "github.com/pinchtab/pinchtab/internal/bridge/runtime"
	"github.com/pinchtab/pinchtab/internal/config"
)

const popupGuardInitScript = bridgeruntime.PopupGuardInitScript

// InitChrome initializes a Chrome browser for a Bridge instance.
func InitChrome(cfg *config.RuntimeConfig) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, error) {
	return bridgeruntime.InitChrome(cfg, bridgeruntime.Hooks{
		SetHumanRandSeed:          SetHumanRandSeed,
		IsChromeProfileLockError:  isChromeProfileLockError,
		ClearStaleChromeProfile:   clearStaleChromeProfileLock,
		ConfigureChromeProcessCmd: configureChromeProcess,
	})
}

func defaultChromeFlagArgs() []string {
	return bridgeruntime.DefaultChromeFlagArgs()
}

func buildChromeArgs(cfg *config.RuntimeConfig, port int) []string {
	return bridgeruntime.BuildChromeArgs(cfg, port)
}
