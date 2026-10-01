package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/safelog"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestApplyServerAddressFlagsPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		bind     string
		port     string
		wantBind string
		wantPort string
	}{
		{name: "no flags leaves config", wantBind: "127.0.0.1", wantPort: "9867"},
		{name: "port flag wins", port: "9880", wantBind: "127.0.0.1", wantPort: "9880"},
		{name: "bind flag wins", bind: "0.0.0.0", wantBind: "0.0.0.0", wantPort: "9867"},
		{name: "both flags win", bind: "0.0.0.0", port: "9880", wantBind: "0.0.0.0", wantPort: "9880"},
		{name: "blank flags leave config", bind: "   ", port: "  ", wantBind: "127.0.0.1", wantPort: "9867"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.RuntimeConfig{Bind: "127.0.0.1", Port: "9867"}
			applyServerAddressFlags(cfg, tt.bind, tt.port)
			if cfg.Bind != tt.wantBind {
				t.Errorf("Bind = %q, want %q", cfg.Bind, tt.wantBind)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %q, want %q", cfg.Port, tt.wantPort)
			}
		})
	}
}

// The server command must accept the same address overrides as `pinchtab
// bridge`, or the two server modes disagree about how to move off the config
// port — which is what the not-running guidance now tells users to do.
func TestServerCmdAddressFlagsMatchBridge(t *testing.T) {
	for _, name := range []string{"bind", "port"} {
		serverFlag := serverCmd.Flags().Lookup(name)
		if serverFlag == nil {
			t.Fatalf("server has no --%s flag", name)
		}
		bridgeFlag := bridgeCmd.Flags().Lookup(name)
		if bridgeFlag == nil {
			t.Fatalf("bridge has no --%s flag", name)
		}
		if serverFlag.Value.Type() != bridgeFlag.Value.Type() {
			t.Errorf("--%s type = %q, want bridge's %q", name, serverFlag.Value.Type(), bridgeFlag.Value.Type())
		}
		if serverFlag.DefValue != "" {
			t.Errorf("--%s default = %q, want empty so config keeps winning", name, serverFlag.DefValue)
		}
	}
}

// The detached child re-parses its own flags, so an address override applied in
// the parent has to travel with it; otherwise the parent waits on a URL the
// child never binds.
func TestBackgroundServerArgsForwardsAddressFlags(t *testing.T) {
	got := backgroundServerArgs("marker-123", serverBackgroundOptions{
		Bind: "0.0.0.0",
		Port: "9880",
	})
	want := []string{
		"server", "--background-child", "marker-123",
		"--bind", "0.0.0.0",
		"--port", "9880",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backgroundServerArgs() = %#v, want %#v", got, want)
	}
}

func TestApplyLogLevelResolvesTheRunThreshold(t *testing.T) {
	t.Cleanup(func() { safelog.SetLevel(safelog.DefaultLevel) })

	for _, tc := range []struct {
		name    string
		level   string
		verbose bool
		want    slog.Level
	}{
		{name: "no flags keeps the default", want: safelog.DefaultLevel},
		{name: "verbose means debug", verbose: true, want: slog.LevelDebug},
		{name: "explicit level wins over verbose", level: "error", verbose: true, want: slog.LevelError},
		{name: "explicit warn", level: "warn", want: slog.LevelWarn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			safelog.SetLevel(slog.LevelInfo)
			applyLogLevel(&config.RuntimeConfig{LogLevel: tc.level}, tc.verbose)
			if got := safelog.CurrentLevel(); got != tc.want {
				t.Errorf("level = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefaultLogLevelRecordsRequestsAndErrors(t *testing.T) {
	if safelog.DefaultLevel > slog.LevelInfo {
		t.Fatalf("DefaultLevel = %v, want info or lower so request lines survive", safelog.DefaultLevel)
	}
}

// A quiet startup banner and recorded errors must be reachable together: the
// banner is a separate field from the level, and neither flag touches the other.
func TestQuietBannerKeepsDefaultLogging(t *testing.T) {
	t.Cleanup(func() { safelog.SetLevel(safelog.DefaultLevel) })
	safelog.SetLevel(slog.LevelError)

	cfg := &config.RuntimeConfig{VerboseBanner: false}
	applyLogLevel(cfg, false)

	if cfg.VerboseBanner {
		t.Errorf("applyLogLevel must not touch the banner")
	}
	if got := safelog.CurrentLevel(); got != safelog.DefaultLevel {
		t.Errorf("level = %v, want the default with a quiet banner", got)
	}
}

// The three-way precedence: --log-level beats server.logLevel, server.logLevel
// beats the default, and -v beats neither. Assigning the flag unconditionally is
// the bug this pins — it erased the configured level on every flagless run.
func TestLogLevelPrecedenceFlagBeatsConfigBeatsDefault(t *testing.T) {
	t.Cleanup(func() { safelog.SetLevel(safelog.DefaultLevel) })

	for _, tc := range []struct {
		name        string
		configLevel string
		flag        string
		verbose     bool
		want        slog.Level
	}{
		{name: "nothing set is the default", want: safelog.DefaultLevel},
		{name: "config alone is honoured", configLevel: "warn", want: slog.LevelWarn},
		{name: "flag beats config", configLevel: "warn", flag: "debug", want: slog.LevelDebug},
		{name: "blank flag leaves config", configLevel: "warn", flag: "   ", want: slog.LevelWarn},
		{name: "verbose loses to config", configLevel: "warn", verbose: true, want: slog.LevelWarn},
		{name: "verbose loses to the flag", flag: "error", verbose: true, want: slog.LevelError},
		{name: "verbose applies with neither", verbose: true, want: slog.LevelDebug},
	} {
		t.Run(tc.name, func(t *testing.T) {
			safelog.SetLevel(slog.LevelInfo)
			cfg := &config.RuntimeConfig{LogLevel: tc.configLevel}
			resolveLogLevel(cfg, tc.flag, tc.verbose)
			if got := safelog.CurrentLevel(); got != tc.want {
				t.Errorf("level = %v, want %v", got, tc.want)
			}
		})
	}
}

// -v always adds the banner but only raises the level when nothing else set one. That
// is the documented precedence and it stays; the notice is what makes it discoverable
// instead of a silent no-op. It is a diagnostic for a person who typed -v, so every
// other shape — the sources agreeing, -v being the thing that sets the level, a run
// with no -v at all — must stay silent.
func TestVerboseLevelNoticeNamesTheSourceThatWon(t *testing.T) {
	for _, tc := range []struct {
		name        string
		flag        string
		configLevel string
		verbose     bool
		want        string
	}{
		{
			name:        "config beats verbose",
			configLevel: "warn",
			verbose:     true,
			want:        "-v: log level stays warn, set by server.logLevel in the config file; pass --log-level debug to raise it",
		},
		{
			name:    "flag beats verbose",
			flag:    "error",
			verbose: true,
			want:    "-v: log level stays error, set by --log-level; pass --log-level debug to raise it",
		},
		{
			name:        "flag beats config and is named as the source",
			flag:        "error",
			configLevel: "warn",
			verbose:     true,
			want:        "-v: log level stays error, set by --log-level; pass --log-level debug to raise it",
		},
		{name: "sources agree via the flag", flag: "debug", verbose: true},
		{name: "sources agree via the config", configLevel: "debug", verbose: true},
		{name: "sources agree in another case", configLevel: "DEBUG", verbose: true},
		{name: "verbose is the thing setting the level", verbose: true},
		{name: "blank flag and blank config", flag: "   ", configLevel: "  ", verbose: true},
		{name: "instance-style config without verbose", configLevel: "warn"},
		{name: "flag without verbose", flag: "error"},
		{name: "nothing at all"},
		{name: "an invalid level is applyLogLevel's business", configLevel: "shout", verbose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := verboseLevelNotice(tc.flag, tc.configLevel, tc.verbose); got != tc.want {
				t.Errorf("verboseLevelNotice(%q, %q, %v) = %q, want %q", tc.flag, tc.configLevel, tc.verbose, got, tc.want)
			}
		})
	}
}

// The notice is only worth anything if resolveLogLevel actually emits it, and it must
// land on stderr: stdout belongs to commands whose output is a consumed value.
func TestResolveLogLevelEmitsTheVerboseNoticeOnStderr(t *testing.T) {
	t.Cleanup(func() { safelog.SetLevel(safelog.DefaultLevel) })

	for _, tc := range []struct {
		name        string
		flag        string
		configLevel string
		verbose     bool
		wantNotice  bool
		wantSource  string
		wantAbsent  string
		wantLevel   slog.Level
	}{
		{
			name:        "verbose against a persisted level",
			configLevel: "warn",
			verbose:     true,
			wantNotice:  true,
			wantSource:  "server.logLevel in the config file",
			wantAbsent:  "set by --log-level",
			wantLevel:   slog.LevelWarn,
		},
		{
			// resolveLogLevel folds the flag into cfg.LogLevel, after which the two
			// sources are one field. Only a flag row driven end to end can tell that
			// the notice was computed before the fold: read cfg.LogLevel afterwards
			// and the notice blames the config file for a level the caller typed.
			name:        "the flag is named even though it lands in the config field",
			flag:        "error",
			configLevel: "warn",
			verbose:     true,
			wantNotice:  true,
			wantSource:  "set by --log-level",
			wantAbsent:  "config file",
			wantLevel:   slog.LevelError,
		},
		{name: "instance-style config with no verbose", configLevel: "warn", wantLevel: slog.LevelWarn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			safelog.SetLevel(slog.LevelInfo)
			cfg := &config.RuntimeConfig{LogLevel: tc.configLevel}

			stdout, stderr := captureStdoutStderr(t, func() { resolveLogLevel(cfg, tc.flag, tc.verbose) })

			if got := strings.Contains(stderr, "pass --log-level debug to raise it"); got != tc.wantNotice {
				t.Errorf("stderr carries the notice = %v, want %v (stderr = %q)", got, tc.wantNotice, stderr)
			}
			if tc.wantNotice {
				if !strings.Contains(stderr, tc.wantSource) {
					t.Errorf("stderr = %q does not name %q as the source", stderr, tc.wantSource)
				}
				if strings.Contains(stderr, tc.wantAbsent) {
					t.Errorf("stderr = %q names %q, which did not set the level", stderr, tc.wantAbsent)
				}
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing; the notice is a diagnostic", stdout)
			}
			if got := safelog.CurrentLevel(); got != tc.wantLevel {
				t.Errorf("level = %v, want %v; the notice must not change the precedence", got, tc.wantLevel)
			}
		})
	}
}

func captureStdoutStderr(t *testing.T, fn func()) (string, string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	fn()

	os.Stdout, os.Stderr = origOut, origErr
	if err := outW.Close(); err != nil {
		t.Fatal(err)
	}
	if err := errW.Close(); err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	if _, err := outBuf.ReadFrom(outR); err != nil {
		t.Fatal(err)
	}
	if _, err := errBuf.ReadFrom(errR); err != nil {
		t.Fatal(err)
	}
	return outBuf.String(), errBuf.String()
}

// The daemon install and the auto-started server both launch `pinchtab server`
// with no flags, so the config file is the only way either can ask for a level.
// This drives the flagless path end to end: file config in, resolved threshold out.
func TestFlaglessServerResolvesTheConfiguredLogLevel(t *testing.T) {
	t.Cleanup(func() { safelog.SetLevel(safelog.DefaultLevel) })
	safelog.SetLevel(slog.LevelInfo)

	var fc config.FileConfig
	if err := json.Unmarshal([]byte(`{"server":{"logLevel":"warn"}}`), &fc); err != nil {
		t.Fatal(err)
	}
	cfg := &config.RuntimeConfig{}
	config.ApplyFileConfigToRuntime(cfg, &fc)

	resolveLogLevel(cfg, "", false)

	if got := safelog.CurrentLevel(); got != slog.LevelWarn {
		t.Fatalf("flagless server resolved %v, want warn from server.logLevel", got)
	}
}

// If either launcher ever grew a --log-level argument the test above would stop
// standing in for it, and a reader would not know which path is authoritative.
func TestDaemonAndAutoStartLaunchWithoutLogLevelFlags(t *testing.T) {
	if args := autoStartServerArgs("marker-123"); slices.Contains(args, "--log-level") {
		t.Errorf("autoStartServerArgs = %v, now passes --log-level; the config path is no longer the only route", args)
	}
}

// resolveLogLevel is only load-bearing if the commands route through it. A unit test
// on the helper cannot see a Run body that settles the level itself, and there are two
// ways to do that: assigning cfg.LogLevel (which bypasses the precedence) or calling
// safelog.SetLevel directly (which overrides the resolved threshold for the whole
// process). Both are banned everywhere in the package except inside the
// resolveLogLevel/applyLogLevel pair that owns the decision.
//
// The scope is derived twice over, which is the property srccensus exists to make
// automatic: the package's non-test sources are enumerated rather than listed, so a new
// cmd_*.go is covered on arrival, and the exemption is the OWNING FUNCTIONS rather than
// the owning file. That closes the limit this guard used to record: cmd_server.go holds
// three legitimate safelog.SetLevel calls inside applyLogLevel, and a stray fourth
// anywhere else in that same file used to pass.
func TestTheCommandPackageSettlesTheLogLevelInOnePlace(t *testing.T) {
	pkg := srccensus.Load(t, ".", 2)

	owners := make([]srccensus.Func, 0, 2)
	for _, name := range []string{"resolveLogLevel", "applyLogLevel"} {
		fn, ok := pkg.Func(name)
		if !ok {
			t.Fatalf("no %s declaration in %s; the level owner was renamed or moved out of the package — re-point this census at whatever settles the level now rather than deleting it", name, pkg.Dir())
		}
		owners = append(owners, fn)
	}

	insideAnOwner := func(site srccensus.Site) bool {
		for _, owner := range owners {
			if pkg.Contains(owner, site) {
				return true
			}
		}
		return false
	}

	for _, rule := range []struct {
		what  string
		sites []srccensus.Site
		why   string
	}{
		{
			// Calls fails on zero by itself, so this rule's vacuity floor is structural.
			what:  "safelog.SetLevel(",
			sites: pkg.Calls(t, "safelog.SetLevel"),
			why:   "a raw level set overrides the resolved threshold for the whole process, so a command that never resolves a level must not touch it",
		},
		{
			// FieldAssignments has no such floor — an assignment is not a call — so the
			// floor for this rule is the explicit check below.
			what:  ".LogLevel = ",
			sites: pkg.FieldAssignments("LogLevel"),
			why:   "the level is settled only inside resolveLogLevel, so a command that spawns or becomes a server must pass its flag there instead",
		},
	} {
		if len(rule.sites) == 0 {
			t.Errorf("no %s anywhere in the command package; the census has nothing to guard and would pass vacuously — re-point it rather than deleting it", rule.what)
			continue
		}
		inside := 0
		for _, site := range rule.sites {
			if insideAnOwner(site) {
				inside++
				continue
			}
			t.Errorf("%s uses %s outside resolveLogLevel/applyLogLevel; %s", site, rule.what, rule.why)
		}
		if inside == 0 {
			t.Errorf("every %s in the package sits outside the owning functions; the rule has stopped applying where it is supposed to hold", rule.what)
		}
	}

	// The positive half: an absence-only census would pass happily if both commands
	// stopped resolving the level at all, which is the defect these guards were built
	// for. Matched on the call rather than an argument spelling, so a legitimate
	// signature change does not red this for no defect.
	callers := map[string]bool{}
	for _, site := range pkg.Calls(t, "resolveLogLevel") {
		callers[site.File] = true
	}
	for _, caller := range []string{"cmd_server.go", "cmd_bridge.go"} {
		if !callers[caller] {
			t.Errorf("%s no longer calls resolveLogLevel, so its --log-level and server.logLevel are silently ignored", caller)
		}
	}
}

// The one-owner rule covers the whole command package, not just the file the
// resolver lives in: cmd_server_ensure.go and cmd_server_background.go hold a
// RuntimeConfig too, so a level assigned there would bypass the precedence
// unseen.
func commandSourceFiles(t *testing.T) []string {
	t.Helper()

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		out = append(out, path)
	}
	return out
}

// captureRunLog installs the handler a real run installs, at the level a real run
// starts from, so the load-then-resolve sequence below is the production one
// rather than a level set up front.
func captureRunLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(safelog.NewDefaultHandler(&buf)))
	safelog.SetLevel(safelog.DefaultLevel)
	t.Cleanup(func() {
		slog.SetDefault(previous)
		safelog.SetLevel(safelog.DefaultLevel)
	})
	return &buf
}

func writeRunConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)
	t.Setenv("PINCHTAB_TOKEN", "test-token")
	return path
}

// The sequence the defect lived in: the loader describes reading the very file
// that carries server.logLevel, so the diagnostics have to outlive the load and be
// emitted once the level is known.
func loadThenResolve(t *testing.T, flag string, verbose bool) *config.RuntimeConfig {
	t.Helper()
	cfg, diags, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	resolveLogLevel(cfg, flag, verbose)
	config.EmitLoadDiagnostics(diags)
	return cfg
}

func TestConfigFileDebugLevelRecordsTheConfigLoadDiagnostic(t *testing.T) {
	path := writeRunConfig(t, `{"server":{"port":"9867","logLevel":"debug"}}`)
	buf := captureRunLog(t)

	loadThenResolve(t, "", false)

	out := buf.String()
	if !strings.Contains(out, "loading config file") {
		t.Errorf("server.logLevel=debug did not record the config-load diagnostic:\n%s", out)
	}
	if !strings.Contains(out, path) {
		t.Errorf("the diagnostic does not name the file that was read (%s):\n%s", path, out)
	}
}

// Shape 2 of the card: the flag route works too, so --log-level debug can tell you
// which config file it read even when the file itself says nothing about levels.
func TestLogLevelFlagRecordsTheConfigLoadDiagnostic(t *testing.T) {
	path := writeRunConfig(t, `{"server":{"port":"9867"}}`)
	buf := captureRunLog(t)

	loadThenResolve(t, "debug", false)

	if out := buf.String(); !strings.Contains(out, "loading config file") || !strings.Contains(out, path) {
		t.Errorf("--log-level debug did not record the config-load diagnostic with its path:\n%s", out)
	}
}

// A silently rewritten config is the case a user most needs to see.
func TestLegacyBrowserMigrationNoticeIsReachableAtDebug(t *testing.T) {
	writeRunConfig(t, `{"server":{"port":"9867","logLevel":"debug"},"browser":{"binary":"/tmp/chrome"}}`)
	buf := captureRunLog(t)

	cfg := loadThenResolve(t, "", false)

	if !cfg.TargetsSynthesized {
		t.Fatalf("precondition: the legacy browser config was not migrated, so there is no notice to record")
	}
	if out := buf.String(); !strings.Contains(out, "migrated legacy browser config") {
		t.Errorf("the migration notice is unreachable at debug:\n%s", out)
	}
}

// The fix must not promote the diagnostics: a default run stays quiet about which
// file it read, while a warn-level diagnostic from the same load still lands.
func TestDefaultRunRecordsNoConfigLoadDiagnosticButKeepsWarnings(t *testing.T) {
	writeRunConfig(t, `{"server":{"port":"9867"},"browsers":{"default":"not-a-browser"}}`)
	buf := captureRunLog(t)

	loadThenResolve(t, "", false)

	out := buf.String()
	if strings.Contains(out, "loading config file") {
		t.Errorf("a default run recorded the debug config-load diagnostic:\n%s", out)
	}
	if !strings.Contains(out, "not a known browser") {
		t.Errorf("a warn diagnostic from the same load was dropped:\n%s", out)
	}
}

// internal/config must not grow its own precedence: it collects diagnostics and
// emits them on request, and the level decision stays in resolveLogLevel.
func TestConfigPackageHoldsNoLogLevelPrecedence(t *testing.T) {
	configDir := filepath.Join("..", "..", "internal", "config")

	configPkg := srccensus.Load(t, configDir, 25)
	loader := configPkg.MustFunc(t, "LoadConfig")
	for _, forbidden := range []string{"safelog.", "SetLevel("} {
		if strings.Contains(configPkg.FileSource(loader), forbidden) {
			t.Errorf("internal/config/%s references %q — the level decision must stay in resolveLogLevel", loader.File, forbidden)
		}
	}

	// The criterion is about the package, not the loader: any file here could
	// apply a level, and `config set server.logLevel` applying it on the spot is
	// the edit that reads most reasonable. Applying is what is forbidden —
	// safelog.ParseLevel is validation and stays allowed, which is why this looks
	// for SetLevel alone rather than for the package.
	paths, err := filepath.Glob(filepath.Join(configDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, err := os.ReadFile(path) // #nosec G304 -- files listed from the config package's own directory.
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		if strings.Contains(string(body), "SetLevel(") {
			t.Errorf("internal/config/%s applies a log level; validating one with safelog.ParseLevel is fine, but resolveLogLevel is the only place that may decide and apply it", filepath.Base(path))
		}
	}
	if scanned < 2 {
		t.Fatalf("scanned %d files in internal/config; the census matched almost nothing and would pass vacuously", scanned)
	}
}

// yoloChildEnv marks the re-executed child that drives serverCmd.Run for real.
const yoloChildEnv = "PINCHTAB_TEST_YOLO_EXIT_CHILD"

// The --yolo branch reports its failures with os.Exit, which no defer and no
// in-process test can observe, so this re-executes the test binary and reads the
// child's own output. Two things have to survive that exit: the loader's warning,
// and the debug line the config asked for — the second one only lands if the level
// is resolved above the --yolo branch rather than merely before the work.
func TestServerYoloExitStillEmitsLoaderDiagnostics(t *testing.T) {
	if os.Getenv(yoloChildEnv) == "1" {
		runYoloExitChild()
		return
	}

	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"configVersion":"` + config.CurrentConfigVersion + `","server":{"token":"test-token","logLevel":"debug"},` +
		`"browser":{"binary":"/tmp/chrome"},"browsers":{"default":"not-a-browser"}}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	child := exec.Command(os.Args[0], "-test.run=TestServerYoloExitStillEmitsLoaderDiagnostics", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
	child.Env = append(os.Environ(),
		yoloChildEnv+"=1",
		"PINCHTAB_CONFIG="+path,
		"PINCHTAB_TOKEN=test-token",
	)
	raw, err := child.CombinedOutput()
	out := string(raw)

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("child exited with %v, want the --yolo os.Exit(1); output:\n%s", err, out)
	}
	if code := exitErr.ExitCode(); code != 1 {
		t.Fatalf("child exit code = %d, want 1; output:\n%s", code, out)
	}
	if !strings.Contains(out, "--yolo:") {
		t.Fatalf("child did not reach the --yolo failure, so the exit path is unproven:\n%s", out)
	}
	if !strings.Contains(out, "not a known browser") {
		t.Errorf("the --yolo exit discarded the loader warning:\n%s", out)
	}
	if !strings.Contains(out, "loading config file") {
		t.Errorf("server.logLevel=debug was not resolved before the --yolo exit, so its diagnostics stayed hidden:\n%s", out)
	}
	if !strings.Contains(out, "migrated legacy browser config") {
		t.Errorf("the legacy-browser migration notice was lost on the --yolo exit path:\n%s", out)
	}
}

// runYoloExitChild is the child half: production logging, the real Run body, the
// real os.Exit.
func runYoloExitChild() {
	safelog.InstallDefault()
	if err := serverCmd.Flags().Set("yolo", "true"); err != nil {
		fmt.Fprintln(os.Stderr, "child could not set --yolo:", err)
		os.Exit(9)
	}
	serverCmd.Run(serverCmd, nil)
}

// The fix is an ordering, and an ordering is only kept by something that checks
// it: the next flag validation added to either prologue would reopen the window
// silently. Every command that defers its diagnostics must emit them before it can
// leave the prologue, so no exit may sit between the load and the emit.
func TestDeferredDiagnosticsAreEmittedBeforeAnyExit(t *testing.T) {
	const (
		load = "= loadConfigDeferringDiagnostics()"
		emit = "config.EmitLoadDiagnostics("
	)

	// Every call site, not the first per file: a second prologue added to a file
	// that already has one is the likeliest future addition, and stopping at the
	// first match would inspect the old one and pass.
	sites := 0
	for _, path := range commandSourceFiles(t) {
		raw, err := os.ReadFile(path) // #nosec G304 -- files listed from this package's own directory.
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)

		for offset := 0; ; {
			rel := strings.Index(src[offset:], load)
			if rel < 0 {
				break
			}
			start := offset + rel
			sites++
			where := fmt.Sprintf("%s (prologue at byte %d)", filepath.Base(path), start)
			offset = start + len(load)

			emitAt := strings.Index(src[start:], emit)
			if emitAt < 0 {
				t.Errorf("%s defers its load diagnostics and never emits them", where)
				continue
			}
			window := src[start : start+emitAt]
			for _, escape := range []string{"return", "os.Exit("} {
				if strings.Contains(window, escape) {
					t.Errorf("%s can %s between the load and %s, which discards the loader's diagnostics; move the emit above it:\n%s",
						where, escape, emit, window)
				}
			}
		}
	}
	if sites < 2 {
		t.Fatalf("inspected %d %s call sites; the census matched almost nothing and would pass vacuously", sites, load)
	}
}

// Only `pinchtab server` ever wrote the config, and the write it performed was the
// defect this card removed. These two load the same file and must stay non-writing —
// the regression guard is at the source, because driving `bridge` needs a browser and
// `security` is interactive.
func TestOnlyTheStartupWizardPathWritesTheConfig(t *testing.T) {
	pkg := srccensus.Load(t, ".", 2)

	// cmd_wizard.go and config_load.go own every config write in this package: the
	// startup stamp goes through recordConfigVersion, and the interactive wizard writes
	// what the user just chose. config.SaveFileConfig must still exist somewhere — the
	// write path is real — so Calls' zero-match failure is this census's vacuity floor.
	allowed := map[string]bool{"cmd_wizard.go": true, "config_load.go": true}
	for _, site := range pkg.Calls(t, "config.SaveFileConfig") {
		if !allowed[site.File] {
			t.Errorf("%s writes the config; only the startup wizard path (cmd_wizard.go, config_load.go) may, and cmd_bridge.go/cmd_server.go/cmd_security.go must stay non-writing", site)
		}
	}
}
