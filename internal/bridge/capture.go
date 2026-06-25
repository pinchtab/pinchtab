package bridge

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type CaptureOpts struct {
	Image              ScreenshotOpts
	Filter             string // snapshot filter ("" or FilterInteractive)
	MaxDepth           int    // -1 for full tree
	ScopeBackendNodeID int64  // optional snapshot subtree scope
	ScopeFrameID       string // optional frame-scoped capture
	DisableAnimations  bool

	// Wait controls the lifecycle wait before the capture window opens.
	// Empty (or "none") skips the wait. "load" polls document.readyState
	// until it reaches "complete" (2s ceiling). "stable" waits for
	// Page.lifecycleEvent quiescence — 250ms of silence, 750ms ceiling.
	Wait string

	// WithBounds populates BoundingBox + Visible on every snapshot node that
	// has a non-zero backend node id. Adds one DOM.getBoxModel round trip
	// per node (~5ms each).
	WithBounds bool
}

const (
	WaitNone   = "none"
	WaitLoad   = "load"
	WaitStable = "stable"
)

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
	// "viewport" by default, "document" when ImageOpts.BeyondViewport is true,
	// and "clip" for selector-clipped captures. Bounding boxes are expressed in
	// the named space.
	Viewport        ViewportInfo
	CoordinateSpace string
	Clip            *page.Viewport

	Filter string
	Nodes  []A11yNode
	Refs   map[string]int64
}

// PairedCapture runs a screenshot and an accessibility snapshot under the
// same chromedp context. The atomicity guarantee is "no main-frame
// navigation between the two CDP calls" — checked by comparing the main
// frame's loaderId before and after the capture window. opts.Wait == "stable"
// adds a Page.lifecycleEvent quiet-window wait before the window opens.
// opts.WithBounds populates a viewport-, document-, or clip-relative
// BoundingBox per snapshot node via DOM.getBoxModel. Residual risk:
// in-document churn (React re-renders, IntersectionObserver mutations) is not
// detected — wait:stable reduces but does not eliminate it.
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

	// Wait errors are non-fatal — degrade rather than fail the capture.
	switch opts.Wait {
	case WaitStable:
		_, _ = WaitForQuietWindow(ctx, 250*time.Millisecond, 750*time.Millisecond)
	case WaitLoad:
		_, _ = WaitForReadyState(ctx, 2*time.Second)
	}

	pre, err := FetchFrameTree(ctx)
	if err != nil {
		return nil, err
	}
	res.FrameID = pre.Frame.ID
	res.LoaderID = pre.Frame.LoaderID

	// Layout metrics: captured BEFORE the screenshot so opts.Image.Scale can
	// synthesize a viewport-covering clip when no other clip is set. Also
	// populates the response viewport / devicePixelRatio for clients.
	if vp, err := FetchLayout(ctx); err == nil {
		res.Viewport = vp
		opts.Image.ViewportWidth = vp.Width
		opts.Image.ViewportHeight = vp.Height
	}

	// Image first. Order matters only when BeyondViewport is true (P3 concern);
	// at viewport scale either order is equivalent.
	imgBytes, err := CaptureScreenshot(ctx, opts.Image)
	if err != nil {
		return nil, err
	}
	res.ImageBytes = imgBytes

	rawNodes, err := FetchAXTree(ctx)
	if err != nil {
		return nil, err
	}
	if opts.ScopeFrameID != "" {
		rawNodes = FilterAXNodesByFrame(rawNodes, opts.ScopeFrameID)
	}
	if opts.ScopeBackendNodeID != 0 {
		rawNodes = FilterSubtree(rawNodes, opts.ScopeBackendNodeID)
	}
	flat, refs := BuildSnapshot(rawNodes, opts.Filter, opts.MaxDepth)
	_ = EnrichA11yNodesWithDOMMetadata(ctx, flat)
	res.Nodes = flat
	res.Refs = refs

	_ = chromedp.Run(ctx,
		chromedp.Location(&res.URL),
		chromedp.Title(&res.Title),
	)

	pageCoords := opts.Image.BeyondViewport
	if opts.Image.Clip != nil {
		res.CoordinateSpace = "clip"
		clip := *opts.Image.Clip
		res.Clip = &clip
		pageCoords = true
	} else if pageCoords {
		res.CoordinateSpace = "document"
	} else {
		res.CoordinateSpace = "viewport"
	}

	if opts.WithBounds {
		_ = AnnotateBounds(ctx, res.Nodes, pageCoords, res.Viewport)
		if opts.Image.Clip != nil {
			projectBoundsToClip(res.Nodes, *opts.Image.Clip)
		}
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

func projectBoundsToClip(nodes []A11yNode, clip page.Viewport) {
	for i := range nodes {
		if nodes[i].BoundingBox == nil {
			continue
		}
		nodes[i].BoundingBox.X -= clip.X
		nodes[i].BoundingBox.Y -= clip.Y
	}
}

// FilterAXNodesByFrame keeps only nodes whose FrameID matches frameID; an empty
// frameID returns nodes unchanged. Shared by paired capture and the handler
// snapshot/annotated-screenshot flows so frame-scoped filtering cannot drift.
func FilterAXNodesByFrame(nodes []RawAXNode, frameID string) []RawAXNode {
	if frameID == "" {
		return nodes
	}
	filtered := make([]RawAXNode, 0, len(nodes))
	for _, n := range nodes {
		if n.FrameID == frameID {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// mintDomEpoch returns an opaque token unique per paired capture. The token
// has no semantic content — consumers should treat it as a black box and use
// it only for handshake comparisons against the cached value on RefCache.
func mintDomEpoch() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "ep_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}
