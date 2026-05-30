package handlers

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/session"
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
	frameNum := rec.frameNum
	fps := rec.fps
	scale := rec.scale
	rec.mu.Unlock()

	<-doneCh

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

func encodeToFile(tmpDir, outputPath, format string, fps int, scale float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
	defer cancel()

	switch format {
	case "gif":
		return encodeGIFToFile(tmpDir, outputPath, fps, scale)
	case "webm":
		return encodeFFmpegToFile(ctx, tmpDir, outputPath, format, fps, scale, "libvpx", "-crf", "10", "-b:v", "1M")
	case "mp4":
		return encodeFFmpegToFile(ctx, tmpDir, outputPath, format, fps, scale, "libx264", "-pix_fmt", "yuv420p", "-crf", "23")
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func encodeGIFToFile(tmpDir, outputPath string, fps int, scale float64) error {
	files, err := filepath.Glob(filepath.Join(tmpDir, "frame_*.jpg"))
	if err != nil {
		return err
	}
	sort.Strings(files)

	if len(files) > maxGIFFrames {
		files = files[:maxGIFFrames]
	}

	delay := 100 / fps
	if delay < 1 {
		delay = 1
	}

	outFile, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create gif output: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	g := &gif.GIF{LoopCount: 0}
	var palettedBytes int64

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			continue
		}

		if scale != 1.0 && scale > 0 {
			img = scaleImage(img, scale)
		}

		bounds := img.Bounds()
		pixels := bounds.Dx() * bounds.Dy()
		if pixels > maxGIFFramePixels {
			ratio := float64(maxGIFFramePixels) / float64(pixels)
			img = scaleImage(img, ratio)
			bounds = img.Bounds()
			pixels = bounds.Dx() * bounds.Dy()
		}

		if palettedBytes+int64(pixels) > maxGIFEncodeBytes {
			slog.Info("GIF encode: memory budget reached, truncating", "frames", len(g.Image))
			break
		}

		paletted := image.NewPaletted(bounds, palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, bounds, img, bounds.Min)
		g.Image = append(g.Image, paletted)
		g.Delay = append(g.Delay, delay)
		palettedBytes += int64(pixels)
	}

	if len(g.Image) == 0 {
		return fmt.Errorf("no frames to encode")
	}

	lw := &limitedWriter{w: outFile, max: int64(maxOutputBytes)}
	if err := gif.EncodeAll(lw, g); err != nil {
		return fmt.Errorf("gif encode: %w", err)
	}
	return outFile.Close()
}

func encodeFFmpegToFile(ctx context.Context, tmpDir, outputPath, format string, fps int, scale float64, codec string, extraArgs ...string) error {
	// Pre-create exclusively to prevent symlink following by ffmpeg.
	f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	_ = f.Close()

	args := []string{
		"-y", // safe: we own the file from O_EXCL above
		"-framerate", strconv.Itoa(fps),
		"-i", filepath.Join(tmpDir, "frame_%06d.jpg"),
	}

	if scale != 1.0 && scale > 0 {
		args = append(args, "-vf",
			fmt.Sprintf("scale=trunc(iw*%g/2)*2:trunc(ih*%g/2)*2", scale, scale))
	}

	args = append(args, "-c:v", codec)
	args = append(args, extraArgs...)
	args = append(args, "-fs", strconv.Itoa(maxOutputBytes))
	args = append(args, outputPath)

	// #nosec G204 -- ffmpeg executable is fixed and args are built from validated,
	// bounded internal values (format/fps/scale/output path), not shell-expanded input.
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg encode: %w\n%s", err, stderr.String())
	}
	return nil
}

func scaleImage(src image.Image, scale float64) image.Image {
	bounds := src.Bounds()
	w := int(float64(bounds.Dx()) * scale)
	h := int(float64(bounds.Dy()) * scale)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			srcX := bounds.Min.X + int(float64(x)/scale)
			srcY := bounds.Min.Y + int(float64(y)/scale)
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			if srcY >= bounds.Max.Y {
				srcY = bounds.Max.Y - 1
			}
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// HandleRecordStart starts a recording session for a tab.
func (h *Handlers) HandleRecordStart(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowScreencast {
		httpx.ErrorCode(w, 403, "recording_disabled",
			httpx.DisabledEndpointMessage("recording", "security.allowScreencast"), false,
			map[string]any{
				"setting": "security.allowScreencast",
				"hint":    "Recording requires screen capture to be enabled.",
				"remedy":  "pinchtab config set security.allowScreencast true",
			})
		return
	}

	if !h.ensureChromeOrRespond(w) {
		return
	}

	var req struct {
		TabID   string  `json:"tabId"`
		Format  string  `json:"format"`
		FPS     int     `json:"fps"`
		Quality int     `json:"quality"`
		Scale   float64 `json:"scale"`
	}
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}

	if req.Format == "" {
		req.Format = "gif"
	}
	if req.FPS <= 0 {
		req.FPS = 5
	}
	if req.FPS > maxFPS {
		req.FPS = maxFPS
	}
	if req.Quality <= 0 {
		req.Quality = 80
	}
	if req.Quality > maxQuality {
		req.Quality = maxQuality
	}
	if req.Scale <= 0 {
		req.Scale = 1.0
	}
	if req.Scale > maxScale {
		req.Scale = maxScale
	}

	switch req.Format {
	case "gif":
	case "webm", "mp4":
		if !ffmpegAvailable() {
			httpx.ErrorCode(w, 400, "ffmpeg_required",
				fmt.Sprintf("recording to .%s requires ffmpeg; install it or use .gif", req.Format),
				false, nil)
			return
		}
	default:
		httpx.ErrorCode(w, 400, "invalid_format",
			"supported formats: gif, webm, mp4", false, nil)
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Problem(w, http.StatusNotFound, "tab_not_found", "tab not found", false, nil)
		return
	}

	owner := authenticatedOwner(r)

	if err := h.recorder.start(ctx, resolvedTabID, owner, req.Format, req.FPS, req.Quality, req.Scale); err != nil {
		httpx.ErrorCode(w, 409, "recording_error", err.Error(), false, nil)
		return
	}

	slog.Info("recording started", "tab", resolvedTabID, "format", req.Format, "fps", req.FPS)
	httpx.JSON(w, 200, map[string]any{
		"status":  "recording",
		"format":  req.Format,
		"fps":     req.FPS,
		"quality": req.Quality,
		"tabId":   resolvedTabID,
	})
}

// HandleRecordStop stops the active recording. If discard is false (default),
// encoding runs in the background into a server-controlled recordings directory
// and the endpoint returns the path immediately. If discard is true, frames are
// dropped without encoding. Use /record/status to check encoding progress.
func (h *Handlers) HandleRecordStop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Discard bool `json:"discard"`
	}
	_ = httpx.DecodeJSONBody(w, r, 0, &req)

	var outputPath string
	if !req.Discard {
		var err error
		outputPath, err = h.recordingsOutputPath()
		if err != nil {
			httpx.ErrorCode(w, 500, "recording_error", err.Error(), false, nil)
			return
		}
	}

	owner := authenticatedOwner(r)
	result, err := h.recorder.stop(owner, outputPath)
	if err != nil {
		httpx.ErrorCode(w, 400, "recording_error", err.Error(), false, nil)
		return
	}

	if req.Discard {
		httpx.JSON(w, 200, map[string]any{
			"status": "discarded",
			"format": result.Format,
			"frames": result.Frames,
		})
		return
	}

	slog.Info("recording stopped, encoding in background",
		"format", result.Format, "frames", result.Frames, "path", result.OutputPath)
	httpx.JSON(w, 200, map[string]any{
		"status": "encoding",
		"path":   result.OutputPath,
		"format": result.Format,
		"frames": result.Frames,
		"hint":   fmt.Sprintf("Encoding %d frames to %s. Use `record status` to check progress — the file will appear at the path once encoding completes.", result.Frames, result.OutputPath),
	})
}

// recordingsOutputPath returns a unique output path inside the server-controlled
// recordings directory. The caller never chooses the path — only the server does.
func (h *Handlers) recordingsOutputPath() (string, error) {
	dir := filepath.Join(h.Config.StateDir, "recordings")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create recordings dir: %w", err)
	}
	format := h.recorder.activeFormat()
	ext := "." + format
	name := fmt.Sprintf("rec_%s%s", time.Now().Format("20060102_150405"), ext)
	return filepath.Join(dir, name), nil
}

// HandleRecordStatus returns the current recording status.
func (h *Handlers) HandleRecordStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, h.recorder.status())
}

// authenticatedOwner derives a non-secret owner key from the authenticated
// session context. Returns "" for anonymous (unauthenticated) requests.
// Anonymous recordings can be stopped by any caller, including other anonymous
// callers — this is intentional for the single-user local model.
func authenticatedOwner(r *http.Request) string {
	if sess, ok := session.FromRequest(r); ok && sess != nil {
		if id := strings.TrimSpace(sess.AgentID); id != "" {
			return "session:" + id
		}
		if id := strings.TrimSpace(sess.ID); id != "" {
			return "session:" + id
		}
	}
	if IsTrustedInternalProxy(r) {
		if id := strings.TrimSpace(r.Header.Get(activity.HeaderPTSessionID)); id != "" {
			return "proxy-session:" + id
		}
		if id := strings.TrimSpace(r.Header.Get(activity.HeaderAgentID)); id != "" {
			return "proxy-agent:" + id
		}
	}
	return ""
}

type limitedWriter struct {
	w   io.Writer
	n   int64
	max int64
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.n+int64(len(p)) > lw.max {
		return 0, fmt.Errorf("output exceeds maximum size (%d bytes)", lw.max)
	}
	n, err := lw.w.Write(p)
	lw.n += int64(n)
	return n, err
}
