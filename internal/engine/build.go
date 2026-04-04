package engine

import (
	"fmt"
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

// BuildLightpanda creates a LightpandaEngine wrapped with SafeEngine.
func BuildLightpanda(wsURL string, cfg BuildConfig) (Engine, error) {
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:19222"
	}
	lp, err := NewLightpandaEngine(wsURL)
	if err != nil {
		return nil, fmt.Errorf("build lightpanda: %w", err)
	}
	safe := NewSafeEngine(lp, cfg.Guard, cfg.WrapContent)
	if safe != lp {
		slog.Info("engine: lightpanda engine wrapped with IDPI SafeEngine")
	}
	return safe, nil
}
