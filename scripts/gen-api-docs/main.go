package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Endpoint struct {
	Method  string
	Path    string
	Handler string
	Doc     string
}

func main() {
	// Parse handlers.go to extract routes
	endpoints := extractEndpoints("internal/handlers/handlers.go")

	// Sort by method then path
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Method != endpoints[j].Method {
			return endpoints[i].Method < endpoints[j].Method
		}
		return endpoints[i].Path < endpoints[j].Path
	})

	// Generate markdown
	fmt.Println("# API Reference (Auto-Generated)")
	fmt.Println("")
	fmt.Printf("Generated: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Println("")
	fmt.Println("---")
	fmt.Println("")

	fmt.Println("## Endpoints Summary")
	fmt.Println("")
	fmt.Println("| Method | Path | Handler | Notes |")
	fmt.Println("|--------|------|---------|-------|")

	for _, ep := range endpoints {
		notes := ""
		if ep.Doc != "" {
			notes = strings.TrimSpace(ep.Doc)
			if len(notes) > 50 {
				notes = notes[:50] + "..."
			}
		}
		fmt.Printf("| %s | `%s` | %s | %s |\n", ep.Method, ep.Path, ep.Handler, notes)
	}

	fmt.Println("")
	fmt.Println("---")
	fmt.Println("")
	fmt.Println("## Detailed Endpoints")
	fmt.Println("")

	for _, ep := range endpoints {
		fmt.Printf("### %s %s\n", ep.Method, ep.Path)
		fmt.Println("")

		if ep.Doc != "" {
			fmt.Printf("%s\n", ep.Doc)
			fmt.Println("")
		}

		fmt.Printf("**Handler:** `%s`\n", ep.Handler)
		fmt.Println("")
	}

	fmt.Println("---")
	fmt.Println("")
	fmt.Println("## Notes")
	fmt.Println("")
	fmt.Println("- This documentation is auto-generated from Go code")
	fmt.Println("- For full implementation details, see `internal/handlers/*.go`")
	fmt.Println("- Query parameters and request bodies are defined in each handler")
	fmt.Println("")
}

func extractEndpoints(filePath string) []Endpoint {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("Error parsing file: %v", err)
	}

	var endpoints []Endpoint

	// Find RegisterRoutes function
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "RegisterRoutes" {
			continue
		}

		// Extract HandleFunc calls
		endpoints = extractHandleFuncCalls(funcDecl.Body)
	}

	return endpoints
}

func extractHandleFuncCalls(body *ast.BlockStmt) []Endpoint {
	var endpoints []Endpoint

	routePattern := regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH)\s+(/[^\s]*)`)

	for _, stmt := range body.List {
		exprStmt, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}

		callExpr, ok := exprStmt.X.(*ast.CallExpr)
		if !ok {
			continue
		}

		// Check if this is mux.HandleFunc or mux.Handle
		selector, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		if selector.Sel.Name != "HandleFunc" && selector.Sel.Name != "Handle" {
			continue
		}

		// Extract route pattern (first argument)
		if len(callExpr.Args) < 1 {
			continue
		}

		basicLit, ok := callExpr.Args[0].(*ast.BasicLit)
		if !ok {
			continue
		}

		// Parse route string
		route := strings.Trim(basicLit.Value, "\"")
		matches := routePattern.FindStringSubmatch(route)
		if len(matches) < 3 {
			continue
		}

		method := matches[1]
		path := matches[2]

		// Extract handler name (second argument)
		var handler string
		if len(callExpr.Args) >= 2 {
			if selectorExpr, ok := callExpr.Args[1].(*ast.SelectorExpr); ok {
				handler = selectorExpr.Sel.Name
			}
		}

		endpoints = append(endpoints, Endpoint{
			Method:  method,
			Path:    path,
			Handler: handler,
		})
	}

	return endpoints
}
