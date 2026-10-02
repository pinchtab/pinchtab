package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	_ "image/png"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
)

// A single element of known size at a known offset, materially smaller than any
// viewport, so the expected clipped dimensions are exact rather than a ratio.
const clipFixtureHTML = `<body style="margin:0;background:#fff">
<div id="small" style="position:absolute;left:40px;top:60px;width:120px;height:60px;background:#c00"></div>
<div id="empty" style="position:absolute;left:0;top:0;width:0;height:0"></div>
<iframe id="f" style="position:absolute;left:400px;top:300px;border:0" width="300" height="200"
	srcdoc="<body style='margin:0'><div id='inner' style='position:absolute;left:10px;top:20px;width:80px;height:40px;background:#00c'></div></body>"></iframe>
</body>`

const clipFixtureSmallWidth, clipFixtureSmallHeight = 120, 60

func newClipFixture(t *testing.T) context.Context {
	t.Helper()
	ctx := newActionTab(t, 60*time.Second)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(clipFixtureHTML))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#small", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func imageSize(t *testing.T, buf []byte) (int, int) {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("decode screenshot: %v", err)
	}
	return cfg.Width, cfg.Height
}

func nodeIDForSelector(t *testing.T, ctx context.Context, css string) int64 {
	t.Helper()
	nodeID, err := ResolveCSSToNodeID(ctx, css)
	if err != nil {
		t.Fatalf("resolve %q: %v", css, err)
	}
	return nodeID
}

// A CSS selector does not cross an iframe boundary, so the in-frame element has
// to be resolved in the child frame's own execution context.
func inFrameNodeID(t *testing.T, ctx context.Context, css string) int64 {
	t.Helper()
	tree, err := bridgeobserve.FetchFrameTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range tree.ChildFrames {
		if nodeID, err := ResolveCSSToNodeIDInFrame(ctx, child.Frame.ID, css); err == nil {
			return nodeID
		}
	}
	t.Fatalf("no child frame resolves %q (%d children)", css, len(tree.ChildFrames))
	return 0
}

func capturePNG(t *testing.T, ctx context.Context, clip *page.Viewport) []byte {
	t.Helper()
	buf, err := CaptureScreenshot(ctx, ScreenshotOpts{Format: ScreenshotFormatPng, Clip: clip})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	return buf
}

// The reported defect: a clip was computed correctly and handed to CDP, and the
// image came back the full viewport anyway — byte-identical to a capture that
// asked for no clip at all.
func TestCaptureScreenshotHonoursClipDimensions(t *testing.T) {
	ctx := newClipFixture(t)

	clip, err := ScreenshotClipForNode(ctx, nodeIDForSelector(t, ctx, "#small"))
	if err != nil {
		t.Fatalf("clip for #small: %v", err)
	}
	if clip.Width != clipFixtureSmallWidth || clip.Height != clipFixtureSmallHeight {
		t.Fatalf("clip = %+v, want %dx%d — the clip itself is wrong, before any capture",
			clip, clipFixtureSmallWidth, clipFixtureSmallHeight)
	}

	clipped := capturePNG(t, ctx, clip)
	w, h := imageSize(t, clipped)
	if w != clipFixtureSmallWidth || h != clipFixtureSmallHeight {
		t.Errorf("clipped capture is %dx%d, want %dx%d", w, h, clipFixtureSmallWidth, clipFixtureSmallHeight)
	}

	full := capturePNG(t, ctx, nil)
	fw, fh := imageSize(t, full)
	if fw == w && fh == h {
		t.Fatalf("clipped and unclipped captures are both %dx%d — the fixture cannot tell them apart", w, h)
	}
	if bytes.Equal(clipped, full) {
		t.Error("clipped and unclipped captures are byte-identical: the clip was accepted and ignored")
	}
}

// ScreenshotClipForNode walks frameElement ancestors adding each frame's offset,
// so an in-frame element must clip the region it occupies in the PAGE. Measured
// inside its own frame the element is at (10,20); in the page it is at (410,320).
func TestScreenshotClipForNodeAppliesFrameOffset(t *testing.T) {
	ctx := newClipFixture(t)

	clip, err := ScreenshotClipForNode(ctx, inFrameNodeID(t, ctx, "#inner"))
	if err != nil {
		t.Fatalf("clip for in-frame element: %v", err)
	}

	const wantX, wantY, wantW, wantH = 410, 320, 80, 40
	if clip.X != wantX || clip.Y != wantY {
		t.Errorf("in-frame clip origin = (%.0f,%.0f), want (%d,%d) — frame offset not applied (frame-local origin is (10,20))",
			clip.X, clip.Y, wantX, wantY)
	}
	if clip.Width != wantW || clip.Height != wantH {
		t.Errorf("in-frame clip size = %.0fx%.0f, want %dx%d", clip.Width, clip.Height, wantW, wantH)
	}

	buf := capturePNG(t, ctx, clip)
	if w, h := imageSize(t, buf); w != wantW || h != wantH {
		t.Errorf("in-frame clipped capture is %dx%d, want %dx%d", w, h, wantW, wantH)
	}
}

// An element with no box cannot be clipped; that must be an error rather than a
// silent full-viewport capture.
func TestScreenshotClipForNodeRejectsEmptyElement(t *testing.T) {
	ctx := newClipFixture(t)

	if clip, err := ScreenshotClipForNode(ctx, nodeIDForSelector(t, ctx, "#empty")); err == nil {
		t.Fatalf("zero-size element produced clip %+v, want an error", clip)
	}
}

// /capture shares this engine and was strictly worse than /screenshot: it
// reported coordinateSpace "clip" and a clip rect while returning the full
// viewport, so a vision model was told to map refs into a 120x60 space while
// looking at a whole page. The image must match the clip the response advertises.
func TestPairedCaptureImageMatchesReportedClip(t *testing.T) {
	ctx := newClipFixture(t)

	clip, err := ScreenshotClipForNode(ctx, nodeIDForSelector(t, ctx, "#small"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := PairedCapture(ctx, CaptureOpts{
		MaxDepth: -1,
		Image:    ScreenshotOpts{Format: ScreenshotFormatPng, Clip: clip},
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.CoordinateSpace != "clip" || res.Clip == nil {
		t.Fatalf("CoordinateSpace = %q, Clip = %+v; want a clip capture", res.CoordinateSpace, res.Clip)
	}
	w, h := imageSize(t, res.ImageBytes)
	if float64(w) != res.Clip.Width || float64(h) != res.Clip.Height {
		t.Errorf("image is %dx%d but the response advertises a %.0fx%.0f clip", w, h, res.Clip.Width, res.Clip.Height)
	}
}
