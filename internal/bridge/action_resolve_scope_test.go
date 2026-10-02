package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// stubScope records what the shared recursion asks of a scope, so the grammar
// can be exercised without a browser. The real scopes differ only in these two
// answers, which is the premise the shared recursion rests on.
type stubScope struct {
	atCalls  []stubAt
	refCalls []string
	nodeID   int64
	refErr   error
}

type stubAt struct {
	kind       selector.Kind
	value      string
	index      int
	fromEnd    bool
	positional bool
}

func (s *stubScope) resolveAt(_ context.Context, sel selector.Selector, index int, fromEnd bool, positional bool) (int64, error) {
	s.atCalls = append(s.atCalls, stubAt{sel.Kind, sel.Value, index, fromEnd, positional})
	return s.nodeID, nil
}

func (s *stubScope) resolveRef(_ context.Context, sel selector.Selector, _ *RefCache) (int64, error) {
	s.refCalls = append(s.refCalls, sel.Value)
	if s.refErr != nil {
		return 0, s.refErr
	}
	return s.nodeID, nil
}

// Every wrapper form must reach the leaf with the same index/fromEnd it did when
// each scope had its own copy of this grammar — and must mark itself POSITIONAL,
// which is what tells the resolver to index the document instead of the semantic
// ranking. A bare selector must not: plain text: and first:text: arrive with the
// same index and direction, so this flag is the only thing separating them.
func TestResolveWrapperUnwrapsToTheSameLeafForEveryForm(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want stubAt
	}{
		{"first:css:#a", stubAt{selector.KindCSS, "#a", 0, false, true}},
		{"last:css:#a", stubAt{selector.KindCSS, "#a", 0, true, true}},
		{"nth:2:css:#a", stubAt{selector.KindCSS, "#a", 2, false, true}},
		{"first:text:Save", stubAt{selector.KindText, "Save", 0, false, true}},
		// Nesting collapses: the innermost wrapper decides.
		{"first:last:css:#a", stubAt{selector.KindCSS, "#a", 0, true, true}},
		{"nth:3:first:css:#a", stubAt{selector.KindCSS, "#a", 0, false, true}},
		// Unwrapped: same index and direction as first:, and NOT positional.
		{"css:#a", stubAt{selector.KindCSS, "#a", 0, false, false}},
		{"text:Save", stubAt{selector.KindText, "Save", 0, false, false}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			scope := &stubScope{nodeID: 42}
			sel := selector.Parse(tc.raw)

			got, err := resolveParsed(context.Background(), scope, sel, nil, 0, false, false)
			if err != nil {
				t.Fatalf("resolveParsed(%q): %v", tc.raw, err)
			}
			if got != 42 {
				t.Errorf("node id = %d, want 42", got)
			}
			if len(scope.atCalls) != 1 || scope.atCalls[0] != tc.want {
				t.Errorf("leaf calls = %+v, want exactly one %+v", scope.atCalls, tc.want)
			}
		})
	}
}

// A wrapped ref must go through the scope's own resolveRef — that is where the
// dialog containment check lives, so routing it anywhere else would let a
// dialog-scoped action reach outside the dialog.
func TestResolveWrapperRoutesRefsThroughTheScope(t *testing.T) {
	outside := errors.New("outside")
	scope := &stubScope{nodeID: 7, refErr: outside}

	_, err := resolveParsed(context.Background(), scope, selector.Parse("first:ref:e0"), nil, 0, false, false)

	if !errors.Is(err, outside) {
		t.Fatalf("err = %v, want the scope's own ref error", err)
	}
	if len(scope.refCalls) != 1 || scope.refCalls[0] != "e0" {
		t.Errorf("ref calls = %v, want exactly [e0]", scope.refCalls)
	}
	if len(scope.atCalls) != 0 {
		t.Errorf("a ref reached the leaf resolver: %+v", scope.atCalls)
	}
}

// last:/nth: cannot select among a single cached ref, and semantic selectors
// belong to the handler layer. Both rejections predate the shared recursion.
func TestResolveWrapperRejectionsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{"last:ref:e0", "ref selector cannot be used with last/nth"},
		{"nth:1:ref:e0", "ref selector cannot be used with last/nth"},
		{"first:find:Save", "semantic selectors must be resolved at the handler layer via /find"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			scope := &stubScope{nodeID: 7}

			_, err := resolveParsed(context.Background(), scope, selector.Parse(tc.raw), nil, 0, false, false)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if len(scope.atCalls) != 0 || len(scope.refCalls) != 0 {
				t.Errorf("rejected selector still reached the scope: at=%+v ref=%v", scope.atCalls, scope.refCalls)
			}
		})
	}
}

// The two scopes are deliberately NOT interchangeable on refs. frameScope hands
// back whatever the cache holds; nodeScope proves containment first. This pins
// the half that needs no browser — the containment half is pinned against a real
// dialog in the modal test.
func TestBothScopesRefuseAZeroBackendNodeIDRef(t *testing.T) {
	sel := selector.Parse("ref:e0")

	cache := &RefCache{Targets: map[string]RefTarget{"e0": {BackendNodeID: 99}}}
	got, err := (frameScope{}).resolveRef(context.Background(), sel, cache)
	if err != nil || got != 99 {
		t.Fatalf("frame scope ref = (%d, %v), want (99, nil)", got, err)
	}

	if _, err := (frameScope{}).resolveRef(context.Background(), sel, nil); !errors.Is(err, ErrSelectorNoMatch) {
		t.Errorf("frame scope with no cache = %v, want ErrSelectorNoMatch", err)
	}
	if _, err := (nodeScope{backendNodeID: 1}).resolveRef(context.Background(), sel, nil); !errors.Is(err, ErrSelectorNoMatch) {
		t.Errorf("node scope with no cache = %v, want ErrSelectorNoMatch", err)
	}

	// The two scopes used to disagree here: a zero backend node id was a
	// successful resolution to node 0 in the frame scope and a not-in-cache error
	// in the node scope. Zero is not a node, so both now refuse it, and the rule
	// lives in RefCache.Lookup rather than in either resolver.
	//
	// Both maps are driven, because a cache built from only one of them cannot
	// tell which branch of Lookup enforces the rule. The ghost-chrome static
	// snapshot route writes a zero into BOTH, so fixing only the Refs fallback
	// would leave the real-world case resolving to node 0 through Targets.
	for _, tc := range []struct {
		name  string
		cache *RefCache
	}{
		{name: "zero in Targets", cache: &RefCache{Targets: map[string]RefTarget{"e0": {BackendNodeID: 0}}}},
		{name: "zero in Refs", cache: &RefCache{Refs: map[string]int64{"e0": 0}}},
		{
			name: "zero in both, as the ghost-chrome static route writes it",
			cache: &RefCache{
				Refs:    map[string]int64{"e0": 0},
				Targets: map[string]RefTarget{"e0": {}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := tc.cache.Lookup("e0"); ok {
				t.Errorf("Lookup reported success for a zero backend node id")
			}
			if got, err := (frameScope{}).resolveRef(context.Background(), sel, tc.cache); !errors.Is(err, ErrSelectorNoMatch) {
				t.Errorf("frame scope zero-id ref = (%d, %v), want ErrSelectorNoMatch", got, err)
			}
			if _, err := (nodeScope{backendNodeID: 1}).resolveRef(context.Background(), sel, tc.cache); !errors.Is(err, ErrSelectorNoMatch) {
				t.Errorf("node scope zero-id ref = %v, want ErrSelectorNoMatch", err)
			}
		})
	}
}

// A ref that later gains a real node id must resolve again: the ghost-chrome
// escalation remap is what turns a static zero into a live Chrome node, so making
// zero unresolvable must not make the ref permanently dead.
func TestARefRemappedFromZeroToALiveNodeResolvesAgain(t *testing.T) {
	sel := selector.Parse("ref:e0")
	cache := &RefCache{
		Refs:    map[string]int64{"e0": 0},
		Targets: map[string]RefTarget{"e0": {}},
	}
	if _, err := (frameScope{}).resolveRef(context.Background(), sel, cache); !errors.Is(err, ErrSelectorNoMatch) {
		t.Fatalf("precondition: the static ref must start unresolvable, got %v", err)
	}

	cache.Refs["e0"] = 77
	cache.Targets["e0"] = RefTarget{BackendNodeID: 77}

	got, err := (frameScope{}).resolveRef(context.Background(), sel, cache)
	if err != nil || got != 77 {
		t.Errorf("after escalation, frame scope ref = (%d, %v), want (77, nil)", got, err)
	}
}

// The stub table above proves what the recursion asks of a scope, but both real
// scopes answer in the same shape, so it is blind to WHICH scope a wrapper form
// is given. That is the one mistake this refactor made cheap: the scope is now a
// value at two call sites, so handing the node entry a frameScope is a one-token
// edit. This is the browser-backed guard for it — the dialog twin must win over
// the background twins that bracket it in document order, which is exactly what
// a frame-rooted search would return instead.
func TestDialogScopedWrapperFormsStayInsideTheScope(t *testing.T) {
	ctx := newScopedWrapperFixture(t)

	modalNodeID, open, err := TopmostModalNodeID(ctx, "")
	if err != nil || !open {
		t.Fatalf("topmost dialog = (%d, %v, %v), want a visible dialog", modalNodeID, open, err)
	}

	for _, tc := range []struct {
		selector string
		wantID   string
	}{
		{selector: "first:text:Save", wantID: "dialog-first"},
		{selector: "last:text:Save", wantID: "dialog-last"},
		{selector: "nth:1:text:Save", wantID: "dialog-last"},
		{selector: "first:css:button", wantID: "dialog-first"},
		{selector: "last:css:button", wantID: "dialog-last"},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			nodeID, err := ResolveUnifiedSelectorWithinNode(ctx, selector.Parse(tc.selector), nil, modalNodeID)
			if err != nil {
				t.Fatalf("resolve %s within the dialog: %v", tc.selector, err)
			}
			var gotID string
			if err := callFunctionOnNodeForTest(ctx, nodeID, `function() { return this.id; }`, &gotID); err != nil {
				t.Fatal(err)
			}
			if gotID != tc.wantID {
				t.Errorf("%s resolved %q, want %q — the wrapper form escaped the dialog scope", tc.selector, gotID, tc.wantID)
			}
		})
	}
}

// The two guards below are browser-backed, so they skip where the machine has no
// browser or the lightweight opt-out is set — the same condition under which the
// pre-existing dialog containment test skips, since both go through
// testbrowser.Path. Neither adds a skip guard of its own. In a skipping
// environment nothing else pins WHICH scope each entry point hands the wrapper
// recursion: the stub table is blind to it by construction. This is the backstop
// for that environment — it needs no browser and it names the one-token swap.
func TestWrapperArmsAreWiredToTheirOwnScope(t *testing.T) {
	pkg := srccensus.Load(t, ".", 70)
	fset, _ := pkg.Syntax()

	// The property, not the argument spelling: each entry point must hand its
	// wrapper arm ITS OWN scope and never mention the other. Pinning the full call
	// text made this red for any signature change — which is what happened when
	// resolveWrapper and its duplicate wrapper arms collapsed into resolveParsed.
	const dispatch = "resolveParsed(ctx, "

	entries := []struct {
		entry string
		want  string
		other string
	}{
		{
			entry: "func ResolveUnifiedSelectorInFrame(",
			want:  "frameScope{frameID}",
			other: "nodeScope{",
		},
		{
			entry: "func ResolveUnifiedSelectorWithinNode(",
			want:  "nodeScope{scopeBackendNodeID}",
			other: "frameScope{",
		},
	}

	// The census is by OWNER rather than by call count, because resolveNested also
	// dispatches here — it is the parse-then-dispatch hop, and it passes the scope
	// it was given rather than constructing one. Any other function reaching
	// resolveParsed is an unguarded scope decision, which is how the swap this test
	// exists to catch comes back.
	allowed := map[string]bool{
		"ResolveUnifiedSelectorInFrame":    true,
		"ResolveUnifiedSelectorWithinNode": true,
		"resolveNested":                    true,
	}
	callers := map[string]bool{}
	for _, site := range pkg.CallsAllowingNone("resolveParsed") {
		callers[site.Func] = true
	}
	if len(callers) == 0 {
		t.Fatalf("nothing in %s calls %s — this guard would pass vacuously; if the dispatcher was renamed, pin the same property against its replacement rather than deleting this test", pkg.Dir(), dispatch)
	}
	for name := range callers {
		if !allowed[name] {
			t.Errorf("%s dispatches to %s but is not a listed scope owner — add it to this guard's table so its scope is pinned too", name, dispatch)
		}
	}
	for name := range allowed {
		if !callers[name] {
			t.Errorf("%s no longer dispatches to %s; this guard's table is stale", name, dispatch)
		}
	}

	for _, tc := range entries {
		name := strings.TrimSuffix(strings.TrimPrefix(tc.entry, "func "), "(")
		decl := wrapperEntryDecl(t, pkg, name)
		var dispatches []string
		for _, call := range resolveParsedCalls(decl) {
			if len(call.Args) >= 2 {
				dispatches = append(dispatches, "resolveParsed("+renderExpr(t, fset, call.Args[0])+", "+renderExpr(t, fset, call.Args[1]))
			}
		}
		if !anyHasPrefix(dispatches, dispatch+tc.want) {
			t.Errorf("%s no longer hands its wrapper arm %s%s — a dialog-scoped first:/last:/nth: would search the wrong root", tc.entry, dispatch, tc.want)
		}
		if anyHasPrefix(dispatches, dispatch+tc.other) {
			t.Errorf("%s hands its wrapper arm a %s: the scopes are swapped", tc.entry, tc.other)
		}
		// The starting index/fromEnd is the other half of the wrapper wiring, and it
		// is invisible to the stub table below: that table calls the dispatcher
		// directly, so it never reads what an ENTRY POINT passes. An entry point
		// starting at fromEnd=true makes every first: resolve like last:, which only
		// the browser-backed scope tests would notice — and they skip here.
		//
		// Read through the AST for the same reason the scope assertions above dropped
		// the full call text: a spelling-pinned version reddened when a parameter was
		// merely RENAMED, and this is the only browserless guard on this wiring — one
		// red for no defect is how it gets deleted. The last two arguments are what the
		// property is about, whatever the ones before them are called.
		assertWrapperArmStartsFresh(t, decl, name)
	}
}

// The counter-direction, on the same fixture and the same selectors: a frame-scoped
// wrapper must still see the whole frame, so it lands on the background twins that
// bracket the dialog. Without this the dialog assertions above could be satisfied by
// making EVERY scope behave like a dialog, which would break every frame-rooted
// action instead of fixing anything.
func TestFrameScopedWrapperFormsSeeTheWholeFrame(t *testing.T) {
	ctx := newScopedWrapperFixture(t)

	for _, tc := range []struct {
		selector string
		wantID   string
	}{
		{selector: "first:text:Save", wantID: "background-first"},
		{selector: "last:text:Save", wantID: "background-last"},
		{selector: "first:css:button", wantID: "background-first"},
		{selector: "last:css:button", wantID: "background-last"},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			nodeID, err := ResolveUnifiedSelectorInFrame(ctx, selector.Parse(tc.selector), nil, "")
			if err != nil {
				t.Fatalf("resolve %s in the frame: %v", tc.selector, err)
			}
			var gotID string
			if err := callFunctionOnNodeForTest(ctx, nodeID, `function() { return this.id; }`, &gotID); err != nil {
				t.Fatal(err)
			}
			if gotID != tc.wantID {
				t.Errorf("%s resolved %q, want %q — a frame-scoped wrapper must reach outside the dialog", tc.selector, gotID, tc.wantID)
			}
		})
	}
}

// Background twins bracket the dialog so that first: and last: each discriminate:
// a frame-rooted first: returns background-first and a frame-rooted last: returns
// background-last, while both correct answers are inside the dialog.
func newScopedWrapperFixture(t *testing.T) context.Context {
	t.Helper()
	ctx := newActionTab(t, 20*time.Second)

	html := `<style>[role=dialog] { position: fixed; inset: 20px; background: white; }</style>
	<button id="background-first">Save</button>
	<div id="dlg" role="dialog" aria-modal="true">
		<button id="dialog-first">Save</button>
		<button id="dialog-last">Save</button>
	</div>
	<button id="background-last">Save</button>`
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#dialog-first", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// assertWrapperArmStartsFresh checks, through the AST rather than the call's text, that
// every resolveParsed call inside the named entry point starts its wrapper at index 0,
// fromEnd false, positional false. A wrapper must derive its own index from the grammar;
// inheriting one from the entry point makes first: behave like last: on every page. The
// third value is the same shape of defect one step further: an entry point that starts
// positional=true makes a BARE text: index the document, silently dropping the control
// ranking that only the bare form carries.
func assertWrapperArmStartsFresh(t *testing.T, decl *ast.FuncDecl, name string) {
	t.Helper()

	calls := resolveParsedCalls(decl)
	for _, call := range calls {
		if len(call.Args) < 3 {
			t.Errorf("%s calls resolveParsed with %d arguments; the starting index, fromEnd and positional are the last three", name, len(call.Args))
			continue
		}
		index := exprText(call.Args[len(call.Args)-3])
		fromEnd := exprText(call.Args[len(call.Args)-2])
		positional := exprText(call.Args[len(call.Args)-1])
		if index != "0" || fromEnd != "false" || positional != "false" {
			t.Errorf("%s starts its wrapper arm at index %s, fromEnd %s, positional %s, want 0, false and false — a wrapper derives its own index from the grammar, and only a wrapper may claim to be positional", name, index, fromEnd, positional)
		}
	}
	if len(calls) == 0 {
		t.Errorf("%s no longer calls resolveParsed, so its wrapper wiring is unpinned", name)
	}
}

func wrapperEntryDecl(t *testing.T, pkg *srccensus.Package, name string) *ast.FuncDecl {
	t.Helper()
	decl, ok := funcDeclNamed(pkg, name)
	if !ok {
		t.Fatalf("%s is not declared in %s; if the entry point was renamed, pin the same property against its replacement rather than dropping it from this table", name, pkg.Dir())
	}
	return decl
}

func funcDeclNamed(pkg *srccensus.Package, name string) (*ast.FuncDecl, bool) {
	found, ok := pkg.Func(name)
	if !ok {
		return nil, false
	}
	_, files := pkg.Syntax()
	for _, decl := range files[found.File].Decls {
		if fn, isFunc := decl.(*ast.FuncDecl); isFunc && fn.Name.Name == name {
			return fn, true
		}
	}
	return nil, false
}

func resolveParsedCalls(decl *ast.FuncDecl) []*ast.CallExpr {
	var calls []*ast.CallExpr
	ast.Inspect(decl, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "resolveParsed" {
			calls = append(calls, call)
		}
		return true
	})
	return calls
}

func renderExpr(t *testing.T, fset *token.FileSet, expr ast.Expr) string {
	t.Helper()
	var out strings.Builder
	if err := printer.Fprint(&out, fset, expr); err != nil {
		t.Fatalf("cannot render %T, so this guard cannot read the dispatch: %v", expr, err)
	}
	return out.String()
}

func anyHasPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

// exprText renders an argument for comparison and for the failure message. Anything that
// is not a plain literal renders as its own syntax, which fails the comparison — an entry
// point threading a variable into the starting index is exactly the inheritance this
// guard rejects.
func exprText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Value
	case *ast.Ident:
		return e.Name
	default:
		return fmt.Sprintf("%T", expr)
	}
}
