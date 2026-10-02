package activity

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

const (
	moduleRoot     = "../.."
	minModuleFiles = 300
	sourceHomeFile = "internal/activity/sources.go"
)

// sourceVocab is derived from the source consts, not hand-listed, so a new SourceX
// const joins the census automatically.
var sourceVocab = map[string]bool{
	SourceClient:       true,
	SourceDashboard:    true,
	SourceServer:       true,
	SourceBridge:       true,
	SourceOrchestrator: true,
	SourceScheduler:    true,
	SourceMCP:          true,
}

// configExemptPrefix is excluded because the config editor and JSON carry a section
// vocabulary ("scheduler", "dashboard", …) that happens to share these spellings but
// names config sections, not activity event sources; it must not be forced onto the
// activity source consts.
const configExemptPrefix = "internal/config/"

// bareSourceLiteralSites returns each place a source-vocabulary string is spelled as a
// bare literal in a source context: as a `Source:` composite-literal value, either side
// of a `.Source ==` / `!=` comparison, or anywhere inside package activity (whose home is
// the source vocabulary) other than the const block itself. A comment naming a source is
// not a literal, so the AST walk does not count it.
func bareSourceLiteralSites(t *testing.T, files []srccensus.SourceFile) []string {
	t.Helper()
	var sites []string
	for _, f := range files {
		if f.Name == sourceHomeFile || strings.HasPrefix(f.Name, configExemptPrefix) {
			continue
		}
		inActivityPkg := strings.HasPrefix(f.Name, "internal/activity/")
		parsed := srccensus.Parse(t, token.NewFileSet(), f.Name, f.Text, 0)
		flag := func(node ast.Node, why string) {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return
			}
			val, err := strconv.Unquote(lit.Value)
			if err != nil || !sourceVocab[val] {
				return
			}
			sites = append(sites, fmt.Sprintf("%s: %q (%s)", f.Name, val, why))
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.KeyValueExpr:
				if key, ok := node.Key.(*ast.Ident); ok && key.Name == "Source" {
					flag(node.Value, "Source: composite-literal value")
				}
			case *ast.BinaryExpr:
				if node.Op != token.EQL && node.Op != token.NEQ {
					return true
				}
				if isSourceSelector(node.X) {
					flag(node.Y, ".Source comparison operand")
				}
				if isSourceSelector(node.Y) {
					flag(node.X, ".Source comparison operand")
				}
			case *ast.AssignStmt:
				for i, lhs := range node.Lhs {
					if isSourceSelector(lhs) && i < len(node.Rhs) {
						flag(node.Rhs[i], ".Source assignment value")
					}
				}
			case *ast.SwitchStmt:
				if node.Tag != nil && isSourceSelector(node.Tag) {
					for _, stmt := range node.Body.List {
						for _, expr := range stmt.(*ast.CaseClause).List {
							flag(expr, "switch on .Source case")
						}
					}
				}
			case *ast.BasicLit:
				if inActivityPkg {
					flag(node, "bare literal in package activity")
				}
			}
			return true
		})
	}
	sort.Strings(sites)
	return sites
}

func isSourceSelector(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Source"
}

func TestNoBareSourceVocabularyLiteralOutsideConstBlock(t *testing.T) {
	if sites := bareSourceLiteralSites(t, srccensus.Tree(t, moduleRoot, minModuleFiles)); len(sites) > 0 {
		t.Errorf("source-vocabulary strings are spelled as bare literals; reference the SourceX consts in %s so a one-character drift is a compile error, not a silent divergence:\n%s", sourceHomeFile, strings.Join(sites, "\n"))
	}
}

// The census bites: the exact drift PIN-386 left — one predicate comparing evt.Source
// against a bare "scheduler" while the other used the const — is reported.
func TestSourceCensusFlagsAPlantedComparison(t *testing.T) {
	planted := []srccensus.SourceFile{
		{Name: "internal/server/evil.go", Text: "package server\nfunc f(evt struct{ Source string }) bool { return evt.Source == \"scheduler\" }\n"},
	}
	if len(bareSourceLiteralSites(t, planted)) == 0 {
		t.Fatal("a bare evt.Source == \"scheduler\" comparison passed the census; the two predicates could silently drift again")
	}
}

func TestSourceCensusFlagsAPlantedAssignmentAndSwitchCase(t *testing.T) {
	for name, text := range map[string]string{
		"assignment": "package server\nfunc f(evt *struct{ Source string }) { evt.Source = \"client\" }\n",
		"switch":     "package server\nfunc f(evt struct{ Source string }) bool {\n\tswitch evt.Source {\n\tcase \"dashboard\":\n\t\treturn true\n\t}\n\treturn false\n}\n",
	} {
		if len(bareSourceLiteralSites(t, []srccensus.SourceFile{{Name: "internal/server/evil.go", Text: text}})) == 0 {
			t.Errorf("a bare source literal in a .Source %s passed the census", name)
		}
	}
}

// The exemption bites in both directions: a config-section literal is not forced onto
// the activity source consts, even in a Source-shaped composite literal.
func TestSourceCensusExcludesConfigByPath(t *testing.T) {
	planted := []srccensus.SourceFile{
		{Name: "internal/config/editor_set.go", Text: "package config\ntype e struct{ Source string }\nvar _ = e{Source: \"scheduler\"}\n"},
	}
	if len(bareSourceLiteralSites(t, planted)) != 0 {
		t.Fatal("a config-section string was flagged; the config vocabulary shares the spelling but is a different vocabulary")
	}
}

// A source named only in a comment is not a spelling: the AST walk sees literals only.
func TestSourceCensusIgnoresComments(t *testing.T) {
	planted := []srccensus.SourceFile{
		{Name: "internal/foo/foo.go", Text: "package foo\n// mentions scheduler and client in prose only\nconst X = 1\n"},
	}
	if got := bareSourceLiteralSites(t, planted); len(got) != 0 {
		t.Fatalf("a source named only in a comment was counted: %v", got)
	}
}

// TestDashboardAgentActivityVerdictPerSource pins which sources are agent activity. The
// live broadcast and the persisted-log rebuild both call IsDashboardAgentActivity, so
// this table is the one place the two paths' agreement is decided.
func TestDashboardAgentActivityVerdictPerSource(t *testing.T) {
	want := map[string]bool{
		SourceClient:       true,
		SourceScheduler:    true,
		SourceDashboard:    false,
		SourceServer:       false,
		SourceBridge:       false,
		SourceOrchestrator: false,
		SourceMCP:          false,
	}
	for source := range sourceVocab {
		if _, ok := want[source]; !ok {
			t.Fatalf("source const %q has no pinned agent-activity verdict; add it to the table", source)
		}
	}
	for source, expected := range want {
		if got := IsDashboardAgentActivity(Event{Source: source}); got != expected {
			t.Errorf("IsDashboardAgentActivity(source=%q) = %v, want %v", source, got, expected)
		}
	}
}

// TestLiveAndRestartPathsCallTheSamePredicate is why the single table above suffices:
// both the live broadcast and the persisted-log rebuild delegate to the one predicate,
// so neither can drift from the table.
func TestLiveAndRestartPathsCallTheSamePredicate(t *testing.T) {
	for _, dir := range []string{"../server", "../dashboard"} {
		pkg := srccensus.Load(t, dir, 2)
		pkg.Calls(t, "activity.IsDashboardAgentActivity")
	}
}
