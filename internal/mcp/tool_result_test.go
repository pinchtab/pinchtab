package mcp

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func inlineTail(body []byte, code int, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return resultFromBytes(body, code)
}

func TestToolResultMatchesTheInlineTailItReplaced(t *testing.T) {
	rows := []struct {
		name    string
		body    []byte
		code    int
		err     error
		isError bool
	}{
		{"transport error", nil, 0, errors.New("dial tcp 127.0.0.1:9867: connection refused"), true},
		{"2xx body", []byte(`{"ok":true}`), 200, nil, false},
		{"4xx body", []byte(`{"error":"tab not found"}`), 404, nil, true},
		{"2xx body reporting no success", []byte(`{"set":0,"failed":2}`), 200, nil, true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got, gotErr := toolResult(row.body, row.code, row.err)
			want, wantErr := inlineTail(row.body, row.code, row.err)

			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotErr, wantErr) {
				t.Fatalf("toolResult = %#v, %v; inline tail = %#v, %v", got, gotErr, want, wantErr)
			}
			if got.IsError != row.isError {
				t.Fatalf("IsError = %v, want %v", got.IsError, row.isError)
			}
		})
	}
}

func clientMethods(files []*ast.File) map[string]bool {
	methods := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if ident, ok := recv.(*ast.Ident); ok && ident.Name == "Client" {
				methods[fn.Name.Name] = true
			}
		}
	}
	return methods
}

func inlineTailSites(fset *token.FileSet, files []*ast.File, methods map[string]bool) []string {
	var sites []string
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i := 0; i+2 < len(block.List); i++ {
				body, code, errName, ok := clientCallTriple(block.List[i], methods)
				if ok && returnsErrorResult(block.List[i+1], errName) && returnsResultFromBytes(block.List[i+2], body, code) {
					sites = append(sites, fset.Position(block.List[i].Pos()).String())
				}
			}
			return true
		})
	}
	return sites
}

func clientCallTriple(stmt ast.Stmt, methods map[string]bool) (string, string, string, bool) {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 3 || len(assign.Rhs) != 1 {
		return "", "", "", false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return "", "", "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !methods[sel.Sel.Name] {
		return "", "", "", false
	}
	names := make([]string, 3)
	for i, lhs := range assign.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			return "", "", "", false
		}
		names[i] = ident.Name
	}
	return names[0], names[1], names[2], true
}

func returnsErrorResult(stmt ast.Stmt, errName string) bool {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok || ifStmt.Init != nil || ifStmt.Else != nil || exprString(ifStmt.Cond) != errName+" != nil" || len(ifStmt.Body.List) != 1 {
		return false
	}
	ret, ok := ifStmt.Body.List[0].(*ast.ReturnStmt)
	return ok && len(ret.Results) == 2 &&
		exprString(ret.Results[0]) == "mcp.NewToolResultError("+errName+".Error())" &&
		exprString(ret.Results[1]) == "nil"
}

func returnsResultFromBytes(stmt ast.Stmt, body, code string) bool {
	ret, ok := stmt.(*ast.ReturnStmt)
	return ok && len(ret.Results) == 1 && exprString(ret.Results[0]) == "resultFromBytes("+body+", "+code+")"
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.BinaryExpr:
		return exprString(v.X) + " " + v.Op.String() + " " + exprString(v.Y)
	case *ast.CallExpr:
		args := make([]string, len(v.Args))
		for i, arg := range v.Args {
			args[i] = exprString(arg)
		}
		return exprString(v.Fun) + "(" + strings.Join(args, ", ") + ")"
	}
	return "?"
}

func parsePackageSources(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset, byName := srccensus.Load(t, ".", 18).Syntax()
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	files := make([]*ast.File, 0, len(names))
	for _, name := range names {
		files = append(files, byName[name])
	}
	return fset, files
}

func TestNoHandlerSpellsOutTheToolResultTail(t *testing.T) {
	fset, files := parsePackageSources(t)
	methods := clientMethods(files)
	for _, want := range []string{"Get", "Post", "Delete", "GetCapturingVocab", "withTimeout"} {
		if !methods[want] {
			t.Fatalf("Client method %s not found; the census would miss its tails", want)
		}
	}

	for _, site := range inlineTailSites(fset, files, methods) {
		t.Errorf("%s spells out the call/error/resultFromBytes tail; return toolResult(<call>) instead", site)
	}
}

func TestTheToolResultTailCensusSeesPlantedCopiesAndNotNearMisses(t *testing.T) {
	_, files := parsePackageSources(t)
	methods := clientMethods(files)
	const planted = `package mcp
func plain(c *Client) (*mcp.CallToolResult, error) {
	body, code, err := c.Get(ctx, "/text", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return resultFromBytes(body, code)
}
func chained(c *Client) (*mcp.CallToolResult, error) {
	out, status, err := c.withTimeout(d).Post(ctx, "/scrape", p)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return resultFromBytes(out, status)
}
func vocab(c *Client) (*mcp.CallToolResult, error) {
	body, code, err := c.GetCapturingVocab(ctx, "/snapshot", q, tab)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return resultFromBytes(body, code)
}
func renamed(c *Client) (*mcp.CallToolResult, error) {
	b, st, e := c.Delete(ctx, "/network", nil)
	if e != nil {
		return mcp.NewToolResultError(e.Error()), nil
	}
	return resultFromBytes(b, st)
}
func nearMiss(c *Client) (*mcp.CallToolResult, error) {
	body, code, err := c.Get(ctx, "/cookies", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if code >= 400 {
		return resultFromBytes(body, code)
	}
	return inspect(body)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "planted.go", planted, 0)
	if err != nil {
		t.Fatal(err)
	}

	sites := inlineTailSites(fset, []*ast.File{file}, methods)

	if len(sites) != 4 {
		t.Fatalf("census found %v in the planted source, want the four tails and not the near miss", sites)
	}
}
