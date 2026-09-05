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
	maxRecordFrames   = 9000      // 5min × 30fps
	maxGIFFrames      = 600       // ~2min at 5fps
	maxTempBytes      = 1 << 30   // 1 GB disk
	maxOutputBytes    = 256 << 20 // 256 MB encoded
	encodeTimeout     = 2 * time.Minute
	limitCleanupGrace = 5 * time.Minute
	maxFPS            = 30
	maxQuality        = 100
	maxScale          = 1.0
	// firstFrameGrace is the slack added to one frame interval before a stop with no
	// frames is called a failure. The tick and the capture are not the same instant —
	// the capture is a round trip — so waiting the bare interval races the frame it is
	// waiting for.
	firstFrameGrace = 50 * time.Millisecond
)

// frameInterval is how long one frame takes at a given rate, and it is the one owner of
// that arithmetic: the capture loop's ticker and stop()'s first-frame wait must agree, or
// stop resumes discarding recordings that were about to produce their first frame.
func frameInterval(fps int) time.Duration {
	if fps <= 0 {
		return 0
	}
	return time.Second / time.Duration(fps)
}

// noFramesError says what went wrong and what to do about it, in the house style the
// ffmpeg refusal set. The bare "no frames captured" it replaces named the symptom the
// encoder saw rather than a cause the caller could act on, and the recording was already
// gone by the time they read it. Every arm names the elapsed time, the rate, and what one
// frame costs at that rate, because those three are what decide the next attempt.
func noFramesError(elapsed time.Duration, fps int, stopReason string) error {
	interval := frameInterval(fps)
	elapsed = elapsed.Round(time.Millisecond)
	switch {
	case stopReason != "":
		return fmt.Errorf("no frames captured: the recording ran %s at %d fps and ended early (%s) before its first frame, which needs %s at that rate",
			elapsed, fps, stopReason, interval)
	case elapsed < interval:
		return fmt.Errorf("no frames captured: the recording ran %s at %d fps and stopped before its first frame, which needs %s at that rate; record for longer or raise the frame rate",
			elapsed, fps, interval)
	default:
		return fmt.Errorf("no frames captured: the recording ran %s at %d fps, longer than the %s one frame needs, so the capture itself produced nothing; check the tab is still open and rendering",
			elapsed, fps, interval)
	}
}

// maxGIFEncodeBytes bounds the total in-memory paletted GIF frame data. It is the
// real GIF safeguard now that --scale is honored at full resolution: oversized
// captures simply fit fewer frames before truncation rather than being downscaled.
// A var (not const) so tests can lower it to exercise budget truncation.
var maxGIFEncodeBytes int64 = 256 << 20 // 256 MB

type recorderState int

const (
	stateIdle         recorderState = iota
	stateRecording                  // capture loop is running
	stateLimitReached               // capture stopped due to limit; frames on disk awaiting stop()
	stateStopping                   // stop() called; waiting for capture loop to finish
	stateEncoding                   // encoding frames to output format
	stateFinished                   // encoding complete; cleanup pending
	stateAborted                    // tab context was cancelled; frames on disk awaiting stop() for the grace window
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
	Warning    string  `json:"warning,omitempty"`
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
	encodeWarn   string // non-fatal encode warning (e.g. GIF truncated to fit budget)
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
	if rec.state == stateIdle {
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
	stopCh := rec.stopCh
	doneCh := rec.doneCh
	format := rec.format
	tmpDir := rec.tmpDir
	fps := rec.fps
	scale := rec.scale
	startTime := rec.startTime
	frames := rec.frameNum
	rec.mu.Unlock()

	// Stopping before the first frame is due is not a failure, it is a recording that
	// has not reached its first tick — which is exactly what "start, one fast action,
	// stop" does. Waiting out the remainder of one frame interval turns the commonest
	// scripted shape from a discarded recording into a one-frame one. Nothing is waited
	// for once a frame exists, so longer recordings stop as immediately as before.
	if frames == 0 {
		if remaining := frameInterval(fps) + firstFrameGrace - time.Since(startTime); remaining > 0 {
			timer := time.NewTimer(remaining)
			select {
			case <-doneCh:
			case <-timer.C:
			}
			timer.Stop()
		}
	}
	close(stopCh)

	<-doneCh

	// Read frameNum only after the capture loop has fully drained: once stopCh
	// closes, the loop can still service an already-ready ticker tick and write
	// one more frame before it observes stopCh and returns, so a pre-drain
	// snapshot under-reports (and can read 0 after a frame was written).
	rec.mu.Lock()
	frameNum := rec.frameNum
	stopReason := rec.stopReason
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
		return result, noFramesError(time.Since(startTime), fps, stopReason)
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
		warn, encErr := encodeToFile(tmpDir, tmpOut, format, fps, scale)
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
		rec.encodeWarn = warn
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
	rec.encodeWarn = ""
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
		s.Warning = rec.encodeWarn
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
