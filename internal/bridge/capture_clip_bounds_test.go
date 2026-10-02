package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"

	"github.com/chromedp/chromedp"
)

// The target sits far enough down and across that a scroll is required to reach
// it, so the clip origin, the scroll offset and the viewport rect are three
// different numbers. At scroll 0 they collapse into one and the clip path's
// mixed-space subtraction is invisible.
const clipScrollFixtureHTML = `<body style="margin:0;width:2400px;height:2400px">
<div style="position:absolute;left:900px;top:1100px;width:120px;height:40px">
<button id="target">target</button>
</div>
</body>`

func newClipScrollFixture(t *testing.T) (context.Context, int64) {
	t.Helper()
	ctx := newActionTab(t, 60*time.Second)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(clipScrollFixtureHTML))
	var scrolled bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#target", chromedp.ByID),
		chromedp.Evaluate(`(() => { window.scrollTo(300, 400); return window.scrollY === 400; })()`, &scrolled),
	); err != nil {
		t.Fatal(err)
	}
	if !scrolled {
		t.Fatal("fixture did not scroll")
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

func boundsForNode(t *testing.T, res *PairedResult, nodeID int64) bridgeobserve.BoundingBox {
	t.Helper()
	for _, n := range res.Nodes {
		if n.NodeID == nodeID && n.BoundingBox != nil {
			return *n.BoundingBox
		}
	}
	t.Fatalf("capture produced no bounding box for node %d (%d nodes)", nodeID, len(res.Nodes))
	return bridgeobserve.BoundingBox{}
}

func clientRectOfTarget(t *testing.T, ctx context.Context) (x, y float64) {
	t.Helper()
	var rect struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`(() => { const r = document.getElementById('target').getBoundingClientRect(); return {x: r.x, y: r.y}; })()`,
		&rect)); err != nil {
		t.Fatal(err)
	}
	return rect.X, rect.Y
}

// The capture path measures the content box and getBoundingClientRect the border
// box, so they differ by the button's border and padding — single digits, not
// the hundreds of pixels a mixed-up coordinate space introduces.
const captureBoxSlack = 12.0

func assertNearRect(t *testing.T, label string, got, want float64) {
	t.Helper()
	if diff := got - want; diff > captureBoxSlack || diff < -captureBoxSlack {
		t.Errorf("%s = %.2f, want %.2f (off by %.2f)", label, got, want, diff)
	}
}

// All three spaces PairedCapture can report, on one scrolled page, each checked
// against the browser's own rect. Each space must mean what CoordinateSpace
// calls it: viewport is the raw rect, document is the rect plus the scroll, and
// clip is measured from the clip origin.
func TestPairedCaptureBoundsInEveryCoordinateSpaceWhenScrolled(t *testing.T) {
	t.Run("viewport", func(t *testing.T) {
		ctx, nodeID := newClipScrollFixture(t)

		res, err := PairedCapture(ctx, CaptureOpts{WithBounds: true, MaxDepth: -1, Image: ScreenshotOpts{Format: ScreenshotFormatPng}})
		if err != nil {
			t.Fatal(err)
		}
		if res.CoordinateSpace != "viewport" {
			t.Fatalf("CoordinateSpace = %q, want %q", res.CoordinateSpace, "viewport")
		}
		if res.Viewport.ScrollY == 0 || res.Viewport.ScrollX == 0 {
			t.Fatalf("capture did not see a scrolled page: %+v", res.Viewport)
		}

		box := boundsForNode(t, res, nodeID)
		rectX, rectY := clientRectOfTarget(t, ctx)
		assertNearRect(t, "viewport x", box.X, rectX)
		assertNearRect(t, "viewport y", box.Y, rectY)
	})

	t.Run("document", func(t *testing.T) {
		ctx, nodeID := newClipScrollFixture(t)

		res, err := PairedCapture(ctx, CaptureOpts{
			WithBounds: true,
			MaxDepth:   -1,
			Image:      ScreenshotOpts{Format: ScreenshotFormatPng, BeyondViewport: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.CoordinateSpace != "document" {
			t.Fatalf("CoordinateSpace = %q, want %q", res.CoordinateSpace, "document")
		}

		box := boundsForNode(t, res, nodeID)
		rectX, rectY := clientRectOfTarget(t, ctx)
		assertNearRect(t, "document x", box.X, rectX+res.Viewport.ScrollX)
		assertNearRect(t, "document y", box.Y, rectY+res.Viewport.ScrollY)
	})

	// The one place two independently-derived spaces meet: the clip origin comes
	// from ScreenshotClipForNode's JS, which adds window.scrollX/scrollY, so it is
	// document-relative and the bounds it is subtracted from must be too. The
	// clip is the target's own border box, so the target must land at the clip's
	// origin — anything else is the scroll leaking in.
	t.Run("clip", func(t *testing.T) {
		ctx, nodeID := newClipScrollFixture(t)

		clip, err := ScreenshotClipForNode(ctx, nodeID)
		if err != nil {
			t.Fatal(err)
		}
		if clip.X < 100 || clip.Y < 100 {
			t.Fatalf("clip origin %+v is too close to the document origin for this test to distinguish spaces", clip)
		}

		res, err := PairedCapture(ctx, CaptureOpts{
			WithBounds: true,
			MaxDepth:   -1,
			Image:      ScreenshotOpts{Format: ScreenshotFormatPng, Clip: clip},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.CoordinateSpace != "clip" {
			t.Fatalf("CoordinateSpace = %q, want %q", res.CoordinateSpace, "clip")
		}

		box := boundsForNode(t, res, nodeID)
		assertNearRect(t, "clip x", box.X, 0)
		assertNearRect(t, "clip y", box.Y, 0)
	})
}
