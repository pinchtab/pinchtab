package cdptk_test

import (
	"context"
	"encoding/base64"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// An external test package: internal/testbrowser reaches cdptk through
// internal/browsers/chrome, so an in-package test importing it would cycle.

// The page redefines the method both clip builders read. In the main world it
// answers them; an isolated-world handle never sees it.
const hijackFixtureHTML = `<body style="margin:0">
<div id="target" style="position:absolute;left:40px;top:60px;width:120px;height:60px;background:#000"></div>
<script>
(() => {
	Element.prototype.getBoundingClientRect = function () {
		return {x: 999, y: 999, left: 999, top: 999, right: 1099, bottom: 1049, width: 100, height: 50};
	};
})();
</script>
</body>`

func newHijackFixture(t *testing.T) context.Context {
	t.Helper()
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(testbrowser.Path(t)),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	})

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(hijackFixtureHTML))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#target", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func targetNodeID(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	var nodeID int64
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var doc struct {
			Root struct {
				NodeID int64 `json:"nodeId"`
			} `json:"root"`
		}
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getDocument", map[string]any{"depth": -1}, &doc); err != nil {
			return err
		}
		var found struct {
			NodeID int64 `json:"nodeId"`
		}
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.querySelector", map[string]any{
			"nodeId":   doc.Root.NodeID,
			"selector": "#target",
		}, &found); err != nil {
			return err
		}
		var described struct {
			Node struct {
				BackendNodeID int64 `json:"backendNodeId"`
			} `json:"node"`
		}
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"nodeId": found.NodeID,
		}, &described); err != nil {
			return err
		}
		nodeID = described.Node.BackendNodeID
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if nodeID == 0 {
		t.Fatal("fixture element #target has no backend node id")
	}
	return nodeID
}

// The cdptk analogue of TestScreenshotClipResistsMainWorldSubstitution: the clip
// origin must come from the element, not from the rectangle page script forges.
func TestClipForNodeResistsMainWorldSubstitution(t *testing.T) {
	ctx := newHijackFixture(t)
	nodeID := targetNodeID(t, ctx)

	clip, err := cdptk.ClipForNode(ctx, nodeID, true)
	if err != nil {
		t.Fatalf("ClipForNode: %v", err)
	}

	// The forged rect is the one value the page can produce; anything else means
	// the read happened somewhere the page could not reach.
	if clip.X == 999 || clip.Y == 999 || (clip.Width == 100 && clip.Height == 50) {
		t.Errorf("clip %+v matches the rectangle page script forged; the origin was read in the main world", clip)
	}
	if clip.Width != 120 || clip.Height != 60 {
		t.Errorf("clip %+v does not match the element's real 120x60 box", clip)
	}
}

// The same boundary on the annotation rect: its frame walk reads
// getBoundingClientRect too, and its output places the overlay boxes a vision
// model is told to read.
func TestAnnotationRectForNodeResistsMainWorldSubstitution(t *testing.T) {
	ctx := newHijackFixture(t)
	nodeID := targetNodeID(t, ctx)

	rect, err := cdptk.AnnotationRectForNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("AnnotationRectForNode: %v", err)
	}
	if rect == nil {
		t.Fatal("AnnotationRectForNode returned no rect for a top-frame element")
	}

	if rect.X == 999 || rect.Y == 999 || (rect.W == 100 && rect.H == 50) {
		t.Errorf("rect %+v matches the rectangle page script forged; it was read in the main world", rect)
	}
	if rect.W != 120 || rect.H != 60 {
		t.Errorf("rect %+v does not match the element's real 120x60 box", rect)
	}
	if rect.X != 40 || rect.Y != 60 {
		t.Errorf("rect %+v is not the element's viewport position (40,60)", rect)
	}
}

// The iframe sits at (400,300) and #inner at (10,20) inside it, so a clip that
// carries the frame offset lands at (410,320) and a frame-local one at (10,20).
const framedFixtureHTML = `<body style="margin:0">
<iframe id="f" style="position:absolute;left:400px;top:300px;border:0" width="300" height="200"
	srcdoc="<body style='margin:0'><div id='inner' style='position:absolute;left:10px;top:20px;width:80px;height:40px;background:#00c'></div></body>"></iframe>
</body>`

func newFramedFixture(t *testing.T) context.Context {
	t.Helper()
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(testbrowser.Path(t)),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	})

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(framedFixtureHTML))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#f", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func innerFrameNodeID(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	var nodeID int64
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		exec := chromedp.FromContext(ctx).Target
		var doc struct {
			Root struct {
				NodeID int64 `json:"nodeId"`
			} `json:"root"`
		}
		if err := exec.Execute(ctx, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true}, &doc); err != nil {
			return err
		}
		var frame struct {
			NodeID int64 `json:"nodeId"`
		}
		if err := exec.Execute(ctx, "DOM.querySelector", map[string]any{
			"nodeId": doc.Root.NodeID, "selector": "#f",
		}, &frame); err != nil {
			return err
		}
		var described struct {
			Node struct {
				ContentDocument struct {
					NodeID int64 `json:"nodeId"`
				} `json:"contentDocument"`
			} `json:"node"`
		}
		if err := exec.Execute(ctx, "DOM.describeNode", map[string]any{
			"nodeId": frame.NodeID, "pierce": true, "depth": -1,
		}, &described); err != nil {
			return err
		}
		var inner struct {
			NodeID int64 `json:"nodeId"`
		}
		if err := exec.Execute(ctx, "DOM.querySelector", map[string]any{
			"nodeId": described.Node.ContentDocument.NodeID, "selector": "#inner",
		}, &inner); err != nil {
			return err
		}
		var innerDesc struct {
			Node struct {
				BackendNodeID int64 `json:"backendNodeId"`
			} `json:"node"`
		}
		if err := exec.Execute(ctx, "DOM.describeNode", map[string]any{"nodeId": inner.NodeID}, &innerDesc); err != nil {
			return err
		}
		nodeID = innerDesc.Node.BackendNodeID
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	if nodeID == 0 {
		t.Fatal("fixture element #inner has no backend node id")
	}
	return nodeID
}

// An isolated-world handle runs in the TOP frame's world, where a bare `window`
// has an empty frameElement chain — so the frame walk must start from the node's
// own view or every frame offset is silently dropped.
func TestClipForNodeAppliesFrameOffset(t *testing.T) {
	ctx := newFramedFixture(t)

	clip, err := cdptk.ClipForNode(ctx, innerFrameNodeID(t, ctx), true)
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
}

// The same isolated-world frame-walk trap as the clip builder, on the path that
// actually has production callers: the annotate handler places the overlay boxes
// a vision model is told to read, so a frame-local rect misplaces every one.
func TestAnnotationRectForNodeAppliesFrameOffset(t *testing.T) {
	ctx := newFramedFixture(t)

	rect, err := cdptk.AnnotationRectForNode(ctx, innerFrameNodeID(t, ctx))
	if err != nil {
		t.Fatalf("AnnotationRectForNode for in-frame element: %v", err)
	}
	if rect == nil {
		t.Fatal("AnnotationRectForNode returned no rect for an in-frame element")
	}

	const wantX, wantY, wantW, wantH = 410, 320, 80, 40
	if rect.X != wantX || rect.Y != wantY {
		t.Errorf("in-frame rect origin = (%.0f,%.0f), want (%d,%d) — frame offset not applied (frame-local origin is (10,20))",
			rect.X, rect.Y, wantX, wantY)
	}
	if rect.W != wantW || rect.H != wantH {
		t.Errorf("in-frame rect size = %.0fx%.0f, want %dx%d", rect.W, rect.H, wantW, wantH)
	}
}

// The fromSurface rule had two implementations in packages that cannot import each other,
// and they had already drifted once. The rule itself is now tabled per-GOOS in the
// in-package TestCaptureFromSurfaceRulePerGOOS, which is the only place it can be checked
// whole: this exported wrapper reads runtime.GOOS, so from out here only the host's own
// answers are knowable. This covers what is left — that the wrapper asks the rule about
// the machine it is running on, rather than deciding anything itself.
//
// It used to assert want:false for a plain capture unconditionally, which is the
// non-Windows answer. Windows always composites a surface (issue #619), so the row was
// wrong on the one platform whose clause it was accidentally exercising.
func TestCaptureFromSurface(t *testing.T) {
	clip := &page.Viewport{Width: 120, Height: 60, Scale: 1}
	hostAlwaysComposites := runtime.GOOS == "windows"

	for _, tc := range []struct {
		name           string
		beyondViewport bool
		clip           *page.Viewport
		want           bool
	}{
		{name: "plain capture keeps the fast read where the host allows it", want: hostAlwaysComposites},
		{name: "any clip needs the surface", clip: clip, want: true},
		{name: "beyond viewport needs it with no clip", beyondViewport: true, want: true},
		{name: "both", beyondViewport: true, clip: clip, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cdptk.CaptureFromSurface(tc.beyondViewport, tc.clip); got != tc.want {
				t.Errorf("cdptk.CaptureFromSurface(%v, %+v) = %v, want %v (GOOS=%s)",
					tc.beyondViewport, tc.clip, got, tc.want, runtime.GOOS)
			}
		})
	}
}

// CDP discards a scale-0 clip exactly as it discards one passed with fromSurface=false —
// whole viewport back, no error. No producer emits a scale-0 clip today, so this is a
// latent guard rather than a live fix, and the clip has to be built by hand to reach it.
func TestClipViewportAppliesTheNonZeroScaleRule(t *testing.T) {
	if got := cdptk.ClipViewport(nil); got != nil {
		t.Errorf("cdptk.ClipViewport(nil) = %+v, want nil so the nil-clip fast path survives", got)
	}

	for _, tc := range []struct {
		name      string
		clip      cdptk.ScreenshotClip
		wantScale float64
	}{
		{name: "scale 0 means native, which CDP spells 1", clip: cdptk.ScreenshotClip{X: 40, Y: 60, Width: 120, Height: 60, Scale: 0}, wantScale: 1},
		{name: "an explicit native scale is unchanged", clip: cdptk.ScreenshotClip{X: 40, Y: 60, Width: 120, Height: 60, Scale: 1}, wantScale: 1},
		{name: "a real rescale is carried through", clip: cdptk.ScreenshotClip{X: 40, Y: 60, Width: 120, Height: 60, Scale: 0.5}, wantScale: 0.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := cdptk.ClipViewport(&tc.clip)
			if got == nil {
				t.Fatal("cdptk.ClipViewport returned nil for a non-nil clip")
			}
			if got.Scale != tc.wantScale {
				t.Errorf("scale = %v, want %v — CDP silently discards a scale-0 clip", got.Scale, tc.wantScale)
			}
			if got.X != tc.clip.X || got.Y != tc.clip.Y || got.Width != tc.clip.Width || got.Height != tc.clip.Height {
				t.Errorf("geometry = %+v, want it copied from %+v", got, tc.clip)
			}
		})
	}
}

// The two rules are only "stated once" while nothing else converts a clip on its own. The
// browser proof of the scale rule needs a browser and skips in the lightweight run, so
// this is the browserless half: exactly one conversion of a cdptk.ScreenshotClip into a
// page.Viewport in non-test code, and it is the one in this package.
func TestOnlyCdptkConvertsAScreenshotClipToAViewport(t *testing.T) {
	// srccensus.Tree owns the enumeration (and with it the nested-checkout skip a
	// hand-rolled name list missed); its keys are module-relative slash paths, so
	// the owning-directory check below reads "internal/cdptk" rather than a
	// walk-root-relative "../..". Message change only — the rule is unchanged.
	files := srccensus.Tree(t, filepath.Join("..", ".."), 100)

	converters := 0
	for _, file := range files {
		if !strings.Contains(file.Text, "page.Viewport{") {
			continue
		}
		converts, parseErr := buildsViewportFromAReceiversFields(file.Text)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", file.Name, parseErr)
		}
		if !converts {
			continue
		}
		converters++
		if path.Dir(file.Name) != "internal/cdptk" {
			t.Errorf("%s builds a page.Viewport from a clip's fields; that conversion carries the non-zero-scale rule and belongs to cdptk.ClipViewport alone", file.Name)
		}
	}
	if converters == 0 {
		t.Fatal("found no clip-to-viewport conversion at all; if ClipViewport was renamed or restructured, re-point this census rather than deleting it")
	}
}

// buildsViewportFromAReceiversFields reports whether src builds a page.Viewport by
// copying two or more of its field values off the SAME receiver, whatever that receiver
// is called. Matching the field reads as text keyed the census to one spelling: the
// identical conversion through a local or parameter named anything else went unseen,
// which is the hole this shape closes.
//
// TWO rather than all, and this is the part a later simplification would break: cdptk's
// own converter takes Scale from a normalised local, so a rule demanding every field be
// a selector would miss the converter the census exists to find. Two is also what keeps
// scaledScreenshotClip out — it synthesises from float64 locals and reads a single
// selector, opts.Scale.
//
// Syntactic on purpose. "A selector on a *cdptk.ScreenshotClip" would need go/types and
// real package loading to know what a receiver is; the two-on-one-receiver shape draws
// the same line with the standard parser, and over this tree it flags exactly the one
// legitimate converter.
func buildsViewportFromAReceiversFields(src string) (bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "src.go", src, 0)
	if err != nil {
		return false, err
	}

	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !isPageViewportLiteral(lit.Type) {
			return true
		}
		fieldsPerReceiver := map[string]int{}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			sel, ok := kv.Value.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			receiver, ok := sel.X.(*ast.Ident)
			if !ok {
				continue
			}
			fieldsPerReceiver[receiver.Name]++
		}
		for _, fields := range fieldsPerReceiver {
			if fields >= 2 {
				found = true
			}
		}
		return true
	})
	return found, nil
}

func isPageViewportLiteral(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "page" && sel.Sel.Name == "Viewport"
}

// TestTheClipToViewportCensusIsBlindToTheReceiverName pins the census's own rule against
// the shapes that matter, so the guard cannot quietly stop catching the thing it was
// widened for. The two evasions are the acceptance: identical conversions differing only
// in what the receiver is called. The nesting cases prove the walk descends — an earlier
// AST census in this repo missed a literal inside a call operand.
func TestTheClipToViewportCensusIsBlindToTheReceiverName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flagged bool
		src     string
	}{
		{"cdptk's own converter, Scale from a local", true, `package p
func ClipViewport(clip *ScreenshotClip) *page.Viewport {
	scale := clip.Scale
	return &page.Viewport{X: clip.X, Y: clip.Y, Width: clip.Width, Height: clip.Height, Scale: scale}
}`},
		{"scaledScreenshotClip's synthesis", false, `package p
func scaledScreenshotClip(opts ScreenshotOpts, width, height float64) *page.Viewport {
	return &page.Viewport{X: 0, Y: 0, Width: width, Height: height, Scale: opts.Scale}
}`},
		{"re-open-coded through a renamed local", true, `package p
func f(clip *cdptk.ScreenshotClip) *page.Viewport {
	sc := clip
	return &page.Viewport{X: sc.X, Y: sc.Y, Width: sc.Width, Height: sc.Height, Scale: sc.Scale}
}`},
		{"re-open-coded through a renamed parameter", true, `package p
func f(region *cdptk.ScreenshotClip) *page.Viewport {
	return &page.Viewport{X: region.X, Y: region.Y, Width: region.Width, Height: region.Height}
}`},
		{"nested in a function literal", true, `package p
func f(sc *cdptk.ScreenshotClip) {
	run(func() *page.Viewport { return &page.Viewport{X: sc.X, Y: sc.Y, Width: sc.Width, Height: sc.Height} })
}`},
		{"nested in a composite literal", true, `package p
func f(sc *cdptk.ScreenshotClip) {
	params := map[string]any{"clip": &page.Viewport{X: sc.X, Y: sc.Y, Width: sc.Width, Height: sc.Height}}
	_ = params
}`},
		{"nested in a call argument", true, `package p
func f(sc *cdptk.ScreenshotClip) {
	capture(&page.Viewport{X: sc.X, Y: sc.Y, Width: sc.Width, Height: sc.Height})
}`},
		{"one field off a receiver", false, `package p
func f(sc *cdptk.ScreenshotClip) *page.Viewport {
	return &page.Viewport{X: 0, Y: 0, Width: 100, Height: sc.Height}
}`},
		{"two fields off different receivers", false, `package p
func f(a, b geometry) *page.Viewport {
	return &page.Viewport{X: 0, Y: 0, Width: a.Width, Height: b.Height}
}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildsViewportFromAReceiversFields(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got != tc.flagged {
				t.Errorf("flagged = %v, want %v", got, tc.flagged)
			}
		})
	}
}

func TestTheClipToViewportCensusFailsOnSourceItCannotParse(t *testing.T) {
	if _, err := buildsViewportFromAReceiversFields("package p\nfunc f( {"); err == nil {
		t.Error("unparseable source reported no conversion instead of an error; a file the census cannot read must fail it, not pass silently")
	}
}
