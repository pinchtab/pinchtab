package bridge

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func withMockedProcessLookup(t *testing.T, pidsFor func() []int) (termCount, killCount *atomic.Int32) {
	t.Helper()
	origFind := findChromePIDsByProfileDirFunc
	origTerm := terminateChromeByProfileDirFunc
	origKill := killChromeByProfileDirFunc
	origPoll := chromeExitPollInterval

	var term atomic.Int32
	var kill atomic.Int32

	findChromePIDsByProfileDirFunc = func(string) []int { return pidsFor() }
	terminateChromeByProfileDirFunc = func(string) int {
		term.Add(1)
		return len(pidsFor())
	}
	killChromeByProfileDirFunc = func(string) int {
		kill.Add(1)
		return len(pidsFor())
	}
	chromeExitPollInterval = 5 * time.Millisecond

	t.Cleanup(func() {
		findChromePIDsByProfileDirFunc = origFind
		terminateChromeByProfileDirFunc = origTerm
		killChromeByProfileDirFunc = origKill
		chromeExitPollInterval = origPoll
	})

	return &term, &kill
}

func TestCleanup_RemovesTempProfileDir(t *testing.T) {
	tmpDir := t.TempDir()
	profileDir := filepath.Join(tmpDir, "pinchtab-profile-test")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := context.TODO()
	b := New(ctx, ctx, &config.RuntimeConfig{})
	b.tempProfileDir = profileDir

	b.Cleanup()

	if _, err := os.Stat(profileDir); !os.IsNotExist(err) {
		t.Errorf("expected temp profile dir to be removed, but it still exists")
	}
	if b.tempProfileDir != "" {
		t.Errorf("expected tempProfileDir to be cleared, got %q", b.tempProfileDir)
	}
}

func TestCleanup_NoTempDir(t *testing.T) {
	ctx := context.TODO()
	b := New(ctx, ctx, &config.RuntimeConfig{})
	// Should not panic with no temp dir
	b.Cleanup()
}

func withShortenedGrace(t *testing.T, grace, termGrace time.Duration) {
	t.Helper()
	origGrace := BridgeShutdownGracePeriod
	origTerm := bridgeShutdownTermGrace
	origFast := bridgeFastShutdownGrace
	BridgeShutdownGracePeriod = grace
	bridgeShutdownTermGrace = termGrace
	bridgeFastShutdownGrace = grace
	t.Cleanup(func() {
		BridgeShutdownGracePeriod = origGrace
		bridgeShutdownTermGrace = origTerm
		bridgeFastShutdownGrace = origFast
	})
}

func TestCleanup_WaitsForChromeExit_BeforeSIGKILL(t *testing.T) {
	withShortenedGrace(t, 500*time.Millisecond, 200*time.Millisecond)

	start := time.Now()
	// Report alive for the first ~100ms then gone.
	termCount, killCount := withMockedProcessLookup(t, func() []int {
		if time.Since(start) < 100*time.Millisecond {
			return []int{12345}
		}
		return nil
	})

	ctx := context.TODO()
	b := New(ctx, ctx, &config.RuntimeConfig{ProfileDir: "/tmp/pinchtab-test-profile"})
	b.Cleanup()

	if got := termCount.Load(); got != 0 {
		t.Errorf("expected 0 SIGTERM escalations when chrome exits during grace, got %d", got)
	}
	if got := killCount.Load(); got != 0 {
		t.Errorf("expected 0 SIGKILL escalations when chrome exits during grace, got %d", got)
	}
}

func TestCleanup_FallsBackToSIGKILL_WhenChromeWontDie(t *testing.T) {
	withShortenedGrace(t, 80*time.Millisecond, 80*time.Millisecond)

	termCount, killCount := withMockedProcessLookup(t, func() []int {
		return []int{67890} // always alive
	})

	ctx := context.TODO()
	b := New(ctx, ctx, &config.RuntimeConfig{ProfileDir: "/tmp/pinchtab-test-profile"})
	b.Cleanup()

	if got := termCount.Load(); got == 0 {
		t.Errorf("expected SIGTERM escalation for persistent chrome profile, got 0")
	}
	if got := killCount.Load(); got == 0 {
		t.Errorf("expected SIGKILL escalation when chrome never exits, got 0")
	}
}

func TestCleanup_TempProfileUsesFastKill(t *testing.T) {
	withShortenedGrace(t, 20*time.Millisecond, 20*time.Millisecond)
	termCount, killCount := withMockedProcessLookup(t, func() []int {
		return []int{67890}
	})

	profileDir := t.TempDir()
	ctx := context.TODO()
	b := New(ctx, ctx, &config.RuntimeConfig{ProfileDir: "/tmp/pinchtab-test-profile"})
	b.tempProfileDir = profileDir
	b.Cleanup()

	if got := termCount.Load(); got != 0 {
		t.Errorf("temp profile cleanup must not SIGTERM, got %d", got)
	}
	if got := killCount.Load(); got == 0 {
		t.Errorf("expected SIGKILL escalation for temp profile when chrome never exits, got 0")
	}
}

func TestCleanup_CloakTermBeforeKill(t *testing.T) {
	withShortenedGrace(t, 80*time.Millisecond, 80*time.Millisecond)

	var termOrder, killOrder atomic.Int64
	var step atomic.Int64

	origFind := findChromePIDsByProfileDirFunc
	origTerm := terminateChromeByProfileDirFunc
	origKill := killChromeByProfileDirFunc
	origPoll := chromeExitPollInterval
	t.Cleanup(func() {
		findChromePIDsByProfileDirFunc = origFind
		terminateChromeByProfileDirFunc = origTerm
		killChromeByProfileDirFunc = origKill
		chromeExitPollInterval = origPoll
	})
	chromeExitPollInterval = 5 * time.Millisecond

	findChromePIDsByProfileDirFunc = func(string) []int { return []int{42} }
	terminateChromeByProfileDirFunc = func(string) int {
		termOrder.Store(step.Add(1))
		return 1
	}
	killChromeByProfileDirFunc = func(string) int {
		killOrder.Store(step.Add(1))
		return 1
	}

	ctx := context.TODO()
	b := New(ctx, ctx, &config.RuntimeConfig{
		BrowserProvider: config.BrowserProviderCloak,
		ProfileDir:      "/tmp/pinchtab-test-profile",
	})
	b.Cleanup()

	to := termOrder.Load()
	ko := killOrder.Load()
	if to == 0 {
		t.Fatalf("expected SIGTERM to be called, but it was not")
	}
	if ko == 0 {
		t.Fatalf("expected SIGKILL to be called, but it was not")
	}
	if to >= ko {
		t.Errorf("expected SIGTERM before SIGKILL, got term=%d kill=%d", to, ko)
	}
}

func TestCmdlineHasExactUserDataDirArgAvoidsPrefixCollision(t *testing.T) {
	args := [][]byte{
		[]byte("/Applications/Chrome"),
		[]byte("--user-data-dir=/var/lib/pt/profile10"),
		[]byte("--remote-debugging-port=9222"),
	}
	if cmdlineHasExactArg(args, "--user-data-dir=/var/lib/pt/profile1") {
		t.Fatal("profile1 matched profile10")
	}
	if !cmdlineHasExactArg(args, "--user-data-dir=/var/lib/pt/profile10") {
		t.Fatal("profile10 should match exact user-data-dir arg")
	}
}

func TestCleanup_RemoteCDP_SkipsKillLogic(t *testing.T) {
	withShortenedGrace(t, 20*time.Millisecond, 20*time.Millisecond)
	termCount, killCount := withMockedProcessLookup(t, func() []int { return []int{1} })

	ctx := context.TODO()
	b := New(ctx, ctx, &config.RuntimeConfig{
		ProfileDir:   "/tmp/pinchtab-test-profile",
		RemoteCDPURL: "http://127.0.0.1:9222",
	})
	b.Cleanup()

	if got := termCount.Load(); got != 0 {
		t.Errorf("remote-CDP cleanup must not SIGTERM, got %d", got)
	}
	if got := killCount.Load(); got != 0 {
		t.Errorf("remote-CDP cleanup must not SIGKILL, got %d", got)
	}
}
