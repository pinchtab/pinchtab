package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
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
	CLI         bool         `json:"cli,omitempty"`
	Curl        bool         `json:"curl,omitempty"`
	Params      []Parameter  `json:"params,omitempty"`
	Examples    ExampleGroup `json:"examples,omitempty"`
}

type Parameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Location    string `json:"location"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type ExampleGroup struct {
	Curl    string `json:"curl,omitempty"`
	CLI     string `json:"cli,omitempty"`
	Payload string `json:"payload,omitempty"`
}

type APIReference struct {
	Version   string     `json:"version"`
	Generated string     `json:"generated"`
	Count     int        `json:"count"`
	Endpoints []Endpoint `json:"endpoints"`
}

func main() {
	dryRun := flag.Bool("dry-run", false, "Show changes without writing")
	flag.Parse()

	// Generate current endpoints from code
	currentEndpoints := generateEndpoints()
	fmt.Printf("Found %d endpoints in code\n", len(currentEndpoints))

	// Read existing API reference
	existingData := readExistingReference()
	fmt.Printf("Found %d endpoints in existing file\n", len(existingData.Endpoints))

	// Merge endpoints
	mergedEndpoints := mergeEndpoints(existingData.Endpoints, currentEndpoints)
	fmt.Printf("Merged to %d endpoints\n", len(mergedEndpoints))

	// Sort by method then path
	sort.Slice(mergedEndpoints, func(i, j int) bool {
		if mergedEndpoints[i].Method != mergedEndpoints[j].Method {
			return mergedEndpoints[i].Method < mergedEndpoints[j].Method
		}
		return mergedEndpoints[i].Path < mergedEndpoints[j].Path
	})

	// Build final reference
	result := APIReference{
		Version:   "2.0",
		Generated: "auto-merged from code + user metadata",
		Count:     len(mergedEndpoints),
		Endpoints: mergedEndpoints,
	}

	// Show changes
	showChanges(existingData.Endpoints, mergedEndpoints)

	if !*dryRun {
		// Write to file
		outputPath := "docs/references/api-reference.json"
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Fatalf("Error marshaling JSON: %v", err)
		}

		err = ioutil.WriteFile(outputPath, append(data, '\n'), 0644)
		if err != nil {
			log.Fatalf("Error writing file: %v", err)
		}
		fmt.Printf("\nWrote %s\n", outputPath)
	} else {
		fmt.Println("\n(dry-run mode - no changes written)")
	}
}

func generateEndpoints() []Endpoint {
	var endpoints []Endpoint

	// Parse all handler files
	endpoints = append(endpoints, extractEndpoints("internal/handlers/handlers.go")...)
	endpoints = append(endpoints, extractEndpoints("internal/dashboard/dashboard.go")...)

	profiles, err := findProfilesFile()
	if err == nil {
		endpoints = append(endpoints, extractEndpoints(profiles)...)
	}

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

	return endpoints
}

func readExistingReference() APIReference {
	path := "docs/references/api-reference.json"
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("Could not read existing file (%v), starting fresh\n", err)
		return APIReference{
			Version:   "2.0",
			Generated: "auto-merged from code + user metadata",
			Endpoints: []Endpoint{},
		}
	}

	var ref APIReference
	err = json.Unmarshal(data, &ref)
	if err != nil {
		fmt.Printf("Error parsing existing file (%v), starting fresh\n", err)
		return APIReference{
			Version:   "2.0",
			Generated: "auto-merged from code + user metadata",
			Endpoints: []Endpoint{},
		}
	}

	return ref
}

func mergeEndpoints(existing, current []Endpoint) []Endpoint {
	// Index existing endpoints by key
	existingMap := make(map[string]Endpoint)
	for _, ep := range existing {
		key := ep.Method + " " + ep.Path
		existingMap[key] = ep
	}

	// Index current endpoints by key
	currentMap := make(map[string]Endpoint)
	for _, ep := range current {
		key := ep.Method + " " + ep.Path
		currentMap[key] = ep
	}

	// Merge: keep existing fields, update method/path/handler
	merged := []Endpoint{}
	for _, current := range current {
		key := current.Method + " " + current.Path

		if existing, ok := existingMap[key]; ok {
			// Endpoint exists: keep all user metadata, update only code-derived fields
			merged = append(merged, Endpoint{
				Method:      current.Method,
				Path:        current.Path,
				Handler:     current.Handler,
				Description: existing.Description, // Keep existing
				Implemented: existing.Implemented, // Keep existing
				CLI:         existing.CLI,         // Keep existing
				Curl:        existing.Curl,        // Keep existing
				Params:      existing.Params,      // Keep existing
				Examples:    existing.Examples,    // Keep existing
			})
		} else {
			// New endpoint: add with basic fields
			merged = append(merged, Endpoint{
				Method:  current.Method,
				Path:    current.Path,
				Handler: current.Handler,
				CLI:     false,
				Curl:    true,
			})
		}
	}

	return merged
}

func showChanges(existing, merged []Endpoint) {
	existingMap := make(map[string]bool)
	for _, ep := range existing {
		key := ep.Method + " " + ep.Path
		existingMap[key] = true
	}

	mergedMap := make(map[string]bool)
	added := []string{}
	removed := []string{}

	for _, ep := range merged {
		key := ep.Method + " " + ep.Path
		mergedMap[key] = true

		if !existingMap[key] {
			added = append(added, fmt.Sprintf("%s %s", ep.Method, ep.Path))
		}
	}

	for _, ep := range existing {
		key := ep.Method + " " + ep.Path
		if !mergedMap[key] {
			removed = append(removed, fmt.Sprintf("%s %s", ep.Method, ep.Path))
		}
	}

	if len(added) > 0 {
		fmt.Println("\n✨ New endpoints:")
		for _, ep := range added {
			fmt.Printf("  + %s\n", ep)
		}
	}

	if len(removed) > 0 {
		fmt.Println("\n❌ Removed endpoints:")
		for _, ep := range removed {
			fmt.Printf("  - %s\n", ep)
		}
	}

	if len(added) == 0 && len(removed) == 0 {
		fmt.Println("✓ No changes needed")
	}
}

func extractEndpoints(filePath string) []Endpoint {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return []Endpoint{}
	}

	var endpoints []Endpoint

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if funcDecl.Name.Name == "RegisterHandlers" || funcDecl.Name.Name == "RegisterRoutes" {
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

		selector, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		if selector.Sel.Name != "HandleFunc" && selector.Sel.Name != "Handle" {
			continue
		}

		if len(callExpr.Args) < 1 {
			continue
		}

		basicLit, ok := callExpr.Args[0].(*ast.BasicLit)
		if !ok {
			continue
		}

		route := strings.Trim(basicLit.Value, "\"")
		matches := routePattern.FindStringSubmatch(route)
		if len(matches) < 3 {
			continue
		}

		method := matches[1]
		path := matches[2]

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

func findProfilesFile() (string, error) {
	matches := []string{
		"internal/profiles/handlers.go",
		"internal/profiles/profiles.go",
	}
	for _, path := range matches {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("profiles file not found")
}

func findOrchestratorFile() (string, error) {
	matches := []string{
		"internal/orchestrator/handlers.go",
		"internal/orchestrator/orchestrator.go",
	}
	for _, path := range matches {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("orchestrator file not found")
}
