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

	interval := time.Second / time.Duration(rec.fps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var diskBytes atomic.Int64

	for {
		select {
		case <-rec.stopCh:
			return
		case <-rec.tabCtx.Done():
			rec.mu.Lock()
			rec.state = stateAborted
			rec.stopReason = "tab_closed"
			rec.cleanup()
			rec.mu.Unlock()
			slog.Info("recording aborted: tab context canceled", "tab", rec.tabID)
			return
		case <-deadline.C:
			rec.transitionToLimitReached("max_duration")
			slog.Info("recording stopped: max duration reached", "tab", rec.tabID)
			rec.scheduleLimitCleanup()
			return
		case <-ticker.C:
			if reason := rec.checkLimits(&diskBytes); reason != "" {
				rec.transitionToLimitReached(reason)
				slog.Info("recording stopped: "+reason, "tab", rec.tabID)
				rec.scheduleLimitCleanup()
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

func (rec *recorder) transitionToLimitReached(reason string) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.state = stateLimitReached
	rec.stopReason = reason
}

// scheduleLimitCleanup starts a goroutine that auto-cleans the recording
// after limitCleanupGrace if nobody calls stop().
func (rec *recorder) scheduleLimitCleanup() {
	go func() {
		timer := time.NewTimer(limitCleanupGrace)
		defer timer.Stop()
		select {
		case <-rec.stopCh:
			return
		case <-timer.C:
			rec.mu.Lock()
			defer rec.mu.Unlock()
			if rec.state == stateLimitReached {
				slog.Info("recording auto-cleanup after limit grace period", "tab", rec.tabID)
				rec.cleanup()
				rec.state = stateIdle
				rec.stopReason = ""
			}
		}
	}()
}
