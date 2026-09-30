package cdpops

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The two spellings of "empty" that the world unification could have collapsed.
// Here, an unnamed frame means "the caller already has a context, do nothing" and
// the zero is what callers branch on; one layer down, in cdptk.IsolatedContextID,
// an unnamed frame means the TOP frame's isolated world. Handing this check to the
// owner would move every unscoped caller into a freshly minted world instead.
//
// No CDP is needed: the contract is that this answers before issuing a call, which
// is also why an unscoped caller costs nothing.
func TestAnUnnamedFrameIsANoOpRatherThanTheTopFrame(t *testing.T) {
	execID, err := FrameExecutionContextID(context.Background(), "")

	if err != nil {
		t.Fatalf("FrameExecutionContextID(ctx, \"\") = (%d, %v), want (0, nil): an unnamed frame must be a no-op the caller can branch on, not an error", execID, err)
	}
	if execID != 0 {
		t.Errorf("FrameExecutionContextID(ctx, \"\") = %d, want 0 — callers read the zero as \"use the context you already have\"; a real id here silently moves them into a minted world", execID)
	}
}

// The frame a caller names must be the frame handed to the world owner. This is a
// source guard because the behavioural half needs a browser: the reachable failure
// is passing "" (or another frame) to the owner, which returns the TOP frame's
// world, and every frame-scoped evaluation then reads the wrong document while
// still succeeding.
//
// Pinned through the AST by argument POSITION rather than by matching call text, so
// renaming the parameter keeps this green while dropping it reds.
func TestTheNamedFrameIsHandedToTheWorldOwner(t *testing.T) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "frame_dom.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var fn *ast.FuncDecl
	for _, decl := range parsed.Decls {
		if candidate, ok := decl.(*ast.FuncDecl); ok && candidate.Name.Name == "FrameExecutionContextID" {
			fn = candidate
		}
	}
	if fn == nil {
		t.Fatal("FrameExecutionContextID is gone from frame_dom.go; re-point this guard at whatever scopes an evaluation to a frame rather than deleting it")
	}
	params := fn.Type.Params.List
	frameParam := params[len(params)-1].Names[0].Name

	handedOver := false
	ast.Inspect(fn, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "IsolatedContextID" {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		last := call.Args[len(call.Args)-1]
		ident, isIdent := last.(*ast.Ident)
		if !isIdent || ident.Name != frameParam {
			t.Errorf("the world owner is called with %s in the frame position, not this function's %s parameter — a frame-scoped evaluation would then run in the top frame's document and still succeed", describeExpr(last), frameParam)
			return true
		}
		handedOver = true
		return true
	})

	if !handedOver {
		t.Errorf("FrameExecutionContextID never hands its %s parameter to IsolatedContextID; the frame the caller asked for has to reach the world creation, or scoping is silently ignored", frameParam)
	}
}

// The world must have exactly one owner: this package kept its own
// resolve-the-frame-then-create-a-world sequence, which is how a second world name
// came to exist for one rule.
func TestThisPackageMintsNoIsolatedWorldOfItsOwn(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	files := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		files++
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("cannot parse %s, so this guard would skip it silently: %v", path, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if strings.Trim(lit.Value, `"`) == "Page.createIsolatedWorld" {
				t.Errorf("%s:%d mints an isolated world in this package; the world has one owner in internal/cdptk so a frame cannot end up with two, and a handle from one world is unusable alongside a handle from the other", path, fset.Position(lit.Pos()).Line)
			}
			return true
		})
	}
	if files < 2 {
		t.Fatalf("scanned %d non-test file(s) in this package, fewer than this guard was written over; it would pass vacuously", files)
	}
}

func describeExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.BasicLit:
		return e.Value
	case *ast.CallExpr:
		return describeExpr(e.Fun) + "(...)"
	case *ast.SelectorExpr:
		return describeExpr(e.X) + "." + e.Sel.Name
	}
	return "a computed expression"
}

func TestADetachedNodeIsRefusedTheSameWayByEveryNodeAction(t *testing.T) {
	orig := resolveNodeAction
	t.Cleanup(func() { resolveNodeAction = orig })
	resolveNodeAction = func(context.Context, int64) (json.RawMessage, error) {
		return json.RawMessage(`{"object":{}}`), nil
	}

	const want = "element not found in DOM (backendNodeId=42)"
	actions := map[string]func(context.Context) error{
		"fill":     func(ctx context.Context) error { return FillByNodeID(ctx, 42, "v") },
		"select":   func(ctx context.Context) error { return SelectByNodeID(ctx, 42, "v") },
		"js click": func(ctx context.Context) error { return JSClickByBackendNode(ctx, 42) },
		"read value": func(ctx context.Context) error {
			_, err := ReadInputValue(ctx, 42)
			return err
		},
	}
	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			err := action(context.Background())
			if err == nil || err.Error() != want {
				t.Fatalf("%s on a detached node = %v, want %q", name, err, want)
			}
		})
	}
}
