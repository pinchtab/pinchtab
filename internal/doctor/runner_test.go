package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestSummarize_Counts(t *testing.T) {
	results := []CheckResult{
		{Status: StatusPass},
		{Status: StatusPass},
		{Status: StatusFail},
		{Status: StatusWarn},
		{Status: StatusSkip},
	}
	got := Summarize(results)
	want := Summary{Passed: 2, Failed: 1, Warnings: 1, Skipped: 1}
	if got != want {
		t.Fatalf("Summarize = %+v, want %+v", got, want)
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		s    Summary
		want int
	}{
		{"all pass", Summary{Passed: 3}, 0},
		{"skip only", Summary{Skipped: 3}, 0},
		{"warn does not fail", Summary{Warnings: 2, Passed: 1}, 0},
		{"any fail -> 1", Summary{Passed: 1, Failed: 1}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExitCode(c.s); got != c.want {
				t.Fatalf("ExitCode(%+v) = %d, want %d", c.s, got, c.want)
			}
		})
	}
}

func TestRun_RegistryOrdering_Chrome(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserChrome, BrowserBinary: "/does/not/exist"}
	got := Run(context.Background(), cfg, "")
	wantOrder := []string{
		"config_file",
		"chrome_present",
		"handle_decisions",
		"binary_exists",
		"binary_executable",
		"binary_starts",
	}
	if len(got) != len(wantOrder) {
		t.Fatalf("expected %d results for chrome provider, got %d (%v)", len(wantOrder), len(got), got)
	}
	for i, name := range wantOrder {
		if got[i].Name != name {
			t.Errorf("result[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestRun_RegistryOrdering_Cloak(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserCloak, BrowserBinary: "/does/not/exist"}
	got := Run(context.Background(), cfg, "")
	// The three cloak-specific checks now come from Browser.DoctorChecks(),
	// so they appear right after cloakbrowser_present (provider block) and
	// before the generic binary checks.
	wantOrder := []string{
		"config_file",
		"cloakbrowser_present",
		"cdp_reachable",
		"fingerprint_flags_accepted",
		"linux_fonts_present",
		"handle_decisions",
		"binary_exists",
		"binary_executable",
		"binary_starts",
	}
	if len(got) != len(wantOrder) {
		t.Fatalf("expected %d results for cloak provider, got %d", len(wantOrder), len(got))
	}
	for i, name := range wantOrder {
		if got[i].Name != name {
			t.Errorf("result[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestRun_CheckFilter(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserChrome, BrowserBinary: "/does/not/exist"}
	got := Run(context.Background(), cfg, "binary_exists")
	if len(got) != 1 {
		t.Fatalf("filter binary_exists: got %d results, want 1", len(got))
	}
	if got[0].Name != "binary_exists" {
		t.Errorf("filtered result name = %q, want binary_exists", got[0].Name)
	}
}

func TestKnownCheck(t *testing.T) {
	cfg := &config.RuntimeConfig{DefaultBrowser: config.BrowserChrome}
	if !KnownCheck(cfg, "binary_exists") {
		t.Error("binary_exists should be known for chrome provider")
	}
	if KnownCheck(cfg, "cdp_reachable") {
		t.Error("cdp_reachable should not be known for chrome provider")
	}
	if KnownCheck(cfg, "nonsense") {
		t.Error("unknown check name should report false")
	}
	cloak := &config.RuntimeConfig{DefaultBrowser: config.BrowserCloak}
	if !KnownCheck(cloak, "cdp_reachable") {
		t.Error("cdp_reachable should be known for cloak provider")
	}
}

func TestBinaryExists_MissingFile(t *testing.T) {
	cfg := &config.RuntimeConfig{BrowserBinary: filepath.Join(t.TempDir(), "nope")}
	r := checkBinaryExists(context.Background(), cfg)
	if r.Status != StatusFail {
		t.Fatalf("status = %v, want fail (err=%v)", r.Status, r.Err)
	}
}

func TestBinaryExists_FoundFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-chrome")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.RuntimeConfig{BrowserBinary: path}
	r := checkBinaryExists(context.Background(), cfg)
	if r.Status != StatusPass {
		t.Fatalf("status = %v, want pass; detail=%q err=%v", r.Status, r.Detail, r.Err)
	}
}

func TestBinaryExecutable_NotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file-mode executable bit is not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "no-exec")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.RuntimeConfig{BrowserBinary: path}
	r := checkBinaryExecutable(context.Background(), cfg)
	if r.Status != StatusFail {
		t.Fatalf("status = %v, want fail", r.Status)
	}
}

func TestBinaryExecutable_Executable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exec")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.RuntimeConfig{BrowserBinary: path}
	r := checkBinaryExecutable(context.Background(), cfg)
	if r.Status != StatusPass {
		t.Fatalf("status = %v, want pass; detail=%q", r.Status, r.Detail)
	}
}

func TestBinaryStarts_FakeVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary requires unix")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-chrome")
	script := "#!/bin/sh\necho 'Chromium 99.0.0.1'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.RuntimeConfig{BrowserBinary: path}
	r := checkBinaryStarts(context.Background(), cfg)
	if r.Status != StatusPass {
		t.Fatalf("status = %v, want pass; detail=%q err=%v", r.Status, r.Detail, r.Err)
	}
	if !strings.Contains(r.Detail, "Chromium 99") {
		t.Errorf("expected version line in detail, got %q", r.Detail)
	}
}

func TestBinaryStarts_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary requires unix")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-chrome-fail")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.RuntimeConfig{BrowserBinary: path}
	r := checkBinaryStarts(context.Background(), cfg)
	if r.Status != StatusFail {
		t.Fatalf("status = %v, want fail", r.Status)
	}
}

func TestParseVersionLine(t *testing.T) {
	got := parseVersionLine("\n\nGoogle Chrome 146.0.7680.177\nextra\n")
	if got != "Google Chrome 146.0.7680.177" {
		t.Errorf("parseVersionLine = %q", got)
	}
}

func TestWriteText_FormatsResults(t *testing.T) {
	results := []CheckResult{
		{Name: "binary_exists", Status: StatusPass, Detail: "/opt/chrome", Duration: 52 * time.Millisecond},
		{Name: "binary_starts", Status: StatusFail, Detail: "boom", Duration: 1500 * time.Millisecond, Err: errors.New("boom")},
		{Name: "linux_fonts_present", Status: StatusSkip, Detail: "not linux", Duration: time.Microsecond},
	}
	var buf bytes.Buffer
	WriteText(&buf, "cloak", "", results)
	out := buf.String()
	if !strings.Contains(out, "pinchtab doctor (browser=cloak)") {
		t.Errorf("missing header in output:\n%s", out)
	}
	if !strings.Contains(out, "binary_exists") || !strings.Contains(out, "/opt/chrome") {
		t.Errorf("missing pass row:\n%s", out)
	}
	if !strings.Contains(out, "1 passed, 1 failed, 1 skipped, 0 warnings.") {
		t.Errorf("missing or wrong summary:\n%s", out)
	}
}

func TestWriteJSON_Structure(t *testing.T) {
	results := []CheckResult{
		{Name: "binary_exists", Status: StatusPass, Detail: "/x", Duration: 5 * time.Millisecond},
		{Name: "binary_starts", Status: StatusFail, Detail: "broken", Err: errors.New("broken")},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, "cloak", "default", results); err != nil {
		t.Fatal(err)
	}
	var report jsonReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v\nbody=%s", err, buf.String())
	}
	if report.Browser != "cloak" || report.Target != "default" {
		t.Errorf("browser/target mismatch: %+v", report)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}
	if report.Results[1].ErrMsg != "broken" {
		t.Errorf("error not propagated to JSON: %+v", report.Results[1])
	}
	if report.Summary.Passed != 1 || report.Summary.Failed != 1 {
		t.Errorf("summary wrong: %+v", report.Summary)
	}
}

// M7 regression: durationMs must marshal milliseconds, not raw nanoseconds.
func TestCheckResultMarshalsDurationInMilliseconds(t *testing.T) {
	r := CheckResult{Name: "x", Status: StatusPass, Duration: 1500 * time.Millisecond}
	r.DurationMS = r.Duration.Milliseconds()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"durationMs":1500`) {
		t.Fatalf("expected durationMs in milliseconds, got %s", data)
	}
}
