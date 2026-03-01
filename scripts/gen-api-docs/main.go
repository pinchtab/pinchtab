package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Endpoint struct {
	Method      string
	Path        string
	Handler     string
	Doc         string
	Description string
	Params      []Param
	Examples    map[string]string // format -> example code
}

type Param struct {
	Name        string
	Type        string
	Location    string // query, body, path
	Description string
}

func main() {
	// Parse all relevant files for routes
	var endpoints []Endpoint

	// Main handlers
	endpoints = append(endpoints, extractEndpoints("internal/handlers/handlers.go")...)

	// Dashboard handlers
	endpoints = append(endpoints, extractEndpoints("internal/dashboard/dashboard.go")...)

	// Profiles service
	profiles, err := findProfilesFile()
	if err == nil {
		endpoints = append(endpoints, extractEndpoints(profiles)...)
	}

	// Orchestrator service
	orchestrator, err := findOrchestratorFile()
	if err == nil {
		endpoints = append(endpoints, extractEndpoints(orchestrator)...)
	}

	// Remove duplicates
	endpointMap := make(map[string]Endpoint)
	for _, ep := range endpoints {
		key := ep.Method + " " + ep.Path
		if _, exists := endpointMap[key]; !exists {
			endpointMap[key] = ep
		}
	}

	endpoints = make([]Endpoint, 0, len(endpointMap))
	for _, ep := range endpointMap {
		endpoints = append(endpoints, ep)
	}

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

func findProfilesFile() (string, error) {
	// Look for profiles service implementation
	matches := []string{
		"internal/profiles/handlers.go",
		"internal/profiles/profiles.go",
		"internal/handler/profiles.go",
		"internal/bridge/profiles.go",
	}
	for _, path := range matches {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("profiles file not found")
}

func findOrchestratorFile() (string, error) {
	// Look for orchestrator service implementation
	matches := []string{
		"internal/orchestrator/handlers.go",
		"internal/orchestrator/orchestrator.go",
		"internal/bridge/orchestrator.go",
	}
	for _, path := range matches {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("orchestrator file not found")
}

func extractEndpoints(filePath string) []Endpoint {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("Error parsing file: %v", err)
	}

	var endpoints []Endpoint

	// Find any RegisterHandlers or RegisterRoutes function
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Look for RegisterHandlers or RegisterRoutes
		if funcDecl.Name.Name == "RegisterHandlers" || funcDecl.Name.Name == "RegisterRoutes" {
			// Extract HandleFunc calls
			endpoints = append(endpoints, extractHandleFuncCalls(funcDecl.Body)...)
		}
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
