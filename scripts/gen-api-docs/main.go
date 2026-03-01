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
	// Parse all relevant files for routes and handler comments
	var endpoints []Endpoint
	handlerComments := make(map[string]string)

	// Main handlers - first get all handler comments from individual files
	handlerComments = mergeComments(handlerComments, extractAllHandlerComments("internal/handlers/"))

	endpoints = append(endpoints, extractEndpoints("internal/handlers/handlers.go")...)

	// Dashboard handlers
	handlerComments = mergeComments(handlerComments, extractAllHandlerComments("internal/dashboard/"))
	endpoints = append(endpoints, extractEndpoints("internal/dashboard/dashboard.go")...)

	// Profiles service
	profiles, err := findProfilesFile()
	if err == nil {
		handlerComments = mergeComments(handlerComments, extractAllHandlerComments("internal/profiles/"))
		endpoints = append(endpoints, extractEndpoints(profiles)...)
	}

	// Orchestrator service
	orchestrator, err := findOrchestratorFile()
	if err == nil {
		handlerComments = mergeComments(handlerComments, extractAllHandlerComments("internal/orchestrator/"))
		endpoints = append(endpoints, extractEndpoints(orchestrator)...)
	}

	// Merge handler comments into endpoints
	for i := range endpoints {
		if handlerComments[endpoints[i].Handler] != "" {
			endpoints[i].Doc = handlerComments[endpoints[i].Handler]
		}
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
			// Extract first line of doc comment
			lines := strings.Split(strings.TrimSpace(ep.Doc), "\n")
			notes = strings.TrimSpace(lines[0])
			// Remove the function name prefix if it exists
			if strings.HasPrefix(notes, "Handle") {
				parts := strings.SplitN(notes, " ", 2)
				if len(parts) > 1 {
					notes = parts[1]
				}
			}
			if len(notes) > 60 {
				notes = notes[:60] + "..."
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
			// Clean up the doc text
			docLines := strings.Split(strings.TrimSpace(ep.Doc), "\n")
			for _, line := range docLines {
				fmt.Println(line)
			}
			fmt.Println("")
		}

		if ep.Handler != "" {
			fmt.Printf("**Handler:** `%s`\n", ep.Handler)
			fmt.Println("")
		}
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

func mergeComments(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
	}
	return result
}

func extractAllHandlerComments(dirPath string) map[string]string {
	comments := make(map[string]string)

	// Read directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return comments
	}

	// Parse all .go files in the directory
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			filePath := dirPath + entry.Name()
			fileComments := extractHandlerComments(filePath)
			for k, v := range fileComments {
				comments[k] = v
			}
		}
	}

	return comments
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

func extractHandlerComments(filePath string) map[string]string {
	comments := make(map[string]string)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return comments
	}

	// Iterate through all declarations
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Look for handler functions (they have "Handle" or "handle" in the name)
		if !strings.Contains(funcDecl.Name.Name, "handle") && !strings.Contains(funcDecl.Name.Name, "Handle") {
			continue
		}

		// Extract doc comment
		if funcDecl.Doc != nil && len(funcDecl.Doc.List) > 0 {
			var docText strings.Builder
			for _, comment := range funcDecl.Doc.List {
				docText.WriteString(strings.TrimPrefix(comment.Text, "// "))
				docText.WriteString("\n")
			}
			comments[funcDecl.Name.Name] = docText.String()
		}
	}

	return comments
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
