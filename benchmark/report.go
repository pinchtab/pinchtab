package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GenerateReport creates a human-readable text report from benchmark results.
// It uses the internal Results field (not the flattened JSON results) to
// produce detailed per-sample output including Shield reasons and patterns.
func GenerateReport(report *Report) string {
	var sb strings.Builder

	sb.WriteString("╔══════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║           IDPI SHIELD BENCHMARK REPORT                     ║\n")
	sb.WriteString("╚══════════════════════════════════════════════════════════════╝\n\n")

	sb.WriteString(fmt.Sprintf("  Timestamp : %s\n", report.Timestamp))
	sb.WriteString(fmt.Sprintf("  Duration  : %s\n", report.Duration))
	sb.WriteString(fmt.Sprintf("  Samples   : %d total (%d malicious, %d safe)\n\n",
		report.Metrics.TotalSamples, report.Metrics.MaliciousCount, report.Metrics.SafeCount))

	// Shield Config
	sb.WriteString("─── Shield Configuration ───────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  Enabled      : %v\n", report.Config.Enabled))
	sb.WriteString(fmt.Sprintf("  StrictMode   : %v\n", report.Config.StrictMode))
	sb.WriteString(fmt.Sprintf("  ScanContent  : %v\n", report.Config.ScanContent))
	sb.WriteString(fmt.Sprintf("  WrapContent  : %v\n", report.Config.WrapContent))
	if len(report.Config.CustomPatterns) > 0 {
		sb.WriteString(fmt.Sprintf("  CustomPat.   : %d patterns\n", len(report.Config.CustomPatterns)))
	}
	sb.WriteString("\n")

	// Overall Metrics
	sb.WriteString("─── Overall Metrics ────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  Accuracy     : %.1f%%  (%d/%d correct)\n",
		report.Metrics.Accuracy*100,
		report.Metrics.TruePositives+report.Metrics.TrueNegatives,
		report.Metrics.TotalSamples))
	sb.WriteString(fmt.Sprintf("  Precision    : %.1f%%\n", report.Metrics.Precision*100))
	sb.WriteString(fmt.Sprintf("  Recall       : %.1f%%\n", report.Metrics.Recall*100))
	sb.WriteString(fmt.Sprintf("  F1 Score     : %.1f%%\n\n", report.Metrics.F1Score*100))

	// Confusion Matrix
	sb.WriteString("─── Confusion Matrix ───────────────────────────────────────────\n")
	sb.WriteString("                        Predicted\n")
	sb.WriteString("                    Malicious    Safe\n")
	sb.WriteString(fmt.Sprintf("  Actual Malicious    TP=%-4d     FN=%-4d\n",
		report.Metrics.TruePositives, report.Metrics.FalseNegatives))
	sb.WriteString(fmt.Sprintf("  Actual Safe         FP=%-4d     TN=%-4d\n\n",
		report.Metrics.FalsePositives, report.Metrics.TrueNegatives))

	// Per-Category Breakdown
	sb.WriteString("─── Per-Category Breakdown ─────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-25s %5s %5s %4s %4s %4s %4s\n",
		"Category", "Total", "OK", "TP", "TN", "FP", "FN"))
	sb.WriteString("  " + strings.Repeat("─", 58) + "\n")
	for _, cat := range report.ByCategory {
		sb.WriteString(fmt.Sprintf("  %-25s %5d %5d %4d %4d %4d %4d\n",
			cat.Category, cat.Total, cat.Correct,
			cat.TruePositives, cat.TrueNegatives,
			cat.FalsePositives, cat.FalseNegatives))
	}
	sb.WriteString("\n")

	// Detailed Results
	sb.WriteString("─── Detailed Results ───────────────────────────────────────────\n\n")
	for _, r := range report.Results {
		icon := "✅"
		if !r.Correct {
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", icon, r.Classification, r.Sample.ID))
		sb.WriteString(fmt.Sprintf("     Label: %s | Category: %s | Severity: %s\n",
			r.Sample.Label, r.Sample.Category, r.Sample.Severity))
		sb.WriteString(fmt.Sprintf("     Desc: %s\n", r.Sample.Description))
		if r.ShieldDetected {
			sb.WriteString(fmt.Sprintf("     Shield: DETECTED — %s\n", r.ShieldReason))
			if r.ShieldPattern != "" {
				sb.WriteString(fmt.Sprintf("     Pattern: %q\n", r.ShieldPattern))
			}
		} else {
			sb.WriteString("     Shield: PASSED (no threat detected)\n")
		}
		sb.WriteString("\n")
	}

	// Failures Summary
	var failures []Result
	for _, r := range report.Results {
		if !r.Correct {
			failures = append(failures, r)
		}
	}

	if len(failures) > 0 {
		sb.WriteString("─── FAILURES (Incorrect Classifications) ───────────────────────\n\n")
		for _, r := range failures {
			sb.WriteString(fmt.Sprintf("  ❌ %s [%s] — %s\n", r.Sample.ID, r.Classification, r.Sample.Description))
			if r.Classification == "FN" {
				sb.WriteString(fmt.Sprintf("     MISSED: Expected pattern %q was not detected\n", r.Sample.ExpectedPattern))
			} else if r.Classification == "FP" {
				sb.WriteString(fmt.Sprintf("     FALSE ALARM: Safe content flagged as: %s\n", r.ShieldReason))
			}
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("─── All samples classified correctly! ──────────────────────────\n\n")
	}

	sb.WriteString("═══════════════════════════════════════════════════════════════\n")
	return sb.String()
}

// SaveReport writes the benchmark report to the output directory in three files:
//   - benchmark_<timestamp>.json  — timestamped archive (machine-readable)
//   - benchmark_<timestamp>.txt   — timestamped archive (human-readable)
//   - latest.json                 — stable path always pointing to the newest run
//
// The latest.json file enables automation: an agent can always read
// benchmark/reports/latest.json without needing to discover timestamped filenames.
func SaveReport(report *Report, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	ts := time.Now().Format("2006-01-02_150405")

	// Marshal the report to JSON (uses FlatResults via the "results" JSON tag).
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	// Save timestamped JSON report.
	jsonPath := filepath.Join(outputDir, fmt.Sprintf("benchmark_%s.json", ts))
	if err := os.WriteFile(jsonPath, jsonData, 0o640); err != nil {
		return fmt.Errorf("writing JSON report: %w", err)
	}

	// Save timestamped text report.
	textPath := filepath.Join(outputDir, fmt.Sprintf("benchmark_%s.txt", ts))
	text := GenerateReport(report)
	if err := os.WriteFile(textPath, []byte(text), 0o640); err != nil {
		return fmt.Errorf("writing text report: %w", err)
	}

	// Write latest.json — a stable path that always contains the most recent run.
	// This is a simple file copy (not a symlink) for cross-platform compatibility.
	latestPath := filepath.Join(outputDir, "latest.json")
	if err := os.WriteFile(latestPath, jsonData, 0o640); err != nil {
		return fmt.Errorf("writing latest.json: %w", err)
	}

	return nil
}
