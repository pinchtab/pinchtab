package mcp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// One rule, module-wide: a short read must never become a successful body. Every
// capped read is classified here, because the discriminator is not whether the
// call writes limit+1 — it is whether the truncated value is then consumed as if
// it were whole. This client capped at exactly the limit and handed a model a
// truncated base64 image as a success; the activity middleware capped the INBOUND
// body and substituted the peek for it, so an oversize action reached the handler
// mangled and PinchTab blamed the client for the JSON it had cut.
//
// Classification is keyed by SITE — file plus the enclosing function — not by file.
// A file-keyed verdict gave one classification to a whole file, so a SECOND capped
// read added to an already-classified file inherited the first's verdict and passed
// unseen: the exact truncate-and-consume shape this census exists to catch, leaking
// back through the guard written for it. Keying by site forces every new capped read
// to be classified on its own, and a second read landing in an already-classified
// FUNCTION is caught by the one-read-per-site rule below.
//
// The census is the record of the two permanent exclusions as well as the guard,
// so a later sweep meets their reasons where it would look rather than re-opening
// them from the outside.
const (
	refusesOverTheCap  = "reads limit+1 and refuses above it"
	peeksWithoutTaking = "peeks a prefix and leaves the stream whole for its consumer"
	unreachableCap     = "the cap cannot be reached by anything that passed the guard in front of it"
	loudOnTruncation   = "a truncated value cannot parse, so the request is refused rather than accepted"
)

var cappedReads = map[string]string{
	"internal/mcp/client.go::doWithHeaders":                       refusesOverTheCap,
	"internal/handlers/download_fetch.go::fetchDirectWithCookies": refusesOverTheCap,
	"internal/orchestrator/tab_extract.go::peekBodyStringField":   refusesOverTheCap,
	"internal/bridge/runtime/cdp_url.go::readCDPVersionBody":      refusesOverTheCap,
	"internal/activity/context.go::EnrichRouteActivity":           peeksWithoutTaking,
	// Permanent: isSmallJSON gates the read on Content-Length >= 0 && <= 64 KB
	// against the same 64 KB cap, so the limit is unreachable. That is a property
	// of the input, not a promise about future behaviour.
	"internal/proxy/proxy.go::Forward": unreachableCap,
	// Loud but misattributed, and left as it is: a truncated config PUT does not
	// parse, so it answers 400 rather than writing a partial config. The only cost
	// is that bad_config_json blames the client for a truncation PinchTab performed.
	"internal/dashboard/config_api.go::parseConfigUpdate": loudOnTruncation,
}

// cappedSite is one io.LimitReader call, attributed to the function that holds it —
// the attribution a file-scoped scan cannot make, and the whole point of this file.
type cappedSite struct {
	key  string
	file string
	line int
	fn   string
}

func TestEveryCappedReadIsClassified(t *testing.T) {
	sites := cappedReadSites(t)
	if len(sites) < len(cappedReads) {
		t.Fatalf("found %d capped-read sites for %d classified; the census is matching almost nothing and would pass vacuously", len(sites), len(cappedReads))
	}

	for _, s := range unclassifiedSites(sites, cappedReads) {
		t.Errorf("%s:%d (in %s) caps a read and is not classified; say which it is — %q, %q, or an exclusion with the reason the truncated value is never consumed as whole",
			s.file, s.line, s.fn, refusesOverTheCap, peeksWithoutTaking)
	}
	for _, key := range multiReadSites(sites) {
		t.Errorf("%s holds more than one capped read under a single classification; each read is a separate decision, so a second one here would inherit the first's verdict — give each read its own function or add a distinct entry", key)
	}
	for _, key := range staleClassifications(sites, cappedReads) {
		t.Errorf("%s is classified here and no longer caps a read; drop the entry rather than leaving the census guarding one site fewer than it claims", key)
	}

	// The two sites this rule fixed carry its own idiom, so a revert is visible here
	// as well as in the behaviour tests.
	for name, want := range map[string]string{
		"internal/mcp/client.go":       "MaxResponseBytes+1",
		"internal/activity/context.go": "io.MultiReader",
	} {
		if !fileText(t, name).Contains(want) {
			t.Errorf("%s no longer carries %q; the classification above says it refuses or replays, and it does neither", name, want)
		}
	}
}

// TestASecondReadInAClassifiedFileIsNotInherited is the mutation the by-file census
// could not catch: appending the truncate-and-consume shape to an already-classified
// file. A second read there lands in a new function, so it is a fresh, unclassified
// SITE — the census must name it rather than inheriting the file's verdict. Both the
// new-function case (a distinct key) and the same-function case (a repeated key) are
// pinned, since the earlier guard was blind to both.
func TestASecondReadInAClassifiedFileIsNotInherited(t *testing.T) {
	base := cappedReadSites(t)

	newFunc := append(append([]cappedSite{}, base...), cappedSite{
		key:  "internal/mcp/client.go::leakedShortRead",
		file: "internal/mcp/client.go",
		line: 999,
		fn:   "leakedShortRead",
	})
	if got := unclassifiedSites(newFunc, cappedReads); len(got) != 1 || got[0].key != "internal/mcp/client.go::leakedShortRead" {
		t.Errorf("a second capped read in a new function of an already-classified file was not reported as unclassified: %v", got)
	}

	sameFunc := append(append([]cappedSite{}, base...), cappedSite{
		key:  "internal/mcp/client.go::doWithHeaders",
		file: "internal/mcp/client.go",
		line: 999,
		fn:   "doWithHeaders",
	})
	if got := multiReadSites(sameFunc); len(got) != 1 || got[0] != "internal/mcp/client.go::doWithHeaders" {
		t.Errorf("a second capped read in the SAME already-classified function was not reported as a shared classification: %v", got)
	}
}

func unclassifiedSites(sites []cappedSite, classified map[string]string) []cappedSite {
	var out []cappedSite
	for _, s := range sites {
		if _, ok := classified[s.key]; !ok {
			out = append(out, s)
		}
	}
	return out
}

func multiReadSites(sites []cappedSite) []string {
	count := map[string]int{}
	for _, s := range sites {
		count[s.key]++
	}
	var out []string
	for key, n := range count {
		if n > 1 {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func staleClassifications(sites []cappedSite, classified map[string]string) []string {
	live := map[string]bool{}
	for _, s := range sites {
		live[s.key] = true
	}
	var out []string
	for key := range classified {
		if !live[key] {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

// cappedReadSites parses every module source that mentions io.LimitReader and
// returns one entry per CALL, keyed by file plus enclosing function. It reads the
// AST rather than the text so a mention inside a comment or string is not counted,
// and so each call carries the function a file-scoped scan could not attribute.
func cappedReadSites(t *testing.T) []cappedSite {
	t.Helper()
	fset := token.NewFileSet()
	var sites []cappedSite
	for _, file := range srccensus.Tree(t, "../..", 200) {
		// A file with no textual mention cannot hold the call, so skipping it is a
		// pure speed-up, not a classification decision: the AST walk below is what
		// decides, and it never sees a commented-out mention as a call.
		if !strings.Contains(file.Text, "io.LimitReader(") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file.Path, file.Text, 0)
		if err != nil {
			t.Fatalf("cannot parse %s, so a capped read there would be missed silently: %v", file.Name, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || qualifiedCalleeName(call.Fun) != "io.LimitReader" {
				return true
			}
			fn := enclosingFunc(parsed, call.Pos())
			sites = append(sites, cappedSite{
				key:  file.Name + "::" + fn,
				file: file.Name,
				line: fset.Position(call.Pos()).Line,
				fn:   fn,
			})
			return true
		})
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].key < sites[j].key })
	return sites
}

// qualifiedCalleeName returns "pkg.Fn" so io.LimitReader is not confused with a
// LimitReader from any other package; the sibling calleeName in this package
// returns the bare selector, which cannot make that distinction.
func qualifiedCalleeName(fun ast.Expr) string {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return pkg.Name + "." + sel.Sel.Name
}

func enclosingFunc(file *ast.File, pos token.Pos) string {
	name := "(file scope)"
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if pos >= fn.Pos() && pos < fn.End() {
			name = fn.Name.Name
		}
	}
	return name
}

type censusText string

func (c censusText) Contains(want string) bool { return strings.Contains(string(c), want) }

func fileText(t *testing.T, name string) censusText {
	t.Helper()
	for _, file := range srccensus.Tree(t, "../..", 200) {
		if file.Name == name {
			return censusText(file.Text)
		}
	}
	t.Fatalf("%s is not in the module source any more", name)
	return ""
}
