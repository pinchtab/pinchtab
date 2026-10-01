package fileout

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// Two writes of the same auto-built name must land on different paths with both files
// intact. The name comes from a second-resolution timestamp, so without the exclusive
// create the second write destroys the first.
func TestWritingTheSameNameTwiceKeepsBothFiles(t *testing.T) {
	dir := t.TempDir()
	first := []byte("first payload, deliberately longer than the second")
	second := []byte("second")

	firstPath, err := WriteUnique(dir, "capture-20260101-120000", ".jpg", first)
	if err != nil {
		t.Fatal(err)
	}
	secondPath, err := WriteUnique(dir, "capture-20260101-120000", ".jpg", second)
	if err != nil {
		t.Fatal(err)
	}

	if firstPath == secondPath {
		t.Fatalf("both writes landed on %s; the first file was destroyed", firstPath)
	}
	if got := filepath.Base(secondPath); got != "capture-20260101-120000-1.jpg" {
		t.Errorf("second path = %s, want the suffix before the extension", got)
	}
	assertContents(t, firstPath, first)
	assertContents(t, secondPath, second)
}

// The suffix goes before the extension so the file is still recognised by its type. A
// naive append would produce capture-<ts>.jpg-1, which no image viewer opens.
func TestTheSuffixLandsBeforeTheExtension(t *testing.T) {
	dir := t.TempDir()
	for i, want := range []string{"page-x.pdf", "page-x-1.pdf", "page-x-2.pdf"} {
		got, err := WriteUniquePath(filepath.Join(dir, "page-x.pdf"), []byte("pdf"))
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(got) != want {
			t.Errorf("write %d landed on %s, want %s", i, filepath.Base(got), want)
		}
	}
}

// The handle is returned open on purpose: a stat-then-create leaves a window in which a
// second caller takes the name between the check and the write.
func TestTheReservationHoldsTheNameWhileTheFirstHandleIsStillOpen(t *testing.T) {
	dir := t.TempDir()

	first, firstPath, err := CreateUnique(dir, "rec", ".gif")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()

	second, secondPath, err := CreateUnique(dir, "rec", ".gif")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()

	if firstPath == secondPath {
		t.Fatalf("both reservations returned %s while the first handle was still open", firstPath)
	}
}

// Exhaustion is a refusal, not a fallback to overwriting: a directory holding this many
// files for one second is a runaway caller.
func TestAFullDirectoryIsRefusedRatherThanOverwritten(t *testing.T) {
	dir := t.TempDir()
	for i := range MaxUniqueNameAttempts {
		name := "full.bin"
		if i > 0 {
			name = strings.Replace(name, ".bin", "-"+strconv.Itoa(i)+".bin", 1)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := CreateUnique(dir, "full", ".bin"); err == nil {
		t.Fatal("CreateUnique succeeded with every name taken; it must refuse rather than overwrite")
	}
}

// ReservePath exists for a caller that renames an already-written file into place. The
// placeholder must hold the name against a second reserver, and a rename over it must
// still land the real bytes.
func TestReservePathHoldsTheNameAndSurvivesARenameOverIt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "recording-20260101-120000.gif")

	firstReserved, err := ReservePath(target)
	if err != nil {
		t.Fatal(err)
	}
	secondReserved, err := ReservePath(target)
	if err != nil {
		t.Fatal(err)
	}
	if firstReserved == secondReserved {
		t.Fatalf("both reservations returned %s; the placeholder did not hold the name", firstReserved)
	}

	source := filepath.Join(dir, "server-encoded.gif")
	payload := []byte("the encoded recording")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, firstReserved); err != nil {
		t.Fatalf("rename over our own reservation failed: %v", err)
	}
	assertContents(t, firstReserved, payload)
}

func assertContents(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path) // #nosec G304 -- path produced by this test.
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s holds %q, want %q", path, got, want)
	}
}

// A reservation that cannot be closed must not survive: the caller gets an error and
// would otherwise be left with a 0-byte file wearing the output's name — the same
// defect the write path removes one layer down. Pinned through the creator seam,
// because a real close failure needs a full or revoked filesystem.
func TestReserveUniqueRemovesThePlaceholderWhenTheCloseFails(t *testing.T) {
	dir := t.TempDir()
	created := ""
	closedHandle := func(dir, base, ext string) (*os.File, string, error) {
		f, path, err := CreateUnique(dir, base, ext)
		if err != nil {
			return nil, "", err
		}
		if err := f.Close(); err != nil {
			return nil, "", err
		}
		created = path
		return f, path, nil
	}

	path, err := reserveUnique(dir, "rec_20260101_000000", ".gif", closedHandle)
	if err == nil {
		t.Fatal("precondition: closing an already-closed handle must fail")
	}
	if path != "" {
		t.Errorf("reserveUnique returned %q alongside its error; a failed reservation has no path to report", path)
	}
	if created == "" {
		t.Fatal("the stub creator never ran, so this guard checked nothing")
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("%s survived a failed close (stat err = %v); the placeholder must be removed", created, err)
	}
}

// ReservePath is a shape over ReserveUnique, not a second implementation: it must hand
// back a real reservation, and the numbering must land before the extension exactly as
// the three-part form does.
func TestReservePathDelegatesToTheThreePartForm(t *testing.T) {
	dir := t.TempDir()

	first, err := ReservePath(filepath.Join(dir, "rec_20260101_000000.gif"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReservePath(filepath.Join(dir, "rec_20260101_000000.gif"))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("both reservations returned %q", first)
	}
	if want := filepath.Join(dir, "rec_20260101_000000-1.gif"); second != want {
		t.Errorf("second reservation = %q, want %q", second, want)
	}
	for _, path := range []string{first, second} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%q was returned but not reserved on disk: %v", path, err)
		}
	}
}

// The whole-path form is a SHAPE over the three-part one, not a second implementation:
// two copies of the close-and-remove pair is how the rule drifted out of one of them
// before. Behavioural tests cannot see the difference — a copy passes them — so this is
// a structural check on the delegation itself.
//
// The declaration is located anywhere in the package and each banned call is judged by
// whether it lies INSIDE that declaration, so moving ReservePath to a sibling file keeps
// the rule rather than reporting the function missing. Non-vacuity rests on the delegation
// assertion, not on a match count: a ReservePath that stops calling ReserveUnique reds by
// name here rather than fataling as an empty scan.
//
// There is deliberately no ban on Close: a method call resolves as "<receiver>.Close", so a
// ban naming Close alone is blind to every real spelling of it, and the only handle
// ReservePath could close is one CreateUnique gave it — which is banned below.
func TestReservePathHoldsNoSecondImplementation(t *testing.T) {
	pkg := srccensus.Load(t, ".", fileoutSourceFileFloor)

	fn, ok := pkg.Func("ReservePath")
	if !ok {
		t.Fatalf("ReservePath is not declared anywhere in %s; this guard is pinned to a function that no longer exists — re-point it at whatever replaced it rather than deleting it", pkg.Dir())
	}

	delegates := false
	for _, site := range pkg.CallsAllowingNone("ReserveUnique") {
		if pkg.Contains(fn, site) {
			delegates = true
		}
	}
	if !delegates {
		t.Errorf("%s (%s:%d) no longer calls ReserveUnique; the reserve-and-close sequence has one implementation and this is meant to be a shape over it", fn.Name, fn.File, fn.Line)
	}

	for _, banned := range []string{"CreateUnique", "os.Remove"} {
		for _, site := range pkg.CallsAllowingNone(banned) {
			if pkg.Contains(fn, site) {
				t.Errorf("%s lies inside ReservePath; that is a second copy of the reserve-and-close sequence, which is how its removal went missing before — delegate to ReserveUnique instead", site)
			}
		}
	}
}

// The package holds one production file today; the floor exists so a scan that stopped
// reading it fails instead of passing over nothing.
const fileoutSourceFileFloor = 1

// TestWriteUniqueRemovesTheFileItCreatedWhenTheWriteFails pins the removal through
// the creator seam: the stub creates a REAL file and hands back a closed handle, so
// the write fails for a reason the caller cannot control and the file it left is the
// thing under test. Without the removal the caller gets an error plus a 0-byte file
// wearing the output's name.
func TestWriteUniqueRemovesTheFileItCreatedWhenTheWriteFails(t *testing.T) {
	dir := t.TempDir()
	created := ""
	closedHandle := func(dir, base, ext string) (uniqueFile, string, error) {
		f, path, err := CreateUnique(dir, base, ext)
		if err != nil {
			return nil, "", err
		}
		if err := f.Close(); err != nil {
			return nil, "", err
		}
		created = path
		return f, path, nil
	}

	path, err := writeUnique(dir, "capture-20260101_000000", ".png", []byte("bytes"), closedHandle)
	if err == nil {
		t.Fatal("precondition: writing through a closed handle must fail")
	}
	if path != "" {
		t.Errorf("WriteUnique returned %q alongside its error; a failed write has no path to report", path)
	}
	if created == "" {
		t.Fatal("the stub creator never ran, so this guard checked nothing")
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("%s survived a failed write (stat err = %v); the created file must be removed", created, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Errorf("a failed write left %s in the output directory", entry.Name())
	}
}

// writeThenFailToClose is the handle a real file cannot be: its Write succeeds and its
// Close fails. That pairing is what reaches writeUnique's close branch, and it is the
// production shape too — delayed allocation on a full filesystem, or NFS flushing at
// close. A closed *os.File cannot stand in, because os.File.Write validates the
// descriptor before it looks at the buffer and so fails one branch earlier.
type writeThenFailToClose struct {
	writeCalls int
	writeN     int
	writeErr   error
	closeCalls int
}

func (f *writeThenFailToClose) Write(p []byte) (int, error) {
	f.writeCalls++
	f.writeN = len(p)
	f.writeErr = nil
	return len(p), nil
}

func (f *writeThenFailToClose) Close() error {
	f.closeCalls++
	return errors.New("close: no space left on device")
}

// The sibling of TestReserveUniqueRemovesThePlaceholderWhenTheCloseFails, for the
// branch that had no test: a write that lands and a close that fails still leaves a
// created file behind, wearing the output's name and holding bytes the caller was told
// were not written.
func TestWriteUniqueRemovesTheFileItCreatedWhenTheCloseFails(t *testing.T) {
	dir := t.TempDir()
	created := ""
	handle := &writeThenFailToClose{}
	uncloseable := func(dir, base, ext string) (uniqueFile, string, error) {
		f, path, err := CreateUnique(dir, base, ext)
		if err != nil {
			return nil, "", err
		}
		if err := f.Close(); err != nil {
			return nil, "", err
		}
		created = path
		return handle, path, nil
	}

	buf := []byte("bytes")
	path, err := writeUnique(dir, "capture-20260101_000000", ".png", buf, uncloseable)

	if err == nil {
		t.Fatal("precondition: a handle whose Close fails must make writeUnique fail")
	}
	if !strings.Contains(err.Error(), "no space left on device") {
		t.Fatalf("writeUnique returned %v, not the close failure — this test would be passing through the write branch, which its sibling already covers", err)
	}
	// The write ran AND succeeded, which is what proves the close branch was reached
	// rather than the write branch next door.
	if handle.writeCalls != 1 || handle.writeErr != nil || handle.writeN != len(buf) {
		t.Fatalf("stub write: calls=%d n=%d err=%v; want exactly one successful write of %d bytes, or this test is not reaching the close branch", handle.writeCalls, handle.writeN, handle.writeErr, len(buf))
	}
	if handle.closeCalls != 1 {
		t.Errorf("stub close called %d times, want 1", handle.closeCalls)
	}
	if path != "" {
		t.Errorf("writeUnique returned %q alongside its error; a failed close has no path to report", path)
	}
	if created == "" {
		t.Fatal("the stub creator never ran, so this guard checked nothing")
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("%s survived a failed close (stat err = %v); the created file must be removed", created, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Errorf("a failed close left %s in the output directory", entry.Name())
	}
}
