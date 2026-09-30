package runtime

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// The retention count is a config setting, and a setting that never reaches the deleter is
// decorative. The recovery path is the only caller of the quarantine hook and cannot be
// driven from a unit test — reaching it needs a provider that classifies a silent CDP drop
// and a real browser launch — so the wiring is pinned at the source instead: the call must
// hand the hook the configured count, in the argument position the hook reads it from.
//
// Receiver-name-blind on purpose: renaming the config variable must not red this, only
// dropping or replacing the value must.
func TestQuarantineHookReceivesTheConfiguredKeepCount(t *testing.T) {
	const field = "ProfileQuarantineKeep"

	fset, files := srccensus.Load(t, ".", 3).Syntax()

	calls := 0
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "QuarantineCorruptedProfile" {
				return true
			}
			calls++
			position := fset.Position(call.Pos())
			if len(call.Args) != 2 {
				t.Errorf("%s:%d calls QuarantineCorruptedProfile with %d arguments, want the profile dir and the keep count",
					filepath.Base(position.Filename), position.Line, len(call.Args))
				return true
			}
			keep, ok := call.Args[1].(*ast.SelectorExpr)
			if !ok || keep.Sel.Name != field {
				t.Errorf("%s:%d passes %s as the keep count; it must pass the configured %s, or the setting changes nothing",
					filepath.Base(position.Filename), position.Line, exprText(fset, call.Args[1]), field)
			}
			return true
		})
	}

	if calls == 0 {
		t.Fatalf("no call to QuarantineCorruptedProfile in this package; the guard has nothing to check — re-point it at whatever quarantines a profile now rather than deleting it")
	}
}

func exprText(fset *token.FileSet, expr ast.Expr) string {
	var out strings.Builder
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		out.WriteString(exprText(fset, e.X))
		out.WriteString("." + e.Sel.Name)
	case *ast.Ident:
		out.WriteString(e.Name)
	case *ast.BasicLit:
		out.WriteString(e.Value)
	default:
		out.WriteString(fset.Position(expr.Pos()).String())
	}
	return out.String()
}
