package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	_ "image/png"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const imageSpaceFixtureHTML = `<body style="margin:0;width:1200px;height:2400px">
<button id="target" style="position:absolute;left:40px;top:60px;width:120px;height:40px">target</button>
</body>`

func newImageSpaceFixture(t *testing.T) (context.Context, int64) {
	t.Helper()

	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(testbrowser.Path(t)),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 90*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	})

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(imageSpaceFixtureHTML))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#target", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}

	rawNodes, err := FetchAXTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := BuildSnapshot(rawNodes, "", -1)
	for _, n := range nodes {
		if n.Role == "button" && n.Name == "target" && n.NodeID != 0 {
			return ctx, n.NodeID
		}
	}
	t.Fatalf("no button named \"target\" in the snapshot (%d nodes)", len(nodes))
	return nil, 0
}

func imageDimensions(t *testing.T, buf []byte) (float64, float64) {
	t.Helper()

	cfg, _, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("capture did not return a decodable image: %v", err)
	}
	return float64(cfg.Width), float64(cfg.Height)
}

func documentSize(t *testing.T, ctx context.Context) (float64, float64) {
	t.Helper()

	var size struct {
		W float64 `json:"w"`
		H float64 `json:"h"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`(() => ({w: document.documentElement.scrollWidth, h: document.documentElement.scrollHeight}))()`,
		&size)); err != nil {
		t.Fatal(err)
	}
	return size.W, size.H
}

// The contract /capture publishes: scaling a node's boundingBox by the reported
// devicePixelRatio, from the origin the reported coordinateSpace names, lands on the
// image's own pixels. That is checkable as a dimension identity — the image measures
// exactly the reported space times the reported ratio — and it has to hold in every
// mode, because a client is promised it never has to branch.
//
// Three modes and two emulation states, because the defect was invisible in a
// plain-only test: the two clipped modes were already correct, and the default one
// happened to match whenever the page's device pixel ratio equalled the screen's.
func TestCaptureImageMatchesTheSpaceItReports(t *testing.T) {
	for _, emulated := range []struct {
		name              string
		width, height     int64
		deviceScaleFactor float64
	}{
		{name: "no emulation"},
		{name: "emulated 400x300 at dpr 1", width: 400, height: 300, deviceScaleFactor: 1},
		{name: "emulated 400x300 at dpr 2", width: 400, height: 300, deviceScaleFactor: 2},
	} {
		t.Run(emulated.name, func(t *testing.T) {
			ctx, nodeID := newImageSpaceFixture(t)
			if emulated.width > 0 {
				if err := chromedp.Run(ctx, emulation.SetDeviceMetricsOverride(
					emulated.width, emulated.height, emulated.deviceScaleFactor, false)); err != nil {
					t.Fatal(err)
				}
			}

			t.Run("viewport", func(t *testing.T) {
				res, err := PairedCapture(ctx, CaptureOpts{MaxDepth: -1, Image: ScreenshotOpts{Format: ScreenshotFormatPng}})
				if err != nil {
					t.Fatal(err)
				}
				if res.CoordinateSpace != "viewport" {
					t.Fatalf("CoordinateSpace = %q, want viewport", res.CoordinateSpace)
				}
				assertImageIs(t, res, res.Viewport.Width, res.Viewport.Height)
			})

			t.Run("beyondViewport", func(t *testing.T) {
				res, err := PairedCapture(ctx, CaptureOpts{MaxDepth: -1, Image: ScreenshotOpts{
					Format: ScreenshotFormatPng, BeyondViewport: true,
				}})
				if err != nil {
					t.Fatal(err)
				}
				if res.CoordinateSpace != "document" {
					t.Fatalf("CoordinateSpace = %q, want document", res.CoordinateSpace)
				}
				docWidth, docHeight := documentSize(t, ctx)
				assertImageIs(t, res, docWidth, docHeight)
			})

			t.Run("selector", func(t *testing.T) {
				clip, err := ScreenshotClipForNode(ctx, nodeID)
				if err != nil {
					t.Fatal(err)
				}
				res, err := PairedCapture(ctx, CaptureOpts{MaxDepth: -1, Image: ScreenshotOpts{
					Format: ScreenshotFormatPng,
					Clip:   clip,
				}})
				if err != nil {
					t.Fatal(err)
				}
				if res.CoordinateSpace != "clip" {
					t.Fatalf("CoordinateSpace = %q, want clip", res.CoordinateSpace)
				}
				assertImageIs(t, res, clip.Width, clip.Height)
			})
		})
	}
}

// assertImageIs is the contract as one identity: the image is the reported space at the
// reported ratio. Rounding is a single pixel per axis — CDP rounds the composited size —
// and anything larger is a client's overlay landing somewhere else.
func assertImageIs(t *testing.T, res *PairedResult, spaceWidth, spaceHeight float64) {
	t.Helper()

	dpr := res.Viewport.DevicePixelRatio
	if dpr <= 0 {
		t.Fatalf("capture reported devicePixelRatio %v; a client scaling by it would map every box onto nothing", dpr)
	}
	gotWidth, gotHeight := imageDimensions(t, res.ImageBytes)
	wantWidth, wantHeight := spaceWidth*dpr, spaceHeight*dpr

	if diff := gotWidth - wantWidth; diff > 1 || diff < -1 {
		t.Errorf("image is %.0f px wide; the response reports %s %.0f at devicePixelRatio %v, so a boundingBox scaled by it lands %.2fx off",
			gotWidth, res.CoordinateSpace, spaceWidth, dpr, gotWidth/wantWidth)
	}
	if diff := gotHeight - wantHeight; diff > 1 || diff < -1 {
		t.Errorf("image is %.0f px tall; the response reports %s %.0f at devicePixelRatio %v, so a boundingBox scaled by it lands %.2fx off",
			gotHeight, res.CoordinateSpace, spaceHeight, dpr, gotHeight/wantHeight)
	}
}
