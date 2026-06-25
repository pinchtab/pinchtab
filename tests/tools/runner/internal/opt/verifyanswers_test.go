package opt

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyAnswersKeepsLastWhenDuplicateStepIDs(t *testing.T) {
	tmp := t.TempDir()
	reportPath := filepath.Join(tmp, "report.json")
	// Step 0.1 has two records: first a fail (would fail verification),
	// then a successful re-record (matches expected pattern for 0.1).
	report := `{
  "steps": [
    {"id": "0.1", "status": "fail", "answer": "Error 403: blocked"},
    {"id": "0.1", "status": "answer", "answer": "ok"}
  ]
}`
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := RunVerifyAnswers([]string{reportPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunVerifyAnswers exit=%d, stderr=%s", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "pass: 1") {
		t.Errorf("expected 'pass: 1' (latest record matches expected pattern), got:\n%s", out)
	}
	if strings.Contains(out, "fail: 1") {
		t.Errorf("did not expect 'fail: 1' — verifier must pick the latest entry, not the first:\n%s", out)
	}
}
