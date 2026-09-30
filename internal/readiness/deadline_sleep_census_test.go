package readiness

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

const moduleGoFileFloor = 400

var deadlineSleepExemptions = map[string]string{
	"internal/cli/actions/actions_audit.go:waitForIsolatedInstance":          "CLI actions are claimed by the apiclient.Conn migration; port with it",
	"tests/tools/runner/internal/bench/setup.go:waitForFixturesReachable":    "benchmark runner tool, not server code",
	"tests/tools/runner/internal/bench/setup.go:waitForPinchtabHealth":       "benchmark runner tool, not server code",
	"tests/tools/runner/internal/e2e/provider.go:Runner.assertStealthStatus": "e2e runner tool, not server code",
}

func TestNoDeadlineLoopSleepsOutsideWaitUntil(t *testing.T) {
	var offenders []string
	seen := map[string]bool{}
	for _, file := range srccensus.Tree(t, "../..", moduleGoFileFloor) {
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file.Name, file.Text, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file.Name, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !computesDeadline(fn.Body) {
				continue
			}
			line, found := sleepInLoop(fset, fn.Body)
			if !found {
				continue
			}
			key := file.Name + ":" + funcName(fn)
			if _, exempt := deadlineSleepExemptions[key]; exempt {
				seen[key] = true
				continue
			}
			offenders = append(offenders, key+" (line "+strconv.Itoa(line)+")")
		}
	}
	for key := range deadlineSleepExemptions {
		if !seen[key] {
			t.Errorf("deadlineSleepExemptions names %s, which no longer sleeps in a deadline loop; delete the entry", key)
		}
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("deadline loops that time.Sleep ignore ctx and re-derive the poll contract; use readiness.WaitUntil, or add an exemption with its reason:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

func computesDeadline(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch {
		case isPkgCall(sel, "time", "Until"), sel.Sel.Name == "Deadline":
			found = true
		case sel.Sel.Name == "Add":
			if inner, ok := sel.X.(*ast.CallExpr); ok {
				if innerSel, ok := inner.Fun.(*ast.SelectorExpr); ok && isPkgCall(innerSel, "time", "Now") {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

func sleepInLoop(fset *token.FileSet, body *ast.BlockStmt) (int, bool) {
	line := 0
	ast.Inspect(body, func(n ast.Node) bool {
		if line > 0 {
			return false
		}
		var loopBody *ast.BlockStmt
		switch loop := n.(type) {
		case *ast.ForStmt:
			loopBody = loop.Body
		case *ast.RangeStmt:
			loopBody = loop.Body
		default:
			return true
		}
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok || line > 0 {
				return line == 0
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && isPkgCall(sel, "time", "Sleep") {
				line = fset.Position(call.Pos()).Line
			}
			return true
		})
		return line == 0
	})
	return line, line > 0
}

func isPkgCall(sel *ast.SelectorExpr, pkg, name string) bool {
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg && sel.Sel.Name == name
}

func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}
