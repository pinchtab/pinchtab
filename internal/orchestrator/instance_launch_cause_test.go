package orchestrator

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/safelog"
)

func captureLaunchLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	previousLevel := safelog.CurrentLevel()
	slog.SetDefault(slog.New(safelog.NewDefaultHandler(&buf)))
	safelog.SetLevel(slog.LevelError)
	t.Cleanup(func() {
		slog.SetDefault(previous)
		safelog.SetLevel(previousLevel)
	})
	fn()
	return buf.String()
}

// The failure this card was written from: a truncated launch binary. The error the
// caller receives says "fork/exec [path]", which names nothing — so the size the exec
// tried has to be on disk, unredacted, or the operator cannot tell a 0-byte install
// from a foreign-architecture build.
func TestLaunchFailureRecordsTheBinaryItExecdAndItsSize(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{portAvail: true, runErr: errors.New("fork/exec: exec format error")}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.hooks.portAvailable = func(int) bool { return true }

	// Point the launch binary at a truncated stand-in. resolveStableBinary prefers
	// the RUNNING executable, which under `go test` is the test binary itself, so
	// writing to o.binary as resolved would truncate the compiled test on disk.
	truncated := filepath.Join(t.TempDir(), "pinchtab")
	if err := os.WriteFile(truncated, nil, 0o755); err != nil {
		t.Fatalf("write truncated binary: %v", err)
	}
	o.binary = truncated

	var launchErr error
	logged := captureLaunchLog(t, func() {
		_, launchErr = o.Launch("test-prof", "9999", true, nil)
	})
	if launchErr == nil {
		t.Fatal("expected the launch to fail")
	}

	if !strings.Contains(logged, o.binary) {
		t.Errorf("the log does not name the binary that was exec'd (%s):\n%s", o.binary, logged)
	}
	if !strings.Contains(logged, "sizeBytes=0") {
		t.Errorf("the log does not record the size, so a truncated install reads the same as any other exec failure:\n%s", logged)
	}
	if !strings.Contains(logged, "exec format error") {
		t.Errorf("the log dropped the underlying cause:\n%s", logged)
	}
	if strings.Contains(logged, "[path]") {
		t.Errorf("the server-side record was path-redacted, which is the hole this card closes:\n%s", logged)
	}
}

// A binary that is gone is a different fault from one that is malformed, and the log
// has to tell them apart rather than going quiet on the stat.
func TestLaunchFailureRecordsWhyTheBinaryCouldNotBeStatted(t *testing.T) {
	t.Parallel()

	runner := &mockRunner{portAvail: true, runErr: errors.New("fork/exec: no such file or directory")}
	o := NewOrchestratorWithRunner(t.TempDir(), runner)
	o.hooks.portAvailable = func(int) bool { return true }
	o.binary = filepath.Join(t.TempDir(), "pinchtab-that-is-gone")

	logged := captureLaunchLog(t, func() { _, _ = o.Launch("test-prof", "9999", true, nil) })

	if !strings.Contains(logged, "statErr=") {
		t.Errorf("a missing launch binary is logged with no statErr, so it is indistinguishable from a malformed one:\n%s", logged)
	}
	if !strings.Contains(logged, o.binary) {
		t.Errorf("the log does not name the missing binary (%s):\n%s", o.binary, logged)
	}
}
