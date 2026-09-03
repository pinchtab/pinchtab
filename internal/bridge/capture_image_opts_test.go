package bridge

import (
	"testing"

	"github.com/chromedp/cdproto/page"
)

// The browserless twin of TestCaptureImageMatchesTheSpaceItReports, which needs a
// browser CI does not have. It cannot see pixels, so it pins the decision that
// produces them: which region each mode asks CDP to render.
func TestCaptureImageOptsRedirectsOnlyTheDefaultMode(t *testing.T) {
	vp := ViewportInfo{Width: 800, Height: 600, ScrollX: 30, ScrollY: 40, DevicePixelRatio: 2}
	callerClip := &page.Viewport{X: 1, Y: 2, Width: 3, Height: 4, Scale: 1}

	t.Run("default capture renders the viewport it reports", func(t *testing.T) {
		got := captureImageOpts(ScreenshotOpts{Format: ScreenshotFormatPng}, vp)
		if got.Clip == nil {
			t.Fatal("the default capture still reads the window surface, which no reported devicePixelRatio describes")
		}
		want := page.Viewport{X: 30, Y: 40, Width: 800, Height: 600, Scale: 1}
		if *got.Clip != want {
			t.Errorf("clip = %+v, want %+v — page coordinates, and scale 1 because CDP applies the page ratio itself", *got.Clip, want)
		}
	})

	t.Run("a caller's own clip is untouched", func(t *testing.T) {
		got := captureImageOpts(ScreenshotOpts{Format: ScreenshotFormatPng, Clip: callerClip}, vp)
		if got.Clip != callerClip {
			t.Errorf("clip = %+v, want the caller's %+v", got.Clip, callerClip)
		}
	})

	t.Run("beyondViewport keeps its nil clip", func(t *testing.T) {
		got := captureImageOpts(ScreenshotOpts{Format: ScreenshotFormatPng, BeyondViewport: true}, vp)
		if got.Clip != nil {
			t.Errorf("clip = %+v, want nil; a clip and captureBeyondViewport are two different regions and CDP honours only one", got.Clip)
		}
	})

	// A layout read can fail, and a zero viewport would ask CDP for an empty region.
	// Falling back to the old behaviour returns a picture that does not match the
	// metadata; returning nothing returns no picture at all.
	t.Run("an unreadable viewport falls back rather than clipping to nothing", func(t *testing.T) {
		got := captureImageOpts(ScreenshotOpts{Format: ScreenshotFormatPng}, ViewportInfo{})
		if got.Clip != nil {
			t.Errorf("clip = %+v, want nil when the layout read gave no viewport", got.Clip)
		}
	})
}
