package cdptk

import (
	"testing"

	"github.com/chromedp/cdproto/page"
)

// The fromSurface rule is platform-dependent: Windows always composites a fresh
// surface, because the read-the-view fast path returns a blank image there
// (issue #619). The exported CaptureFromSurface reads runtime.GOOS, so a table
// driven through it can only ever assert the rule for the machine running the
// test — and it asserted the non-Windows answers, which is why it failed on
// Windows while leaving the Windows clause untested everywhere else.
//
// captureFromSurface takes goos precisely so the whole rule can be checked from
// any host. This is that table.
func TestCaptureFromSurfaceRulePerGOOS(t *testing.T) {
	clip := &page.Viewport{Width: 120, Height: 60, Scale: 1}

	for _, tc := range []struct {
		name           string
		goos           string
		beyondViewport bool
		clip           *page.Viewport
		want           bool
	}{
		{name: "linux plain capture keeps the fast read", goos: "linux", want: false},
		{name: "darwin plain capture keeps the fast read", goos: "darwin", want: false},
		{name: "windows always composites a surface", goos: "windows", want: true},

		{name: "linux any clip needs the surface", goos: "linux", clip: clip, want: true},
		{name: "darwin any clip needs the surface", goos: "darwin", clip: clip, want: true},
		{name: "windows with a clip still needs it", goos: "windows", clip: clip, want: true},

		{name: "linux beyond viewport needs it with no clip", goos: "linux", beyondViewport: true, want: true},
		{name: "darwin beyond viewport needs it with no clip", goos: "darwin", beyondViewport: true, want: true},
		{name: "windows beyond viewport needs it too", goos: "windows", beyondViewport: true, want: true},

		{name: "linux both", goos: "linux", beyondViewport: true, clip: clip, want: true},
		{name: "windows both", goos: "windows", beyondViewport: true, clip: clip, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := captureFromSurface(tc.goos, tc.beyondViewport, tc.clip); got != tc.want {
				t.Errorf("captureFromSurface(%q, %v, %+v) = %v, want %v",
					tc.goos, tc.beyondViewport, tc.clip, got, tc.want)
			}
		})
	}
}
