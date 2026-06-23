package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

const (
	maxRecordDuration = 5 * time.Minute
	maxRecordFrames   = 9000 // 5min × 30fps
	maxGIFFrames      = 600  // ~2min at 5fps
	maxGIFFramePixels = 1280 * 720
	maxGIFEncodeBytes = 256 << 20 // 256 MB total paletted frame budget
	maxTempBytes      = 1 << 30   // 1 GB disk
	maxOutputBytes    = 256 << 20 // 256 MB encoded
	encodeTimeout     = 2 * time.Minute
	limitCleanupGrace = 5 * time.Minute
	maxFPS            = 30
	maxQuality        = 100
	maxScale          = 1.0
)

type recorderState int

const (
	stateIdle         recorderState = iota
	stateRecording                  // capture loop is running
	stateLimitReached               // capture stopped due to limit; frames on disk awaiting stop()
	stateStopping                   // stop() called; waiting for capture loop to finish
	stateEncoding                   // encoding frames to output format
	stateFinished                   // encoding complete; cleanup pending
	stateAborted                    // tab context was cancelled; resources cleaned up
)

func (s recorderState) String() string {
	switch s {
	case stateIdle:
		return "idle"
	case stateRecording:
		return "recording"
	case stateLimitReached:
		return "limit_reached"
	case stateStopping:
		return "stopping"
	case stateEncoding:
		return "encoding"
	case stateFinished:
		return "finished"
	case stateAborted:
		return "aborted"
	default:
		return "unknown"
	}
}

type RecordingStatus struct {
	Active     bool    `json:"active"`
	State      string  `json:"state"`
	StopReason string  `json:"stopReason,omitempty"`
	Format     string  `json:"format,omitempty"`
	Duration   float64 `json:"durationSeconds,omitempty"`
	Frames     int     `json:"frames"`
	TabID      string  `json:"tabId,omitempty"`
	FPS        int     `json:"fps,omitempty"`
	OutputPath string  `json:"outputPath,omitempty"`
	Error      string  `json:"error,omitempty"`
}

type recorder struct {
	mu           sync.Mutex
	state        recorderState
	stopReason   string
	tabCtx       context.Context
	tabCancel    context.CancelFunc
	tabID        string
	owner        string // opaque key derived from authenticated session, never exposed
	format       string
	fps          int
	quality      int
	scale        float64
	tmpDir       string
	frameNum     int
	startTime    time.Time
	stopCh       chan struct{}
	doneCh       chan struct{}
	outputPath   string // final destination set by stop(); encoding writes here
	encodeErr    error  // set by background encode goroutine
	captureFrame func(ctx context.Context, quality int) ([]byte, error)
}

func (rec *recorder) start(tabCtx context.Context, tabID, owner, format string, fps, quality int, scale float64) error {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.state == stateAborted || rec.state == stateFinished {
		rec.cleanup()
		rec.state = stateIdle
		rec.stopReason = ""
	}
	if rec.state != stateIdle {
		return fmt.Errorf("recording already in progress")
	}

	tmpDir, err := os.MkdirTemp("", "pinchtab-recording-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	ctx, cancel := context.WithCancel(tabCtx)

	rec.state = stateRecording
	rec.stopReason = ""
	rec.tabCtx = ctx
	rec.tabCancel = cancel
	rec.tabID = tabID
	rec.owner = owner
	rec.format = format
	rec.fps = fps
	rec.quality = quality
	rec.scale = scale
	rec.tmpDir = tmpDir
	rec.frameNum = 0
	rec.startTime = time.Now()
	rec.stopCh = make(chan struct{})
	rec.doneCh = make(chan struct{})

	go rec.captureLoop()
	return nil
}

func (rec *recorder) stop(callerOwner, outputPath string) (RecordStopResult, error) {
	rec.mu.Lock()
	if rec.state == stateIdle || rec.state == stateAborted {
		rec.mu.Unlock()
		return RecordStopResult{}, fmt.Errorf("no active recording")
	}
	if rec.state == stateEncoding {
		r := RecordStopResult{
			OutputPath: rec.outputPath,
			Format:     rec.format,
			Frames:     rec.frameNum,
		}
		rec.mu.Unlock()
		return r, fmt.Errorf("already encoding to %s — use record status to check progress", rec.outputPath)
	}
	if rec.state == stateStopping || rec.state == stateFinished {
		rec.mu.Unlock()
		return RecordStopResult{}, fmt.Errorf("no active recording")
	}
	if rec.owner != "" && callerOwner != rec.owner {
		rec.mu.Unlock()
		return RecordStopResult{}, fmt.Errorf("recording owned by another session")
	}
	rec.state = stateStopping
	rec.outputPath = outputPath
	close(rec.stopCh)
	doneCh := rec.doneCh
	format := rec.format
	tmpDir := rec.tmpDir
	fps := rec.fps
	scale := rec.scale
	rec.mu.Unlock()

	<-doneCh

	// Read frameNum only after the capture loop has fully drained: once stopCh
	// closes, the loop can still service an already-ready ticker tick and write
	// one more frame before it observes stopCh and returns, so a pre-drain
	// snapshot under-reports (and can read 0 after a frame was written).
	rec.mu.Lock()
	frameNum := rec.frameNum
	rec.mu.Unlock()

	result := RecordStopResult{
		OutputPath: outputPath,
		Format:     format,
		Frames:     frameNum,
	}

	if frameNum == 0 {
		rec.mu.Lock()
		rec.state = stateFinished
		rec.cleanup()
		rec.state = stateIdle
		rec.mu.Unlock()
		return result, fmt.Errorf("no frames captured")
	}

	if outputPath == "" {
		rec.mu.Lock()
		rec.state = stateFinished
		rec.cleanup()
		rec.state = stateIdle
		rec.mu.Unlock()
		slog.Info("recording discarded", "frames", frameNum, "format", format)
		return result, nil
	}

	rec.mu.Lock()
	rec.state = stateEncoding
	rec.mu.Unlock()

	go func() {
		tmpOut := outputPath + ".encoding.tmp"
		encErr := encodeToFile(tmpDir, tmpOut, format, fps, scale)
		if encErr == nil {
			encErr = os.Rename(tmpOut, outputPath)
			if encErr != nil {
				_ = os.Remove(tmpOut)
			}
		} else {
			_ = os.Remove(tmpOut)
		}

		rec.mu.Lock()
		rec.encodeErr = encErr
		rec.state = stateFinished
		if rec.tabCancel != nil {
			rec.tabCancel()
			rec.tabCancel = nil
		}
		if rec.tmpDir != "" {
			_ = os.RemoveAll(rec.tmpDir)
			rec.tmpDir = ""
		}
		rec.mu.Unlock()

		if encErr != nil {
			slog.Error("recording encode failed", "path", outputPath, "err", encErr)
		} else {
			info, _ := os.Stat(outputPath)
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			slog.Info("recording saved", "path", outputPath, "bytes", size)
		}
	}()

	return result, nil
}

// RecordStopResult is returned immediately by stop(); encoding continues in the background.
type RecordStopResult struct {
	OutputPath string
	Format     string
	Frames     int
}

// cleanup releases resources but does not change state — callers set the
// final state (stateIdle, stateAborted, etc.) before or after calling this.
func (rec *recorder) cleanup() {
	if rec.tabCancel != nil {
		rec.tabCancel()
		rec.tabCancel = nil
	}
	if rec.tmpDir != "" {
		_ = os.RemoveAll(rec.tmpDir)
	}
	rec.tmpDir = ""
	rec.owner = ""
	rec.outputPath = ""
	rec.encodeErr = nil
}

func (rec *recorder) activeFormat() string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.format == "" {
		return "gif"
	}
	return rec.format
}

func (rec *recorder) status() RecordingStatus {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.state == stateIdle {
		return RecordingStatus{State: stateIdle.String()}
	}
	if rec.state == stateFinished {
		s := RecordingStatus{
			State:      stateFinished.String(),
			Format:     rec.format,
			Frames:     rec.frameNum,
			OutputPath: rec.outputPath,
		}
		if rec.encodeErr != nil {
			s.Error = rec.encodeErr.Error()
		}
		return s
	}
	return RecordingStatus{
		Active:     rec.state != stateAborted,
		State:      rec.state.String(),
		StopReason: rec.stopReason,
		Format:     rec.format,
		Duration:   time.Since(rec.startTime).Seconds(),
		Frames:     rec.frameNum,
		TabID:      rec.tabID,
		FPS:        rec.fps,
		OutputPath: rec.outputPath,
	}
}
