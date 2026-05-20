package stealth

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeReport(t *testing.T, path string, r Report) {
	t.Helper()
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestCompareDivergences(t *testing.T) {
	tmp := t.TempDir()
	resultsDir := filepath.Join(tmp, "results")

	chromePath := filepath.Join(resultsDir, "chrome.json")
	cloakPath := filepath.Join(resultsDir, "cloak.json")

	writeReport(t, chromePath, Report{
		Provider:  "chrome",
		Timestamp: "20260520T100000Z",
		Sites: []SiteRow{
			{
				Site: "sannysoft",
				URL:  "https://bot.sannysoft.com/",
				Metrics: map[string]string{
					"webdriver":    "passed",
					"webgl_vendor": "Canvas has no webgl context",
				},
			},
			{
				Site: "creepjs",
				URL:  "https://creepjs.org/",
				Metrics: map[string]string{
					"trust_score": "38",
					"bot":         "probably",
					"gpu":         "unavailable",
				},
			},
		},
	})
	writeReport(t, cloakPath, Report{
		Provider:  "cloak",
		Timestamp: "20260520T100000Z",
		Sites: []SiteRow{
			{
				Site: "sannysoft",
				URL:  "https://bot.sannysoft.com/",
				Metrics: map[string]string{
					"webdriver":    "passed",
					"webgl_vendor": "Google Inc. (NVIDIA)",
				},
			},
			{
				Site: "creepjs",
				URL:  "https://creepjs.org/",
				Metrics: map[string]string{
					"trust_score": "62",
					"bot":         "no",
					"gpu":         "RTX 4060",
				},
			},
		},
	})

	var stdout, stderr bytes.Buffer
	rc := RunCompare([]string{chromePath, cloakPath}, &stdout, &stderr)
	if rc != 0 {
		t.Fatalf("exit code = %d; stderr=%q", rc, stderr.String())
	}

	out := stdout.String()

	wantDivergences := []string{
		"sannysoft | webgl_vendor",
		"creepjs | trust_score",
		"creepjs | bot",
	}
	for _, want := range wantDivergences {
		if !strings.Contains(out, want) {
			t.Errorf("missing divergent row %q in output", want)
		}
	}

	// "webdriver" is the same on both sides — should NOT appear in divergences section
	// but should still appear in the per-site full table.
	divIdx := strings.Index(out, "## Divergences")
	fullIdx := strings.Index(out, "## Full per-site comparison")
	if divIdx == -1 || fullIdx == -1 || divIdx > fullIdx {
		t.Fatalf("expected Divergences before Full per-site comparison; got divIdx=%d fullIdx=%d", divIdx, fullIdx)
	}
	divergencesSection := out[divIdx:fullIdx]
	if strings.Contains(divergencesSection, "webdriver") {
		t.Errorf("webdriver should not be in Divergences section (same on both providers)")
	}

	// gpu was "unavailable" on chrome — should be skipped from divergences even though
	// values differ.
	if strings.Contains(divergencesSection, "gpu") {
		t.Errorf("gpu should be skipped from Divergences (chrome value was 'unavailable')")
	}

	// History should have been written to tmp/ (default = parent of reports/).
	historyPath := filepath.Join(tmp, "history.jsonl")
	hist, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("history.jsonl missing: %v", err)
	}
	if !strings.Contains(string(hist), `"divergences":3`) {
		t.Errorf("history entry should record 3 divergences; got: %s", hist)
	}
	if !strings.Contains(string(hist), `"run_id":"20260520T100000Z"`) {
		t.Errorf("history entry missing run_id; got: %s", hist)
	}

	mdPath := filepath.Join(tmp, "history.md")
	md, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("history.md missing: %v", err)
	}
	if !strings.Contains(string(md), "20260520T100000Z") {
		t.Errorf("history.md should reference run id")
	}
}

func TestCompareNoHistory(t *testing.T) {
	tmp := t.TempDir()
	resultsDir := filepath.Join(tmp, "results")
	chromePath := filepath.Join(resultsDir, "chrome.json")
	cloakPath := filepath.Join(resultsDir, "cloak.json")

	writeReport(t, chromePath, Report{Provider: "chrome", Timestamp: "ts", Sites: []SiteRow{{Site: "a", Metrics: map[string]string{"x": "1"}}}})
	writeReport(t, cloakPath, Report{Provider: "cloak", Timestamp: "ts", Sites: []SiteRow{{Site: "a", Metrics: map[string]string{"x": "2"}}}})

	var stdout, stderr bytes.Buffer
	if rc := RunCompare([]string{"--no-history", chromePath, cloakPath}, &stdout, &stderr); rc != 0 {
		t.Fatalf("exit code = %d; stderr=%q", rc, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(tmp, "history.jsonl")); !os.IsNotExist(err) {
		t.Errorf("--no-history should suppress history.jsonl creation; err=%v", err)
	}
}

func TestCompareSingleReport(t *testing.T) {
	tmp := t.TempDir()
	chromePath := filepath.Join(tmp, "chrome.json")
	writeReport(t, chromePath, Report{Provider: "chrome", Timestamp: "ts", Sites: []SiteRow{{Site: "a", URL: "u", Metrics: map[string]string{"x": "1"}}}})

	var stdout, stderr bytes.Buffer
	if rc := RunCompare([]string{chromePath}, &stdout, &stderr); rc != 0 {
		t.Fatalf("exit code = %d; stderr=%q", rc, stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "## Divergences") {
		t.Errorf("single-report run should not render Divergences section")
	}
	if !strings.Contains(out, "## Full per-site comparison") {
		t.Errorf("single-report run should still render per-site table")
	}
	// Single-report runs should NOT touch history.
	if _, err := os.Stat(filepath.Join(tmp, "history.jsonl")); !os.IsNotExist(err) {
		t.Errorf("single-report run should not create history.jsonl; err=%v", err)
	}
}

func TestCompareMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := RunCompare([]string{"/no/such/path.json"}, &stdout, &stderr)
	if rc == 0 {
		t.Errorf("expected non-zero exit code for missing file")
	}
	if !strings.Contains(stderr.String(), "/no/such/path.json") {
		t.Errorf("stderr should mention missing path; got %q", stderr.String())
	}
}

func TestCompareUsageOnNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := RunCompare(nil, &stdout, &stderr)
	if rc != 2 {
		t.Errorf("expected exit 2 for no args; got %d", rc)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("expected usage in stderr; got %q", stderr.String())
	}
}

func TestHistoryAppendsAndCapsAtLimit(t *testing.T) {
	tmp := t.TempDir()

	// Pre-seed history.jsonl with N existing entries to exercise the limit cap.
	historyPath := filepath.Join(tmp, "history.jsonl")
	var buf bytes.Buffer
	for i := 0; i < historyRenderLimit+5; i++ {
		entry := HistoryEntry{
			RunID:            "old",
			AppendedAt:       "t",
			Providers:        []string{"chrome", "cloak"},
			SitesTotal:       1,
			Captured:         map[string]int{"chrome": 1, "cloak": 1},
			Divergences:      0,
			DivergentMetrics: []Divergence{},
		}
		ln, _ := json.Marshal(entry)
		buf.Write(ln)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(historyPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("seed history: %v", err)
	}

	chromePath := filepath.Join(tmp, "chrome.json")
	cloakPath := filepath.Join(tmp, "cloak.json")
	writeReport(t, chromePath, Report{Provider: "chrome", Timestamp: "new", Sites: []SiteRow{{Site: "a", Metrics: map[string]string{"x": "1"}}}})
	writeReport(t, cloakPath, Report{Provider: "cloak", Timestamp: "new", Sites: []SiteRow{{Site: "a", Metrics: map[string]string{"x": "2"}}}})

	var stdout, stderr bytes.Buffer
	if rc := RunCompare([]string{"--history-dir", tmp, chromePath, cloakPath}, &stdout, &stderr); rc != 0 {
		t.Fatalf("exit %d; stderr=%q", rc, stderr.String())
	}

	mdRaw, err := os.ReadFile(filepath.Join(tmp, "history.md"))
	if err != nil {
		t.Fatalf("read history.md: %v", err)
	}
	md := string(mdRaw)
	// history.md should only show the last historyRenderLimit rows.
	if got := strings.Count(md, "`"); got > (historyRenderLimit*2)+5 {
		t.Errorf("history.md should cap at %d rows; counted %d backticks", historyRenderLimit, got)
	}
	// Newest first: "new" row should appear before any "old" row.
	newIdx := strings.Index(md, "`new`")
	oldIdx := strings.Index(md, "`old`")
	if newIdx == -1 || oldIdx == -1 || newIdx > oldIdx {
		t.Errorf("history.md should show newest first; newIdx=%d oldIdx=%d", newIdx, oldIdx)
	}
}
