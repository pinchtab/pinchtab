package config

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestFileConfigFromRuntimeSharesNothingWithTheRuntime(t *testing.T) {
	cfg := populatedRuntimeConfig(t)
	before := *cfg
	beforePatterns := append([]string(nil), cfg.IDPI.CustomPatterns...)
	beforeAllowHosts := append([]string(nil), cfg.AttachAllowHosts...)

	fc := FileConfigFromRuntime(cfg)
	*fc.Server.TrustProxyHeaders = !*fc.Server.TrustProxyHeaders
	fc.Security.IDPI.CustomPatterns[0] = "mutated"
	fc.Security.IDPI.Enabled = !fc.Security.IDPI.Enabled
	fc.Security.Attach.AllowHosts[0] = "mutated"
	*fc.Security.AllowEvaluate = !*fc.Security.AllowEvaluate
	*fc.MultiInstance.Restart.MaxRestarts++
	fc.Browser.ExtensionPaths[0] = "mutated"

	if cfg.TrustProxyHeaders != before.TrustProxyHeaders {
		t.Fatalf("Server.TrustProxyHeaders aliases the runtime field")
	}
	if cfg.IDPI.CustomPatterns[0] != beforePatterns[0] || cfg.IDPI.Enabled != before.IDPI.Enabled {
		t.Fatalf("Security.IDPI shares memory with the runtime: %+v", cfg.IDPI)
	}
	if cfg.AttachAllowHosts[0] != beforeAllowHosts[0] || cfg.AllowEvaluate != before.AllowEvaluate || cfg.RestartMaxRestarts != before.RestartMaxRestarts || cfg.ExtensionPaths[0] == "mutated" {
		t.Fatalf("a FileConfig write reached the runtime: %+v", cfg)
	}
}

func TestFileConfigFromRuntimeDelegatesToPerSectionBuilders(t *testing.T) {
	fset, funcs := packageFuncs(t)
	fn := declaredFunc(t, funcs, "FileConfigFromRuntime")
	lines := fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line
	if lines > 60 {
		t.Fatalf("FileConfigFromRuntime spans %d lines, want under 60 with the sections built by their own functions", lines)
	}
	for _, b := range []string{"serverConfigFromRuntime", "securityConfigFromRuntime", "instanceDefaultsFromRuntime"} {
		if !callsFunction(fn.Body, b) {
			t.Errorf("FileConfigFromRuntime does not delegate to %s", b)
		}
	}
}

func TestSectionBuildersNeverTakeTheAddressOfARuntimeField(t *testing.T) {
	fset, funcs := packageFuncs(t)
	checked := 0
	for _, fn := range reachableFrom(t, funcs, "FileConfigFromRuntime") {
		if fn.Type.Params.NumFields() != 1 || len(fn.Type.Params.List[0].Names) != 1 {
			continue
		}
		param := fn.Type.Params.List[0]
		star, ok := param.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if ident, ok := star.X.(*ast.Ident); !ok || ident.Name != "RuntimeConfig" {
			continue
		}
		checked++
		cfgName := param.Names[0].Name
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			u, ok := n.(*ast.UnaryExpr)
			if !ok || u.Op != token.AND {
				return true
			}
			if sel, ok := u.X.(*ast.SelectorExpr); ok {
				if root, ok := sel.X.(*ast.Ident); ok && root.Name == cfgName {
					t.Errorf("%s: %s takes the address of a runtime field; copy the value through ptr instead", fset.Position(u.Pos()), fn.Name.Name)
				}
			}
			return true
		})
	}
	if checked < 10 {
		t.Fatalf("checked %d runtime-taking builders, want the section builders plus FileConfigFromRuntime", checked)
	}
}

type funcIndex struct {
	dir   string
	decls map[string]*ast.FuncDecl
}

func packageFuncs(t *testing.T) (*token.FileSet, funcIndex) {
	t.Helper()
	pkg := srccensus.Load(t, ".", 25)
	fset, files := pkg.Syntax()
	funcs := map[string]*ast.FuncDecl{}
	for _, f := range files {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil {
				funcs[fn.Name.Name] = fn
			}
		}
	}
	return fset, funcIndex{dir: pkg.Dir(), decls: funcs}
}

func declaredFunc(t *testing.T, funcs funcIndex, name string) *ast.FuncDecl {
	t.Helper()
	fn := funcs.decls[name]
	if fn == nil {
		t.Fatalf("%s not found in %s", name, funcs.dir)
	}
	return fn
}

func reachableFrom(t *testing.T, funcs funcIndex, root string) []*ast.FuncDecl {
	t.Helper()
	declaredFunc(t, funcs, root)
	seen := map[string]bool{root: true}
	queue := []string{root}
	var reached []*ast.FuncDecl
	for len(queue) > 0 {
		fn := funcs.decls[queue[0]]
		queue = queue[1:]
		reached = append(reached, fn)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok && funcs.decls[id.Name] != nil && !seen[id.Name] {
				seen[id.Name] = true
				queue = append(queue, id.Name)
			}
			return true
		})
	}
	return reached
}

func callsFunction(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name {
				found = true
			}
		}
		return true
	})
	return found
}
