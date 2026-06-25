package bridge

import (
	"context"

	bridgeruntime "github.com/pinchtab/pinchtab/internal/bridge/runtime"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/stealth"
)

const popupGuardInitScript = stealth.PopupGuardInitScript

func InitBrowser(cfg *config.RuntimeConfig, bundle *stealth.Bundle) (context.Context, context.CancelFunc, context.Context, context.CancelFunc, stealth.LaunchMode, error) {
	return bridgeruntime.InitBrowser(cfg, bundle, bridgeruntime.Hooks{
		SetHumanRandSeed:           SetHumanRandSeed,
		IsProfileLockError:         isProfileLockError,
		ClearStaleProfileLocks:     clearStaleProfileLocks,
		ConfigureBrowserProcess:    configureBrowserProcess,
		QuarantineCorruptedProfile: quarantineCorruptedProfile,
	})
}

func baseBrowserFlagArgs() []string {
	return bridgeruntime.BaseBrowserFlagArgs()
}

func buildBrowserArgs(cfg *config.RuntimeConfig, port int) []string {
	return bridgeruntime.BuildBrowserArgs(cfg, port)
}
