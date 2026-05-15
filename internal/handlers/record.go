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
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

const (
	maxRecordDuration = 5 * time.Minute
	maxRecordFrames   = 9000      // 5min × 30fps
	maxTempBytes      = 1 << 30   // 1 GB disk
	maxOutputBytes    = 256 << 20 // 256 MB encoded
	encodeTimeout     = 2 * time.Minute
	maxFPS            = 30
	maxQuality        = 100
	maxScale          = 1.0
)

type RecordingStatus struct {
	Active   bool    `json:"active"`
	Format   string  `json:"format,omitempty"`
	Duration float64 `json:"durationSeconds,omitempty"`
	Frames   int     `json:"frames"`
	TabID    string  `json:"tabId,omitempty"`
	FPS      int     `json:"fps,omitempty"`
	Owner    string  `json:"owner,omitempty"`
}

type recorder struct {
	mu        sync.Mutex
	active    bool
	stopping  bool
	tabCtx    context.Context
	tabCancel context.CancelFunc
	tabID     string
	owner     string
	format    string
	fps       int
	quality   int
	scale     float64
	tmpDir    string
	frameNum  int
	tempBytes int64
	startTime time.Time
	stopCh    chan struct{}
	doneCh    chan struct{}
}

func (rec *recorder) start(tabCtx context.Context, tabID, owner, format string, fps, quality int, scale float64) error {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.active {
		return fmt.Errorf("recording already in progress")
	}

	tmpDir, err := os.MkdirTemp("", "pinchtab-recording-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	ctx, cancel := context.WithCancel(tabCtx)

	rec.active = true
	rec.stopping = false
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
	rec.tempBytes = 0
	rec.startTime = time.Now()
	rec.stopCh = make(chan struct{})
	rec.doneCh = make(chan struct{})

	go rec.captureLoop()
	return nil
}

func (rec *recorder) stop(callerOwner string) ([]byte, string, error) {
	rec.mu.Lock()
	if !rec.active {
		rec.mu.Unlock()
		return nil, "", fmt.Errorf("no active recording")
	}
	if rec.stopping {
		doneCh := rec.doneCh
		rec.mu.Unlock()
		<-doneCh
		return nil, "", fmt.Errorf("no active recording")
	}
	if callerOwner != "" && rec.owner != "" && callerOwner != rec.owner {
		rec.mu.Unlock()
		return nil, "", fmt.Errorf("recording owned by another session")
	}
	rec.stopping = true
	close(rec.stopCh)
	doneCh := rec.doneCh
	format := rec.format
	tmpDir := rec.tmpDir
	frameNum := rec.frameNum
	fps := rec.fps
	scale := rec.scale
	rec.mu.Unlock()

	<-doneCh

	rec.mu.Lock()
	defer rec.mu.Unlock()
	defer rec.cleanup()

	if frameNum == 0 {
		return nil, "", fmt.Errorf("no frames captured")
	}

	data, err := encode(tmpDir, format, fps, scale)
	if err != nil {
		return nil, "", err
	}

	if len(data) > maxOutputBytes {
		return nil, "", fmt.Errorf("encoded output too large (%d bytes, max %d)", len(data), maxOutputBytes)
	}

	return data, format, nil
}

func (rec *recorder) cleanup() {
	if rec.tabCancel != nil {
		rec.tabCancel()
		rec.tabCancel = nil
	}
	if rec.tmpDir != "" {
		_ = os.RemoveAll(rec.tmpDir)
	}
	rec.active = false
	rec.stopping = false
	rec.tmpDir = ""
	rec.owner = ""
}

func (rec *recorder) status() RecordingStatus {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	if !rec.active {
		return RecordingStatus{}
	}
	return RecordingStatus{
		Active:   true,
		Format:   rec.format,
		Duration: time.Since(rec.startTime).Seconds(),
		Frames:   rec.frameNum,
		TabID:    rec.tabID,
		FPS:      rec.fps,
		Owner:    rec.owner,
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
			rec.cleanup()
			rec.mu.Unlock()
			slog.Info("recording aborted: tab context canceled", "tab", rec.tabID)
			return
		case <-deadline.C:
			slog.Info("recording stopped: max duration reached", "tab", rec.tabID)
			return
		case <-ticker.C:
			rec.mu.Lock()
			if rec.frameNum >= maxRecordFrames {
				rec.mu.Unlock()
				slog.Info("recording stopped: max frames reached", "tab", rec.tabID)
				return
			}
			rec.mu.Unlock()

			if diskBytes.Load() >= int64(maxTempBytes) {
				slog.Info("recording stopped: temp disk limit reached", "tab", rec.tabID)
				return
			}

			frame, err := captureScreencastJPEG(rec.tabCtx, rec.quality)
			if err != nil {
				slog.Debug("recording frame capture failed", "err", err)
				continue
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
	}
}

func encode(tmpDir, format string, fps int, scale float64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
	defer cancel()

	switch format {
	case "gif":
		return encodeGIF(tmpDir, fps, scale)
	case "webm":
		return encodeFFmpeg(ctx, tmpDir, format, fps, scale, "libvpx", "-crf", "10", "-b:v", "1M")
	case "mp4":
		return encodeFFmpeg(ctx, tmpDir, format, fps, scale, "libx264", "-pix_fmt", "yuv420p", "-crf", "23")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

func encodeGIF(tmpDir string, fps int, scale float64) ([]byte, error) {
	files, err := filepath.Glob(filepath.Join(tmpDir, "frame_*.jpg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	delay := 100 / fps
	if delay < 1 {
		delay = 1
	}

	g := &gif.GIF{LoopCount: 0}

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
		paletted := image.NewPaletted(bounds, palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, bounds, img, bounds.Min)
		g.Image = append(g.Image, paletted)
		g.Delay = append(g.Delay, delay)
	}

	if len(g.Image) == 0 {
		return nil, fmt.Errorf("no frames to encode")
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		return nil, fmt.Errorf("gif encode: %w", err)
	}
	return buf.Bytes(), nil
}

func encodeFFmpeg(ctx context.Context, tmpDir, format string, fps int, scale float64, codec string, extraArgs ...string) ([]byte, error) {
	outFile := filepath.Join(tmpDir, "output."+format)

	args := []string{
		"-y",
		"-framerate", strconv.Itoa(fps),
		"-i", filepath.Join(tmpDir, "frame_%06d.jpg"),
	}

	if scale != 1.0 && scale > 0 {
		args = append(args, "-vf",
			fmt.Sprintf("scale=trunc(iw*%g/2)*2:trunc(ih*%g/2)*2", scale, scale))
	}

	args = append(args, "-c:v", codec)
	args = append(args, extraArgs...)
	args = append(args, outFile)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg encode: %w\n%s", err, stderr.String())
	}
	return os.ReadFile(outFile)
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
			map[string]any{"setting": "security.allowScreencast"})
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

	owner := requestOwner(r)

	if err := h.recorder.start(ctx, resolvedTabID, owner, req.Format, req.FPS, req.Quality, req.Scale); err != nil {
		httpx.ErrorCode(w, 409, "recording_error", err.Error(), false, nil)
		return
	}

	slog.Info("recording started", "tab", resolvedTabID, "format", req.Format, "fps", req.FPS, "owner", owner)
	httpx.JSON(w, 200, map[string]any{
		"status":  "recording",
		"format":  req.Format,
		"fps":     req.FPS,
		"quality": req.Quality,
		"tabId":   resolvedTabID,
	})
}

// HandleRecordStop stops the active recording and returns the encoded file.
func (h *Handlers) HandleRecordStop(w http.ResponseWriter, r *http.Request) {
	owner := requestOwner(r)
	data, format, err := h.recorder.stop(owner)
	if err != nil {
		httpx.ErrorCode(w, 400, "recording_error", err.Error(), false, nil)
		return
	}

	contentType := "application/octet-stream"
	switch format {
	case "gif":
		contentType = "image/gif"
	case "webm":
		contentType = "video/webm"
	case "mp4":
		contentType = "video/mp4"
	}

	slog.Info("recording stopped", "format", format, "size", len(data))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=recording.%s", format))
	w.WriteHeader(200)
	_, _ = w.Write(data)
}

// HandleRecordStatus returns the current recording status.
func (h *Handlers) HandleRecordStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, h.recorder.status())
}

func requestOwner(r *http.Request) string {
	if sid := r.Header.Get("X-Pinchtab-Session-Id"); sid != "" {
		return sid
	}
	if aid := r.Header.Get("X-Agent-Id"); aid != "" {
		return aid
	}
	return ""
}
