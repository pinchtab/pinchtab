package mcp

import (
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

var routingHelpers = map[string]bool{
	"routedPath":         true,
	"routedPathWithBody": true,
	"routedQuery":        true,
}

// packageCallGraph maps every package-local function to the package-local
// functions it calls, closures included, so reachability can be asked of a
// handler that routes through a shared helper rather than calling the routing
// helper itself.
func packageCallGraph(t *testing.T) map[string]map[string]bool {
	t.Helper()
	_, files := srccensus.Load(t, ".", 18).Syntax()

	calls := map[string]map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			callee := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if ident, ok := call.Fun.(*ast.Ident); ok {
					callee[ident.Name] = true
				}
				return true
			})
			calls[fn.Name.Name] = callee
		}
	}
	if len(calls) == 0 {
		t.Fatal("call graph is empty; the parse matched no functions and this guard would pass vacuously")
	}
	return calls
}

func reachesRouting(calls map[string]map[string]bool, start string) bool {
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(fn string) bool {
		if seen[fn] {
			return false
		}
		seen[fn] = true
		for callee := range calls[fn] {
			if routingHelpers[callee] || walk(callee) {
				return true
			}
		}
		return false
	}
	return walk(start)
}

// toolHandlers reads the tool→handler pairs off rawHandlerMap's source, so the
// guard cannot drift from the dispatch table it is asking about.
func toolHandlers(t *testing.T) map[string]string {
	t.Helper()
	pkg := srccensus.Load(t, ".", 18)
	var registry *ast.FuncDecl
	_, files := pkg.Syntax()
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "rawHandlerMap" && fn.Body != nil {
				registry = fn
			}
		}
	}
	if registry == nil {
		t.Fatalf("no rawHandlerMap function found in %s, so this guard has no dispatch table to read", pkg.Dir())
	}

	pairs := map[string]string{}
	ast.Inspect(registry.Body, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.BasicLit)
		if !ok || key.Kind != token.STRING {
			return true
		}
		name, err := strconv.Unquote(key.Value)
		if err != nil {
			return true
		}
		call, ok := kv.Value.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok {
			pairs[name] = ident.Name
		}
		return true
	})

	if len(pairs) == 0 {
		t.Fatal("found no tool→handler pairs in rawHandlerMap; a rename has made this guard vacuous")
	}
	return pairs
}

func toolsDeclaringBrowser(t *testing.T) map[string]bool {
	t.Helper()
	declared := map[string]bool{}
	for _, tool := range allTools() {
		if _, ok := tool.InputSchema.Properties["browser"]; ok {
			declared[tool.Name] = true
		}
	}
	return declared
}

func TestEveryToolWhoseHandlerRoutesDeclaresBrowser(t *testing.T) {
	calls := packageCallGraph(t)
	handlers := toolHandlers(t)
	declared := toolsDeclaringBrowser(t)

	var routed, missing []string
	for tool, handler := range handlers {
		if !reachesRouting(calls, handler) {
			continue
		}
		routed = append(routed, tool)
		if !declared[tool] {
			missing = append(missing, tool+" (via "+handler+")")
		}
	}
	sort.Strings(routed)
	sort.Strings(missing)

	if len(routed) < 10 {
		t.Fatalf("only %d tools reach a routing helper (%v); the reachability walk has stopped following the call graph and this guard would pass while checking almost nothing", len(routed), routed)
	}
	if len(missing) > 0 {
		t.Errorf("%d tool(s) route on browser but never declare it, so an agent on a multi-browser server cannot aim them anywhere but the default and gets no signal why:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

func TestNoParameterDescriptionIsWrittenTwice(t *testing.T) {
	_, files := srccensus.Load(t, ".", 18).Syntax()

	seen := map[string]int{}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Description" || len(call.Args) != 1 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			seen[text]++
			return true
		})
	}

	if len(seen) == 0 {
		t.Fatal("found no description literals in the mcp package; the scan matched nothing and this guard is vacuous")
	}

	var repeated []string
	for text, count := range seen {
		if count > 1 {
			repeated = append(repeated, strconv.Itoa(count)+"x "+strconv.Quote(text))
		}
	}
	sort.Strings(repeated)
	if len(repeated) > 0 {
		t.Errorf("%d description string(s) are written more than once across the mcp package; give each a shared constructor in tools_params.go so a reword is one edit with a uniform effect:\n  %s",
			len(repeated), strings.Join(repeated, "\n  "))
	}
}

func TestSharedParamsHaveOneWordingPerConcept(t *testing.T) {
	wordings := map[string]map[string]bool{}
	for _, tool := range allTools() {
		for name, value := range tool.InputSchema.Properties {
			fields, ok := value.(map[string]any)
			if !ok {
				continue
			}
			text, _ := fields["description"].(string)
			if wordings[name] == nil {
				wordings[name] = map[string]bool{}
			}
			wordings[name][text] = true
		}
	}

	// tabId keeps a second, required wording (network route/unroute target one
	// tab by contract) and a third for the clear-all semantics, so it is asked
	// for the count it should have rather than for one.
	for _, tc := range []struct {
		param string
		want  int
	}{
		{"ref", 1},
		{"selector", 7},
		{"browser", 2},
		{"tabId", 3},
	} {
		if got := len(wordings[tc.param]); got != tc.want {
			var texts []string
			for text := range wordings[tc.param] {
				texts = append(texts, strconv.Quote(text))
			}
			sort.Strings(texts)
			t.Errorf("%q has %d distinct wordings across the tool set, want %d; an agent reading the tool list gets inconsistent guidance for one concept:\n  %s",
				tc.param, got, tc.want, strings.Join(texts, "\n  "))
		}
	}
}

// Collapsing a tool family moves per-variant capability into one schema, so a
// parameter can be dropped without any handler losing a caller — the agent
// simply stops being able to discover it. These are the unions the replaced
// tools carried, as promised by the migration table in RELEASE.md.
var collapsedToolCapabilities = map[string][]string{
	"pinchtab_wait":   {"for", "value", "state", "timeout", "tabId", "browser"},
	"pinchtab_key":    {"action", "key", "text", "nodeId", "tabId", "browser"},
	"pinchtab_record": {"action", "file", "fps", "quality", "scale", "tabId"},
}

func TestCollapsedToolsStillDeclareEveryReplacedCapability(t *testing.T) {
	declared := map[string]map[string]bool{}
	for _, tool := range allTools() {
		want, ok := collapsedToolCapabilities[tool.Name]
		if !ok {
			continue
		}
		props := map[string]bool{}
		for name := range tool.InputSchema.Properties {
			props[name] = true
		}
		if len(props) == 0 {
			t.Fatalf("%s declares no parameters at all; this guard would pass vacuously", tool.Name)
		}
		declared[tool.Name] = props
		for _, param := range want {
			if !props[param] {
				t.Errorf("%s no longer declares %q; the tools it replaced carried it, and an MCP client validates arguments against the declared schema, so the capability becomes undiscoverable rather than merely undocumented",
					tool.Name, param)
			}
		}
	}
	for name := range collapsedToolCapabilities {
		if _, ok := declared[name]; !ok {
			t.Errorf("%s is missing from allTools(); the consolidated tool that replaced a whole family is gone", name)
		}
	}
}
