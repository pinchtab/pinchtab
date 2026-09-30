package autosolver

import (
	"go/ast"
	"go/importer"
	"go/types"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// intentTypeReadCensus reports every read of an Intent's Type field in the
// package's production sources, keyed by the function that performs it. It type-
// checks rather than grepping: the defect this guards was three bare reads on a
// variable named something else, which a census keyed on a spelling walks past.
func intentTypeReadCensus(t *testing.T) (violations []string, allowed int) {
	t.Helper()

	fset, parsed := srccensus.Load(t, ".", 5).Syntax()
	files := make([]*ast.File, 0, len(parsed))
	for _, file := range parsed {
		files = append(files, file)
	}

	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	if _, err := conf.Check("github.com/pinchtab/pinchtab/internal/autosolver", fset, files, info); err != nil {
		t.Fatalf("type check: %v", err)
	}

	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				sel, ok := node.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Type" || !isIntentExpr(info, sel.X) {
					return true
				}
				if fn.Name.Name == "intentTypeOf" {
					allowed++
					return true
				}
				violations = append(violations, fset.Position(sel.Pos()).String()+" in "+fn.Name.Name)
				return true
			})
		}
	}
	return violations, allowed
}

func isIntentExpr(info *types.Info, expr ast.Expr) bool {
	typ := info.Types[expr].Type
	if typ == nil {
		return false
	}
	if ptr, ok := typ.Underlying().(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	named, ok := typ.(*types.Named)
	return ok && named.Obj() != nil && named.Obj().Name() == "Intent"
}

// A SemanticEngine may return (nil, nil), so intentTypeOf exists to answer
// IntentUnknown for a nil intent. It only holds while it is the ONLY place that
// touches the field: the original fix routed one dereference in trySemantic
// through it and left three in Solve, because the check was a grep for one
// variable name. This pins the meaning instead — no function but intentTypeOf
// reads an Intent's Type, whatever the variable is called.
func TestIntentTypeIsReadOnlyThroughIntentTypeOf(t *testing.T) {
	violations, allowed := intentTypeReadCensus(t)

	for _, violation := range violations {
		t.Errorf("bare Intent.Type read at %s; call intentTypeOf(intent) instead — a nil intent panics here", violation)
	}
	if allowed == 0 {
		t.Error("intentTypeOf no longer reads Intent.Type; if the field or the helper was renamed, this census now bans nothing")
	}
}
