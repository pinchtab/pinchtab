package main

import (
	"log/slog"

	"github.com/pinchtab/pinchtab/internal/config"
)

func logSecurityWarnings(cfg *config.RuntimeConfig) {
	if cfg == nil {
		return
	}

	enabled := cfg.EnabledSensitiveEndpoints()
	if len(enabled) > 0 {
		slog.Warn("sensitive endpoints enabled", "endpoints", enabled, "hint", "only enable them in trusted environments")
	}

	if cfg.Token == "" {
		slog.Warn("api authentication disabled", "hint", "set PINCHTAB_TOKEN to require bearer auth for all endpoints")
	}

	if len(enabled) > 0 && cfg.Token == "" {
		slog.Warn("high-risk configuration: sensitive endpoints enabled without API authentication", "endpoints", enabled, "hint", "set PINCHTAB_TOKEN or disable the sensitive endpoints")
	}
}
