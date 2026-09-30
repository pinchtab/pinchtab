package types

import (
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestEveryExportedDeclarationIsInTheGeneratedTypeScript(t *testing.T) {
	generated, err := os.ReadFile(filepath.Join("..", "..", "..", "dashboard", "src", "generated", "types.ts"))
	if err != nil {
		t.Fatalf("read generated TypeScript: %v", err)
	}
	ts := string(generated)

	_, files := srccensus.Load(t, ".", 3).Syntax()
	checked := 0
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				for _, name := range exportedNames(spec) {
					checked++
					if !regexp.MustCompile(`export (interface|type|const) ` + name + `\b`).MatchString(ts) {
						t.Errorf("%s is exported from internal/api/types but missing from dashboard/src/generated/types.ts; run tygo generate and prettier in dashboard/ and commit the file", name)
					}
				}
			}
		}
	}
	if checked < 10 {
		t.Fatalf("checked only %d exported declarations; this parity test would prove little", checked)
	}
}

func exportedNames(spec ast.Spec) []string {
	var names []string
	switch s := spec.(type) {
	case *ast.TypeSpec:
		if s.Name.IsExported() {
			names = append(names, s.Name.Name)
		}
	case *ast.ValueSpec:
		for _, n := range s.Names {
			if n.IsExported() {
				names = append(names, n.Name)
			}
		}
	}
	return names
}
