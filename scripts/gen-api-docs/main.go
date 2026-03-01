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
	Method      string       `json:"method"`
	Path        string       `json:"path"`
	Handler     string       `json:"handler"`
	Description string       `json:"description,omitempty"`
	Implemented *bool        `json:"implemented,omitempty"`
	Params      []Parameter  `json:"params,omitempty"`
	Examples    ExampleGroup `json:"examples,omitempty"`
}

type Parameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Location    string `json:"location"` // query, body, path
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type ExampleGroup struct {
	Curl string `json:"curl,omitempty"`
	CLI  string `json:"cli,omitempty"`
	Payload string `json:"payload,omitempty"` // Example JSON payload
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
	handlerMetadata := make(map[string]Endpoint)

	// First, extract metadata from all handler functions
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Look for handler functions and extract their metadata
		if strings.Contains(funcDecl.Name.Name, "Handle") || strings.Contains(funcDecl.Name.Name, "handle") {
			if funcDecl.Doc != nil {
				commentText := ""
				for _, comment := range funcDecl.Doc.List {
					commentText += strings.TrimPrefix(comment.Text, "// ") + "\n"
				}
				metadata := extractMetadata(commentText)
				if metadata.Description != "" || len(metadata.Params) > 0 || metadata.Examples.Curl != "" {
					handlerMetadata[funcDecl.Name.Name] = metadata
				}
			}
		}
	}

	// Then, extract routes from RegisterHandlers/RegisterRoutes
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Look for RegisterHandlers or RegisterRoutes
		if funcDecl.Name.Name == "RegisterHandlers" || funcDecl.Name.Name == "RegisterRoutes" {
			// Extract HandleFunc calls
			routeEndpoints := extractHandleFuncCalls(funcDecl.Body)

			// Merge with metadata
			for _, ep := range routeEndpoints {
				if meta, ok := handlerMetadata[ep.Handler]; ok {
					ep.Description = meta.Description
					ep.Implemented = meta.Implemented
					ep.Params = meta.Params
					ep.Examples = meta.Examples
				}
				endpoints = append(endpoints, ep)
			}
		}
	}

	return endpoints
}

func extractMetadata(commentText string) Endpoint {
	ep := Endpoint{}
	lines := strings.Split(commentText, "\n")

	for _, line := range lines {
		// Remove "// " prefix if present, then trim
		line = strings.TrimPrefix(line, "// ")
		line = strings.TrimSpace(line)

		if line == "" || !strings.HasPrefix(line, "@") {
			continue
		}

		// Split on first space after the tag
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}

		tag := parts[0]
		value := strings.TrimSpace(parts[1])

		switch tag {
		case "@Description":
			ep.Description = value
		case "@Implemented":
			implemented := value == "true"
			ep.Implemented = &implemented
		case "@Param":
			param := parseParam(value)
			ep.Params = append(ep.Params, param)
		case "@Curl":
			ep.Examples.Curl = value
		case "@CLI":
			ep.Examples.CLI = value
		case "@Example":
			ep.Examples.Payload = value
		}
	}

	return ep
}

func parseParam(paramStr string) Parameter {
	// Format: name type location required/optional description
	// Example: url string body required URL to navigate to
	parts := strings.SplitN(paramStr, " ", 5)

	p := Parameter{}
	if len(parts) >= 1 {
		p.Name = parts[0]
	}
	if len(parts) >= 2 {
		p.Type = parts[1]
	}
	if len(parts) >= 3 {
		p.Location = parts[2]
	}
	if len(parts) >= 4 {
		p.Required = parts[3] == "required"
	}
	if len(parts) >= 5 {
		p.Description = parts[4]
	}

	return p
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
