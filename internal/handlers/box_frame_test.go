package handlers

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// frameFixtureHTML mirrors the reported fixture: two top-level buttons and an
// iframe holding a third. The defect is invisible on a single-document page —
// only a node inside the frame can be measured in the wrong origin.
const frameFixtureHTML = `<h1>Dialog fixture</h1>
<button id="a">Alert</button>
<button id="c">Confirm</button>
<iframe id="f" srcdoc="<p>inside frame</p><button id='ib'>frame button</button>" width="300" height="100"></iframe>`

type frameFixture struct {
	handlers *Handlers
	ctx      context.Context
	nodes    []bridge.A11yNode
}

func (f frameFixture) nodeIDByName(t *testing.T, role, name string) int64 {
	t.Helper()
	for _, node := range f.nodes {
		if node.Role == role && node.Name == name && node.NodeID != 0 {
			return node.NodeID
		}
	}
	t.Fatalf("no %s node named %q in the snapshot (%d nodes)", role, name, len(f.nodes))
	return 0
}

func (f frameFixture) iframeNodeID(t *testing.T) int64 {
	t.Helper()
	for _, node := range f.nodes {
		if node.Tag == "iframe" && node.NodeID != 0 {
			return node.NodeID
		}
	}
	for _, node := range f.nodes {
		if node.Role == "Iframe" && node.NodeID != 0 {
			return node.NodeID
		}
	}
	t.Fatalf("no iframe node in the snapshot (%d nodes)", len(f.nodes))
	return 0
}

func newFrameFixture(t *testing.T) frameFixture {
	t.Helper()

	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	t.Cleanup(cancelTimeout)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(frameFixtureHTML))
	if err := chromedp.Run(ctx, chromedp.Navigate(dataURL), chromedp.WaitVisible("#f", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{ActionTimeout: 5 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := browserBridge(ctx, cfg)
	b.RegisterTab("tab-frame", ctx)

	rawNodes, err := bridge.FetchAXTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := bridge.BuildSnapshot(rawNodes, "", -1)
	if err := bridge.EnrichA11yNodesWithDOMMetadata(ctx, nodes); err != nil {
		t.Fatal(err)
	}

	return frameFixture{handlers: New(b, cfg, nil, nil, nil), ctx: ctx, nodes: nodes}
}

func boxForNode(t *testing.T, f frameFixture, nodeID int64) boundingBox {
	t.Helper()
	f.handlers.Bridge.SetRefCache("tab-frame", &bridge.RefCache{Targets: map[string]bridge.RefTarget{
		"e0": {BackendNodeID: nodeID},
	}})
	box, err := f.handlers.getElementBox(f.ctx, "tab-frame", "ref:e0")
	if err != nil {
		t.Fatalf("getElementBox: %v", err)
	}
	return *box
}

func boxContains(outer, inner boundingBox) bool {
	return inner.Left >= outer.Left && inner.Top >= outer.Top &&
		inner.Right <= outer.Right && inner.Bottom <= outer.Bottom
}

// The reported defect: an element inside an iframe was measured in the frame's
// own origin, so its rectangle landed entirely outside the iframe that contains
// it — geometrically impossible in page space.
func TestBoxForInFrameRefIsInsideItsIframe(t *testing.T) {
	f := newFrameFixture(t)

	iframeBox := boxForNode(t, f, f.iframeNodeID(t))
	buttonBox := boxForNode(t, f, f.nodeIDByName(t, "button", "frame button"))

	if buttonBox.Width <= 0 || buttonBox.Height <= 0 {
		t.Fatalf("in-frame button box has no area: %+v", buttonBox)
	}
	if !boxContains(iframeBox, buttonBox) {
		t.Fatalf("in-frame button %+v is not inside its iframe %+v — the frame offset was not applied", buttonBox, iframeBox)
	}
}

// /box and the capture path must report the same coordinate space. The capture
// path measures the content box and /box the border box, so allow a small
// padding/border difference but nothing like the frame-sized offset the defect
// produced.
func TestBoxAgreesWithCaptureCoordinateSpaceForInFrameRef(t *testing.T) {
	f := newFrameFixture(t)
	nodeID := f.nodeIDByName(t, "button", "frame button")

	boxed := boxForNode(t, f, nodeID)

	vp, err := bridge.FetchLayout(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	captureNodes := []bridge.A11yNode{{NodeID: nodeID}}
	if err := observe.AnnotateBounds(f.ctx, captureNodes, false, vp); err != nil {
		t.Fatal(err)
	}
	captured := captureNodes[0].BoundingBox
	if captured == nil {
		t.Fatal("capture path produced no bounding box for the in-frame node")
	}

	const boxModelSlack = 12.0
	if diff := boxed.X - captured.X; diff > boxModelSlack || diff < -boxModelSlack {
		t.Errorf("x differs by %.2f (box %.2f vs capture %.2f) — more than a border/padding difference", diff, boxed.X, captured.X)
	}
	if diff := boxed.Y - captured.Y; diff > boxModelSlack || diff < -boxModelSlack {
		t.Errorf("y differs by %.2f (box %.2f vs capture %.2f) — more than a border/padding difference", diff, boxed.Y, captured.Y)
	}
}

// Top-level refs keep the coordinates they already had: same origin, and the
// border box getBoundingClientRect reported.
func TestBoxForTopLevelRefMatchesGetBoundingClientRect(t *testing.T) {
	f := newFrameFixture(t)

	boxed := boxForNode(t, f, f.nodeIDByName(t, "button", "Alert"))

	var rect struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := chromedp.Run(f.ctx, chromedp.Evaluate(
		`(() => { const r = document.getElementById('a').getBoundingClientRect(); return {x: r.x, y: r.y, width: r.width, height: r.height}; })()`,
		&rect)); err != nil {
		t.Fatal(err)
	}

	const tolerance = 0.5
	for _, cmp := range []struct {
		name      string
		got, want float64
	}{
		{"x", boxed.X, rect.X},
		{"y", boxed.Y, rect.Y},
		{"width", boxed.Width, rect.Width},
		{"height", boxed.Height, rect.Height},
	} {
		if diff := cmp.got - cmp.want; diff > tolerance || diff < -tolerance {
			t.Errorf("%s = %.2f, want %.2f (getBoundingClientRect)", cmp.name, cmp.got, cmp.want)
		}
	}
}

// /visible is the sibling that evaluates the same raw rect in the element's own
// context. Frame-local geometry is the right answer for a visibility question —
// an element can be visible within its frame — so this pins the behaviour that
// its description documents rather than changing it.
func TestVisibleIsFrameLocalForInFrameRef(t *testing.T) {
	f := newFrameFixture(t)
	nodeID := f.nodeIDByName(t, "button", "frame button")

	visible, err := f.handlers.elementRendered(f.ctx, nodeID)
	if err != nil {
		t.Fatalf("elementRendered: %v", err)
	}
	if !visible {
		t.Fatal("an in-frame button that is on screen reported not visible")
	}
}

// The reported space is the top-level viewport, so a scrolled page must not
// shift the box by the scroll offset: document-relative measurement has to be
// projected back into viewport space.
func TestBoxForTopLevelRefTracksScroll(t *testing.T) {
	f := newFrameFixture(t)

	if err := chromedp.Run(f.ctx, chromedp.Evaluate(
		`(() => { document.body.style.height = '3000px'; window.scrollTo(0, 400); return window.scrollY; })()`,
		nil)); err != nil {
		t.Fatal(err)
	}

	boxed := boxForNode(t, f, f.nodeIDByName(t, "button", "Alert"))

	var rect struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := chromedp.Run(f.ctx, chromedp.Evaluate(
		`(() => { const r = document.getElementById('a').getBoundingClientRect(); return {x: r.x, y: r.y}; })()`,
		&rect)); err != nil {
		t.Fatal(err)
	}

	const tolerance = 0.5
	if diff := boxed.Y - rect.Y; diff > tolerance || diff < -tolerance {
		t.Fatalf("y = %.2f on a scrolled page, want %.2f (viewport space)", boxed.Y, rect.Y)
	}
}

// scrollintoview shared the same defect: it measured the scrolled-to element
// with the same raw rect, so an in-frame ref reported a frame-relative box.
func TestScrollIntoViewBoxForInFrameRefIsInsideItsIframe(t *testing.T) {
	f := newFrameFixture(t)

	iframeBox := boxForNode(t, f, f.iframeNodeID(t))

	result, err := bridge.ScrollIntoViewAndGetBox(f.ctx, f.nodeIDByName(t, "button", "frame button"))
	if err != nil {
		t.Fatalf("ScrollIntoViewAndGetBox: %v", err)
	}
	raw, ok := result["box"].(map[string]any)
	if !ok {
		t.Fatalf("result has no box: %+v", result)
	}
	scrolled := boundingBox{
		X:      raw["x"].(float64),
		Y:      raw["y"].(float64),
		Width:  raw["width"].(float64),
		Height: raw["height"].(float64),
	}
	scrolled.Left, scrolled.Top = scrolled.X, scrolled.Y
	scrolled.Right, scrolled.Bottom = scrolled.X+scrolled.Width, scrolled.Y+scrolled.Height

	if scrolled.Width <= 0 || scrolled.Height <= 0 {
		t.Fatalf("scrollintoview box has no area: %+v", scrolled)
	}
	// Rounded to whole pixels by the action contract, so allow a pixel of slack
	// on each edge rather than demanding strict containment.
	const roundingSlack = 1.0
	if scrolled.Left < iframeBox.Left-roundingSlack || scrolled.Top < iframeBox.Top-roundingSlack ||
		scrolled.Right > iframeBox.Right+roundingSlack || scrolled.Bottom > iframeBox.Bottom+roundingSlack {
		t.Fatalf("scrollintoview box %+v is not inside its iframe %+v — the frame offset was not applied", scrolled, iframeBox)
	}
}
