package fileout

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// This census lives beside the rule it defends, and it walks the MODULE. Two
// directory-scoped censuses stood here before — one in internal/handlers, one in
// internal/cli/actions — each reading os.ReadDir(".") with its own accounted map. Between
// them they covered the two packages that had the defect and nothing else, so the next
// auto-naming site, in any third package, would have shipped unseen. Walking from the
// module root is the same fix the isolated-world boundary census in internal/bridge got
// after the same blind spot. srccensus.Tree owns the enumeration — the rule stays a
// literal scan over each file's text, but the walk exclusions (including the
// nested-checkout skip a hand-rolled name list missed) are inherited, not remembered.

// timestampLayouts are the second-resolution spellings this census recognises. A stamp is
// what makes two outputs in the same second collide, so the layout is the signal.
//
// STATED BLIND SPOT, because a census that reads as module-wide while checking one
// spelling is worse than one whose limits are written down: colon-bearing layouts
// (15:04:05) and named constants (time.RFC3339, time.DateTime) are NOT scanned. Colons
// are illegal in Windows filenames and vanishingly rare in output names, and the named
// constants are overwhelmingly used to format timestamps for display, where they would
// produce noise on every log line. A filename built from either is an unguarded site.
//
// Coarser-than-second stamps are unscanned too, and this is the limit a reader is most
// likely to guess wrong: a date-only name (Format("20060102")) collides between any two
// outputs on the same DAY, so it is worse than the shapes above rather than safer. It is
// left out because 20060102 opens every layout listed here and appears throughout code
// that names no file, so scanning it would cry wolf everywhere. Nothing in the module
// builds one today; the next one is unguarded.
var timestampLayouts = []string{"150405", "15-04-05", "15_04_05", "15.04.05"}

// autoNamingSites maps a MODULE-RELATIVE path to why its timestamp cannot destroy an
// earlier output. Package-qualified, so one map serves the whole module and a reader can
// see which package each decision belongs to. Every entry is a boundary decision, which
// is why each carries its reason rather than just a path.
//
// The KEYS are checked in both directions; a reason's TEXT is only printed. It is
// documentation for the next reader, not a check, so it rots silently unless someone
// updating a site updates it here too — this entry credited a helper deleted a card ago.
var autoNamingSites = map[string]string{
	"internal/handlers/binary_export.go": "exportTimestamp only builds the base name; fileout.WriteUnique reserves the path and writes it",
	"internal/handlers/record_handlers.go": "the recording base name is reserved by " +
		"fileout.ReserveUnique before it is returned",
	"internal/handlers/memory.go":         "the heap snapshot path is reserved by fileout.ReserveUnique before the stream writes it",
	"internal/cli/actions/output_file.go": "autoNamedOutputPath only builds the name; callers reserve it via writeOutputFile or fileout.ReservePath",

	// The two developer-tool artefact writers, accounted DELIBERATELY rather than fixed.
	// They are benchmark outputs a human runs one at a time, their paths are also
	// caller-specifiable, and comparison scripts glob for the exact names — so routing
	// them through fileout would start appending -1 suffixes to filenames those scripts
	// expect. Losing one benchmark CSV to a same-second collision is artefact loss in a
	// tool someone is watching, which is not the same severity as a server destroying a
	// capture it just reported to a caller. Changing that is its own card.
	"tests/tools/runner/internal/bench/browserbench.go": "developer benchmark artefact; path is caller-specifiable and globbed by comparison scripts",
	"tests/tools/runner/internal/bench/setup.go":        "developer benchmark artefact; path is caller-specifiable and globbed by comparison scripts",

	// These two do not name a file at all — the stamp is a value INSIDE the output. They
	// are listed because the scan cannot tell a filename from a field, and saying so is
	// the point of carrying reasons.
	"tests/tools/runner/internal/stealth/compare.go":  "the stamp is a run id printed inside a markdown report, not a filename",
	"tests/tools/runner/internal/opt/mergereports.go": "the stamp is a JSON field value in the merged report, not a filename",
}

// The census fails BOTH ways. An unaccounted site reds, so a new one cannot land quietly;
// and an accounted entry whose stamp is gone reds too, so the recorded boundary cannot
// drift from the real one — a stale entry is how a site that stopped reserving hides.
func TestNoPackageAutoNamesAFileItThenOverwrites(t *testing.T) {
	// Tree's keys are module-relative slash paths — the shape autoNamingSites was
	// already keyed on, so the accounted map is unchanged. The vacuity floor the
	// scanned counter carried moves to Tree's minFiles at the same value.
	files := srccensus.Tree(t, filepath.Join("..", ".."), 200)

	found := map[string]bool{}
	for _, file := range files {
		if buildsASecondResolutionStamp(file.Text) {
			found[file.Name] = true
		}
	}

	if len(found) == 0 {
		t.Fatalf("found no file building a second-resolution timestamp anywhere in the module; either every auto-named output is gone or the layouts this census recognises (%s) no longer appear — re-point it at whatever replaced them rather than deleting it",
			strings.Join(timestampLayouts, ", "))
	}

	for path := range found {
		if autoNamingSites[path] == "" {
			t.Errorf("%s builds a name from a second-resolution timestamp and is not accounted for; route it through fileout (WriteUnique, WriteUniquePath, ReserveUnique or ReservePath) or add it to autoNamingSites with the reason it cannot collide.\n\tRecognised layouts: %s",
				path, strings.Join(timestampLayouts, ", "))
		}
	}
	for path, reason := range autoNamingSites {
		if reason == "" {
			t.Errorf("%s is accounted for with no reason; every entry records a boundary decision", path)
		}
		if !found[path] {
			t.Errorf("%s is accounted for but the walk found no second-resolution timestamp in it; the file moved, was renamed, or stopped building a timestamped name — drop or re-point its entry so the census keeps meaning what it says", path)
		}
	}
}

func buildsASecondResolutionStamp(body string) bool {
	for _, layout := range timestampLayouts {
		if strings.Contains(body, layout) {
			return true
		}
	}
	return false
}
