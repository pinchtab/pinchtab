package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

// M10 regression: the recovery ladder's profile-lock hook path. A nonexistent
// binary makes the exec allocator fail fast and deterministically; the
// profile-lock clear must run exactly once (the retriedProfileLock guard
// prevents loops), and chrome's classifier never reports silent-drop, so the
// quarantine hook must not fire.
func TestStartBrowserWithRecovery_ProfileLockClearedOnce(t *testing.T) {
	cfg := &config.RuntimeConfig{
		DefaultBrowser: config.BrowserChrome,
		ProfileDir:     t.TempDir(),
	}
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath("/nonexistent/pinchtab-m10-test-binary"),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	}

	var clearCalls, quarantineCalls atomic.Int32
	hooks := Hooks{
		IsProfileLockError: func(string) bool { return true },
		ClearStaleProfileLocks: func(profileDir, errMsg string) (bool, error) {
			clearCalls.Add(1)
			return true, nil
		},
		QuarantineCorruptedProfile: func(profileDir string) (string, error) {
			quarantineCalls.Add(1)
			return profileDir + ".quarantine", nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _, _, err := startBrowserWithRecovery(ctx, cfg, nil, opts, 0, hooks, launchGeoAlignment{}, false, false)
	if err == nil {
		t.Fatal("expected startup failure with a nonexistent binary")
	}
	if got := clearCalls.Load(); got != 1 {
		t.Fatalf("ClearStaleProfileLocks calls = %d, want exactly 1 (retry guard)", got)
	}
	if got := quarantineCalls.Load(); got != 0 {
		t.Fatalf("QuarantineCorruptedProfile calls = %d, want 0 (chrome never classifies silent-drop)", got)
	}
}
