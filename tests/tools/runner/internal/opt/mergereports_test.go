package opt

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestMergeReportsKeepsLastWhenDuplicateStepIDs locks in the merger's
// "keep last" dedup semantics. step-end appends a new row per call, so an
// initial fail followed by a successful re-record produces two rows for the
// same id within a single report file. The merger must keep the later one
// so downstream verify-answers / summarize see the agent's final answer.
// Previously this used "keep first", which surfaced stale failure records.
func TestMergeReportsKeepsLastWhenDuplicateStepIDs(t *testing.T) {
	tmp := t.TempDir()
	input := filepath.Join(tmp, "agentA.json")
	output := filepath.Join(tmp, "merged.json")

	report := `{
  "steps": [
    {"id": "7.1", "group": 7, "step": 1, "status": "fail", "answer": "Error 403: blocked"},
    {"id": "7.1", "group": 7, "step": 1, "status": "answer", "answer": "COMMENT_POSTED_RATING_5"}
  ]
}`
	if err := os.WriteFile(input, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := RunMergeReports([]string{"-o", output, input}, &stdout, &stderr); code != 0 {
		t.Fatalf("RunMergeReports exit=%d, stderr=%s", code, stderr.String())
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var merged struct {
		Steps []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Answer string `json:"answer"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(data, &merged); err != nil {
		t.Fatal(err)
	}

	if len(merged.Steps) != 1 {
		t.Fatalf("expected 1 deduplicated step, got %d: %+v", len(merged.Steps), merged.Steps)
	}
	got := merged.Steps[0]
	if got.Status != "answer" || got.Answer != "COMMENT_POSTED_RATING_5" {
		t.Errorf("expected last record to win (status=answer, answer=COMMENT_...), got status=%q answer=%q", got.Status, got.Answer)
	}
}
