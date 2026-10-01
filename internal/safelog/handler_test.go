package safelog

import (
	"bytes"
	"io/fs"
	"log/slog"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestHandlerRedactsAndSanitizesStringAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(slog.NewJSONHandler(&buf, nil)))

	logger.Info("hello\x1b[31mworld", "token", "secret-token", "path", "/Users/tester/private.txt\x00")

	out := buf.String()
	if strings.Contains(out, "secret-token") {
		t.Fatalf("expected token to be redacted, got %q", out)
	}
	if strings.Contains(out, "\x1b") {
		t.Fatalf("expected ANSI escapes to be stripped, got %q", out)
	}
	if strings.Contains(out, "\x00") {
		t.Fatalf("expected null bytes to be stripped, got %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected redacted marker, got %q", out)
	}
}

func TestHandlerTruncatesOversizedStrings(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&buf, nil)))

	logger.Info("msg", "payload", strings.Repeat("x", MaxStringValueBytes+512))

	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected log output")
	}
	if strings.Contains(out, strings.Repeat("x", MaxStringValueBytes+128)) {
		t.Fatalf("expected oversized value to be truncated, got %q", out)
	}
}

func TestDefaultLevelRecordsRequestsWarningsAndErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewDefaultHandler(&buf))
	SetLevel(DefaultLevel)
	t.Cleanup(func() { SetLevel(DefaultLevel) })

	logger.Debug("chatty detail")
	logger.Info("request", "requestId", "b5fc54c8642370c7", "status", 200)
	logger.Warn("instance stopped deliberately")
	logger.Error("TARGET CRASHED")

	out := buf.String()
	for _, want := range []string{"msg=request", "requestId=b5fc54c8642370c7", "level=WARN", "level=ERROR", "TARGET CRASHED"} {
		if !strings.Contains(out, want) {
			t.Errorf("default level dropped %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "chatty detail") {
		t.Errorf("default level should not record debug lines:\n%s", out)
	}
}

func TestSetLevelAdjustsTheInstalledHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewDefaultHandler(&buf))
	t.Cleanup(func() { SetLevel(DefaultLevel) })

	SetLevel(slog.LevelError)
	logger.Info("request", "requestId", "abc")
	logger.Error("still recorded")
	if strings.Contains(buf.String(), "msg=request") {
		t.Errorf("an explicit error level should drop request lines:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "still recorded") {
		t.Errorf("errors must survive every level:\n%s", buf.String())
	}

	buf.Reset()
	SetLevel(slog.LevelDebug)
	logger.Debug("chatty detail")
	if !strings.Contains(buf.String(), "chatty detail") {
		t.Errorf("debug level should record debug lines:\n%s", buf.String())
	}
	if got := CurrentLevel(); got != slog.LevelDebug {
		t.Errorf("CurrentLevel() = %v, want debug", got)
	}
}

func TestParseLevel(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{in: "", want: DefaultLevel},
		{in: "debug", want: slog.LevelDebug},
		{in: "INFO", want: slog.LevelInfo},
		{in: " warn ", want: slog.LevelWarn},
		{in: "warning", want: slog.LevelWarn},
		{in: "error", want: slog.LevelError},
		{in: "silent", want: DefaultLevel, wantErr: true},
	} {
		got, err := ParseLevel(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseLevel(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// InstallDefault owns the process default logger, and a second owner — the server
// installing a discard handler — is what silenced every default run. RunDashboard starts
// a server, so no unit test reaches that line: the invariant is only checkable
// structurally, and it belongs to the package that holds the rule.
//
// The scope is derived: every package directory under cmd/, internal/ and pkg/, so a new
// installer anywhere in this repo's own source reddens it. tests/, tools/, scripts/,
// plugins/ and docs/examples/ are deliberately out of scope — a harness or example binary
// installing its own logger is legitimate, and walking them means walking node_modules.
func TestOnlySafelogInstallsTheProcessLogger(t *testing.T) {
	installers := []string{}
	scanned := 0
	for _, dir := range sourcePackageDirs(t) {
		pkg := srccensus.Load(t, filepath.Join("..", "..", dir), 1)
		scanned += len(pkg.Files())
		for _, site := range pkg.CallsAllowingNone("slog.SetDefault") {
			installers = append(installers, path.Join(dir, site.File))
		}
	}
	if scanned < minRepoSourceFiles {
		t.Fatalf("censused %d non-test files under cmd/, internal/ and pkg/, want at least %d; the scan lost most of the repo and would pass vacuously", scanned, minRepoSourceFiles)
	}

	want := []string{"internal/safelog/handler.go"}
	if !reflect.DeepEqual(installers, want) {
		t.Errorf("slog.SetDefault callers = %v, want only %v; two writers to the process default logger is what silenced the server — route the new caller through InstallDefault, or if ownership genuinely moved, re-point this census at the new owner rather than deleting it", installers, want)
	}
}

// The discard handler lived in internal/server, and a package-local test there could not
// state why it must never come back: recording a run is safelog's job, not the server's.
func TestTheServerNeverDiscardsLogOutput(t *testing.T) {
	for name, text := range srccensus.Load(t, filepath.Join("..", "server"), minServerSourceFiles).Sources() {
		for i, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "io."+"Discard") {
				t.Errorf("internal/server/%s:%d discards log output; a run must record its requests and errors without a flag, and the level belongs in safelog", name, i+1)
			}
		}
	}
}

const (
	minRepoSourceFiles   = 500
	minServerSourceFiles = 5
)

// sourcePackageDirs is safe from the nested-worktree false positives that hit the
// module-ROOT censuses by SCOPE, not by exclusion: it walks three named subtrees
// (cmd, internal, pkg), and a worktree created at the repo root sits outside all of
// them. That safety is conditional — adding a root here, or a worktree created
// INSIDE one of these subtrees, puts this walk in the vulnerable class, and then the
// enumeration belongs on srccensus like the module-wide censuses.
func sourcePackageDirs(t *testing.T) []string {
	t.Helper()

	dirs := []string{}
	for _, root := range []string{"cmd", "internal", "pkg"} {
		err := filepath.WalkDir(filepath.Join("..", "..", root), func(sourcePath string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(sourcePath, ".go") || strings.HasSuffix(sourcePath, "_test.go") {
				return nil
			}
			dir := path.Clean(filepath.ToSlash(strings.TrimPrefix(filepath.Dir(sourcePath), filepath.Join("..", "..")+string(filepath.Separator))))
			if len(dirs) == 0 || dirs[len(dirs)-1] != dir {
				dirs = append(dirs, dir)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot enumerate %s, so this census would silently skip it: %v", root, err)
		}
	}
	if len(dirs) == 0 {
		t.Fatal("no package directory found under cmd/, internal/ or pkg/; this census is checking nothing")
	}
	return dirs
}
