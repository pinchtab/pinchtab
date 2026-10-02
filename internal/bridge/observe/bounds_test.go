package observe

import (
	"context"
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
)

// The scroll offset must not enter the test: boxes reach IsOnScreen in viewport
// coordinates, so a page scrolled to y=1000 sees the same numbers as an
// unscrolled one.
func TestIsVisibleUsesViewportCoordinates(t *testing.T) {
	vp := ViewportInfo{Width: 800, Height: 600, ScrollX: 0, ScrollY: 1000}

	if !IsOnScreen(BoundingBox{X: 20, Y: 50, W: 100, H: 40}, vp) {
		t.Fatal("box inside the viewport should be visible")
	}
	if IsOnScreen(BoundingBox{X: 20, Y: -200, W: 100, H: 40}, vp) {
		t.Fatal("box scrolled above the viewport should not be visible")
	}
	if IsOnScreen(BoundingBox{X: 20, Y: 700, W: 100, H: 40}, vp) {
		t.Fatal("box below the viewport should not be visible")
	}
}

// scrollFixtureHTML is tall enough to scroll in both axes, with a target that
// sits below the fold so a scroll is required to bring it into view. Only a
// scrolled page can tell the three coordinate spaces apart — at scroll 0 every
// transform is the identity, which is why this defect went unnoticed.
const scrollFixtureHTML = `<body style="margin:0;width:3000px;height:3000px">
<div style="position:absolute;left:900px;top:1200px;width:120px;height:40px">
<button id="target">target</button>
</div>
</body>`

type scrollFixture struct {
	ctx    context.Context
	nodeID int64
}

func newScrollFixture(t *testing.T, scrollX, scrollY int) scrollFixture {
	t.Helper()
	return newScrollFixtureFrom(t, scrollFixtureHTML, scrollX, scrollY)
}

func newScrollFixtureFrom(t *testing.T, html string, scrollX, scrollY int) scrollFixture {
	t.Helper()
	ctx := fixtureTab(t, html, "#target")
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var done bool
		return chromedp.Evaluate(scrollScript(scrollX, scrollY), &done).Do(ctx)
	})); err != nil {
		t.Fatal(err)
	}

	return scrollFixture{ctx: ctx, nodeID: targetNodeID(t, ctx)}
}

func scrollScript(x, y int) string {
	return fmt.Sprintf(`(() => { window.scrollTo(%d, %d); return window.scrollY === %d; })()`, x, y, y)
}

func targetNodeID(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	rawNodes, err := FetchAXTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := BuildSnapshot(rawNodes, "", -1)
	for _, n := range nodes {
		if n.Role == "button" && n.Name == "target" && n.NodeID != 0 {
			return n.NodeID
		}
	}
	t.Fatalf("no button named %q in the snapshot (%d nodes)", "target", len(nodes))
	return 0
}

// clientRect is the browser's own answer for the target, in viewport space.
func (f scrollFixture) clientRect(t *testing.T) BoundingBox {
	t.Helper()
	var rect struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		W float64 `json:"width"`
		H float64 `json:"height"`
	}
	if err := chromedp.Run(f.ctx, chromedp.Evaluate(
		`(() => { const r = document.getElementById('target').getBoundingClientRect(); return {x: r.x, y: r.y, width: r.width, height: r.height}; })()`,
		&rect)); err != nil {
		t.Fatal(err)
	}
	return BoundingBox{X: rect.X, Y: rect.Y, W: rect.W, H: rect.H}
}

func (f scrollFixture) annotate(t *testing.T, pageCoords bool) (BoundingBox, bool, ViewportInfo) {
	t.Helper()
	vp, err := FetchLayout(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	nodes := []A11yNode{{NodeID: f.nodeID}}
	if err := AnnotateBounds(f.ctx, nodes, pageCoords, vp); err != nil {
		t.Fatal(err)
	}
	if nodes[0].BoundingBox == nil {
		t.Fatal("AnnotateBounds produced no bounding box for the target")
	}
	if nodes[0].Visible == nil {
		t.Fatal("AnnotateBounds measured the target but left visible unset")
	}
	return *nodes[0].BoundingBox, *nodes[0].Visible, vp
}

// AnnotateBounds and getBoundingClientRect now report the same box-model edge —
// the border box — so the only slack left is float rounding. The 12px this used
// to allow was absorbing the content-box inset (the button's border plus
// padding), which is the defect these bounds no longer have; anything larger
// than a rounding error is a real disagreement.
const boxModelSlack = 1.0

func assertNear(t *testing.T, label string, got, want float64) {
	t.Helper()
	if diff := got - want; diff > boxModelSlack || diff < -boxModelSlack {
		t.Errorf("%s = %.2f, want %.2f (off by %.2f)", label, got, want, diff)
	}
}

// The default /capture path. The raw quad is already viewport-relative, so
// pageCoords=false must hand it back untouched — subtracting the scroll moved
// every annotated bound on every scrolled page a full scrollY away.
func TestAnnotateBoundsViewportSpaceMatchesClientRectWhenScrolled(t *testing.T) {
	f := newScrollFixture(t, 300, 400)

	box, _, vp := f.annotate(t, false)
	if vp.ScrollY == 0 || vp.ScrollX == 0 {
		t.Fatalf("fixture did not scroll: %+v", vp)
	}
	rect := f.clientRect(t)

	assertNear(t, "viewport x", box.X, rect.X)
	assertNear(t, "viewport y", box.Y, rect.Y)
}

// The "document" label has to be true: beyondViewport and clip captures report
// this space, and a caller doing its own document arithmetic from the label was
// silently off by the scroll.
func TestAnnotateBoundsDocumentSpaceAddsScrollWhenScrolled(t *testing.T) {
	f := newScrollFixture(t, 300, 400)

	box, _, vp := f.annotate(t, true)
	rect := f.clientRect(t)

	assertNear(t, "document x", box.X, rect.X+vp.ScrollX)
	assertNear(t, "document y", box.Y, rect.Y+vp.ScrollY)

	if box.Y-rect.Y < vp.ScrollY/2 {
		t.Errorf("document y (%.2f) is not a full scroll (%.2f) past the viewport y (%.2f)", box.Y, vp.ScrollY, rect.Y)
	}
}

// At scroll 0 both spaces coincide, which is why this was invisible until a
// fixture scrolled. Pins that the fix moved nothing for unscrolled pages.
func TestAnnotateBoundsSpacesAgreeWhenUnscrolled(t *testing.T) {
	f := newScrollFixture(t, 0, 0)

	viewportBox, viewportVisible, vp := f.annotate(t, false)
	documentBox, documentVisible, _ := f.annotate(t, true)

	if vp.ScrollX != 0 || vp.ScrollY != 0 {
		t.Fatalf("fixture unexpectedly scrolled: %+v", vp)
	}
	if viewportBox != documentBox {
		t.Errorf("unscrolled page: viewport %+v and document %+v must be identical", viewportBox, documentBox)
	}
	if viewportVisible != documentVisible {
		t.Errorf("unscrolled page: Visible differs between spaces (%v vs %v)", viewportVisible, documentVisible)
	}
}

// Visible is a viewport-intersection test taken before the transform, so it must
// not move when the caller asks for document space.
func TestAnnotateBoundsVisibleIsIndependentOfCoordinateSpace(t *testing.T) {
	f := newScrollFixture(t, 300, 400)

	_, viewportVisible, _ := f.annotate(t, false)
	_, documentVisible, _ := f.annotate(t, true)

	if viewportVisible != documentVisible {
		t.Errorf("Visible = %v in viewport space and %v in document space; it is computed from the same viewport-relative box either way", viewportVisible, documentVisible)
	}
}

// onScreenFixtureHTML places the target where a scroll to (300,400) leaves it at
// viewport (50,100) — well inside any default viewport. The existing scroll
// fixture puts its target below the fold, so no test could tell a correct
// Visible from one computed in the wrong coordinate space.
const onScreenFixtureHTML = `<body style="margin:0;width:3000px;height:3000px">
<div style="position:absolute;left:350px;top:500px;width:120px;height:40px">
<button id="target">target</button>
</div>
</body>`

func TestAnnotateBoundsVisibleForOnScreenNodeWhenScrolled(t *testing.T) {
	f := newScrollFixtureFrom(t, onScreenFixtureHTML, 300, 400)

	rect := f.clientRect(t)
	if rect.X < 0 || rect.Y < 0 {
		t.Fatalf("fixture target is not on screen: %+v", rect)
	}

	for _, pageCoords := range []bool{false, true} {
		box, visible, vp := f.annotate(t, pageCoords)
		if !visible {
			t.Errorf("pageCoords=%v: target at viewport %+v in a %.0fx%.0f viewport reported not visible (box %+v)",
				pageCoords, rect, vp.Width, vp.Height, box)
		}
	}
}

// ElementBorderBox is the third consumer of the getBoxModel premise, behind
// /box and ScrollIntoViewAndGetBox, and both hand its numbers to callers
// unchanged. The premise is only checkable on a scrolled page: at scroll 0 the
// viewport and document answers are the same number, which is why the existing
// /box fixture could not tell them apart.
func TestElementBorderBoxIsViewportRelativeWhenScrolled(t *testing.T) {
	f := newScrollFixtureFrom(t, onScreenFixtureHTML, 300, 400)

	vp, err := FetchLayout(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if vp.ScrollX == 0 || vp.ScrollY == 0 {
		t.Fatalf("fixture did not scroll: %+v", vp)
	}

	box, ok := ElementBorderBox(f.ctx, f.nodeID)
	if !ok {
		t.Fatal("ElementBorderBox produced no box for the target")
	}
	rect := f.clientRect(t)

	assertNear(t, "border-box x", box.X, rect.X)
	assertNear(t, "border-box y", box.Y, rect.Y)
}

// Every unmeasured path for real, beside a measured one, in a single snapshot:
// a node the bounds pass skips (NodeID == 0), a node whose getBoxModel call
// fails (a backend id no document holds), and the target that measures fine.
// The invariant has to hold across the mix, since it is what lets a client read
// an absent "visible" as "not measured" rather than "off-screen".
func TestAnnotateBoundsPairsVisibleWithBoundsAcrossUnmeasuredNodes(t *testing.T) {
	f := newScrollFixture(t, 300, 400)

	vp, err := FetchLayout(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	nodes := []A11yNode{
		{Ref: "e0", NodeID: 0},
		{Ref: "e1", NodeID: f.nodeID},
		{Ref: "e2", NodeID: 1 << 40},
	}
	if err := AnnotateBounds(f.ctx, nodes, false, vp); err != nil {
		t.Fatal(err)
	}

	encoded := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		encoded = append(encoded, marshalNode(t, node))
	}
	assertVisiblePairsWithBounds(t, encoded)

	if _, ok := encoded[1]["visible"]; !ok {
		t.Fatalf("the measured target lost its visible key: %v", encoded[1])
	}
	for _, i := range []int{0, 2} {
		if _, ok := encoded[i]["visible"]; ok {
			t.Errorf("unmeasured node %d published a visible key: %v", i, encoded[i])
		}
	}

	// The target sits below the fold at this scroll, so its measured answer is
	// false — the value that used to vanish.
	if encoded[1]["visible"] != false {
		t.Errorf("target visible = %v, want a measured false", encoded[1]["visible"])
	}
}
