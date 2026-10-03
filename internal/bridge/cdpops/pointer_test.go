package cdpops

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"go/ast"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func TestDispatchMouseMoveFallsBackToSyntheticOnDeadline(t *testing.T) {
	origReal := dispatchRealMouseMoveFunc
	origSynthetic := dispatchSyntheticMouseMoveFunc
	t.Cleanup(func() {
		dispatchRealMouseMoveFunc = origReal
		dispatchSyntheticMouseMoveFunc = origSynthetic
	})

	dispatchRealMouseMoveFunc = func(context.Context, float64, float64, input.MouseButton, int64) error {
		return context.DeadlineExceeded
	}

	called := false
	dispatchSyntheticMouseMoveFunc = func(_ context.Context, x, y float64, button input.MouseButton, buttons int64) error {
		called = true
		if x != 12 || y != 34 {
			t.Fatalf("synthetic move coordinates = (%v, %v), want (12, 34)", x, y)
		}
		if button != input.Left || buttons != 1 {
			t.Fatalf("synthetic move button state = (%v, %d), want (%v, 1)", button, buttons, input.Left)
		}
		return nil
	}

	if err := dispatchMouseMove(context.Background(), 12, 34, input.Left, 1); err != nil {
		t.Fatalf("dispatchMouseMove returned error: %v", err)
	}
	if !called {
		t.Fatal("expected synthetic fallback to run")
	}
}

func TestDispatchMouseMoveDoesNotFallbackOnNonDeadlineError(t *testing.T) {
	origReal := dispatchRealMouseMoveFunc
	origSynthetic := dispatchSyntheticMouseMoveFunc
	t.Cleanup(func() {
		dispatchRealMouseMoveFunc = origReal
		dispatchSyntheticMouseMoveFunc = origSynthetic
	})

	want := errors.New("cdp failed")
	dispatchRealMouseMoveFunc = func(context.Context, float64, float64, input.MouseButton, int64) error {
		return want
	}
	dispatchSyntheticMouseMoveFunc = func(context.Context, float64, float64, input.MouseButton, int64) error {
		t.Fatal("synthetic fallback should not run for non-timeout CDP errors")
		return nil
	}

	if err := dispatchMouseMove(context.Background(), 12, 34, input.None, 0); !errors.Is(err, want) {
		t.Fatalf("dispatchMouseMove error = %v, want %v", err, want)
	}
}

func TestDispatchMouseMoveContextCancellationWinsOverFallback(t *testing.T) {
	origReal := dispatchRealMouseMoveFunc
	origSynthetic := dispatchSyntheticMouseMoveFunc
	t.Cleanup(func() {
		dispatchRealMouseMoveFunc = origReal
		dispatchSyntheticMouseMoveFunc = origSynthetic
	})

	dispatchRealMouseMoveFunc = func(context.Context, float64, float64, input.MouseButton, int64) error {
		return context.DeadlineExceeded
	}
	dispatchSyntheticMouseMoveFunc = func(context.Context, float64, float64, input.MouseButton, int64) error {
		t.Fatal("synthetic fallback should not run after caller context cancellation")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := dispatchMouseMove(ctx, 12, 34, input.None, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatchMouseMove error = %v, want context.Canceled", err)
	}
}

func TestDispatchMouseMoveToNodeFallsBackToSyntheticNodeMove(t *testing.T) {
	origReal := dispatchRealMouseMoveFunc
	origSyntheticNode := dispatchSyntheticMouseMoveOnNodeFunc
	t.Cleanup(func() {
		dispatchRealMouseMoveFunc = origReal
		dispatchSyntheticMouseMoveOnNodeFunc = origSyntheticNode
	})

	dispatchRealMouseMoveFunc = func(context.Context, float64, float64, input.MouseButton, int64) error {
		return context.DeadlineExceeded
	}

	called := false
	dispatchSyntheticMouseMoveOnNodeFunc = func(_ context.Context, nodeID int64, button input.MouseButton, buttons int64) error {
		called = true
		if nodeID != 42 {
			t.Fatalf("nodeID = %d, want 42", nodeID)
		}
		if button != input.Right || buttons != 2 {
			t.Fatalf("button state = (%v, %d), want (%v, 2)", button, buttons, input.Right)
		}
		return nil
	}

	if err := dispatchMouseMoveToNode(context.Background(), 42, 12, 34, input.Right, 2); err != nil {
		t.Fatalf("dispatchMouseMoveToNode returned error: %v", err)
	}
	if !called {
		t.Fatal("expected synthetic node fallback to run")
	}
}

// An unspecified button is a default; an unrecognised NAME is a caller error. Those two
// shared one answer, so a mistyped right-click dispatched a left-click and reported success.
func TestValidateMouseButtonSeparatesUnspecifiedFromUnrecognised(t *testing.T) {
	for _, tc := range []struct {
		in      string
		wantErr bool
		why     string
	}{
		{in: "", why: "unspecified is the default, not an error — refusing it would break every caller that never named one"},
		{in: "   ", why: "whitespace-only is still unspecified"},
		{in: "left"},
		{in: "right"},
		{in: "middle"},
		{in: "RIGHT", why: "case tolerance is worth keeping"},
		{in: " right ", why: "surrounding whitespace is worth keeping"},
		{in: "rihgt", wantErr: true, why: "a misspelling used to dispatch left"},
		{in: "primary", wantErr: true, why: "the DOM vocabulary is refused, not mapped: primary happens to mean left, which is what made this class look harmless"},
		{in: "secondary", wantErr: true, why: "the case primary hides — mapping the DOM names would silently make this left"},
		{in: "0", wantErr: true, why: "a numeric button is not this vocabulary"},
	} {
		err := ValidateMouseButton(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateMouseButton(%q) accepted it; %s", tc.in, tc.why)
			continue
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ValidateMouseButton(%q) = %v; %s", tc.in, err, tc.why)
			continue
		}
		if tc.wantErr {
			for _, name := range MouseButtons() {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("ValidateMouseButton(%q) = %v, want it to name the valid button %q", tc.in, err, name)
				}
			}
		}
	}
}

// One vocabulary owner: the normalizer accepts exactly what MouseButtons lists and the
// refusal names exactly that list, so a fourth button added there cannot be accepted in one
// place and refused in another.
func TestTheButtonVocabularyHasOneOwner(t *testing.T) {
	buttons := MouseButtons()
	if len(buttons) < 3 {
		t.Fatalf("MouseButtons() = %v, want at least the three this CLI documents", buttons)
	}
	for _, name := range buttons {
		if err := ValidateMouseButton(name); err != nil {
			t.Errorf("%q is listed as a button but refused: %v", name, err)
		}
		if got := normalizeMouseButton(name); got != name {
			t.Errorf("normalizeMouseButton(%q) = %q; a listed button must survive normalization", name, got)
		}
	}
	if got := normalizeMouseButton(""); got != DefaultMouseButton {
		t.Errorf("normalizeMouseButton(\"\") = %q, want the default %q", got, DefaultMouseButton)
	}
}

// Derived from the callers rather than a hand-listed set: every exported entry point taking
// a button must route it through the normalizer, so none reaches CDP with a raw caller
// string. Adding an entry point that forgets is what this catches.
func TestEveryButtonTakingEntryPointNormalizesIt(t *testing.T) {
	pkg := srccensus.Load(t, ".", 7)
	_, files := pkg.Syntax()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	checked := 0
	for _, name := range names {
		for _, decl := range files[name].Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !takesAButton(fn) {
				continue
			}
			if fn.Name.Name == "normalizeMouseButton" || fn.Name.Name == "ValidateMouseButton" {
				continue
			}
			checked++
			if !callsNormalizer(fn) && !passesButtonOn(fn) {
				t.Errorf("%s takes a button and neither normalizes it nor passes it to something that does, so a caller string reaches CDP raw", fn.Name.Name)
			}
		}
	}
	if checked < 4 {
		t.Fatalf("found only %d button-taking functions in %s; this census is not reading the callers", checked, pkg.Dir())
	}
}

func takesAButton(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil {
		return false
	}
	for _, param := range fn.Type.Params.List {
		ident, ok := param.Type.(*ast.Ident)
		if !ok || ident.Name != "string" {
			continue
		}
		for _, name := range param.Names {
			if name.Name == "button" {
				return true
			}
		}
	}
	return false
}

func callsNormalizer(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "normalizeMouseButton" {
			found = true
		}
		return true
	})
	return found
}

// A wrapper that hands the button to another button-taking helper is covered by that
// helper's own normalization, so it counts.
func passesButtonOn(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, arg := range call.Args {
			if ident, ok := arg.(*ast.Ident); ok && ident.Name == "button" {
				found = true
			}
		}
		return true
	})
	return found
}

// dragCapture records what one DragBetweenPoints put on the wire, without a browser: the
// press/release payloads and the button state of every interpolated move.
type dragCapture struct {
	pressed  []map[string]any
	moveEnum []input.MouseButton
	moveMask []int64
}

func captureDrag(t *testing.T, button string) (*dragCapture, error) {
	t.Helper()
	origEvent, origMove := dispatchMouseEventFunc, dispatchRealMouseMoveFunc
	t.Cleanup(func() {
		dispatchMouseEventFunc = origEvent
		dispatchRealMouseMoveFunc = origMove
	})

	got := &dragCapture{}
	dispatchMouseEventFunc = func(_ context.Context, payload map[string]any) error {
		got.pressed = append(got.pressed, payload)
		return nil
	}
	dispatchRealMouseMoveFunc = func(_ context.Context, _, _ float64, b input.MouseButton, mask int64) error {
		got.moveEnum = append(got.moveEnum, b)
		got.moveMask = append(got.moveMask, mask)
		return nil
	}
	return got, DragBetweenPoints(context.Background(), 10, 10, 200, 200, button)
}

// THE ASSERTION THAT WOULD HAVE CAUGHT THE SPLIT, driven at the call site and enumerated
// over the table rather than over three restated names: a drag used to take its press
// button from the NAME and its held moves from a hand-written switch, so a fourth entry
// would have pressed one button and moved under another. Adding a row to the table brings
// it into this test with no edit here.
func TestEveryButtonInTheTableIsPressedAndHeldAsItself(t *testing.T) {
	for _, want := range mouseButtonTable {
		t.Run(want.name, func(t *testing.T) {
			got, err := captureDrag(t, want.name)
			if err != nil {
				t.Fatalf("DragBetweenPoints(%q): %v", want.name, err)
			}

			if len(got.pressed) != 2 {
				t.Fatalf("press/release events = %d, want 2: %v", len(got.pressed), got.pressed)
			}
			for _, payload := range got.pressed {
				if payload["button"] != want.name {
					t.Errorf("%v carries button %v, want %q", payload["type"], payload["button"], want.name)
				}
			}

			if len(got.moveEnum) == 0 {
				t.Fatal("no interpolated moves were dispatched, so nothing pins what the drag held")
			}
			heldMoves := 0
			for i, enum := range got.moveEnum {
				if got.moveMask[i] == 0 && enum == input.None {
					continue // the initial position move, dispatched before the press
				}
				heldMoves++
				if enum != want.enum {
					t.Errorf("move %d held enum %v, want %v — the press said %q", i, enum, want.enum, want.name)
				}
				if got.moveMask[i] != want.held {
					t.Errorf("move %d held mask %d, want %d", i, got.moveMask[i], want.held)
				}
			}
			if heldMoves == 0 {
				t.Fatal("every move was button-less, so the held state is unpinned")
			}
		})
	}
}

// Unspecified means the default; unrecognised is a caller error. The second half is the
// silent fallback this card removed — heldButton answered left for any name, so a button
// the validator would have refused still dragged, as left.
func TestHeldButtonSeparatesUnspecifiedFromUnrecognised(t *testing.T) {
	for _, unspecified := range []string{"", "   ", "\t"} {
		row, err := heldButton(unspecified)
		if err != nil {
			t.Errorf("heldButton(%q) = %v; an unspecified button is the default, not an error", unspecified, err)
			continue
		}
		if row.name != DefaultMouseButton {
			t.Errorf("heldButton(%q) = %q, want the default %q", unspecified, row.name, DefaultMouseButton)
		}
	}

	// Derived, not listed: a name that later joins the table must stop being a fixture
	// here rather than turning this test into a false alarm about its own staleness.
	for _, unrecognised := range namesOutsideTheTable("back", "forward", "primary", "LEFTish") {
		if row, err := heldButton(unrecognised); err == nil {
			t.Errorf("heldButton(%q) = %+v with no error; an unrecognised name must not be reinterpreted", unrecognised, row)
		} else if !strings.Contains(err.Error(), strings.Join(MouseButtons(), ", ")) {
			t.Errorf("heldButton(%q) error = %q; it must name the buttons that do exist", unrecognised, err)
		}
	}

	outside := namesOutsideTheTable("back", "forward")
	if len(outside) == 0 {
		t.Skip("every candidate name is now a real button, so there is no unrecognised case to drive")
	}
	if _, err := captureDrag(t, outside[0]); err == nil {
		t.Errorf("DragBetweenPoints accepted %q; the drag is where the silent left used to happen", outside[0])
	}
}

// namesOutsideTheTable keeps the candidates that are genuinely not buttons.
func namesOutsideTheTable(candidates ...string) []string {
	outside := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := mouseButtonNamed(strings.ToLower(strings.TrimSpace(candidate))); !ok {
			outside = append(outside, candidate)
		}
	}
	return outside
}

// Every fact about a button is on its row, so the enum, the mask and the JS code cannot be
// spelled a second time. The masks are the CDP "buttons" bits and must stay distinct powers
// of two, or two buttons would be indistinguishable while held.
func TestTheButtonTableIsTheOnlyPlaceAButtonFactIsWritten(t *testing.T) {
	if len(mouseButtonTable) < 3 {
		t.Fatalf("table = %+v; a census over fewer than the three documented buttons would pass vacuously", mouseButtonTable)
	}

	seenName, seenEnum, seenMask := map[string]bool{}, map[input.MouseButton]bool{}, map[int64]bool{}
	for _, b := range mouseButtonTable {
		if seenName[b.name] || seenEnum[b.enum] || seenMask[b.held] {
			t.Errorf("row %+v repeats a name, enum or mask already in the table", b)
		}
		seenName[b.name], seenEnum[b.enum], seenMask[b.held] = true, true, true
		if b.held == 0 || b.held&(b.held-1) != 0 {
			t.Errorf("row %+v has held mask %d; the CDP buttons field is a bitmask, so each button owns one bit", b, b.held)
		}
		if _, ok := mouseButtonForEnum(b.enum); !ok {
			t.Errorf("row %+v is not reachable by enum, so mouseButtonCode cannot derive its JS code", b)
		}
		if code := mouseButtonCode(b.enum); code != b.jsCode {
			t.Errorf("mouseButtonCode(%v) = %d, want the row's %d", b.enum, code, b.jsCode)
		}
	}

	// A move with nothing pressed is not a left click: input.None has no row, and its zero
	// is the DOM's "no button" rather than a stand-in for left.
	if _, ok := mouseButtonForEnum(input.None); ok {
		t.Error("input.None has a table row; it is the absence of a button, not one of them")
	}
	if got := mouseButtonCode(input.None); got != noButtonJSCode {
		t.Errorf("mouseButtonCode(input.None) = %d, want %d", got, noButtonJSCode)
	}
}

// A link click whose trusted press already navigated leaves the old document's
// context gone before jsClickIfLink's .click() runs. That is the click landing,
// not failing: reported as an error it became a retryable 500, and a retry would
// click on the page the link led to.
func TestLinkContextGoneOnlyMatchesADestroyedContext(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{&cdproto.Error{Code: -32000, Message: "Cannot find context with specified id"}, true},
		{fmt.Errorf("click: %w", &cdproto.Error{Code: -32000, Message: "Execution context was destroyed."}), true},
		{&cdproto.Error{Code: -32000, Message: "Node is detached from document"}, false},
		{&cdproto.Error{Code: -32602, Message: "Cannot find context with specified id"}, false},
		{errors.New("Cannot find context with specified id"), false},
		{nil, false},
	} {
		if got := linkContextGone(tc.err); got != tc.want {
			t.Errorf("linkContextGone(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func stubMouseMoves(t *testing.T, real func(context.Context) error) *bool {
	t.Helper()
	origReal, origSynthetic := dispatchRealMouseMoveFunc, dispatchSyntheticMouseMoveFunc
	t.Cleanup(func() {
		dispatchRealMouseMoveFunc = origReal
		dispatchSyntheticMouseMoveFunc = origSynthetic
	})
	dispatchRealMouseMoveFunc = func(ctx context.Context, _, _ float64, _ input.MouseButton, _ int64) error {
		return real(ctx)
	}
	fellBack := false
	dispatchSyntheticMouseMoveFunc = func(context.Context, float64, float64, input.MouseButton, int64) error {
		fellBack = true
		return nil
	}
	return &fellBack
}

func TestASlowRealMoveWithSecondsLeftSucceedsWithoutTheSyntheticFallback(t *testing.T) {
	fellBack := stubMouseMoves(t, func(ctx context.Context) error {
		select {
		case <-time.After(120 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := dispatchMouseMove(ctx, 1, 2, input.None, 0); err != nil {
		t.Fatalf("dispatchMouseMove: %v", err)
	}
	if *fellBack {
		t.Fatal("a 120ms real move with seconds left fell back to synthetic events")
	}
}

func TestAHangingRealMoveFallsBackBeforeTheCeiling(t *testing.T) {
	fellBack := stubMouseMoves(t, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	start := time.Now()
	if err := dispatchMouseMove(ctx, 1, 2, input.None, 0); err != nil {
		t.Fatalf("dispatchMouseMove: %v", err)
	}
	if elapsed := time.Since(start); elapsed > mouseMoveBudgetCeiling+time.Second {
		t.Errorf("hanging move fell back after %v, want by the %v ceiling", elapsed, mouseMoveBudgetCeiling)
	}
	if !*fellBack {
		t.Fatal("a hanging real move did not fall back to synthetic events")
	}
}

func TestTheMoveBudgetScalesWithTheDeadlineBetweenFloorAndCeiling(t *testing.T) {
	for _, tc := range []struct {
		left time.Duration
		want time.Duration
	}{
		{100 * time.Millisecond, mouseMoveBudgetFloor},
		{3 * time.Second, 300 * time.Millisecond},
		{time.Minute, mouseMoveBudgetCeiling},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), tc.left)
		got := mouseMoveBudget(ctx)
		cancel()
		if got > tc.want || got < tc.want-10*time.Millisecond {
			t.Errorf("budget with %v left = %v, want %v", tc.left, got, tc.want)
		}
	}
	if got := mouseMoveBudget(context.Background()); got != mouseMoveBudgetCeiling {
		t.Errorf("budget with no deadline = %v, want the %v ceiling", got, mouseMoveBudgetCeiling)
	}
}

func TestElementNotFocusableOnlyMatchesTheFocusRefusal(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{&cdproto.Error{Code: -32000, Message: "Element is not focusable"}, true},
		{fmt.Errorf("focus: %w", &cdproto.Error{Code: -32000, Message: "Element is not focusable"}), true},
		{&cdproto.Error{Code: -32000, Message: "Node is detached from document"}, false},
		{&cdproto.Error{Code: -32602, Message: "Element is not focusable"}, false},
		{errors.New("Element is not focusable"), false},
		{nil, false},
	} {
		if got := elementNotFocusable(tc.err); got != tc.want {
			t.Errorf("elementNotFocusable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

const doubleClickFixtureHTML = `<!doctype html><html><body>
<h1 id="heading" style="width:200px;height:60px" ondblclick="window.dblclicks=(window.dblclicks||0)+1">Shop</h1>
<input id="field" style="width:200px;height:30px" onmousedown="window.focusedAtPress=window.focusedAtPress||document.activeElement.id">
</body></html>`

func TestDoubleClickFocusesOnlyWhatCanTakeFocus(t *testing.T) {
	ctx := testbrowser.Launch(t)
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(doubleClickFixtureHTML))
	if err := chromedp.Run(ctx, chromedp.Navigate(dataURL)); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	stepCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := DoubleClickByNodeID(stepCtx, backendNodeIDOf(t, stepCtx, "#heading")); err != nil {
		t.Fatalf("DoubleClickByNodeID(heading) = %v, want the press dispatched without focus", err)
	}
	var dblclicks int
	if err := chromedp.Run(stepCtx, chromedp.Evaluate(`window.dblclicks||0`, &dblclicks)); err != nil {
		t.Fatal(err)
	}
	if dblclicks != 1 {
		t.Fatalf("heading dblclick events = %d, want 1", dblclicks)
	}

	if err := DoubleClickByNodeID(stepCtx, backendNodeIDOf(t, stepCtx, "#field")); err != nil {
		t.Fatalf("DoubleClickByNodeID(field) = %v", err)
	}
	var focusedAtPress string
	if err := chromedp.Run(stepCtx, chromedp.Evaluate(`window.focusedAtPress`, &focusedAtPress)); err != nil {
		t.Fatal(err)
	}
	if focusedAtPress != "field" {
		t.Fatalf("active element at the input's first mousedown = %q, want field focused before the press", focusedAtPress)
	}
}

func backendNodeIDOf(t *testing.T, ctx context.Context, selector string) int64 {
	t.Helper()
	var nodeID int64
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		obj, _, err := runtime.Evaluate(fmt.Sprintf("document.querySelector(%q)", selector)).Do(ctx)
		if err != nil {
			return err
		}
		node, err := dom.DescribeNode().WithObjectID(obj.ObjectID).Do(ctx)
		if err != nil {
			return err
		}
		nodeID = int64(node.BackendNodeID)
		return nil
	})); err != nil {
		t.Fatalf("resolve %s: %v", selector, err)
	}
	return nodeID
}
