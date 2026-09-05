package handlers

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

func (rec *recorder) captureLoop() {
	defer close(rec.doneCh)

	deadline := time.NewTimer(maxRecordDuration)
	defer deadline.Stop()

	ticker := time.NewTicker(frameInterval(rec.fps))
	defer ticker.Stop()

	var diskBytes atomic.Int64

	for {
		select {
		case <-rec.stopCh:
			return
		case <-rec.tabCtx.Done():
			// The tab is gone — closed, or its browser died — but the frames are
			// not: they stay on disk for the grace window exactly as a limit does,
			// so stop() can still encode what was captured. Warn, not info: the
			// run this was filming is usually the one that cannot be repeated.
			frames := rec.transitionToEnded(stateAborted, "tab_closed")
			slog.Warn("recording ended early: tab closed", "tab", rec.tabID, "reason", "tab_closed", "frames", frames)
			rec.scheduleGraceCleanup()
			return
		case <-deadline.C:
			rec.transitionToEnded(stateLimitReached, "max_duration")
			slog.Info("recording stopped: max duration reached", "tab", rec.tabID)
			rec.scheduleGraceCleanup()
			return
		case <-ticker.C:
			if reason := rec.checkLimits(&diskBytes); reason != "" {
				rec.transitionToEnded(stateLimitReached, reason)
				slog.Info("recording stopped: "+reason, "tab", rec.tabID)
				rec.scheduleGraceCleanup()
				return
			}
			rec.writeFrame(&diskBytes)
		}
	}
}

func (rec *recorder) checkLimits(diskBytes *atomic.Int64) string {
	rec.mu.Lock()
	frames := rec.frameNum
	rec.mu.Unlock()

	if frames >= maxRecordFrames {
		return "max_frames"
	}
	if diskBytes.Load() >= int64(maxTempBytes) {
		return "disk_limit"
	}
	return ""
}

func (rec *recorder) writeFrame(diskBytes *atomic.Int64) {
	frame, err := rec.captureFrame(rec.tabCtx, rec.quality)
	if err != nil {
		slog.Debug("recording frame capture failed", "err", err)
		return
	}

	rec.mu.Lock()
	rec.frameNum++
	path := filepath.Join(rec.tmpDir, fmt.Sprintf("frame_%06d.jpg", rec.frameNum))
	rec.mu.Unlock()

	if err := os.WriteFile(path, frame, 0600); err != nil {
		slog.Debug("recording frame write failed", "err", err)
	} else {
		diskBytes.Add(int64(len(frame)))
	}
}

// transitionToEnded records that capture stopped without tearing anything
// down: the frames stay for stop() to collect. It reports the frame count.
func (rec *recorder) transitionToEnded(state recorderState, reason string) int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.state = state
	rec.stopReason = reason
	return rec.frameNum
}

// scheduleGraceCleanup starts a goroutine that auto-cleans an ended recording
// after limitCleanupGrace if nobody calls stop().
func (rec *recorder) scheduleGraceCleanup() {
	go func() {
		timer := time.NewTimer(limitCleanupGrace)
		defer timer.Stop()
		select {
		case <-rec.stopCh:
			return
		case <-timer.C:
			rec.mu.Lock()
			defer rec.mu.Unlock()
			if rec.state == stateLimitReached || rec.state == stateAborted {
				slog.Info("recording auto-cleanup after grace period", "tab", rec.tabID, "reason", rec.stopReason)
				rec.cleanup()
				rec.state = stateIdle
				rec.stopReason = ""
			}
		}
	}()
}
