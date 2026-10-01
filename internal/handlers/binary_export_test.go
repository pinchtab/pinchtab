package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// Two saves in immediate succession must land on different paths with both files intact.
// The name came from a second-resolution timestamp and was written with no existence
// check, so the first file was destroyed while its response still reported that path,
// its byte count and its clip rect.
func TestSavingTwiceBackToBackKeepsBothFiles(t *testing.T) {
	stateDir := t.TempDir()

	first := []byte("first capture bytes, deliberately longer than the second")
	second := []byte("second")

	firstPath, _, err := saveBinaryToStateDir(stateDir, "captures", "cap", ".png", first)
	if err != nil {
		t.Fatal(err)
	}
	secondPath, _, err := saveBinaryToStateDir(stateDir, "captures", "cap", ".png", second)
	if err != nil {
		t.Fatal(err)
	}

	if firstPath == secondPath {
		t.Fatalf("both saves returned %q; the second overwrote the first", firstPath)
	}
	// Each reported path must hold its OWN bytes: a distinct path is worthless if the
	// content moved.
	assertFileHolds(t, firstPath, first)
	assertFileHolds(t, secondPath, second)
}

// Issued concurrently, not serially: a serial two-call test passes on a millisecond
// timestamp while concurrent requests still collide, which is why the guarantee has to
// come from an exclusive create rather than from more precision.
func TestConcurrentSavesProduceDistinctFiles(t *testing.T) {
	const n = 8
	stateDir := t.TempDir()

	paths := make([]string, n)
	contents := make([][]byte, n)
	errs := make([]error, n)

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	for i := 0; i < n; i++ {
		// Distinct lengths so a file holding the wrong capture is detectable by size
		// alone, which is the mismatch the card measured.
		contents[i] = []byte(fmt.Sprintf("capture-%d%s", i, string(make([]byte, i))))
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			paths[i], _, errs[i] = saveBinaryToStateDir(stateDir, "captures", "cap", ".png", contents[i])
		}(i)
	}
	start.Done()
	done.Wait()

	seen := map[string]int{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("save %d: %v", i, errs[i])
		}
		if prev, dup := seen[paths[i]]; dup {
			t.Errorf("saves %d and %d both returned %q", prev, i, paths[i])
		}
		seen[paths[i]] = i
		assertFileHolds(t, paths[i], contents[i])
	}
	if len(seen) != n {
		t.Errorf("%d distinct paths for %d concurrent saves", len(seen), n)
	}

	entries, err := os.ReadDir(filepath.Join(stateDir, "captures"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != n {
		t.Errorf("%d files on disk for %d concurrent saves; a save was lost", len(entries), n)
	}
}

// The reported size must match the file at that path — the mismatch a caller storing the
// returned paths would otherwise hit.
func assertFileHolds(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path) // #nosec G304 -- path produced by the code under test.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(got) != len(want) {
		t.Errorf("%s holds %d bytes, want %d — the reported size describes a file that was replaced", path, len(got), len(want))
	}
	if string(got) != string(want) {
		t.Errorf("%s holds another save's content", path)
	}
}

// AC6, recording: recordingsOutputPath's own comment claimed the path was unique while
// nothing checked — two stops in the same second returned one path and the encoder that
// finished second overwrote the first recording. The path is now reserved, so a second
// call cannot return it.
func TestRecordingsOutputPathReservesEachPath(t *testing.T) {
	h := Handlers{
		Config:   &config.RuntimeConfig{StateDir: t.TempDir()},
		recorder: &recorder{},
	}

	first, err := h.recordingsOutputPath()
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.recordingsOutputPath()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("two stops in the same second both got %q", first)
	}
	for _, path := range []string{first, second} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%q was returned but not reserved on disk: %v", path, err)
		}
	}
}

// Reservation is fileout's rule — create exclusively, close, remove the placeholder when
// the close fails — and this package owes only proof that it ROUTES there.
//
// The scope is the PACKAGE, not one file. The guard this replaces opened record_handlers.go
// by name, so network_export.go reserved with no guard at all: the exact sequence the
// reservation fix removed could be reinstated there today with a green suite. Deriving the
// scope covers both sites and every future one for free.
//
// The ban is on the CALL, not on tokens. fileout.CreateUnique hands back an OPEN handle, and
// closing it to reserve a name is the hand-rolled sequence whose removal-on-failed-close went
// missing here; everything in this package reserves through ReserveUnique or writes through
// WriteUnique instead. Banning ".Close()" over a whole package would red on eleven correct
// calls across five files — network_export.go closes the export file it wrote, twice — and a
// census that reds on correct code gets loosened or deleted, which is how the gap came back
// the first time.
func TestEveryReservationInThisPackageRoutesThroughFileout(t *testing.T) {
	pkg := srccensus.Load(t, ".", handlersSourceFileFloor)

	for _, site := range pkg.CallsAllowingNone("fileout.CreateUnique") {
		t.Errorf("%s hands back an OPEN handle, and closing it to reserve a name is the copy whose removal-on-failed-close went missing — call fileout.ReserveUnique, or fileout.WriteUnique to write in one step", site)
	}

	routed := append(
		pkg.CallsAllowingNone("fileout.ReserveUnique"),
		pkg.CallsAllowingNone("fileout.WriteUnique")...,
	)
	if len(routed) < reservationRouteFloor {
		t.Fatalf("found %d fileout reservation calls in %s, want at least %d; the routing this guard defends is gone, so the ban above now guards air — re-point it at whatever replaced it rather than deleting it", len(routed), pkg.Dir(), reservationRouteFloor)
	}
}

const (
	// Well under the real count, so ordinary growth or deletion does not trip it while a
	// scan that stopped seeing most of the package still fails.
	handlersSourceFileFloor = 90
	// One per production reserver: record_handlers, network_export and binary_export.
	reservationRouteFloor = 3
)

// stubExportEncoder is the smallest ExportEncoder writeExportFile will accept, so the
// naming and overwrite policy can be driven without a browser.
type stubExportEncoder struct {
	w    io.Writer
	body string
}

func (s *stubExportEncoder) ContentType() string   { return "application/json" }
func (s *stubExportEncoder) FileExtension() string { return ".har" }
func (s *stubExportEncoder) Start(w io.Writer) error {
	s.w = w
	_, err := io.WriteString(w, s.body)
	return err
}
func (s *stubExportEncoder) Encode(observe.ExportEntry) error { return nil }
func (s *stubExportEncoder) Finish() error                    { return nil }

// AC6, network export: two auto-named exports in the same second used to rename onto one
// file — and share one <path>.tmp while writing, so the loser was corrupt as well as
// lost. AC8 in the same test: a caller ?path= still overwrites, because this fix is about
// generated default names only.
func TestNetworkExportAutoNamesUniquelyButHonoursAnExplicitPath(t *testing.T) {
	stateDir := t.TempDir()
	h := Handlers{Config: &config.RuntimeConfig{StateDir: stateDir}}

	export := func(query, body string) string {
		t.Helper()
		req := httptest.NewRequest("GET", "/network/export?output=file"+query, nil)
		w := httptest.NewRecorder()
		enc := &stubExportEncoder{body: body}
		if err := h.writeExportFile(w, req, enc, "har", func(func(observe.ExportEntry) error) error { return nil }); err != nil {
			t.Fatalf("writeExportFile: %v", err)
		}
		var resp struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse response: %v", err)
		}
		return resp.Path
	}

	firstAuto := export("", "first-export")
	secondAuto := export("", "second")
	if firstAuto == secondAuto {
		t.Fatalf("two auto-named exports both wrote %q", firstAuto)
	}
	assertFileHolds(t, firstAuto, []byte("first-export"))
	assertFileHolds(t, secondAuto, []byte("second"))

	// The explicit path is the boundary this fix must not cross.
	firstNamed := export("&path=chosen.har", "named-one")
	secondNamed := export("&path=chosen.har", "named-two")
	if firstNamed != secondNamed {
		t.Errorf("an explicit ?path= produced two different files (%q then %q); a caller who names a file is entitled to replace it", firstNamed, secondNamed)
	}
	assertFileHolds(t, secondNamed, []byte("named-two"))

	// No stray .tmp left behind by either route.
	entries, err := os.ReadDir(filepath.Join(stateDir, "exports"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("export left a temp file behind: %s", e.Name())
		}
	}
}

// AC6, snapshot: HandleSnapshot's file branch cannot be driven here — TopmostModalNodeID
// needs a real CDP context, so any mocked request 500s before extraction. What IS
// checkable without a browser is that the branch routes the generated name through the
// collision-proof helper while the caller-supplied path still uses a plain write.
func TestSnapshotFileBranchRoutesTheGeneratedNameThroughTheUniqueHelper(t *testing.T) {
	pkg := srccensus.Load(t, ".", 80)
	writer := pkg.MustFunc(t, "writeSnapshotFile")
	src := pkg.Source(writer)

	if !strings.Contains(src, `writeUniqueFile(snapshotDir, "snapshot-"+timestamp, ext, content)`) {
		t.Error("the auto-named snapshot no longer goes through writeUniqueFile, so two snapshots in one second can overwrite again")
	}
	if strings.Contains(pkg.FileSource(writer), "os.WriteFile(") {
		t.Errorf("%s writes a file directly; the caller-named path goes through saveToCallerPath", writer.File)
	}
	explicitBranch := src[strings.Index(src, `if q.outputPath != "" {`):]
	if end := strings.Index(explicitBranch, "\n\t} else {"); end >= 0 {
		explicitBranch = explicitBranch[:end]
	}
	if !strings.Contains(explicitBranch, "saveToCallerPath(h.Config.StateDir, q.outputPath, content)") {
		t.Error("the caller-supplied-path branch no longer goes through saveToCallerPath")
	}
}

func TestSaveToCallerPathOverwritesInsideStateDirAndRefusesEverythingElse(t *testing.T) {
	stateDir := t.TempDir()

	first, status, err := saveToCallerPath(stateDir, "out/page.pdf", []byte("one"))
	if err != nil || status != 200 {
		t.Fatalf("first write: status %d err %v", status, err)
	}
	second, _, err := saveToCallerPath(stateDir, "out/page.pdf", []byte("two"))
	if err != nil || second != first {
		t.Fatalf("second write to the same name: path %q err %v, want overwrite of %q", second, err, first)
	}
	if got, _ := os.ReadFile(first); string(got) != "two" {
		t.Fatalf("file content = %q, want the caller's replacement", got)
	}
	if info, _ := os.Stat(first); info.Mode().Perm() != 0600 {
		t.Fatalf("file mode = %v, want 0600", info.Mode().Perm())
	}

	for _, bad := range []string{"../escape.pdf", "/abs.pdf", ".", ""} {
		if _, status, err := saveToCallerPath(stateDir, bad, []byte("x")); err == nil || status != 400 {
			t.Errorf("saveToCallerPath(%q) = status %d err %v, want a 400 refusal", bad, status, err)
		}
	}
}

// A reserved name must be given back when the stop it was reserved for is refused.
// Reserving creates a real file, so without this every rejected /record/stop leaves a
// 0-byte recording behind — which is how this showed up: a test with an empty StateDir
// started writing into the package directory.
func TestARefusedRecordStopLeavesNoReservedFile(t *testing.T) {
	stateDir := t.TempDir()
	// Screencast on: this pins the reserve-then-release path, which is only reachable
	// once the capability guard has let the request through.
	h := New(&mockBridge{}, &config.RuntimeConfig{StateDir: stateDir, AllowScreencast: true}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/record/stop", nil)
	w := httptest.NewRecorder()
	h.HandleRecordStop(w, req)

	if w.Code != 400 {
		t.Fatalf("precondition: stopping with no active recording must be a 400, got %d", w.Code)
	}
	entries, err := os.ReadDir(filepath.Join(stateDir, "recordings"))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("a refused stop left %s behind", e.Name())
	}
}

// The module-wide census for this rule lives in internal/fileout
// (TestNoPackageAutoNamesAFileItThenOverwrites), beside the owner it defends. The
// package-scoped version that stood here could not see an auto-naming site in any third
// package, which is the coverage the consolidation buys.
