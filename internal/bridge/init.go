package bridge

import (
	"context"

	bridgeruntime "github.com/pinchtab/pinchtab/internal/bridge/runtime"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

const popupGuardInitScript = stealth.PopupGuardInitScript

// InitChrome initializes a Chrome browser for a Bridge instance.
func InitChrome(cfg *config.RuntimeConfig, bundle *stealth.Bundle) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	return bridgeruntime.InitChrome(cfg, bundle, bridgeruntime.Hooks{
		SetHumanRandSeed:          SetHumanRandSeed,
		IsChromeProfileLockError:  isChromeProfileLockError,
		ClearStaleChromeProfile:   clearStaleChromeProfileLock,
		ConfigureChromeProcessCmd: configureChromeProcess,
	})
}

func baseChromeFlagArgs() []string {
	return bridgeruntime.BaseChromeFlagArgs()
}

func buildChromeArgs(cfg *config.RuntimeConfig, port int) []string {
	return bridgeruntime.BuildChromeArgs(cfg, port)
}
