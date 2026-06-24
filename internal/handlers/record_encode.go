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
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
)

func encodeToFile(tmpDir, outputPath, format string, fps int, scale float64) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), encodeTimeout)
	defer cancel()

	switch format {
	case "gif":
		return encodeGIFToFile(tmpDir, outputPath, fps, scale)
	case "webm":
		return "", encodeFFmpegToFile(ctx, tmpDir, outputPath, format, fps, scale, "libvpx", "-crf", "10", "-b:v", "1M")
	case "mp4":
		return "", encodeFFmpegToFile(ctx, tmpDir, outputPath, format, fps, scale, "libx264", "-pix_fmt", "yuv420p", "-crf", "23")
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

func encodeGIFToFile(tmpDir, outputPath string, fps int, scale float64) (string, error) {
	files, err := discoverFrames(tmpDir, maxGIFFrames)
	if err != nil {
		return "", err
	}

	delay := 100 / fps
	if delay < 1 {
		delay = 1
	}

	outFile, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", fmt.Errorf("create gif output: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	g := &gif.GIF{LoopCount: 0}
	var palettedBytes int64
	truncated := false

	for _, f := range files {
		img, ok := loadFrame(f)
		if !ok {
			continue
		}

		img = scaleFrameForGIF(img, scale)
		bounds := img.Bounds()
		pixels := bounds.Dx() * bounds.Dy()

		// Honor the requested resolution; cap total frames (not per-frame size) so
		// memory stays bounded. The first frame is always kept so a single large
		// frame still produces a valid GIF.
		if len(g.Image) > 0 && palettedBytes+int64(pixels) > maxGIFEncodeBytes {
			slog.Info("GIF encode: memory budget reached, truncating", "frames", len(g.Image))
			truncated = true
			break
		}

		paletted := image.NewPaletted(bounds, palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, bounds, img, bounds.Min)
		g.Image = append(g.Image, paletted)
		g.Delay = append(g.Delay, delay)
		palettedBytes += int64(pixels)
	}

	if len(g.Image) == 0 {
		return "", fmt.Errorf("no frames to encode")
	}

	lw := &limitedWriter{w: outFile, max: int64(maxOutputBytes)}
	if err := gif.EncodeAll(lw, g); err != nil {
		return "", fmt.Errorf("gif encode: %w", err)
	}

	var warning string
	if truncated {
		warning = fmt.Sprintf(
			"GIF was truncated to %d frames to stay within the ~%d MB in-memory budget at this "+
				"resolution. Lower --scale or record to .mp4 or .webm for the full-length clip.",
			len(g.Image), maxGIFEncodeBytes>>20)
		slog.Warn("GIF encode: truncated to fit memory budget", "frames", len(g.Image))
	}
	return warning, outFile.Close()
}

// discoverFrames returns the sorted list of captured JPEG frame files in tmpDir,
// capped at limit frames.
func discoverFrames(tmpDir string, limit int) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(tmpDir, "frame_*.jpg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) > limit {
		files = files[:limit]
	}
	return files, nil
}

// loadFrame reads and decodes a single JPEG frame file. ok is false when the
// frame cannot be read or decoded, in which case the caller skips it.
func loadFrame(path string) (img image.Image, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	img, err = jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	return img, true
}

// scaleFrameForGIF applies the caller-requested scale. At scale 1.0 the frame is
// kept at full resolution — no forced downscale. Memory is bounded by the total
// frame budget in encodeGIFToFile, not by shrinking individual frames.
func scaleFrameForGIF(img image.Image, scale float64) image.Image {
	if scale != 1.0 && scale > 0 {
		return scaleImage(img, scale)
	}
	return img
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
	// The output path carries a ".encoding.tmp" suffix (see recorder.stop), so
	// ffmpeg cannot infer the container from the extension. Name the muxer
	// explicitly — "mp4"/"webm" are valid format names — or it aborts with
	// "Unable to choose an output format" (issue #585).
	args = append(args, "-f", format)
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
