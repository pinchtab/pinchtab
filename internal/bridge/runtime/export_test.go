package runtime

import (
	"context"
	"log/slog"

	"github.com/pinchtab/pinchtab/internal/browsers/runtimekit"
	"github.com/pinchtab/pinchtab/internal/config"
)

func browserLaunchArgs(headless bool) []string {
	return runtimekit.BaseFlagArgs("chrome", headless)
}

func BaseBrowserFlagArgs() []string {
	return browserLaunchArgs(false)
}

func BuildBrowserArgs(cfg *config.RuntimeConfig, port int) []string {
	cfg, effective := effectiveLaunchTarget(cfg)
	geoAlignment, err := resolveLaunchGeoAlignment(context.Background(), cfg)
	if err != nil {
		args, _, buildErr := buildBrowserArgsWithBundle(cfg, effective.Binary, nil, port, launchGeoAlignment{})
		if buildErr != nil {
			slog.Error("build browser args failed", "err", buildErr)
			return nil
		}
		return args
	}
	args, _, err := buildBrowserArgsWithBundle(cfg, effective.Binary, nil, port, geoAlignment)
	if err != nil {
		slog.Error("build browser args failed", "err", err)
		return nil
	}
	return args
}

func CloakBrowserFlagArgs(cfg *config.RuntimeConfig) []string {
	return runtimekit.CloakBrowserFlagArgs(cfg)
}
