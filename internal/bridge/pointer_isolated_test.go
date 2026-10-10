package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/config"
)

func newPointerFixture(t *testing.T, html string) context.Context {
	t.Helper()
	ctx := newActionTab(t, 30*time.Second)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#target", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// The page forges both methods the probe reads. getBoundingClientRect moves the
// element's reported box far from where it renders, and getComputedStyle would
// let the page fake visibility or pointer-events. In the main world the probe
// believes them and the synthetic click lands wherever the page said.
const pointerHijackHTML = `<body style="margin:0">
<button id="target" style="position:absolute;left:40px;top:60px;width:120px;height:40px">Accept</button>
<script>
Element.prototype.getBoundingClientRect = function () {
	return {x: 700, y: 500, left: 700, top: 500, right: 800, bottom: 540, width: 100, height: 40};
};
const realGCS = window.getComputedStyle;
window.getComputedStyle = function (el) {
	const s = realGCS.call(window, el);
	return new Proxy(s, {get: (t, k) => (k === 'pointerEvents' ? 'auto' : Reflect.get(t, k))});
};
</script>
</body>`

func TestPointerPointResistsForgedGeometry(t *testing.T) {
	ctx := newPointerFixture(t, pointerHijackHTML)

	nodeID, err := ResolveCSSToNodeID(ctx, "#target")
	if err != nil {
		t.Fatal(err)
	}

	x, y, err := bridgecdpops.PointerPointForNode(ctx, nodeID, false)
	if err != nil {
		t.Fatalf("pointer point: %v", err)
	}

	// The element really renders at (40,60)-(160,100), so its centre is (100,80).
	// The forged rect would put the click at (750,520).
	const wantX, wantY = 100.0, 80.0
	if x != wantX || y != wantY {
		t.Errorf("pointer point = (%.0f,%.0f), want (%.0f,%.0f); page script steered the click", x, y, wantX, wantY)
	}
}

const pointerFrameHTML = `<body style="margin:0">
<div id="target" style="position:absolute;left:0;top:0;width:10px;height:10px"></div>
<iframe id="outer" style="position:absolute;left:120px;top:80px;border:0" width="400" height="300"
	srcdoc="<body style='margin:0'><iframe id='inner' style='position:absolute;left:30px;top:20px;border:0' width='300' height='200' srcdoc=&quot;<body style='margin:0'><button id='deep' style='position:absolute;left:25px;top:15px;width:80px;height:30px' onclick='this.dataset.hits=String(Number(this.dataset.hits||0)+1)'>Deep</button></body>&quot;></iframe></body>"></iframe>
</body>`

// The frame-offset walk reads current.frameElement up the chain. It ran in the
// node's own frame before; from an isolated world it must still start there, or
// a nested click lands at frame-local coordinates in the top document.
func TestPointerPointWalksNestedFrameOffsets(t *testing.T) {
	ctx := newPointerFixture(t, pointerFrameHTML)

	deepNodeID := deepFrameNode(t, ctx)

	x, y, err := bridgecdpops.PointerPointForNode(ctx, deepNodeID, false)
	if err != nil {
		t.Fatalf("pointer point: %v", err)
	}

	// 120+30+25 + 80/2 = 215, 80+20+15 + 30/2 = 130.
	const wantX, wantY = 215.0, 130.0
	if x != wantX || y != wantY {
		t.Errorf("nested pointer point = (%.0f,%.0f), want (%.0f,%.0f); frame offsets were not applied", x, y, wantX, wantY)
	}

	if err := bridgecdpops.ClickByCoordinate(ctx, x, y, 0); err != nil {
		t.Fatalf("click: %v", err)
	}
	var hits int
	if err := bridgecdpops.CallFunctionOnNode(ctx, deepNodeID,
		`function(){ return Number(this.dataset.hits || 0); }`, nil, &hits); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("deep button received %d clicks, want 1 — the coordinate did not land on it", hits)
	}
}

func deepFrameNode(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	var walk func(tree bridgeobserve.RawFrameTree) []string
	walk = func(tree bridgeobserve.RawFrameTree) []string {
		ids := []string{tree.Frame.ID}
		for _, child := range tree.ChildFrames {
			ids = append(ids, walk(child)...)
		}
		return ids
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		tree, err := bridgeobserve.FetchFrameTree(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, frameID := range walk(tree) {
			if nodeID, err := ResolveCSSToNodeIDInFrame(ctx, frameID, "#deep"); err == nil {
				return nodeID
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("nested #deep button never appeared")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

const pointerOcclusionHTML = `<body style="margin:0">
<button id="target" style="position:absolute;left:40px;top:60px;width:120px;height:40px">Accept</button>
<div id="cover" style="position:absolute;left:0;top:0;width:400px;height:300px;background:#000"></div>
</body>`

// requireTopMost is the guard that refuses a click the user could not have made.
// Both of its answers are asserted: an element under an overlay must still be
// refused, and a clear one must still be allowed.
func TestPointerPointOcclusionUnchanged(t *testing.T) {
	ctx := newPointerFixture(t, pointerOcclusionHTML)

	nodeID, err := ResolveCSSToNodeID(ctx, "#target")
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := bridgecdpops.PointerPointForNode(ctx, nodeID, true); !errors.Is(err, bridgecdpops.ErrElementOccluded) {
		t.Errorf("covered element: err = %v, want ErrElementOccluded", err)
	}
	if _, _, err := bridgecdpops.PointerPointForNode(ctx, nodeID, false); err != nil {
		t.Errorf("covered element without requireTopMost should still resolve: %v", err)
	}

	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById('cover').remove(), true`, new(bool))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := bridgecdpops.PointerPointForNode(ctx, nodeID, true); err != nil {
		t.Errorf("uncovered element: err = %v, want a clickable point", err)
	}
}

const pointerCookieWallHTML = `<body style="margin:0">
<button id="target" style="position:absolute;left:40px;top:60px;width:120px;height:40px">Add</button>
<div id="wall" role="dialog" aria-label="Cookies" style="position:absolute;left:0;top:0;width:400px;height:300px;background:#fff">
  <div id="pane"><button id="accept">Accept all</button></div>
</div>
</body>`

// Nearly every page has a div on top, so "top=div" told the agent nothing. The
// refusal has to name the overlay the way the snapshot does, and its ref when
// the tab's cache still holds that ancestor, so the agent's next move is
// "dismiss e0" rather than a retry or a DOM-mode click through the wall.
func TestOccludedClickNamesTheOverlayAndItsRef(t *testing.T) {
	ctx := newPointerFixture(t, pointerCookieWallHTML)

	targetID, err := ResolveCSSToNodeID(ctx, "#target")
	if err != nil {
		t.Fatal(err)
	}
	wallID, err := ResolveCSSToNodeID(ctx, "#wall")
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = bridgecdpops.PointerPointForNode(ctx, targetID, true)
	var occluded *bridgecdpops.OccludedError
	if !errors.As(err, &occluded) || !errors.Is(err, bridgecdpops.ErrElementOccluded) {
		t.Fatalf("err = %v, want OccludedError", err)
	}
	if got, want := err.Error(), `element is occluded by dialog "Cookies"`; got != want {
		t.Errorf("without a ref cache: %q, want %q", got, want)
	}
	if len(occluded.Ancestry) < 2 || occluded.Ancestry[0] != wallID {
		t.Errorf("ancestry = %v, want dialog %d then its ancestors", occluded.Ancestry, wallID)
	}

	b := &Bridge{TabManager: NewTabManager(ctx, &config.RuntimeConfig{}, nil, nil)}
	b.SetRefCache("tab1", &RefCache{Nodes: []A11yNode{{Ref: "e0", Role: "dialog", Name: "Cookies", NodeID: wallID}}})
	if got := b.nameOccluder("other", err).Error(); strings.Contains(got, "(e0)") {
		t.Errorf("a tab without that snapshot must not borrow its ref: %q", got)
	}
	if got, want := b.nameOccluder("tab1", err).Error(), `element is occluded by dialog "Cookies" (e0)`; got != want {
		t.Errorf("with a ref cache: %q, want %q", got, want)
	}
}

func TestOccludedAncestryIsCappedAndHitFirst(t *testing.T) {
	const depth = 80
	html := `<body style="margin:0"><button id="target" style="position:absolute;left:40px;top:60px;width:120px;height:40px">Add</button>` +
		`<div id="wall" style="position:absolute;left:0;top:0;width:400px;height:300px;background:#fff">` +
		strings.Repeat(`<div style="width:100%;height:100%">`, depth) + `<span id="hit" style="display:block;width:100%;height:100%"></span>` +
		strings.Repeat(`</div>`, depth) + `</div></body>`
	ctx := newPointerFixture(t, html)

	targetID, err := ResolveCSSToNodeID(ctx, "#target")
	if err != nil {
		t.Fatal(err)
	}
	hitID, err := ResolveCSSToNodeID(ctx, "#hit")
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = bridgecdpops.PointerPointForNode(ctx, targetID, true)
	var occluded *bridgecdpops.OccludedError
	if !errors.As(err, &occluded) {
		t.Fatalf("err = %v, want OccludedError", err)
	}
	if len(occluded.Ancestry) != 64 || occluded.Ancestry[0] != hitID {
		t.Errorf("ancestry has %d entries starting at %d, want 64 starting at the hit %d", len(occluded.Ancestry), occluded.Ancestry[0], hitID)
	}
}

func TestExecuteActionNamesTheNearestCachedAncestorOfTheOccluder(t *testing.T) {
	b := newTestBridge()
	b.Actions = map[string]ActionFunc{
		ActionClick: func(context.Context, ActionRequest) (map[string]any, error) {
			return nil, &bridgecdpops.OccludedError{Ancestry: []int64{91, 57, 12}}
		},
	}
	b.SetRefCache("tab1", &RefCache{Nodes: []A11yNode{
		{Ref: "e0", Role: "dialog", Name: "Cookies", NodeID: 57},
		{Ref: "e4", Role: "main", NodeID: 12},
	}})

	_, err := b.ExecuteAction(context.Background(), ActionClick, ActionRequest{TabID: "tab1"})

	if !errors.Is(err, ErrElementOccluded) {
		t.Fatalf("err = %v, want ErrElementOccluded", err)
	}
	if got, want := err.Error(), `element is occluded by dialog "Cookies" (e0)`; got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
}

// A probe that throws must say why. It once decoded the exception as a zero probe
// and reported ErrElementHidden, sending callers after visibility when the script
// itself had failed. A text node has no getBoundingClientRect, so the probe throws.
func TestPointerPointSurfacesAProbeException(t *testing.T) {
	ctx := newPointerFixture(t, `<body><button id="target">Accept</button></body>`)

	buttonID, err := ResolveCSSToNodeID(ctx, "#target")
	if err != nil {
		t.Fatal(err)
	}
	var textNodeID int64
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var res struct {
			Node struct {
				Children []struct {
					NodeType      int   `json:"nodeType"`
					BackendNodeID int64 `json:"backendNodeId"`
				} `json:"children"`
			} `json:"node"`
		}
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"backendNodeId": buttonID,
			"depth":         1,
		}, &res); err != nil {
			return err
		}
		for _, child := range res.Node.Children {
			if child.NodeType == 3 {
				textNodeID = child.BackendNodeID
			}
		}
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if textNodeID == 0 {
		t.Fatal("button has no text node child")
	}

	_, _, err = bridgecdpops.PointerPointForNode(ctx, textNodeID, false)
	if err == nil {
		t.Fatal("a probe that threw returned a pointer point")
	}
	if errors.Is(err, bridgecdpops.ErrElementHidden) {
		t.Fatalf("a probe exception was reported as hidden: %v", err)
	}
	if !strings.Contains(err.Error(), "pointer probe") || !strings.Contains(err.Error(), "getBoundingClientRect is not a function") {
		t.Errorf("error %q does not carry the probe's exception text", err)
	}
}
