package handlers

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestStartBackgroundCleanup(t *testing.T) {
	// Nil and zero receivers must no-op rather than panic.
	var nilH *Handlers
	nilH.StartBackgroundCleanup()
	(&Handlers{}).StartBackgroundCleanup()

	// A configured receiver spawns cleanup over an empty temp dir without error.
	h := &Handlers{Config: &config.RuntimeConfig{StateDir: t.TempDir()}}
	h.StartBackgroundCleanup()
}
