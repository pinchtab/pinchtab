package bridge

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
		DefaultBrowser: config.BrowserCloak,
		ProfileDir:     "/tmp/pinchtab-test-profile",
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

func TestCleanup_CDPAttachURLOnly_SkipsKillLogic(t *testing.T) {
	withShortenedGrace(t, 20*time.Millisecond, 20*time.Millisecond)
	termCount, killCount := withMockedProcessLookup(t, func() []int { return []int{1} })

	ctx := context.TODO()
	b := New(ctx, ctx, &config.RuntimeConfig{
		ProfileDir:   "/tmp/pinchtab-test-profile",
		CDPAttachURL: "http://127.0.0.1:9222",
	})
	b.Cleanup()

	if got := termCount.Load(); got != 0 {
		t.Errorf("CDP attach cleanup must not SIGTERM, got %d", got)
	}
	if got := killCount.Load(); got != 0 {
		t.Errorf("CDP attach cleanup must not SIGKILL, got %d", got)
	}
}

func TestRestartBrowser_CDPAttachURLOnly_SkipsKillLogic(t *testing.T) {
	origDrain := browserRestartDrainWindow
	browserRestartDrainWindow = time.Millisecond
	t.Cleanup(func() { browserRestartDrainWindow = origDrain })
	_, killCount := withMockedProcessLookup(t, func() []int { return []int{1} })

	cfg := &config.RuntimeConfig{
		ProfileDir:         t.TempDir(),
		CDPAttachURL:       "http://127.0.0.1:1",
		AttachAllowHosts:   []string{"127.0.0.1"},
		AttachAllowSchemes: []string{"http"},
	}
	b := New(context.Background(), nil, cfg)
	if err := b.RestartBrowser(cfg); err == nil {
		t.Fatal("restart against an unreachable external CDP endpoint unexpectedly succeeded")
	}
	if got := killCount.Load(); got != 0 {
		t.Errorf("external restart must not SIGKILL, got %d", got)
	}
}

func TestFetchPauseSuppressionLifecycle(t *testing.T) {
	b := &Bridge{}
	flag := b.fetchPauseSuppression("t1")
	if flag == nil || flag.Load() {
		t.Fatal("fresh flag should exist and be false")
	}
	b.SetFetchPauseSuppressed("t1", true)
	if !flag.Load() {
		t.Fatal("the same flag instance must observe SetFetchPauseSuppressed")
	}
	b.dropFetchPauseSuppression("t1")
	if recreated := b.fetchPauseSuppression("t1"); recreated == flag || recreated.Load() {
		t.Fatal("dropped flag should be recreated fresh and unset")
	}
	if b.fetchPauseSuppression("") != nil {
		t.Fatal("empty tab ID must not allocate a flag")
	}
}

const (
	fakeInit       = 1
	fakePeerServer = 1 << 28
	fakeChromeMain = 1<<28 + 1
	fakeChromeGPU  = 1<<28 + 2
	fakeZygote     = 1<<28 + 3
	fakeRenderer   = 1<<28 + 4
	fakeOwnBridge  = 1<<28 + 5
	fakeDeadParent = 1<<28 + 6
)

func withProcessTree(t *testing.T, parents map[int]int, chromeByDir map[string][]int) *[]int {
	t.Helper()
	origParents := parentPIDsFunc
	origAll := allChromePIDsByProfileDirFunc
	origKill := forceKillPIDsFunc
	t.Cleanup(func() {
		parentPIDsFunc = origParents
		allChromePIDsByProfileDirFunc = origAll
		forceKillPIDsFunc = origKill
	})

	var killed []int
	parentPIDsFunc = func() map[int]int { return parents }
	allChromePIDsByProfileDirFunc = func(dir string) []int { return chromeByDir[dir] }
	forceKillPIDsFunc = func(pids []int) int {
		killed = append(killed, pids...)
		return len(pids)
	}
	return &killed
}

func TestChromeOwnership(t *testing.T) {
	self := os.Getpid()
	chrome := []int{fakeChromeMain, fakeChromeGPU}

	tests := []struct {
		name            string
		parents         map[int]int
		pids            []int
		wantReclaimable []int
		wantOrphaned    []int
	}{
		{
			name: "live peer server's chrome is spared",
			parents: map[int]int{
				fakeInit: 0, fakePeerServer: fakeInit, self: fakeInit,
				fakeChromeMain: fakePeerServer, fakeChromeGPU: fakeChromeMain,
			},
			pids: chrome,
		},
		{
			name: "chrome reparented to init is ownerless",
			parents: map[int]int{
				fakeInit: 0, self: fakeInit,
				fakeChromeMain: fakeInit, fakeChromeGPU: fakeChromeMain,
			},
			pids:            chrome,
			wantReclaimable: chrome,
			wantOrphaned:    chrome,
		},
		{
			name: "windows leaves the dead parent pid in place",
			parents: map[int]int{
				self:           4,
				fakeChromeMain: fakeDeadParent, fakeChromeGPU: fakeChromeMain,
			},
			pids:            chrome,
			wantReclaimable: chrome,
			wantOrphaned:    chrome,
		},
		{
			name: "chrome under this process's own bridge is reclaimable but no orphan",
			parents: map[int]int{
				fakeInit: 0, self: fakeInit, fakeOwnBridge: self,
				fakeChromeMain: fakeOwnBridge, fakeChromeGPU: fakeChromeMain,
			},
			pids:            chrome,
			wantReclaimable: chrome,
		},
		{
			name: "helper behind an unmatched zygote follows the main process",
			parents: map[int]int{
				fakeInit: 0, self: fakeInit,
				fakeChromeMain: fakeInit, fakeZygote: fakeChromeMain, fakeRenderer: fakeZygote,
			},
			pids:            []int{fakeChromeMain, fakeRenderer},
			wantReclaimable: []int{fakeChromeMain, fakeRenderer},
			wantOrphaned:    []int{fakeChromeMain, fakeRenderer},
		},
		{
			name:    "failed process probe proves nothing ownerless",
			parents: nil,
			pids:    chrome,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withProcessTree(t, tt.parents, nil)
			if got := reclaimableChromePIDs(tt.pids); !slices.Equal(got, tt.wantReclaimable) {
				t.Errorf("reclaimable = %v, want %v", got, tt.wantReclaimable)
			}
			if got := orphanedChromePIDs(tt.pids); !slices.Equal(got, tt.wantOrphaned) {
				t.Errorf("orphaned = %v, want %v", got, tt.wantOrphaned)
			}
		})
	}
}

func TestCleanupOrphanedChromeProcesses_KillsOnlyOwnerlessChrome(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	const peerProfile, orphanProfile = "/profiles/peer", "/profiles/orphan"
	killed := withProcessTree(t,
		map[int]int{
			fakeInit: 0, fakePeerServer: fakeInit, os.Getpid(): fakeInit,
			fakeChromeMain: fakePeerServer, fakeChromeGPU: fakeChromeMain,
			fakeRenderer: fakeInit,
		},
		map[string][]int{
			peerProfile:   {fakeChromeMain, fakeChromeGPU},
			orphanProfile: {fakeRenderer},
		})

	CleanupOrphanedChromeProcesses(peerProfile)
	if len(*killed) != 0 {
		t.Fatalf("live peer's chrome was killed: %v", *killed)
	}

	CleanupOrphanedChromeProcesses(orphanProfile)
	if !slices.Equal(*killed, []int{fakeRenderer}) {
		t.Fatalf("killed = %v, want the orphan %d only", *killed, fakeRenderer)
	}
}

func TestParsePIDPairs(t *testing.T) {
	got := parsePIDPairs([]byte("  1     0\r\n  312     1\nnot a row\n 9 x\n\n"))
	want := map[int]int{1: 0, 312: 1}
	if !maps.Equal(got, want) {
		t.Fatalf("parsePIDPairs = %v, want %v", got, want)
	}
}
