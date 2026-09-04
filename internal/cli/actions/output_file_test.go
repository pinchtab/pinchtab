package actions

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// Every auto-named CLI output builds its name from a second-resolution timestamp, so a
// run whose name is already taken used to destroy the file holding it — silently, while
// the command printed that path as if it held the new bytes. Driven through the real
// commands rather than the helper, because the defect was as much in what each site
// PRINTS as in what it writes.
//
// The collision is FORCED rather than hoped for. This test used to perform two runs back
// to back and assume they landed in one second; when they straddled a boundary the names
// differed, no collision occurred, and correct code reported "0 collision suffixes" — a
// red that pointed at the collision rule while the clock was to blame. Pre-creating one
// name would only move that race earlier, since the name would come from the test's own
// clock. So every name the runs could reach inside a window of consecutive seconds is
// taken first, and a run landing outside that window is reported as what it is.
func TestARunWhoseAutoNameIsTakenSuffixesAndPrintsTheNameItUsed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prefix string
		ext    string
		run    func(t *testing.T) string
	}{
		{
			name:   "capture",
			prefix: "capture-",
			ext:    ".jpg",
			run: func(t *testing.T) string {
				m := newMockServer()
				defer m.close()
				m.response = `{"status":"ok","image":{"format":"jpeg","base64":"aW1n"}}`
				cmd := captureCmd()
				return captureStdout(t, func() { Capture(m.server.Client(), m.base(), "", cmd) })
			},
		},
		{
			name:   "pdf",
			prefix: "page-",
			ext:    ".pdf",
			run: func(t *testing.T) string {
				m := newMockServer()
				defer m.close()
				m.response = "%PDF-1.4 fake"
				cmd := pdfCmd()
				return captureStdout(t, func() { PDF(m.server.Client(), m.base(), "", cmd) })
			},
		},
		{
			name:   "screenshot",
			prefix: "screenshot-",
			ext:    ".jpg",
			run: func(t *testing.T) string {
				m := newMockServer()
				defer m.close()
				m.response = "rawimagebytes"
				cmd := screenshotCmd()
				return captureStdout(t, func() { Screenshot(m.server.Client(), m.base(), "", cmd) })
			},
		},
		{
			name:   "screenshot --annotate",
			prefix: "screenshot-",
			ext:    ".jpg",
			run: func(t *testing.T) string {
				m := newMockServer()
				defer m.close()
				m.response = `{"format":"jpeg","base64":"aW1n","annotations":[]}`
				cmd := screenshotCmd()
				_ = cmd.Flags().Set("annotate", "true")
				return captureStdout(t, func() { Screenshot(m.server.Client(), m.base(), "", cmd) })
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := chdirTemp(t)
			taken := takeEveryNameInWindow(t, dir, tc.prefix, tc.ext)

			firstOut := tc.run(t)
			secondOut := tc.run(t)

			// Diversion, not coexistence: the files that were already there must still hold
			// their own bytes. Files existing proves nothing if one of them was rewritten,
			// so this is asked first — it is the defect the collision rule exists to stop.
			for name := range taken {
				got, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- path built by this test.
				if err != nil {
					t.Fatalf("reserved %s disappeared: %v", name, err)
				}
				if string(got) != reservedContents {
					t.Errorf("reserved %s holds %q, want %q — a run wrote over a name it should have stepped around", name, got, reservedContents)
				}
			}

			// Each run must print the path it actually used. Printing the name it BUILT
			// would have both runs name a file neither of them holds — the taken one.
			printed := []string{
				printedName(t, firstOut, tc.prefix, tc.ext),
				printedName(t, secondOut, tc.prefix, tc.ext),
			}
			if printed[0] == printed[1] {
				t.Fatalf("both runs printed %s, so one of them named a file it did not write", printed[0])
			}
			for i, name := range printed {
				// The suffix is the evidence the collision branch ran, and the base it hangs
				// off says which second the run used. A name from outside the reserved
				// window means the runs took longer than the window — a clock result, not a
				// collision-rule failure — so it is reported as that and nothing else.
				base, suffixed := splitCollisionSuffix(t, name, tc.ext)
				if !taken[base+tc.ext] {
					t.Fatalf("run %d printed %s, whose second is outside the window this test reserved (%v): the runs left the window, so the collision branch never ran — this is a clock result, not a collision-rule failure",
						i, name, sortedNames(taken))
				}
				if !suffixed {
					t.Errorf("run %d printed %s with no collision suffix although that name was already taken; the run overwrote a file or invented a name", i, name)
				}
				if info, err := os.Stat(filepath.Join(dir, name)); err != nil || info.Size() == 0 {
					t.Errorf("run %d printed %s, which is missing or empty: %v", i, name, err)
				}
			}
		})
	}
}

// The boundary the parent card pinned: a path the user typed is written as they typed
// it. Overwriting there is their instruction, not a collision to resolve — and a
// suffixed name would break a script that goes on to read the path it passed.
func TestAnExplicitOutputPathStillOverwrites(t *testing.T) {
	dir := chdirTemp(t)
	target := filepath.Join(dir, "chosen.jpg")
	if err := os.WriteFile(target, []byte("stale bytes from an earlier run"), 0600); err != nil {
		t.Fatal(err)
	}

	m := newMockServer()
	defer m.close()
	m.response = `{"status":"ok","image":{"format":"jpeg","base64":"aW1n"}}`
	cmd := captureCmd()
	_ = cmd.Flags().Set("output", target)

	captureStdout(t, func() { Capture(m.server.Client(), m.base(), "", cmd) })

	got, err := os.ReadFile(target) // #nosec G304 -- path built by this test.
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "img" {
		t.Errorf("explicit path holds %q, want the new bytes", got)
	}
	if extra := autoNamedFiles(t, dir, "chosen", ".jpg"); len(extra) != 1 {
		t.Errorf("explicit path produced %v; it must not gain a suffix", extra)
	}
}

// The module-wide census for this rule lives in internal/fileout
// (TestNoPackageAutoNamesAFileItThenOverwrites): one accounted map for the whole module,
// so a site in a package neither of the two old directory-scoped censuses read cannot
// land unseen.

// printedName pulls the output filename out of whatever sentence a site wraps it in —
// each of the four phrases it differently, and the assertion is about the name.
func printedName(t *testing.T, out, prefix, ext string) string {
	t.Helper()
	for _, field := range strings.Fields(out) {
		// record prints an absolute path, the other four a bare name; the assertion is
		// about which file was named, so compare the basename either way.
		if name := filepath.Base(field); strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ext) {
			return name
		}
	}
	t.Fatalf("output %q names no %s*%s file", strings.TrimSpace(out), prefix, ext)
	return ""
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	// t.TempDir can hand back a symlinked path (/var vs /private/var on darwin), which
	// would make the printed path and the listed name disagree for reasons unrelated to
	// the guard.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func autoNamedFiles(t *testing.T, dir, prefix, ext string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ext) {
			names = append(names, entry.Name())
		}
	}
	return names
}

func captureCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("format", "", "")
	cmd.Flags().String("quality", "", "")
	cmd.Flags().String("selector", "", "")
	cmd.Flags().String("filter", "", "")
	cmd.Flags().String("depth", "", "")
	cmd.Flags().String("wait", "", "")
	cmd.Flags().Bool("beyond-viewport", false, "")
	cmd.Flags().String("scale", "", "")
	cmd.Flags().Bool("require-pair", false, "")
	cmd.Flags().Bool("with-bounds", true, "")
	cmd.Flags().String("tab", "", "")
	return cmd
}

func pdfCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("landscape", false, "")
	cmd.Flags().String("scale", "", "")
	cmd.Flags().String("paper-width", "", "")
	cmd.Flags().String("paper-height", "", "")
	cmd.Flags().String("margin-top", "", "")
	cmd.Flags().String("margin-bottom", "", "")
	cmd.Flags().String("margin-left", "", "")
	cmd.Flags().String("margin-right", "", "")
	cmd.Flags().String("page-ranges", "", "")
	cmd.Flags().Bool("prefer-css-page-size", false, "")
	cmd.Flags().Bool("display-header-footer", false, "")
	cmd.Flags().String("header-template", "", "")
	cmd.Flags().String("footer-template", "", "")
	cmd.Flags().Bool("generate-tagged-pdf", false, "")
	cmd.Flags().Bool("generate-document-outline", false, "")
	cmd.Flags().Bool("file-output", false, "")
	cmd.Flags().String("path", "", "")
	return cmd
}

func screenshotCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "", "")
	cmd.Flags().Bool("annotate", false, "")
	cmd.Flags().String("format", "", "")
	cmd.Flags().String("quality", "", "")
	cmd.Flags().String("selector", "", "")
	cmd.Flags().String("scale", "", "")
	cmd.Flags().Bool("beyond-viewport", false, "")
	cmd.Flags().String("tab", "", "")
	return cmd
}

// recordStopBody renders a /record/stop response carrying a path.
//
// Built with the JSON encoder rather than string concatenation. A Windows path
// is full of backslashes, and pasted raw between quotes they read as JSON
// escapes: "C:\Users\Temp\..." offers \U and \T, neither of which is one. The
// body was then unparseable, RecordStop took its "cannot decode" branch and
// returned before reaching the rename these tests exist to exercise, and the
// failure surfaced as a missing output file rather than as bad test data.
func recordStopBody(t *testing.T, path string) string {
	t.Helper()
	body, err := json.Marshal(recordStopResponse(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// recordStopBodyOrPanic is recordStopBody for the child process, which has no
// *testing.T to fail through.
func recordStopBodyOrPanic(path string) string {
	body, err := json.Marshal(recordStopResponse(path))
	if err != nil {
		panic(err)
	}
	return string(body)
}

func recordStopResponse(path string) map[string]any {
	return map[string]any{"status": "ok", "path": path, "frames": 3}
}

// The record site cannot use the writer form: the bytes are already on disk under the
// server's name, so it reserves and renames. Two stops in one second must therefore
// still keep both recordings and print the name each one took.
func TestTwoRecordStopsInOneSecondKeepBothRecordings(t *testing.T) {
	dir := chdirTemp(t)
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	stop := func(encoded string) string {
		serverPath := filepath.Join(dir, encoded)
		if err := os.WriteFile(serverPath, []byte("encoded "+encoded), 0600); err != nil {
			t.Fatal(err)
		}
		m := newMockServer()
		defer m.close()
		m.setResponse("POST", "/record/stop", 200, recordStopBody(t, serverPath))
		m.setResponse("GET", "/record/status", 200, `{"state":"finished"}`)
		return captureStdout(t, func() { RecordStop(m.server.Client(), m.base(), "") })
	}

	firstOut := stop("srv-a.gif")
	secondOut := stop("srv-b.gif")

	written := autoNamedFiles(t, dir, "recording-", ".gif")
	if len(written) != 2 {
		t.Fatalf("two stops left %v; the second rename destroyed the first recording", written)
	}
	for _, name := range written {
		assertNonEmpty(t, filepath.Join(dir, name))
	}

	first := printedName(t, firstOut, "recording-", ".gif")
	second := printedName(t, secondOut, "recording-", ".gif")
	if first == second {
		t.Fatalf("both stops reported %s, but two files were written (%v)", first, written)
	}
	if !strings.HasSuffix(strings.TrimSuffix(second, ".gif"), "-1") {
		t.Errorf("second stop reported %s, want the collision suffix it actually used", second)
	}
}

// A reservation is a side effect: the name is claimed before the rename, so a rename
// that fails must release it rather than leave an empty file wearing an output's name —
// zero bytes under recording-<ts>.gif reads as a recording that was made.
//
// Driven in a child process because the failure path is cli.Fatal, which calls os.Exit.
func TestAFailedRenameReleasesTheReservation(t *testing.T) {
	if dir := os.Getenv(recordFatalChildEnv); dir != "" {
		runFailedRenameChild(dir)
		return
	}

	dir := t.TempDir()
	child := exec.Command(os.Args[0], "-test.run=TestAFailedRenameReleasesTheReservation", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
	child.Env = append(os.Environ(), recordFatalChildEnv+"="+dir)
	raw, err := child.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("child exited with %v, want the failed rename's os.Exit(1); output:\n%s", err, raw)
	}
	if left := autoNamedFiles(t, dir, "recording-", ".gif"); len(left) != 0 {
		t.Errorf("the failed stop left %v behind; an abandoned reservation is an empty file wearing an output's name", left)
	}
}

const recordFatalChildEnv = "PINCHTAB_TEST_RECORD_FATAL_DIR"

func runFailedRenameChild(dir string) {
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state")); err != nil {
		panic(err)
	}

	missing := filepath.Join(dir, "never-encoded.gif")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/record/stop" {
			_, _ = w.Write([]byte(recordStopBodyOrPanic(missing)))
			return
		}
		_, _ = w.Write([]byte(`{"state":"finished"}`))
	}))
	defer srv.Close()

	RecordStop(srv.Client(), srv.URL, "")
}

func assertNonEmpty(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Errorf("%s is empty", path)
	}
}

// reservedContents is what the names this test takes hold before the runs, so a run that
// overwrote one instead of stepping around it is visible in the file rather than only in
// a count of how many files exist.
const reservedContents = "bytes an earlier run left under this name"

// autoNameWindowSeconds is how many consecutive seconds of candidate names are reserved.
// One name is not enough: the run would have to land in the exact second the test chose,
// which is the coin flip this test was rewritten to remove. A window makes the collision
// certain for any run that starts within it, and leaving it is reported as a clock result.
const autoNameWindowSeconds = 5

// takeEveryNameInWindow creates the auto-name a run would build in each of the next few
// seconds, so whichever second a run lands in, its name is already taken and it must
// suffix. Returns the set of names it created.
func takeEveryNameInWindow(t *testing.T, dir, prefix, ext string) map[string]bool {
	t.Helper()
	taken := map[string]bool{}
	start := time.Now()
	for i := 0; i < autoNameWindowSeconds; i++ {
		name := prefix + start.Add(time.Duration(i)*time.Second).Format("20060102-150405") + ext
		if err := os.WriteFile(filepath.Join(dir, name), []byte(reservedContents), 0600); err != nil {
			t.Fatal(err)
		}
		taken[name] = true
	}
	return taken
}

// autoName matches an auto-built output name: a prefix, the 20060102-150405 timestamp
// every site formats, and the optional -N a collision appends. The timestamp carries its
// own hyphen and its time half is all digits, so "the last hyphen-separated number" reads
// every uncollided name as suffixed — the shape has to be matched, not scanned backwards.
var autoName = regexp.MustCompile(`^(.*-\d{8}-\d{6})(?:-(\d+))?$`)

// splitCollisionSuffix returns the name without its extension or -N, and whether a
// collision suffix was there at all.
func splitCollisionSuffix(t *testing.T, name, ext string) (base string, suffixed bool) {
	t.Helper()
	m := autoName.FindStringSubmatch(strings.TrimSuffix(name, ext))
	if m == nil {
		t.Fatalf("%s is not an auto-built name, so this test cannot say whether it collided", name)
	}
	return m[1], m[2] != ""
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// The other half of the rule, and the half the forced collision above cannot see: a name
// that is free is used as built. Without this, a run that suffixed unconditionally would
// satisfy every assertion above.
func TestARunWhoseAutoNameIsFreeUsesItUnsuffixed(t *testing.T) {
	dir := chdirTemp(t)

	m := newMockServer()
	defer m.close()
	m.response = `{"status":"ok","image":{"format":"jpeg","base64":"aW1n"}}`
	out := captureStdout(t, func() { Capture(m.server.Client(), m.base(), "", captureCmd()) })

	name := printedName(t, out, "capture-", ".jpg")
	if _, suffixed := splitCollisionSuffix(t, name, ".jpg"); suffixed {
		t.Errorf("a run in an empty directory printed %s; a collision suffix on a free name is not a collision", name)
	}
	if written := autoNamedFiles(t, dir, "capture-", ".jpg"); len(written) != 1 || written[0] != name {
		t.Errorf("the run printed %s but the directory holds %v", name, written)
	}
}
