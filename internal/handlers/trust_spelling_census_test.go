package handlers

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestTheInternalTokenEnvIsSpelledInOnePlace(t *testing.T) {
	const declaration = "internal/handlers/trust.go"
	declared := false
	for _, file := range srccensus.Tree(t, filepath.Join("..", ".."), 200) {
		if strings.HasSuffix(file.Name, "_test.go") || !strings.Contains(file.Text, `"`+InternalTokenEnv+`"`) {
			continue
		}
		if file.Name == declaration {
			declared = true
			continue
		}
		t.Errorf("%s spells %q instead of using handlers.InternalTokenEnv; the parent sets it and the child reads it, so a rename that moves one side silently untrusts every proxy hop", file.Name, InternalTokenEnv)
	}
	if !declared {
		t.Fatalf("%s no longer declares the literal %q; re-point this census at its new home", declaration, InternalTokenEnv)
	}
}
