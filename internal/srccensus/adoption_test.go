package srccensus

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEveryASTParsingTestUnderInternalBuildsOnSrccensus(t *testing.T) {
	files := TestTree(t, filepath.Join("..", ".."), 100)

	censuses := 0
	for _, file := range files {
		if !strings.HasPrefix(file.Name, "internal/") || strings.HasPrefix(file.Name, "internal/srccensus/") {
			continue
		}
		consults, err := callsSrccensus(file.Name, file.Text)
		if err != nil {
			t.Errorf("%s: cannot parse, so this guard can neither clear nor flag it: %v", file.Name, err)
			continue
		}
		if consults {
			censuses++
			continue
		}
		if containsAnyMarker(file.Text, []string{"parser.ParseFile(", "parser.ParseDir("}) {
			t.Errorf("%s parses Go source without srccensus; obtain the files from srccensus.Load (Syntax for raw AST) or srccensus.Tree, so its scope is derived rather than hand-listed", file.Name)
		}
	}
	if censuses < 60 {
		t.Fatalf("found only %d srccensus-based tests under internal/; the detector matched almost nothing and would pass vacuously", censuses)
	}
}

func TestNoTestUnderInternalReadsSourceByLiteralFilename(t *testing.T) {
	files := TestTree(t, filepath.Join("..", ".."), 100)

	for _, file := range files {
		if !strings.HasPrefix(file.Name, "internal/") || strings.HasPrefix(file.Name, "internal/srccensus/") {
			continue
		}
		reads, err := literalSourceReads(file.Name, file.Text)
		if err != nil {
			t.Errorf("%s: cannot parse, so this guard can neither clear nor flag it: %v", file.Name, err)
			continue
		}
		if len(reads) > 0 {
			t.Errorf("%s reads Go source by literal filename (%s); load the package with srccensus.Load and locate the declaration by name, so a rename or a move to a sibling file keeps the guard pointed at the code", file.Name, strings.Join(reads, ", "))
		}
	}
}

func TestLiteralSourceReadsFlagsNamedReadsAndAllowsPlantedSource(t *testing.T) {
	src := `package p

func f() {
	_, _ = os.ReadFile("handlers.go")
	_, _ = parser.ParseFile(fset, "snapshot.go", nil, 0)
	_, _ = parser.ParseFile(fset, "mutation.go", planted, 0)
	_, _ = os.ReadFile("fixture.json")
}
`
	got, err := literalSourceReads("planted_test.go", src)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`line 4: os.ReadFile("handlers.go")`, `line 5: parser.ParseFile(_, "snapshot.go", nil)`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("literalSourceReads = %q, want %q", got, want)
	}
}

func literalSourceReads(name, src string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}
	var reads []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		line := fset.Position(call.Pos()).Line
		switch {
		case pkg.Name == "os" && sel.Sel.Name == "ReadFile" && len(call.Args) == 1:
			if lit, ok := goFileLiteral(call.Args[0]); ok {
				reads = append(reads, "line "+strconv.Itoa(line)+": os.ReadFile("+lit+")")
			}
		case pkg.Name == "parser" && sel.Sel.Name == "ParseFile" && len(call.Args) >= 3:
			lit, ok := goFileLiteral(call.Args[1])
			if nilSrc, isIdent := call.Args[2].(*ast.Ident); ok && isIdent && nilSrc.Name == "nil" {
				reads = append(reads, "line "+strconv.Itoa(line)+": parser.ParseFile(_, "+lit+", nil)")
			}
		}
		return true
	})
	return reads, nil
}

func goFileLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil || !strings.HasSuffix(value, ".go") {
		return "", false
	}
	return lit.Value, true
}
