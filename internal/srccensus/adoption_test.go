package srccensus

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryASTParsingTestUnderInternalBuildsOnSrccensus(t *testing.T) {
	files := TestTree(t, filepath.Join("..", ".."), 100)

	censuses := 0
	for _, file := range files {
		if !strings.HasPrefix(file.Name, "internal/") || strings.HasPrefix(file.Name, "internal/srccensus/") {
			continue
		}
		consults, err := callsSrccensus(file.Name, file.Text)
		if err != nil {
			t.Errorf("%s: cannot parse, so this guard can neither clear nor flag it: %v", file.Name, err)
			continue
		}
		if consults {
			censuses++
			continue
		}
		if containsAnyMarker(file.Text, []string{"parser.ParseFile(", "parser.ParseDir("}) {
			t.Errorf("%s parses Go source without srccensus; obtain the files from srccensus.Load (Syntax for raw AST) or srccensus.Tree, so its scope is derived rather than hand-listed", file.Name)
		}
	}
	if censuses < 60 {
		t.Fatalf("found only %d srccensus-based tests under internal/; the detector matched almost nothing and would pass vacuously", censuses)
	}
}
