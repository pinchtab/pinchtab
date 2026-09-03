package httpx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// The request id is a join key: the middleware stamps it, the proxy forwards it,
// the failure logger keys the cause on it, the activity recorder copies it into
// its event, and the audit log records it. Those five readers only agree while
// they spell the header the same way, and the name was written out twice in this
// package and as a bare literal in three more — so a rename would have moved some
// and not others, and every half fails quietly. A stamp nobody reads logs an empty
// id; a reader whose writer moved on lets the outer chain's id through twice and a
// caller sees two.
//
// So the spelling has one home, and this walks the module rather than a list of
// the packages that happened to have it when the rule was written.
func TestTheRequestIDHeaderIsSpelledInOnePlace(t *testing.T) {
	// The declaration itself is the one site that must contain the literal —
	// checked below rather than exempted, so deleting the constant fails too.
	const declaration = "internal/httpx/httpx.go"

	// A site that legitimately writes the name out, with the reason at the entry.
	// Checked in both directions: an entry naming a file the walk no longer finds,
	// or one that no longer contains the literal, fails as a stale exemption.
	exempt := map[string]string{}

	files := srccensus.Tree(t, filepath.Join("..", ".."), 200)

	declared := false
	var offenders []string
	for _, file := range files {
		if !strings.Contains(file.Text, `"`+RequestIDHeader+`"`) {
			continue
		}
		if file.Name == declaration {
			declared = true
			continue
		}
		if reason, ok := exempt[file.Name]; ok {
			if reason == "" {
				t.Errorf("%s is exempt with no reason recorded", file.Name)
			}
			continue
		}
		offenders = append(offenders, file.Name)
	}

	if !declared {
		t.Fatalf("%s no longer contains the literal %q; if the constant moved, point this census at its new home rather than deleting it", declaration, RequestIDHeader)
	}
	for _, name := range offenders {
		t.Errorf("%s spells %q instead of using httpx.RequestIDHeader; a rename would move this reader and not the others", name, RequestIDHeader)
	}
	for name, reason := range exempt {
		found := false
		for _, file := range files {
			if file.Name == name && strings.Contains(file.Text, `"`+RequestIDHeader+`"`) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is exempt (%s) but no longer spells the literal, so the exemption is stale", name, reason)
		}
	}
}
