package cdptk

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// moduleGoFileFloor is the vacuity floor for the module-wide walk below: a walk
// that stops seeing the tree would otherwise report a clean single world.
const moduleGoFileFloor = 400

type worldSite struct {
	file  string
	line  int
	value string
}

// worldNameExemptions are isolated worlds that are deliberately NOT the scope world,
// keyed by the constant that names them with the reason each cannot participate in the
// hazard this census guards.
//
// The rule the gate stated was "one world per frame". Measured against the tree that is
// not the rule that holds, and stating it absolutely while an exception stands would be
// dishonest: the rule is one world per frame AMONG HANDLE-PRODUCING RESOLVERS. The hazard
// is a Runtime.callFunctionOn given handles from two worlds, so a world whose execution
// context id never becomes an object handle cannot reach it, whatever frame it is in.
// TestAnExemptWorldCannotProduceObjectHandles checks that condition rather than trusting
// this note, because an exemption whose reason is only prose is one that outlives its
// reason.
var worldNameExemptions = map[string]struct{ file, why string }{
	"ScreencastRepaintWorldName": {
		file: "internal/cdptk/screencast.go",
		why:  "a repaint-forcing world with its own start/stop lifecycle inside the screencast loop. Its context id is consumed only by runtime.Evaluate().WithContextID, never as a callFunctionOn object handle, so no handle from it can meet a scope-world handle. It is NOT routed through IsolatedContextID on purpose: sharing the scope world would put the repaint loop's injected JS state beside selector resolution's, coupling two lifecycles to remove a hazard it cannot reach",
	},
}

// Two isolated world names existed for one rule — a node scope here and a frame
// scope in the bridge — and nobody noticed the second arrive, which is the whole
// reason this census exists rather than a comment asking for one world.
//
// The rule is one world name in the whole module, named by a constant rather than
// spelled inline at the call, so a third cannot be added quietly: an inline
// literal at a second call site is exactly how the second one appeared.
//
// BOTH spellings are counted. The first version of this census read only the raw
// "Page.createIsolatedWorld" string passed to Target.Execute and a map key "worldName",
// and was blind to the chromedp BUILDER form — page.CreateIsolatedWorld(f).WithWorldName(n)
// — which is the spelling the module's other world already used. A guard whose stated
// purpose is to make the next world impossible to add quietly could not see the one that
// was already there.
func TestOnlyOneIsolatedWorldNameExists(t *testing.T) {
	var creates, names []worldSite

	for _, file := range srccensus.Tree(t, filepath.Join("..", ".."), moduleGoFileFloor) {
		fset := token.NewFileSet()
		parsed := srccensus.Parse(t, fset, file.Name, file.Text, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.BasicLit:
				if n.Kind == token.STRING && strings.Trim(n.Value, `"`) == "Page.createIsolatedWorld" {
					creates = append(creates, worldSite{file: file.Name, line: fset.Position(n.Pos()).Line})
				}
			case *ast.KeyValueExpr:
				key, ok := n.Key.(*ast.BasicLit)
				if !ok || key.Kind != token.STRING || strings.Trim(key.Value, `"`) != "worldName" {
					return true
				}
				names = append(names, worldNameSite(file.Name, fset.Position(n.Pos()).Line, n.Value))
			case *ast.CallExpr:
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				// page.CreateIsolatedWorld(frame) — the chromedp builder that mints it.
				if sel.Sel.Name == "CreateIsolatedWorld" {
					creates = append(creates, worldSite{file: file.Name, line: fset.Position(n.Pos()).Line})
				}
				// .WithWorldName(name) — the builder that names it.
				if sel.Sel.Name == "WithWorldName" && len(n.Args) == 1 {
					names = append(names, worldNameSite(file.Name, fset.Position(n.Pos()).Line, n.Args[0]))
				}
			}
			return true
		})
	}

	if len(creates) == 0 || len(names) == 0 {
		t.Fatalf("found %d isolated-world creation(s) and %d world-name argument(s) in the whole module; the census has nothing to guard and would pass vacuously — re-point it at whatever mints the isolated world now rather than deleting it", len(creates), len(names))
	}

	const why = "One world per frame is the rule among handle-producing resolvers: Page.createIsolatedWorld keys on frame and name, so a second name means two worlds in the same frame, and a handle from one is not usable in a Runtime.callFunctionOn with a handle from the other. Nothing in the code says which world a handle came from, and the failure is silent. Mint every isolated context through cdptk.IsolatedContextID, which takes the frame as a parameter — or, if the new world genuinely never yields an object handle, add it to worldNameExemptions with that reason."

	exempt := map[string]bool{}
	for _, site := range names {
		if _, ok := worldNameExemptions[site.value]; ok {
			exempt[site.value] = true
		}
	}

	if got := len(creates) - len(exempt); got != 1 {
		t.Errorf("isolated worlds are created at %d site(s) (%v) with %d exempt, want exactly one unexempted. %s", len(creates), describeWorldSites(creates), len(exempt), why)
	}
	for _, site := range names {
		if _, ok := worldNameExemptions[site.value]; ok {
			continue
		}
		if site.value != "isolatedWorldName" {
			t.Errorf("%s:%d names an isolated world %s rather than the isolatedWorldName constant; a name spelled at the call site is how the second world arrived. %s", site.file, site.line, site.value, why)
		}
	}

	// Both directions on the exemption table: a stale entry naming a world that no longer
	// exists must fail too, or the reason outlives the thing it excused.
	for name, exemption := range worldNameExemptions {
		if !exempt[name] {
			t.Errorf("%s is exempted (%s) but no longer names an isolated world anywhere; drop the entry rather than leaving a reason with nothing to excuse", name, exemption.file)
		}
	}
}

func worldNameSite(file string, line int, value ast.Expr) worldSite {
	site := worldSite{file: file, line: line}
	switch v := value.(type) {
	case *ast.BasicLit:
		site.value = "inline literal " + v.Value
	case *ast.Ident:
		site.value = v.Name
	case *ast.SelectorExpr:
		site.value = v.Sel.Name
	default:
		site.value = "a computed expression"
	}
	return site
}

// The condition every exemption rests on, checked rather than promised: an exempt world
// is excused because its context id never becomes a callFunctionOn object handle, so the
// file that mints it must not produce one. If a repaint loop ever starts resolving nodes,
// the exemption's reason stops being true and this reds before the silent cross-world
// call can be written.
func TestAnExemptWorldCannotProduceObjectHandles(t *testing.T) {
	if len(worldNameExemptions) == 0 {
		t.Skip("no exemptions to check")
	}

	for name, exemption := range worldNameExemptions {
		found := false
		for _, file := range srccensus.Tree(t, filepath.Join("..", ".."), moduleGoFileFloor) {
			if filepath.ToSlash(file.Name) != exemption.file {
				continue
			}
			found = true
			for _, banned := range []string{"CallFunctionOn", "callFunctionOn"} {
				if strings.Contains(file.Text, banned) {
					t.Errorf("%s mints the exempt world %s and also calls %s; the exemption's whole reason is that its context id never becomes an object handle, so either stop producing handles there or route it through cdptk.IsolatedContextID", exemption.file, name, banned)
				}
			}
		}
		if !found {
			t.Errorf("%s is exempted at %s but that file is not in the module walk; fix the path or drop the exemption, since an entry pointing nowhere checks nothing", name, exemption.file)
		}
	}
}

func describeWorldSites(sites []worldSite) []string {
	out := make([]string, 0, len(sites))
	for _, site := range sites {
		out = append(out, filepath.ToSlash(site.file)+":"+itoa(site.line))
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// The frame a caller names must be the frame the world is created for. Merging the
// two worlds is easiest to get wrong here: hand the owner an empty frame and every
// frame-scoped evaluation silently moves to the top frame's document, which reads
// as a selector that stopped matching rather than as a scope bug.
func TestTheNamedFrameReachesTheWorldCreation(t *testing.T) {
	pkg := srccensus.Load(t, ".", 5)
	_, files := pkg.Syntax()
	var owner *ast.FuncDecl
	for _, file := range files {
		if fn := findFuncDecl(file, "IsolatedContextID"); fn != nil {
			owner = fn
		}
	}
	if owner == nil {
		t.Fatalf("IsolatedContextID is gone from %s; re-point this guard at whatever mints the isolated world rather than deleting it", pkg.Dir())
	}
	param := owner.Type.Params.List[len(owner.Type.Params.List)-1].Names[0].Name

	found := false
	ast.Inspect(owner, func(node ast.Node) bool {
		kv, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, isLit := kv.Key.(*ast.BasicLit)
		if !isLit || strings.Trim(key.Value, `"`) != "frameId" {
			return true
		}
		ident, isIdent := kv.Value.(*ast.Ident)
		if !isIdent || ident.Name != param {
			t.Errorf("frameId is passed as %s, not the %s parameter — the frame the caller asked for must be the frame the world is created for", exprText(kv.Value), param)
			return true
		}
		found = true
		return true
	})
	if !found {
		t.Errorf("no frameId argument in IsolatedContextID carries its %s parameter, so a caller naming a frame cannot be sure the world belongs to it", param)
	}
}

func findFuncDecl(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

func exprText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.BasicLit:
		return e.Value
	case *ast.CallExpr:
		return exprText(e.Fun) + "(...)"
	case *ast.SelectorExpr:
		return exprText(e.X) + "." + e.Sel.Name
	}
	return "a computed expression"
}
