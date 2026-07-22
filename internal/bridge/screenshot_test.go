package bridge

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
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
	got, err := captureScreenshotWithoutActivation(ctx, page.CaptureScreenshot())
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if string(got) != "png" {
		t.Fatalf("capture bytes = %q, want png", got)
	}
	wantMethods := []string{
		emulation.CommandSetFocusEmulationEnabled,
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
	if _, err := captureScreenshotWithoutActivation(ctx, page.CaptureScreenshot()); err == nil {
		t.Fatal("expected capture error")
	}
	if !reflect.DeepEqual(exec.focusFlags, []bool{true, false}) {
		t.Fatalf("focus flags = %v, want [true false]", exec.focusFlags)
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

func TestCaptureFromSurface(t *testing.T) {
	cases := []struct {
		name           string
		beyondViewport bool
		clip           *page.Viewport
		want           bool
	}{
		{name: "plain viewport capture", want: false},
		{name: "beyond viewport forces surface", beyondViewport: true, want: true},
		{name: "native-scale clip stays off", clip: &page.Viewport{Scale: 1}, want: false},
		{name: "zero-scale clip treated as native", clip: &page.Viewport{Scale: 0}, want: false},
		{name: "downscaled clip forces surface", clip: &page.Viewport{Scale: 0.25}, want: true},
		{name: "upscaled clip forces surface", clip: &page.Viewport{Scale: 2}, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := captureFromSurface(c.beyondViewport, c.clip); got != c.want {
				t.Fatalf("captureFromSurface(%v, %+v) = %v, want %v", c.beyondViewport, c.clip, got, c.want)
			}
		})
	}
}

func TestProjectBoundsToClip(t *testing.T) {
	nodes := []A11yNode{
		{
			Ref:         "e1",
			BoundingBox: &BoundingBox{X: 125, Y: 240, W: 30, H: 20},
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

func TestScaledScreenshotClip(t *testing.T) {
	t.Run("scales existing clip", func(t *testing.T) {
		clip := scaledScreenshotClip(ScreenshotOpts{
			Clip:  &page.Viewport{X: 10, Y: 20, Width: 30, Height: 40, Scale: 2},
			Scale: 0.5,
		}, 0, 0, 0, 0)
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

	t.Run("uses viewport size for scaled viewport capture", func(t *testing.T) {
		clip := scaledScreenshotClip(ScreenshotOpts{Scale: 0.25}, 1280, 720, 0, 0)
		if clip == nil {
			t.Fatal("expected clip")
		}
		if clip.Width != 1280 || clip.Height != 720 || clip.Scale != 0.25 {
			t.Fatalf("clip = %+v", clip)
		}
	})

	t.Run("uses document size for scaled beyond-viewport capture", func(t *testing.T) {
		clip := scaledScreenshotClip(ScreenshotOpts{
			Scale:          0.25,
			BeyondViewport: true,
		}, 1280, 720, 4096, 8192)
		if clip == nil {
			t.Fatal("expected clip")
		}
		if clip.Width != 4096 || clip.Height != 8192 || clip.Scale != 0.25 {
			t.Fatalf("clip = %+v", clip)
		}
	})

	t.Run("keeps full-page semantics when document size is unavailable", func(t *testing.T) {
		clip := scaledScreenshotClip(ScreenshotOpts{
			Scale:          0.25,
			BeyondViewport: true,
		}, 1280, 720, 0, 0)
		if clip != nil {
			t.Fatalf("expected nil clip, got %+v", clip)
		}
	})
}
