package httpx

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

var optionalBodyOwners = map[string]bool{
	"internal/httpx/httpx.go::DecodeJSON": true,
}

func decodeEOFSites(t *testing.T, name, src string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var sites []string
	scan := func(fnName string, body *ast.BlockStmt) {
		decodeErrs := decodeErrorNames(body)
		ast.Inspect(body, func(n ast.Node) bool {
			if subject, ok := eofComparisonSubject(n); ok && decodeErrs[subject] {
				sites = append(sites, name+"::"+fnName)
			}
			return true
		})
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body != nil {
				scan(d.Name.Name, d.Body)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, v := range value.Values {
					if lit, ok := v.(*ast.FuncLit); ok && i < len(value.Names) {
						scan(value.Names[i].Name, lit.Body)
					}
				}
			}
		}
	}
	return sites
}

func decodeErrorNames(body *ast.BlockStmt) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || !isDecodeCall(call.Fun) {
			return true
		}
		for _, lhs := range assign.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				names[ident.Name] = true
			}
		}
		return true
	})
	return names
}

func isDecodeCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name == "Decode" || f.Sel.Name == "DecodeJSONBody"
	case *ast.Ident:
		return f.Name == "DecodeJSONBody"
	}
	return false
}

func isIOEOF(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "EOF" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "io"
}

func eofComparisonSubject(n ast.Node) (string, bool) {
	switch e := n.(type) {
	case *ast.BinaryExpr:
		if e.Op != token.EQL && e.Op != token.NEQ {
			break
		}
		for _, pair := range [][2]ast.Expr{{e.X, e.Y}, {e.Y, e.X}} {
			if ident, ok := pair[0].(*ast.Ident); ok && isIOEOF(pair[1]) {
				return ident.Name, true
			}
		}
	case *ast.SwitchStmt:
		ident, ok := e.Tag.(*ast.Ident)
		if !ok {
			break
		}
		for _, stmt := range e.Body.List {
			for _, expr := range stmt.(*ast.CaseClause).List {
				if isIOEOF(expr) {
					return ident.Name, true
				}
			}
		}
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "Is" && len(e.Args) == 2 && isIOEOF(e.Args[1]) {
			if ident, ok := e.Args[0].(*ast.Ident); ok {
				return ident.Name, true
			}
		}
	}
	return "", false
}

func TestOnlyDecodeJSONTreatsADecodeEOFAsAnEmptyBody(t *testing.T) {
	found := map[string]bool{}
	sawTokenizer := false
	for _, file := range srccensus.Tree(t, "../..", 200) {
		if !strings.HasPrefix(file.Name, "internal/") {
			continue
		}
		for _, site := range decodeEOFSites(t, file.Name, file.Text) {
			found[site] = true
		}
		if file.Name == "internal/browsers/ghostchrome/staticfetch/staticfetch.go" {
			sawTokenizer = strings.Contains(file.Text, "z.Err() == io.EOF")
		}
	}

	var stray []string
	for site := range found {
		if !optionalBodyOwners[site] {
			stray = append(stray, site)
		}
	}
	sort.Strings(stray)
	for _, site := range stray {
		t.Errorf("%s treats a JSON decode io.EOF as an empty body by hand; call httpx.DecodeJSON with Optional set", site)
	}
	for owner := range optionalBodyOwners {
		if !found[owner] {
			t.Errorf("%s no longer matches; the census would guard nothing there, so update the owner table", owner)
		}
	}
	if !sawTokenizer {
		t.Error("staticfetch's tokenizer io.EOF check is gone; the near-miss this census must leave alone is no longer exercised on real code")
	}
}

func TestTheDecodeEOFCensusSeesAPlantedCopyAndNotANonDecodeRead(t *testing.T) {
	const planted = `package p
func strict(w http.ResponseWriter, r *http.Request) {
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil && !errors.Is(err, io.EOF) {
		return
	}
}
func lenient(r *http.Request) {
	decodeErr := json.NewDecoder(r.Body).Decode(&req)
	if decodeErr != nil && decodeErr != io.EOF {
		return
	}
}
func reversed(r *http.Request) {
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil && io.EOF != err {
		return
	}
}
func switched(r *http.Request) {
	err := json.NewDecoder(r.Body).Decode(&req)
	switch err {
	case nil, io.EOF:
	default:
		return
	}
}
var packageLevel = func(w http.ResponseWriter, r *http.Request) {
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil && !errors.Is(err, io.EOF) {
		return
	}
}
func read(r io.Reader) {
	n, err := r.Read(buf)
	if err == io.EOF {
		return
	}
	_ = n
}
func tokenizer(z *html.Tokenizer) {
	if z.Err() == io.EOF {
		return
	}
}
`
	got := decodeEOFSites(t, "planted.go", planted)

	if strings.Join(got, ",") != "planted.go::strict,planted.go::lenient,planted.go::reversed,planted.go::switched,planted.go::packageLevel" {
		t.Fatalf("census found %v, want the five planted decode copies and neither the read nor the tokenizer", got)
	}
}
