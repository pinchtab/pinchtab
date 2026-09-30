package bridge

import (
	"context"
	"time"

	"github.com/pinchtab/pinchtab/internal/readiness"
)

// var (not const) so tests can shrink the poll interval.
var chromeExitPollInterval = 50 * time.Millisecond

// Test seams: counted by unit tests instead of signalling real processes.
var (
	terminateChromeByProfileDirFunc = terminateChromeByProfileDir
	killChromeByProfileDirFunc      = killChromeByProfileDir
)

func waitForChromeExit(profileDir string, timeout time.Duration) bool {
	if profileDir == "" {
		return true
	}
	_, err := readiness.WaitUntil(context.Background(), timeout, chromeExitPollInterval, func() (struct{}, bool, error) {
		return struct{}{}, len(findChromePIDsByProfileDirFunc(profileDir)) == 0, nil
	})
	return err == nil
}
