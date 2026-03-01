package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
)

type Endpoint struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Handler string `json:"handler"`
}

type APIReference struct {
	Version   string     `json:"version"`
	Generated string     `json:"generated"`
	Count     int        `json:"count"`
	Endpoints []Endpoint `json:"endpoints"`
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

	// Build API reference
	ref := APIReference{
		Version:   "1.0",
		Generated: "auto-generated from Go code",
		Count:     len(endpoints),
		Endpoints: endpoints,
	}

	// Output as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(ref); err != nil {
		log.Fatalf("Error encoding JSON: %v", err)
	}
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
