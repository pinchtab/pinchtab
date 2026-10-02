package srccensus

import (
	"go/ast"
	"go/token"
	"path"
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

func TestNoTestReadsSourceByLiteralFilename(t *testing.T) {
	files := TestTree(t, filepath.Join("..", ".."), 100)

	var inScope []SourceFile
	for _, file := range files {
		underGuard := strings.HasPrefix(file.Name, "internal/") || strings.HasPrefix(file.Name, "cmd/")
		if underGuard && !strings.HasPrefix(file.Name, "internal/srccensus/") {
			inScope = append(inScope, file)
		}
	}
	reads := literalSourceReads(t, inScope)
	for _, file := range inScope {
		if found := reads[file.Name]; len(found) > 0 {
			t.Errorf("%s reads Go source by literal filename (%s); load the package with srccensus.Load and locate the declaration by name, so a rename or a move to a sibling file keeps the guard pointed at the code", file.Name, strings.Join(found, ", "))
		}
	}
}

func TestLiteralSourceReadsFlagsNamedReadsAndAllowsPlantedSource(t *testing.T) {
	calls := `package p

func f() {
	_, _ = os.ReadFile("handlers.go")
	_, _ = parser.ParseFile(fset, "snapshot.go", nil, 0)
	_, _ = parser.ParseFile(fset, "mutation.go", planted, 0)
	_, _ = os.ReadFile("fixture.json")
	_, _ = os.ReadFile(filepath.Join(dir, "config_load.go"))
	_, _ = os.ReadFile(filepath.Join(dir, "fixture.json"))
	_ = readHelper(t, "bridge/action_request.go")
	_ = readHelper(t, "docs/click.md")
	_ = pureHelper("x.go")
}
`
	helpers := `package p

func readHelper(t *testing.T, rel string) string {
	raw, _ := os.ReadFile(filepath.Join("..", rel))
	return string(raw)
}

func pureHelper(name string) string { return name }
`
	otherPackage := `package q

func g() {
	_ = readHelper(t, "bridge/action_request.go")
}
`
	reads := literalSourceReads(t, []SourceFile{
		{Name: "internal/p/planted_test.go", Text: calls},
		{Name: "internal/p/helpers_test.go", Text: helpers},
		{Name: "internal/q/planted_test.go", Text: otherPackage},
	})
	got := reads["internal/p/planted_test.go"]
	want := []string{`line 4: os.ReadFile("handlers.go")`, `line 5: parser.ParseFile(_, "snapshot.go", nil)`, `line 8: os.ReadFile("config_load.go")`, `line 10: readHelper(_, "bridge/action_request.go")`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("literalSourceReads = %q, want %q", got, want)
	}
	if len(reads) != 1 {
		t.Fatalf("literalSourceReads flagged files beyond the planted caller: %q", reads)
	}
}

func literalSourceReads(t testing.TB, files []SourceFile) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	for _, file := range files {
		parsed[file.Name] = Parse(t, fset, file.Name, file.Text, 0)
	}
	readers := map[string]map[string]bool{}
	for name, syntax := range parsed {
		key := testPackageKey(name, syntax)
		if readers[key] == nil {
			readers[key] = map[string]bool{}
		}
		addFileReadingHelpers(readers[key], syntax)
	}
	reads := map[string][]string{}
	for name, syntax := range parsed {
		if found := sourceReadsIn(fset, syntax, readers[testPackageKey(name, syntax)]); len(found) > 0 {
			reads[name] = found
		}
	}
	return reads
}

func testPackageKey(name string, file *ast.File) string {
	return path.Dir(name) + " " + file.Name.Name
}

func sourceReadsIn(fset *token.FileSet, file *ast.File, readers map[string]bool) []string {
	var reads []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if helper, isIdent := call.Fun.(*ast.Ident); isIdent && readers[helper.Name] {
			for _, arg := range call.Args {
				if lit, isGoFile := goFileLiteral(arg); isGoFile {
					reads = append(reads, "line "+strconv.Itoa(fset.Position(call.Pos()).Line)+": "+helper.Name+"(_, "+lit+")")
				}
			}
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
	return reads
}

func addFileReadingHelpers(readers map[string]bool, file *ast.File) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Recv != nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if call, isCall := node.(*ast.CallExpr); isCall && calleeName(call.Fun) == "os.ReadFile" {
				readers[fn.Name.Name] = true
			}
			return true
		})
	}
}

func goFileLiteral(expr ast.Expr) (string, bool) {
	if join, ok := expr.(*ast.CallExpr); ok && calleeName(join.Fun) == "filepath.Join" && len(join.Args) > 0 {
		expr = join.Args[len(join.Args)-1]
	}
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
