package handlers

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestFrames generates n solid-colour JPEG frames named frame_%06d.jpg in
// dir, matching the names ffmpeg expects (see encodeFFmpegToFile).
func writeTestFrames(t *testing.T, dir string, n, w, h int) {
	t.Helper()
	for i := 0; i < n; i++ {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		shade := uint8((i * 40) % 256)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{R: shade, G: uint8(x % 256), B: uint8(y % 256), A: 255})
			}
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			t.Fatalf("encode frame %d: %v", i, err)
		}
		name := filepath.Join(dir, fmt.Sprintf("frame_%06d.jpg", i))
		if err := os.WriteFile(name, buf.Bytes(), 0600); err != nil {
			t.Fatalf("write frame %d: %v", i, err)
		}
	}
}

// TestEncodeToFile_TempSuffixProducesValidContainer is the regression test for
// issue #585: recordings are encoded to a "<output>.encoding.tmp" path (see
// recorder.stop), so ffmpeg cannot infer the container from the file extension.
// Before the fix the mp4/webm muxer aborts with "Unable to choose an output
// format ... .encoding.tmp"; after the fix the format is passed explicitly and
// a valid container is written. The test drives the exact production call path:
// encodeToFile(tmpDir, output+".encoding.tmp", format, ...).
func TestEncodeToFile_TempSuffixProducesValidContainer(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not available")
	}

	cases := []struct {
		format    string
		muxerName string // container ffprobe should report
	}{
		{"mp4", "mp4"},
		{"webm", "webm"},
	}

	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			frameDir := t.TempDir()
			writeTestFrames(t, frameDir, 4, 320, 240)

			outDir := t.TempDir()
			finalPath := filepath.Join(outDir, "capture."+tc.format)
			// Replicate recorder.stop(): encode into the .encoding.tmp sibling.
			tmpOut := finalPath + ".encoding.tmp"

			if _, err := encodeToFile(frameDir, tmpOut, tc.format, 30, 1.0); err != nil {
				t.Fatalf("encodeToFile(%s) failed (issue #585): %v", tc.format, err)
			}

			info, err := os.Stat(tmpOut)
			if err != nil {
				t.Fatalf("expected encoded file at %s: %v", tmpOut, err)
			}
			if info.Size() == 0 {
				t.Fatalf("encoded %s file is empty", tc.format)
			}

			// Prove the bytes are a real container of the requested format, not
			// just a non-empty file, by asking ffprobe to name the format.
			assertContainerFormat(t, tmpOut, tc.muxerName)
		})
	}
}

// TestEncodeGIF_HonorsScaleAndWarnsOnTruncation covers the GIF half of issue #585.
// After the fix the requested --scale is honored at full resolution (no forced
// per-frame downscale); memory is bounded by a total-frame budget, and when that
// budget truncates the frame count a warning is surfaced so it is not silent.
func TestEncodeGIF_HonorsScaleAndWarnsOnTruncation(t *testing.T) {
	// A large HiDPI-style frame that the old code would have force-downscaled.
	const bigW, bigH = 1600, 1000

	t.Run("scale_1_keeps_full_resolution", func(t *testing.T) {
		frameDir := t.TempDir()
		writeTestFrames(t, frameDir, 3, bigW, bigH)
		out := filepath.Join(t.TempDir(), "capture.gif.encoding.tmp")

		warning, err := encodeToFile(frameDir, out, "gif", 5, 1.0)
		if err != nil {
			t.Fatalf("encodeToFile(gif) failed: %v", err)
		}
		if warning != "" {
			t.Fatalf("expected no warning at scale 1 within budget, got %q", warning)
		}
		w, h := gifDimensions(t, out)
		if w != bigW || h != bigH {
			t.Fatalf("scale 1 must keep full resolution %dx%d, got %dx%d", bigW, bigH, w, h)
		}
	})

	t.Run("scale_downscale_honored", func(t *testing.T) {
		frameDir := t.TempDir()
		writeTestFrames(t, frameDir, 3, bigW, bigH)
		out := filepath.Join(t.TempDir(), "capture.gif.encoding.tmp")

		warning, err := encodeToFile(frameDir, out, "gif", 5, 0.5)
		if err != nil {
			t.Fatalf("encodeToFile(gif) failed: %v", err)
		}
		if warning != "" {
			t.Fatalf("expected no warning at scale 0.5, got %q", warning)
		}
		w, h := gifDimensions(t, out)
		if w != bigW/2 || h != bigH/2 {
			t.Fatalf("expected --scale honored to %dx%d, got %dx%d", bigW/2, bigH/2, w, h)
		}
	})

	t.Run("budget_truncation_warns", func(t *testing.T) {
		// Shrink the budget so a handful of frames overflows it, exercising the
		// truncation path cheaply. Restore afterwards.
		orig := maxGIFEncodeBytes
		maxGIFEncodeBytes = int64(bigW*bigH) * 2 // room for ~2 frames
		defer func() { maxGIFEncodeBytes = orig }()

		frameDir := t.TempDir()
		writeTestFrames(t, frameDir, 5, bigW, bigH) // 5 recorded, only ~2 fit
		out := filepath.Join(t.TempDir(), "capture.gif.encoding.tmp")

		warning, err := encodeToFile(frameDir, out, "gif", 5, 1.0)
		if err != nil {
			t.Fatalf("encodeToFile(gif) failed: %v", err)
		}
		if warning == "" {
			t.Fatal("expected a truncation warning when frames overflow the budget, got none")
		}
		// Full resolution is still preserved on the frames that were kept.
		w, h := gifDimensions(t, out)
		if w != bigW || h != bigH {
			t.Fatalf("truncated GIF must keep full resolution %dx%d, got %dx%d", bigW, bigH, w, h)
		}
	})
}

// gifDimensions decodes the GIF at path and returns its frame dimensions.
func gifDimensions(t *testing.T, path string) (w, h int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read gif: %v", err)
	}
	img, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode gif: %v", err)
	}
	if len(img.Image) == 0 {
		t.Fatal("gif has no frames")
	}
	b := img.Image[0].Bounds()
	return b.Dx(), b.Dy()
}

// assertContainerFormat uses ffprobe (shipped with ffmpeg) to confirm the file
// is a valid container whose detected format includes want.
func assertContainerFormat(t *testing.T, path, want string) {
	t.Helper()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Logf("ffprobe not available; skipping container-format assertion")
		return
	}
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=format_name",
		"-of", "default=nokey=1:noprint_wrappers=1", path).Output()
	if err != nil {
		t.Fatalf("ffprobe could not parse %s as a media file: %v", path, err)
	}
	got := strings.TrimSpace(string(out))
	if !strings.Contains(got, want) {
		t.Fatalf("ffprobe reports format %q, want it to contain %q", got, want)
	}
}
