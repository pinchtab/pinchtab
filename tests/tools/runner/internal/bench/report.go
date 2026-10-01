package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	MaxSummarySteps     = 12
	MaxSummaryTextChars = 220
)

type ReportTotals struct {
	StepsAnswered            int `json:"steps_answered"`
	StepsFailed              int `json:"steps_failed"`
	StepsSkipped             int `json:"steps_skipped"`
	StepsVerifiedPassed      int `json:"steps_verified_passed"`
	StepsVerifiedFailed      int `json:"steps_verified_failed"`
	StepsVerifiedSkipped     int `json:"steps_verified_skipped"`
	StepsPendingVerification int `json:"steps_pending_verification"`
}

type ReportStep struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Answer       string `json:"answer"`
	Notes        string `json:"notes"`
	Verification *struct {
		Status string `json:"status"`
	} `json:"verification,omitempty"`
}

type Report struct {
	Totals ReportTotals `json:"totals"`
	Steps  []ReportStep `json:"steps"`
}

func ReadReport(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func shorten(text string, maxChars int) string {
	ws := regexp.MustCompile(`\s+`)
	normalized := strings.TrimSpace(ws.ReplaceAllString(text, " "))
	if len(normalized) <= maxChars {
		return normalized
	}
	return normalized[:maxChars-3] + "..."
}

func BuildProgressSummary(reportFile string) string {
	report, err := ReadReport(reportFile)
	if err != nil {
		return "No benchmark report file exists yet."
	}

	totals := report.Totals
	steps := report.Steps

	startIdx := max(len(steps)-MaxSummarySteps, 0)
	recentSteps := steps[startIdx:]

	var lines []string
	lines = append(lines, "Benchmark progress summary from the external report.")
	lines = append(lines, fmt.Sprintf("- answered=%d", totals.StepsAnswered))
	lines = append(lines, fmt.Sprintf("- execution_failed=%d", totals.StepsFailed))
	lines = append(lines, fmt.Sprintf("- execution_skipped=%d", totals.StepsSkipped))
	lines = append(lines, fmt.Sprintf("- verified_passed=%d", totals.StepsVerifiedPassed))
	lines = append(lines, fmt.Sprintf("- verified_failed=%d", totals.StepsVerifiedFailed))
	lines = append(lines, fmt.Sprintf("- verified_skipped=%d", totals.StepsVerifiedSkipped))
	lines = append(lines, fmt.Sprintf("- pending_verification=%d", totals.StepsPendingVerification))

	if len(recentSteps) > 0 {
		lines = append(lines, "Recent recorded steps:")
		for _, step := range recentSteps {
			verification := ""
			if step.Verification != nil && step.Verification.Status != "" {
				verification = fmt.Sprintf(" / verify=%s", step.Verification.Status)
			}
			answer := step.Answer
			if answer == "" {
				answer = step.Notes
			}
			lines = append(lines, fmt.Sprintf("- %s: status=%s%s; %s",
				step.ID, step.Status, verification, shorten(answer, MaxSummaryTextChars)))
		}
	} else {
		lines = append(lines, "No steps recorded yet.")
	}

	lines = append(lines, "")
	lines = append(lines, "NEXT ACTION: Execute the next benchmark step using the wrapper (./scripts/ab or ./scripts/pt).")
	lines = append(lines, "After each step: ./scripts/runner step-end (records + verifies in one call) -> continue to next step.")
	lines = append(lines, "Do not explore files or read documentation. Execute benchmark steps directly.")

	return strings.Join(lines, "\n")
}

type Progress struct {
	Answered       int
	Failed         int
	Skipped        int
	VerifiedPassed int
}

func ReadProgress(reportFile string) Progress {
	report, err := ReadReport(reportFile)
	if err != nil {
		return Progress{}
	}
	return Progress{
		Answered:       report.Totals.StepsAnswered,
		Failed:         report.Totals.StepsFailed,
		Skipped:        report.Totals.StepsSkipped,
		VerifiedPassed: report.Totals.StepsVerifiedPassed,
	}
}
