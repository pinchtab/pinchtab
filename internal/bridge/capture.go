package bridge

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// CaptureOpts controls a paired screenshot + accessibility capture. The image
// half is delegated to CaptureScreenshot; the snapshot half mirrors the
// /snapshot handler's BuildSnapshot path.
type CaptureOpts struct {
	Image              ScreenshotOpts
	Filter             string // snapshot filter ("" or FilterInteractive)
	MaxDepth           int    // -1 for full tree
	ScopeBackendNodeID int64  // optional snapshot subtree scope
	ScopeFrameID       string // optional frame-scoped capture
	DisableAnimations  bool

	// Wait controls the lifecycle wait before the capture window opens.
	// Empty (or "none") skips the wait. "stable" waits for
	// Page.lifecycleEvent quiescence — 250ms of silence or 750ms ceiling.
	// "load" is currently a no-op alias for "none"; reserved for a future
	// document.readyState gate.
	Wait string

	// WithBounds populates BoundingBox + Visible on every snapshot node that
	// has a non-zero backend node id. Adds one DOM.getBoxModel round trip
	// per node (~5ms each).
	WithBounds bool
}

// Wait values understood by PairedCapture.
const (
	WaitNone   = "none"
	WaitLoad   = "load"
	WaitStable = "stable"
)

// PairedResult is the in-process return shape of PairedCapture. The HTTP
// handler turns this into the over-the-wire JSON; the field set is chosen to
// keep that translation mechanical.
type PairedResult struct {
	URL        string
	Title      string
	CapturedAt time.Time
	DurationMs int64

	FrameID   string
	LoaderID  string
	DomEpoch  string
	Navigated bool

	ImageBytes  []byte
	ImageFormat string // "jpeg" or "png"

	// Viewport metadata captured alongside the image. CoordinateSpace is
	// "viewport" by default and "document" when ImageOpts.BeyondViewport is
	// true — bounding boxes are expressed in the named space.
	Viewport        ViewportInfo
	CoordinateSpace string

	Filter string
	Nodes  []A11yNode
	Refs   map[string]int64
}

// PairedCapture runs a screenshot and an accessibility snapshot under the same
// chromedp context. The atomicity guarantee is "no main-frame navigation
// between the two CDP calls" — checked by comparing the main frame's loaderId
// before and after the capture window. Drift inside the same document
// (re-renders, observer mutations) is out of scope for P1; later phases add
// wait:stable and bounding-box harvesting.
func PairedCapture(ctx context.Context, opts CaptureOpts) (*PairedResult, error) {
	start := time.Now()
	res := &PairedResult{
		CapturedAt:  start,
		ImageFormat: imageFormatString(opts.Image.Format),
		Filter:      opts.Filter,
	}

	if opts.DisableAnimations {
		if err := DisableAnimationsOnce(ctx); err != nil {
			return nil, err
		}
	}

	if opts.Wait == WaitStable {
		// Errors here are non-fatal — a failed wait should still produce a
		// capture, just without the quiet-window guarantee. The duration is
		// captured for diagnostics but not currently surfaced in PairedResult.
		_, _ = WaitForQuietWindow(ctx, 250*time.Millisecond, 750*time.Millisecond)
	}

	// Pre-capture frame info — root frame id + loader id.
	pre, err := FetchFrameTree(ctx)
	if err != nil {
		return nil, err
	}
	res.FrameID = pre.Frame.ID
	res.LoaderID = pre.Frame.LoaderID

	// Image first. Order matters only when BeyondViewport is true (P3 concern);
	// at viewport scale either order is equivalent.
	imgBytes, err := CaptureScreenshot(ctx, opts.Image)
	if err != nil {
		return nil, err
	}
	res.ImageBytes = imgBytes

	// AX tree → flat node list with refs. Mirrors HandleSnapshot's pipeline.
	rawNodes, err := FetchAXTree(ctx)
	if err != nil {
		return nil, err
	}
	if opts.ScopeBackendNodeID != 0 {
		rawNodes = FilterSubtree(rawNodes, opts.ScopeBackendNodeID)
	}
	flat, refs := BuildSnapshot(rawNodes, opts.Filter, opts.MaxDepth)
	_ = EnrichA11yNodesWithDOMMetadata(ctx, flat)
	res.Nodes = flat
	res.Refs = refs

	// URL + title for response metadata.
	_ = chromedp.Run(ctx,
		chromedp.Location(&res.URL),
		chromedp.Title(&res.Title),
	)

	// Layout metrics: needed both for the response (viewport, devicePixelRatio)
	// and for the bounds visibility heuristic. Captured AFTER the screenshot so
	// that beyondViewport's reflow side effects are reflected.
	if vp, err := FetchLayout(ctx); err == nil {
		res.Viewport = vp
	}
	pageCoords := opts.Image.BeyondViewport
	if pageCoords {
		res.CoordinateSpace = "document"
	} else {
		res.CoordinateSpace = "viewport"
	}

	if opts.WithBounds {
		_ = AnnotateBounds(ctx, res.Nodes, pageCoords, res.Viewport)
	}

	// Post-capture frame info. Compare root frame id + loader id to detect
	// navigation that happened during the capture window. We do not assert on
	// in-document churn (React re-renders, observer mutations) — that's the
	// residual risk wait:stable mitigates in P2.
	post, err := FetchFrameTree(ctx)
	if err == nil {
		res.Navigated = pre.Frame.ID != post.Frame.ID || pre.Frame.LoaderID != post.Frame.LoaderID
	}

	res.DomEpoch = mintDomEpoch()
	res.DurationMs = time.Since(start).Milliseconds()
	return res, nil
}

func imageFormatString(f page.CaptureScreenshotFormat) string {
	return string(f)
}

// mintDomEpoch returns an opaque token unique per paired capture. The token
// has no semantic content — consumers should treat it as a black box and use
// it only for handshake comparisons against the cached value on RefCache.
func mintDomEpoch() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "ep_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}
