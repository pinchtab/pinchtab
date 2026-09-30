package mcp

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// consumedElsewhere records the payload keys the bridge ACTION never reads because another
// layer owns them, with that owner. They are the reason this census can be strict about the
// rest: everything not listed here has to be read by the action it is sent to.
var consumedElsewhere = map[string]string{
	"tabId":        "resolved into a tab context by the handler before dispatch",
	"nodeId":       "resolved from selector/ref by the handler; the action reads NodeID it sets",
	"selector":     "normalised and resolved to a node by the handler",
	"waitNav":      "the handler waits for navigation around the action",
	"dialogAction": "the handler arms the dialog watcher around the action",
	"dialogText":   "the handler arms the dialog watcher around the action",
	"vocab":        "the handler's vocabulary-supersession gate reads it before dispatch; no action reads it",
}

// A payload key that is a real ActionRequest field but not one the target action READS is
// invisible to every structural check: the key is spelled correctly, the field exists, and
// the write silently does nothing. That is how fill posted its text under "value" — the
// field select reads — and answered filled:true with len:0. So the contract this pins is
// key-to-CONSUMING-ACTION, not key-to-struct.
func TestEveryMCPActionPayloadKeyIsReadByTheActionItIsSentTo(t *testing.T) {
	keysByKind := mcpPayloadKeysByKind(t)
	if len(keysByKind) < 5 {
		t.Fatalf("found payload keys for only %d action kinds, so this census is not reading the handler", len(keysByKind))
	}
	fieldByKey := actionRequestFieldsByJSONKey()
	readsByKind := actionRequestReadsByKind(t)
	if !readsByKind[bridge.ActionFill]["Text"] {
		t.Fatalf("the fill action is not seen reading Text, so the bridge side of this census read nothing useful: %v", readsByKind[bridge.ActionFill])
	}

	checked := 0
	for _, kind := range sortedKeys(keysByKind) {
		entry := keysByKind[kind]
		for _, key := range entry.keys {
			if _, ok := consumedElsewhere[key]; ok {
				continue
			}
			field, ok := fieldByKey[key]
			if !ok {
				t.Errorf("%s posts %q, which is not a field of bridge.ActionRequest at all", kind, key)
				continue
			}
			checked++
			read := readByAnyDispatchableKind(readsByKind, entry.dispatchesTo, field)
			reason, accounted := postedButUnread[kind+"/"+key]
			switch {
			case read && accounted:
				t.Errorf("%s posts %q and the action now reads ActionRequest.%s; drop its entry so this census keeps meaning what it says", kind, key, field)
			case read:
			case accounted:
				t.Logf("accounted: %s posts %q (ActionRequest.%s) unread — %s", kind, key, field, reason)
			default:
				t.Errorf("%s posts %q (ActionRequest.%s), which no action it dispatches to (%v) reads; the write would be silently dropped",
					kind, key, field, entry.dispatchesTo)
			}
		}
	}
	if checked < 5 {
		t.Fatalf("only %d payload keys were checked against a consuming action, so this census proves little", checked)
	}

	for key := range consumedElsewhere {
		if !anyKindPosts(keysByKind, key) {
			t.Errorf("%q is recorded as consumed by another layer but nothing posts it any more; drop the entry", key)
		}
	}
}

type payloadCase struct {
	keys []string
	// dispatchesTo is the kind itself plus any kind the handler rewrites payload["kind"]
	// to, since the scroll case turns itself into mouse-wheel for the wheel form.
	dispatchesTo []string
}

// gatedByDeclaringSet covers the keys whose per-kind gate the walker cannot see, because the
// handler computes a local flag under the gate and writes the key under the flag. Naming the
// SET rather than the kinds keeps it derived: shrink xyAction and this narrows with it.
var gatedByDeclaringSet = map[string]string{
	"x":     "xyAction",
	"y":     "xyAction",
	"hasXY": "xyAction",
}

// postedButUnread records keys a tool forwards that its action does not read — the same
// class as this card's defect, found by this census and filed separately rather than fixed
// here. Each entry names the card, and it reds again the moment the action starts reading.
var postedButUnread = map[string]string{}

// declaringSets are the per-kind argument gates this handler uses, read from the package
// itself so the census follows them: a key inside `if humanizeAction[kind]` belongs to the
// kinds that map declares and to no others. A gate this test does not know about is a
// failure, not a silent widening.
var declaringSets = map[string]map[string]bool{
	"humanizeAction": humanizeAction,
	"xyAction":       xyAction,
}

// mcpPayloadKeysByKind reads the handler rather than a hand-written list: a tool that starts
// posting a new key has to be measured, and a list would simply not mention it. Keys set
// before the switch belong to every kind the tool set can send, which is itself derived from
// the handler registration.
func mcpPayloadKeysByKind(t *testing.T) map[string]payloadCase {
	t.Helper()

	pkg := srccensus.Load(t, ".", 18)
	fn := findFuncDecl(t, pkg, "handleAction")
	keys := map[string]map[string]bool{}
	rewrites := map[string]map[string]bool{}
	collectPayloadKeys(t, fn.Body, mcpActionKinds(t, pkg), keys, rewrites)

	for key, setName := range gatedByDeclaringSet {
		set, known := declaringSets[setName]
		if !known {
			t.Fatalf("%q is recorded as gated by the unknown declaring set %q", key, setName)
		}
		for kind := range keys {
			if !set[kind] {
				delete(keys[kind], key)
			}
		}
	}

	out := map[string]payloadCase{}
	for kind, set := range keys {
		entry := payloadCase{dispatchesTo: []string{kind}}
		for key := range set {
			entry.keys = append(entry.keys, key)
		}
		for rewritten := range rewrites[kind] {
			if rewritten != kind {
				entry.dispatchesTo = append(entry.dispatchesTo, rewritten)
			}
		}
		sort.Strings(entry.keys)
		sort.Strings(entry.dispatchesTo)
		out[kind] = entry
	}
	return out
}

// mcpActionKinds is the set of kinds handleAction is instantiated with, taken from the tool
// registration so a new action tool joins this census by existing.
func mcpActionKinds(t *testing.T, pkg *srccensus.Package) []string {
	t.Helper()

	var kinds []string
	_, files := pkg.Syntax()
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isIdent(call.Fun, "handleAction") || len(call.Args) != 2 {
				return true
			}
			if kind, ok := stringLit(call.Args[1]); ok {
				kinds = append(kinds, kind)
			}
			return true
		})
	}
	if len(kinds) < 5 {
		t.Fatalf("found only %d action tools, so this census is not reading the tool registration", len(kinds))
	}
	sort.Strings(kinds)
	return kinds
}

// collectPayloadKeys walks the handler body carrying the kinds the current statement applies
// to. A `switch kind` case narrows it to that case's kinds, an `if kind == "x"` to x, and a
// declaring-set lookup to the kinds that set declares — which is what keeps `mode`, gated on
// click inside a clause shared with hover and focus, from being attributed to all three.
func collectPayloadKeys(t *testing.T, node ast.Node, kinds []string, keys, rewrites map[string]map[string]bool) {
	t.Helper()

	switch n := node.(type) {
	case nil:
		return
	case *ast.AssignStmt:
		for i, lhs := range n.Lhs {
			index, ok := lhs.(*ast.IndexExpr)
			if !ok || !isIdent(index.X, "payload") {
				continue
			}
			key, ok := stringLit(index.Index)
			if !ok {
				continue
			}
			for _, kind := range kinds {
				if key == "kind" {
					if i < len(n.Rhs) {
						if rewritten, ok := stringLit(n.Rhs[i]); ok {
							addTo(rewrites, kind, rewritten)
						}
					}
					continue
				}
				addTo(keys, kind, key)
			}
		}
	case *ast.IfStmt:
		body := kinds
		if restricted, ok := restrictKinds(t, n.Cond, kinds, assignsPayload(n.Body)); ok {
			body = restricted
		}
		collectPayloadKeys(t, n.Body, body, keys, rewrites)
		if n.Else != nil {
			collectPayloadKeys(t, n.Else, kinds, keys, rewrites)
		}
		return
	case *ast.SwitchStmt:
		if isIdent(n.Tag, "kind") {
			for _, stmt := range n.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				caseKinds := []string{}
				for _, expr := range clause.List {
					if kind, ok := stringLit(expr); ok && contains(kinds, kind) {
						caseKinds = append(caseKinds, kind)
					}
				}
				for _, stmt := range clause.Body {
					collectPayloadKeys(t, stmt, caseKinds, keys, rewrites)
				}
			}
			return
		}
	}

	for _, child := range childNodes(node) {
		collectPayloadKeys(t, child, kinds, keys, rewrites)
	}
}

// restrictKinds narrows the applicable kinds for a conditional: `kind == "x"` to x, and a
// declaring-set lookup to that set's members.
func restrictKinds(t *testing.T, cond ast.Expr, kinds []string, gatesPayload bool) ([]string, bool) {
	t.Helper()

	switch c := cond.(type) {
	case *ast.BinaryExpr:
		switch c.Op {
		case token.EQL:
			if isIdent(c.X, "kind") {
				if kind, ok := stringLit(c.Y); ok {
					return intersect(kinds, []string{kind}), true
				}
			}
		case token.LOR:
			left, okLeft := restrictKinds(t, c.X, kinds, gatesPayload)
			right, okRight := restrictKinds(t, c.Y, kinds, gatesPayload)
			if okLeft && okRight {
				return union(left, right), true
			}
		}
	case *ast.IndexExpr:
		name := calleeName(c.X)
		if !isIdent(c.Index, "kind") {
			return nil, false
		}
		set, known := declaringSets[name]
		if !known {
			if gatesPayload {
				t.Fatalf("payload keys are gated on the unknown declaring set %q; register it in declaringSets so this census keeps meaning what it says", name)
			}
			return nil, false
		}
		declared := []string{}
		for kind := range set {
			declared = append(declared, kind)
		}
		return intersect(kinds, declared), true
	case *ast.ParenExpr:
		return restrictKinds(t, c.X, kinds, gatesPayload)
	}
	return nil, false
}

// assignsPayload reports whether a subtree writes any payload key, so an unknown per-kind
// gate is a failure only when it actually decides one.
func assignsPayload(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if index, ok := lhs.(*ast.IndexExpr); ok && isIdent(index.X, "payload") {
				found = true
			}
		}
		return true
	})
	return found
}

func childNodes(node ast.Node) []ast.Node {
	var out []ast.Node
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil || n == node {
			return true
		}
		out = append(out, n)
		return false
	})
	return out
}

func addTo(m map[string]map[string]bool, outer, inner string) {
	if m[outer] == nil {
		m[outer] = map[string]bool{}
	}
	m[outer][inner] = true
}

func contains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}

func intersect(a, b []string) []string {
	out := []string{}
	for _, item := range a {
		if contains(b, item) {
			out = append(out, item)
		}
	}
	return out
}

func union(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, item := range b {
		if !contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

func findFuncDecl(t *testing.T, pkg *srccensus.Package, name string) *ast.FuncDecl {
	t.Helper()
	_, files := pkg.Syntax()
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name && fn.Body != nil {
				return fn
			}
		}
	}
	t.Fatalf("no %s function found in %s, so this census has nothing to read", name, pkg.Dir())
	return nil
}

func actionRequestFieldsByJSONKey() map[string]string {
	out := map[string]string{}
	typ := reflect.TypeOf(bridge.ActionRequest{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		out[tag] = field.Name
	}
	return out
}

// actionRequestReadsByKind answers, per action kind, which ActionRequest fields that kind's
// action reads — transitively, since actions delegate (humanized type, the pointer helpers)
// and pass the same request down. The kind-to-function mapping comes from the live registry,
// so rewiring an action moves the census with it.
func actionRequestReadsByKind(t *testing.T) map[string]map[string]bool {
	t.Helper()

	reads, calls := bridgeRequestReads(t)

	b := &bridge.Bridge{}
	b.InitActionRegistry()
	if len(b.Actions) == 0 {
		t.Fatal("the action registry is empty, so this census has no kinds to check")
	}

	out := map[string]map[string]bool{}
	for kind, fn := range b.Actions {
		name := methodName(fn)
		if name == "" {
			t.Fatalf("cannot resolve the function behind action %q", kind)
		}
		out[kind] = transitiveReads(name, reads, calls, map[string]bool{})
	}
	return out
}

func transitiveReads(name string, reads map[string]map[string]bool, calls map[string]map[string]bool, seen map[string]bool) map[string]bool {
	out := map[string]bool{}
	if seen[name] {
		return out
	}
	seen[name] = true
	for field := range reads[name] {
		out[field] = true
	}
	for callee := range calls[name] {
		for field := range transitiveReads(callee, reads, calls, seen) {
			out[field] = true
		}
	}
	return out
}

// bridgeRequestReads parses the bridge package for every function taking an ActionRequest,
// recording the fields it reads off that parameter and the functions it hands the whole
// request to.
func bridgeRequestReads(t *testing.T) (reads map[string]map[string]bool, calls map[string]map[string]bool) {
	t.Helper()

	reads = map[string]map[string]bool{}
	calls = map[string]map[string]bool{}

	_, files := srccensus.Load(t, filepath.Join("..", "bridge"), 70).Syntax()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		file := files[name]
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			param := actionRequestParamName(fn)
			if param == "" {
				continue
			}
			reads[fn.Name.Name] = map[string]bool{}
			calls[fn.Name.Name] = map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.SelectorExpr:
					if isIdent(node.X, param) {
						reads[fn.Name.Name][node.Sel.Name] = true
					}
				case *ast.CallExpr:
					for _, arg := range node.Args {
						if isRequestArg(arg, param) {
							calls[fn.Name.Name][calleeName(node.Fun)] = true
						}
					}
				}
				return true
			})
		}
	}
	return reads, calls
}

func actionRequestParamName(fn *ast.FuncDecl) string {
	if fn.Type.Params == nil {
		return ""
	}
	for _, field := range fn.Type.Params.List {
		if !isActionRequestType(field.Type) || len(field.Names) == 0 {
			continue
		}
		return field.Names[0].Name
	}
	return ""
}

func isActionRequestType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == "ActionRequest"
	case *ast.StarExpr:
		return isActionRequestType(t.X)
	}
	return false
}

func isRequestArg(arg ast.Expr, param string) bool {
	switch a := arg.(type) {
	case *ast.Ident:
		return a.Name == param
	case *ast.UnaryExpr:
		return isIdent(a.X, param)
	}
	return false
}

func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// methodName turns a bound method value from the registry back into its declared name, so
// the census follows the registry instead of a second list of action functions.
func methodName(fn bridge.ActionFunc) string {
	full := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	full = strings.TrimSuffix(full, "-fm")
	if at := strings.LastIndex(full, "."); at >= 0 {
		return full[at+1:]
	}
	return full
}

func readByAnyDispatchableKind(readsByKind map[string]map[string]bool, kinds []string, field string) bool {
	for _, kind := range kinds {
		if readsByKind[kind][field] {
			return true
		}
	}
	return false
}

func anyKindPosts(keysByKind map[string]payloadCase, key string) bool {
	for _, entry := range keysByKind {
		for _, posted := range entry.keys {
			if posted == key {
				return true
			}
		}
	}
	return false
}

func isIdent(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

func stringLit(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

func sortedKeys(m map[string]payloadCase) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
