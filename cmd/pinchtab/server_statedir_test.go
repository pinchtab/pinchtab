package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestStateDirForConfig(t *testing.T) {
	if got := stateDirForConfig(&config.RuntimeConfig{StateDir: "/tmp/iso-state"}); got != "/tmp/iso-state" {
		t.Fatalf("explicit stateDir should win, got %q", got)
	}
	if got := stateDirForConfig(&config.RuntimeConfig{StateDir: "  /tmp/trim  "}); got != "/tmp/trim" {
		t.Fatalf("stateDir should be trimmed, got %q", got)
	}

	want := filepath.Dir(config.DefaultConfigPath())
	if got := stateDirForConfig(&config.RuntimeConfig{}); got != want {
		t.Fatalf("empty stateDir should fall back to %q, got %q", want, got)
	}
	if got := stateDirForConfig(nil); got != want {
		t.Fatalf("nil cfg should fall back to %q, got %q", want, got)
	}
}

func TestGracefulStopTimeout(t *testing.T) {
	if got := gracefulStopTimeout(nil); got != 12*time.Second {
		t.Fatalf("nil cfg should give the 10s default + 2s buffer, got %s", got)
	}
	if got := gracefulStopTimeout(&config.RuntimeConfig{}); got != 12*time.Second {
		t.Fatalf("zero ShutdownTimeout should give 10s + 2s buffer, got %s", got)
	}
	if got := gracefulStopTimeout(&config.RuntimeConfig{ShutdownTimeout: 20 * time.Second}); got != 22*time.Second {
		t.Fatalf("should be ShutdownTimeout + 2s buffer, got %s", got)
	}
}
