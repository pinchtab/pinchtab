package bridge

import "time"

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
	deadline := time.Now().Add(timeout)
	for {
		if len(findChromePIDsByProfileDirFunc(profileDir)) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		remaining := time.Until(deadline)
		sleep := chromeExitPollInterval
		if remaining < sleep {
			sleep = remaining
		}
		if sleep <= 0 {
			return false
		}
		time.Sleep(sleep)
	}
}
