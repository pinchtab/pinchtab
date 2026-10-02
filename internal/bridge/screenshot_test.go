package bridge

import (
	"context"
	"errors"
	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
	"reflect"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

type screenshotExecutor struct {
	methods    []string
	focusFlags []bool
	captureErr error
}

func (e *screenshotExecutor) Execute(_ context.Context, method string, params, result any) error {
	e.methods = append(e.methods, method)
	if method == emulation.CommandSetFocusEmulationEnabled {
		e.focusFlags = append(e.focusFlags, params.(*emulation.SetFocusEmulationEnabledParams).Enabled)
		return nil
	}
	if method == page.CommandCaptureScreenshot {
		if e.captureErr != nil {
			return e.captureErr
		}
		result.(*page.CaptureScreenshotReturns).Data = "cG5n"
	}
	return nil
}

func TestCaptureScreenshotWithoutActivationRestoresFocusEmulation(t *testing.T) {
	exec := &screenshotExecutor{}
	ctx := cdp.WithExecutor(context.Background(), exec)
	got, err := captureScreenshotWithoutActivation(ctx, page.CaptureScreenshot(), false)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if string(got) != "png" {
		t.Fatalf("capture bytes = %q, want png", got)
	}
	wantMethods := []string{
		emulation.CommandSetFocusEmulationEnabled,
		page.CommandBringToFront,
		page.CommandCaptureScreenshot,
		emulation.CommandSetFocusEmulationEnabled,
	}
	if !reflect.DeepEqual(exec.methods, wantMethods) {
		t.Fatalf("CDP methods = %v, want %v", exec.methods, wantMethods)
	}
	if !reflect.DeepEqual(exec.focusFlags, []bool{true, false}) {
		t.Fatalf("focus flags = %v, want [true false]", exec.focusFlags)
	}
}

func TestCaptureScreenshotWithoutActivationRestoresAfterCaptureError(t *testing.T) {
	exec := &screenshotExecutor{captureErr: errors.New("capture failed")}
	ctx := cdp.WithExecutor(context.Background(), exec)
	if _, err := captureScreenshotWithoutActivation(ctx, page.CaptureScreenshot(), false); err == nil {
		t.Fatal("expected capture error")
	}
	if !reflect.DeepEqual(exec.focusFlags, []bool{true, false}) {
		t.Fatalf("focus flags = %v, want [true false]", exec.focusFlags)
	}
}

func TestCaptureScreenshotWithoutActivationSkipsBringToFrontWhenDisallowed(t *testing.T) {
	exec := &screenshotExecutor{}
	ctx := cdp.WithExecutor(context.Background(), exec)
	if _, err := captureScreenshotWithoutActivation(ctx, page.CaptureScreenshot(), true); err != nil {
		t.Fatalf("capture: %v", err)
	}
	for _, m := range exec.methods {
		if m == page.CommandBringToFront {
			t.Fatalf("CDP methods = %v, want no %s", exec.methods, page.CommandBringToFront)
		}
	}
}

func TestClampScale(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 1},
		{-1, 1},
		{0.01, 0.05},
		{0.05, 0.05},
		{0.5, 0.5},
		{1, 1},
		{2, 2},
		{4, 4},
		{4.5, 4},
		{1000, 4},
	}
	for _, c := range cases {
		got := ClampScale(c.in)
		if got != c.want {
			t.Errorf("ClampScale(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestProjectBoundsToClip(t *testing.T) {
	nodes := []A11yNode{
		{
			Ref:         "e1",
			BoundingBox: &bridgeobserve.BoundingBox{X: 125, Y: 240, W: 30, H: 20},
		},
		{Ref: "e2"},
	}
	projectBoundsToClip(nodes, page.Viewport{X: 100, Y: 200, Width: 300, Height: 200})

	if nodes[0].BoundingBox.X != 25 || nodes[0].BoundingBox.Y != 40 {
		t.Fatalf("projected box = (%v,%v), want (25,40)", nodes[0].BoundingBox.X, nodes[0].BoundingBox.Y)
	}
	if nodes[0].BoundingBox.W != 30 || nodes[0].BoundingBox.H != 20 {
		t.Fatalf("projected size = %vx%v, want 30x20", nodes[0].BoundingBox.W, nodes[0].BoundingBox.H)
	}
	if nodes[1].BoundingBox != nil {
		t.Fatal("node without bounding box should remain unchanged")
	}
}

func TestScreenshotClip(t *testing.T) {
	scrolled := cdptk.LayoutMetrics{Width: 1280, Height: 720, ScrollX: 15, ScrollY: 400, DevicePixelRatio: 2}

	t.Run("scales existing clip", func(t *testing.T) {
		clip := screenshotClip(ScreenshotOpts{
			Clip:  &page.Viewport{X: 10, Y: 20, Width: 30, Height: 40, Scale: 2},
			Scale: 0.5,
		}, scrolled, 0, 0)
		if clip == nil {
			t.Fatal("expected clip")
		}
		if clip.X != 10 || clip.Y != 20 || clip.Width != 30 || clip.Height != 40 {
			t.Fatalf("clip geometry changed: %+v", clip)
		}
		if clip.Scale != 1 {
			t.Fatalf("clip scale = %v, want 1", clip.Scale)
		}
	})

	for _, tc := range []struct {
		name      string
		scale     float64
		wantScale float64
	}{
		{name: "default", scale: 0, wantScale: 1},
		{name: "explicit native", scale: 1, wantScale: 1},
		{name: "rescaled", scale: 0.5, wantScale: 0.5},
	} {
		t.Run(tc.name+" viewport capture clips the scrolled viewport in CSS pixels", func(t *testing.T) {
			clip := screenshotClip(ScreenshotOpts{Scale: tc.scale}, scrolled, 0, 0)
			want := page.Viewport{X: 15, Y: 400, Width: 1280, Height: 720, Scale: tc.wantScale}
			if clip == nil || *clip != want {
				t.Fatalf("clip = %+v, want %+v", clip, want)
			}
		})
	}

	t.Run("unmeasured viewport falls back to an unclipped capture", func(t *testing.T) {
		if clip := screenshotClip(ScreenshotOpts{}, cdptk.LayoutMetrics{}, 0, 0); clip != nil {
			t.Fatalf("expected nil clip, got %+v", clip)
		}
	})

	t.Run("native beyond-viewport capture leaves the full page to CDP", func(t *testing.T) {
		if clip := screenshotClip(ScreenshotOpts{BeyondViewport: true}, scrolled, 4096, 8192); clip != nil {
			t.Fatalf("expected nil clip, got %+v", clip)
		}
	})

	t.Run("uses document size for scaled beyond-viewport capture", func(t *testing.T) {
		clip := screenshotClip(ScreenshotOpts{
			Scale:          0.25,
			BeyondViewport: true,
		}, scrolled, 4096, 8192)
		want := page.Viewport{Width: 4096, Height: 8192, Scale: 0.25}
		if clip == nil || *clip != want {
			t.Fatalf("clip = %+v, want %+v", clip, want)
		}
	})

	t.Run("keeps full-page semantics when document size is unavailable", func(t *testing.T) {
		clip := screenshotClip(ScreenshotOpts{
			Scale:          0.25,
			BeyondViewport: true,
		}, scrolled, 0, 0)
		if clip != nil {
			t.Fatalf("expected nil clip, got %+v", clip)
		}
	})
}

// CDP discards a scale-0 clip and returns the whole viewport, no error. The chrome path
// once shipped exactly that defect, so every path a clip takes to CDP must leave the
// conversion with a non-zero Scale — including the no-rescale early return, which used to
// hand the caller's clip through verbatim.
func TestEveryCapturePathNormalisesAZeroScaleClip(t *testing.T) {
	unset := func() *page.Viewport {
		return &page.Viewport{X: 40, Y: 60, Width: 120, Height: 60}
	}
	for _, tc := range []struct {
		name      string
		clip      *page.Viewport
		optsScale float64
		wantScale float64
	}{
		{name: "no rescale requested", clip: unset(), optsScale: 0, wantScale: 1},
		{name: "explicit native rescale", clip: unset(), optsScale: 1, wantScale: 1},
		{name: "real rescale multiplies the native default", clip: unset(), optsScale: 2, wantScale: 2},
		{name: "real rescale multiplies an explicit scale", clip: &page.Viewport{X: 40, Y: 60, Width: 120, Height: 60, Scale: 1.5}, optsScale: 2, wantScale: 3},
		{name: "Bridge.CaptureScreenshot converts through cdptk first", clip: cdptk.ClipViewport(&cdptk.ScreenshotClip{X: 40, Y: 60, Width: 120, Height: 60}), optsScale: 0, wantScale: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := screenshotClip(ScreenshotOpts{Clip: tc.clip, Scale: tc.optsScale}, cdptk.LayoutMetrics{}, 0, 0)
			if got == nil {
				t.Fatal("expected a clip")
			}
			if got.Scale != tc.wantScale {
				t.Fatalf("scale = %v, want %v — a scale-0 clip reaching CDP is silently discarded", got.Scale, tc.wantScale)
			}
			if got.X != 40 || got.Y != 60 || got.Width != 120 || got.Height != 60 {
				t.Fatalf("geometry changed: %+v", got)
			}
		})
	}
}

// An unclipped viewport capture returns the window surface, sized by the host screen
// and untouched by device-metrics emulation, so on a HiDPI host the default shot came
// back at twice the page size while every other mode answered in CSS pixels.
func TestDefaultScreenshotMatchesTheEmulatedViewport(t *testing.T) {
	ctx := newClipFixture(t)
	const width, height = 400, 300
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(width, height)); err != nil {
		t.Fatalf("emulate viewport: %v", err)
	}

	for _, tc := range []struct {
		scale        float64
		wantW, wantH int
	}{
		{scale: 0, wantW: width, wantH: height},
		{scale: 1, wantW: width, wantH: height},
		{scale: 0.5, wantW: width / 2, wantH: height / 2},
	} {
		buf, err := CaptureScreenshot(ctx, ScreenshotOpts{Format: ScreenshotFormatPng, Scale: tc.scale})
		if err != nil {
			t.Fatalf("capture at scale %v: %v", tc.scale, err)
		}
		if w, h := imageSize(t, buf); w != tc.wantW || h != tc.wantH {
			t.Errorf("scale %v capture is %dx%d, want %dx%d", tc.scale, w, h, tc.wantW, tc.wantH)
		}
	}
}
