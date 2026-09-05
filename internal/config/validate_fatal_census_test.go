package config

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var fatalAtLoadMembers = map[string]string{
	"sessions.agent.mode": "an uninterpretable authentication posture must not silently fall back to a weaker mode",
}

func TestFatalAtLoadMembershipIsExplicitAndExhaustive(t *testing.T) {
	fset := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[path] = file
	}
	if len(files) == 0 {
		t.Fatal("no config production files found; fatal-at-load census would pass vacuously")
	}

	fields, literals := fatalAtLoadDeclarations(files)
	for _, problem := range fatalAtLoadCensusProblems(fields, literals, fatalAtLoadMembers) {
		t.Error(problem)
	}
}

func fatalAtLoadDeclarations(files map[string]*ast.File) ([]string, int) {
	var fields []string
	literals := 0
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			typeName, ok := literal.Type.(*ast.Ident)
			if !ok || typeName.Name != "ValidationError" {
				return true
			}
			literals++
			field := ""
			fatal := false
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, _ := pair.Key.(*ast.Ident)
				if key == nil {
					continue
				}
				switch key.Name {
				case "Field":
					if value, ok := pair.Value.(*ast.BasicLit); ok && value.Kind == token.STRING {
						field, _ = strconv.Unquote(value.Value)
					}
				case "FatalAtLoad":
					value, _ := pair.Value.(*ast.Ident)
					fatal = value != nil && value.Name == "true"
				}
			}
			if fatal {
				if field == "" {
					field = "<non-literal field>"
				}
				fields = append(fields, field)
			}
			return true
		})
	}
	return fields, literals
}

func fatalAtLoadCensusProblems(fields []string, literals int, expected map[string]string) []string {
	if literals < 20 {
		return []string{fmt.Sprintf("found only %d ValidationError literals; the census is no longer walking the validator", literals)}
	}
	actual := map[string]bool{}
	for _, field := range fields {
		actual[field] = true
	}
	var problems []string
	for field := range actual {
		if _, ok := expected[field]; !ok {
			problems = append(problems, fmt.Sprintf("%s newly declares FatalAtLoad; add it to fatalAtLoadMembers with the deliberate reason", field))
		}
	}
	for field, reason := range expected {
		if strings.TrimSpace(reason) == "" {
			problems = append(problems, fmt.Sprintf("%s has no recorded reason for being fatal at load", field))
		}
		if !actual[field] {
			problems = append(problems, fmt.Sprintf("%s is recorded as fatal at load but no declaration remains", field))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestFatalAtLoadCensusMutationsTurnRed(t *testing.T) {
	expected := fatalAtLoadMembers
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"new member", `package config
var _ = ValidationError{Field: "sessions.agent.mode", FatalAtLoad: true}
var _ = ValidationError{Field: "server.logLevel", FatalAtLoad: true}
`, "server.logLevel newly declares FatalAtLoad"},
		{"removed member", `package config
var _ = ValidationError{Field: "sessions.agent.mode"}
`, "sessions.agent.mode is recorded as fatal at load but no declaration remains"},
		{"vacuous walk", "package config\n", "census is no longer walking the validator"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "mutation.go", tc.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			fields, literals := fatalAtLoadDeclarations(map[string]*ast.File{"mutation.go": file})
			if literals > 0 && literals < 20 {
				literals = 20 // isolate the membership mutation from the separately tested vacuity floor
			}
			problems := strings.Join(fatalAtLoadCensusProblems(fields, literals, expected), "\n")
			if !strings.Contains(problems, tc.want) {
				t.Fatalf("mutation problems = %q, want %q", problems, tc.want)
			}
		})
	}
}
