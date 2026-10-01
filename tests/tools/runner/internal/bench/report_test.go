package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShorten(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello world", 20, "hello world"},
		{"hello   world\n\tfoo", 20, "hello world foo"},
		{"this is a very long string", 10, "this is..."},
	}

	for _, tc := range tests {
		got := shorten(tc.input, tc.max)
		if got != tc.expected {
			t.Errorf("shorten(%q, %d) = %q; want %q", tc.input, tc.max, got, tc.expected)
		}
	}
}

func TestBuildProgressSummaryNoFile(t *testing.T) {
	summary := BuildProgressSummary("/nonexistent/file.json")
	if !strings.Contains(summary, "No benchmark report file") {
		t.Errorf("unexpected summary for missing file: %q", summary)
	}
}

func TestBuildProgressSummaryWithData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")

	report := map[string]any{
		"totals": map[string]any{
			"steps_answered":             5,
			"steps_failed":               1,
			"steps_skipped":              0,
			"steps_verified_passed":      4,
			"steps_verified_failed":      1,
			"steps_verified_skipped":     0,
			"steps_pending_verification": 0,
		},
		"steps": []map[string]any{
			{"id": "0.1", "status": "answered", "answer": "ok", "verification": map[string]string{"status": "pass"}},
			{"id": "0.2", "status": "answered", "answer": "done"},
			{"id": "1.1", "status": "failed", "notes": "error occurred"},
		},
	}
	data, _ := json.Marshal(report)
	_ = os.WriteFile(path, data, 0o644)

	summary := BuildProgressSummary(path)

	checks := []string{
		"answered=5",
		"execution_failed=1",
		"verified_passed=4",
		"0.1: status=answered / verify=pass",
		"1.1: status=failed",
	}
	for _, check := range checks {
		if !strings.Contains(summary, check) {
			t.Errorf("summary missing %q:\n%s", check, summary)
		}
	}
}

func TestReadProgress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")

	report := map[string]any{
		"totals": map[string]any{
			"steps_answered":        10,
			"steps_failed":          2,
			"steps_skipped":         1,
			"steps_verified_passed": 8,
		},
	}
	data, _ := json.Marshal(report)
	_ = os.WriteFile(path, data, 0o644)

	p := ReadProgress(path)
	if p.Answered != 10 {
		t.Errorf("Answered = %d; want 10", p.Answered)
	}
	if p.Failed != 2 {
		t.Errorf("Failed = %d; want 2", p.Failed)
	}
	if p.VerifiedPassed != 8 {
		t.Errorf("VerifiedPassed = %d; want 8", p.VerifiedPassed)
	}
}

func TestReadProgressMissingFile(t *testing.T) {
	p := ReadProgress("/nonexistent.json")
	if p.Answered != 0 || p.Failed != 0 {
		t.Errorf("expected zero progress for missing file; got %+v", p)
	}
}
