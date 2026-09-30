package bridge

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

func TestPointerPointForNode_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := bridgecdpops.PointerPointForNode(ctx, 1, true)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

const censusFile = "cdp_geometry_test.go"

// DOM.getBoxModel quads were measured to be VIEWPORT-relative. A comment in cdpops
// still said the shared box-model path was document-relative, one call boundary above
// the facade comment that says the opposite — and a corrected claim next to a stale one
// is how the disproven premise gets reconstructed and a scroll transform reintroduced.
// Deleting it once is not enough; this is what keeps it deleted.
func TestNoCdpopsCommentClaimsTheBoxModelIsDocumentRelative(t *testing.T) {
	const claim = "document-relative"

	var scanned, offenders int
	err := filepath.WalkDir("cdpops", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		body, readErr := os.ReadFile(path) // #nosec G304 -- files walked from this repo's own tree.
		if readErr != nil {
			return readErr
		}
		scanned++
		if strings.Contains(string(body), claim) {
			offenders++
			t.Errorf("%s claims %q; getBoxModel quads are viewport-relative, and the caller already states the space once", path, claim)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("cannot walk cdpops, so this census guards nothing: %v", err)
	}
	if scanned == 0 {
		t.Fatal("scanned no cdpops files; this census would pass vacuously")
	}
	if _, err := os.Stat(censusFile); err != nil {
		t.Fatalf("censusFile %q does not exist, so the floor excludes the wrong file: %v", censusFile, err)
	}
	if offenders == 0 && !phraseStillUsedInBridge(t, claim) {
		t.Errorf("%q appears nowhere under internal/bridge, so this ban no longer bans a phrase anyone writes; re-point it at the wording actually in use", claim)
	}
}

// phraseStillUsedInBridge is the floor: the ban is only meaningful while the phrase is
// still the one the codebase uses for real document space. If a rename retires it, the
// ban silently passes over nothing.
//
// This file is excluded, and that exclusion is the whole point: the census names the
// banned phrase to ban it, so without this it finds its own literal and every floor
// passes — a floor that counts itself is not a floor.
func phraseStillUsedInBridge(t *testing.T, phrase string) bool {
	t.Helper()
	self := filepath.Base(censusFile)
	var found bool
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasPrefix(path, "cdpops/") || filepath.Base(path) == self {
			return nil //nolint:nilerr // an unreadable sibling must not decide the floor
		}
		body, readErr := os.ReadFile(path) // #nosec G304 -- files walked from this repo's own tree.
		if readErr == nil && strings.Contains(string(body), phrase) {
			found = true
		}
		return nil
	})
	return found
}
