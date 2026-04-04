package engine

import (
	"log/slog"

	"github.com/pinchtab/pinchtab/internal/idpi"
)

// BuildConfig holds the parameters for building an engine pipeline.
type BuildConfig struct {
	Mode        Mode
	Guard       idpi.Guard
	WrapContent bool
}

// BuildLite creates a LiteEngine wrapped with SafeEngine when IDPI is enabled.
func BuildLite(cfg BuildConfig) Engine {
	lite := NewLiteEngine()
	safe := NewSafeEngine(lite, cfg.Guard, cfg.WrapContent)
	if safe != lite {
		slog.Info("engine: lite engine wrapped with IDPI SafeEngine")
	}
	return safe
}
