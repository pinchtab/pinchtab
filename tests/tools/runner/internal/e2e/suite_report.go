package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type suiteTestResult struct {
	Name       string
	Status     string
	DurationMs int64
}

type suiteReportData struct {
	Results   []suiteTestResult
	Passed    int
	Failed    int
	TotalMs   int64
	ErrorTail string
}

func (r *Runner) writeSuiteReports(def suiteDef, duration time.Duration, exitCode int) suiteReportData {
	data := r.buildSuiteReportData(def, duration, exitCode)
	if r.args.DryRun {
		return data
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	summary := fmt.Sprintf("passed=%d\nfailed=%d\ntotal_time=%dms\ntimestamp=%s\nsuite_wall_time=%dms\n",
		data.Passed, data.Failed, data.TotalMs, timestamp, duration.Milliseconds())
	if err := os.WriteFile(filepath.Join(r.repoRoot, def.Summary), []byte(summary), 0o644); err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to write %s: %v\n", def.Summary, err)
	}

	report := renderSuiteReport(suiteReportTitle(def), data.Results, data.Passed, data.Failed, data.TotalMs, duration, timestamp)
	if err := os.WriteFile(filepath.Join(r.repoRoot, def.Report), []byte(report), 0o644); err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to write %s: %v\n", def.Report, err)
	}

	r.writeSuiteTimings(def, data, timestamp)
	return data
}

func (r *Runner) buildSuiteReportData(def suiteDef, duration time.Duration, exitCode int) suiteReportData {
	results := r.parseSuiteResults(def)
	var errorTail string
	if exitCode != 0 && !hasFailedResult(results) {
		name := "Suite failed before test results were emitted"
		if len(results) > 0 {
			name = "Suite exited with an error after the last emitted test result"
		}
		results = append(results, suiteTestResult{
			Name:       name,
			Status:     "failed",
			DurationMs: duration.Milliseconds(),
		})
		errorTail = r.extractOutputTail(def, 20)
	}

	passed, failed, totalMs := countSuiteResults(results)
	return suiteReportData{
		Results:   results,
		Passed:    passed,
		Failed:    failed,
		TotalMs:   totalMs,
		ErrorTail: errorTail,
	}
}

func hasFailedResult(results []suiteTestResult) bool {
	for _, result := range results {
		if result.Status == "failed" {
			return true
		}
	}
	return false
}

func (r *Runner) parseSuiteResults(def suiteDef) []suiteTestResult {
	data, err := os.ReadFile(filepath.Join(r.repoRoot, def.Output))
	if err != nil {
		return nil
	}
	var results []suiteTestResult
	for line := range strings.SplitSeq(string(data), "\n") {
		if !strings.HasPrefix(line, "E2E_RESULT\t") {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			continue
		}
		durationMs, _ := strconv.ParseInt(parts[2], 10, 64)
		results = append(results, suiteTestResult{
			Status:     parts[1],
			DurationMs: durationMs,
			Name:       cleanResultName(parts[3]),
		})
	}
	return results
}

func cleanResultName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "✅ ")
	name = strings.TrimPrefix(name, "❌ ")
	return name
}

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func (r *Runner) extractOutputTail(def suiteDef, maxLines int) string {
	data, err := os.ReadFile(filepath.Join(r.repoRoot, def.Output))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")

	var meaningful []string
	for _, line := range lines {
		stripped := ansiEscapeRe.ReplaceAllString(line, "")
		stripped = strings.TrimSpace(stripped)
		if stripped == "" || strings.HasPrefix(stripped, "E2E_RESULT") {
			continue
		}
		meaningful = append(meaningful, stripped)
	}

	if len(meaningful) == 0 {
		return ""
	}
	if len(meaningful) > maxLines {
		meaningful = meaningful[len(meaningful)-maxLines:]
	}
	return strings.Join(meaningful, "\n")
}

func countSuiteResults(results []suiteTestResult) (passed, failed int, totalMs int64) {
	for _, result := range results {
		totalMs += result.DurationMs
		if result.Status == "failed" {
			failed++
		} else {
			passed++
		}
	}
	return passed, failed, totalMs
}

func (r *Runner) recordOverallSummary(data suiteReportData) {
	if r.args.DryRun {
		return
	}
	r.overall.Suites++
	r.overall.Passed += data.Passed
	r.overall.Failed += data.Failed
	r.overall.TotalMs += data.TotalMs
	for _, result := range data.Results {
		if result.Status == "failed" {
			r.overall.Failures = append(r.overall.Failures, result.Name)
		}
	}
}

func (r *Runner) printSuiteSummary(def suiteDef, data suiteReportData, duration time.Duration) {
	if r.args.DryRun {
		return
	}

	total := data.Passed + data.Failed

	if data.Failed == 0 && total > 0 {
		_, _ = fmt.Fprintf(r.stdout, "  %s: %d/%d passed (%s)\n",
			suiteReportTitle(def), data.Passed, total, formatDuration(duration))
		return
	}

	_, _ = fmt.Fprintln(r.stdout, "")
	_, _ = fmt.Fprintf(r.stdout, "== %s summary ==\n", suiteReportTitle(def))
	if total == 0 {
		_, _ = fmt.Fprintln(r.stdout, "  no test results emitted")
	} else {
		_, _ = fmt.Fprintf(r.stdout, "  Passed: %d/%d | Failed: %d | Wall time: %s\n",
			data.Passed, total, data.Failed, formatDuration(duration))
		_, _ = fmt.Fprintln(r.stdout, "")
		_, _ = fmt.Fprintln(r.stdout, "  Failed tests:")
		for _, result := range data.Results {
			if result.Status == "failed" {
				_, _ = fmt.Fprintf(r.stdout, "    ✗ %s (%dms)\n", result.Name, result.DurationMs)
			}
		}
	}

	if data.ErrorTail != "" {
		_, _ = fmt.Fprintln(r.stdout, "")
		_, _ = fmt.Fprintln(r.stdout, "  Error output (last lines):")
		for line := range strings.SplitSeq(data.ErrorTail, "\n") {
			_, _ = fmt.Fprintf(r.stdout, "    %s\n", line)
		}
	}
	_, _ = fmt.Fprintln(r.stdout, "")
}

func (r *Runner) printOverallSummary(duration time.Duration) {
	if r.args.DryRun {
		return
	}

	total := r.overall.Passed + r.overall.Failed
	_, _ = fmt.Fprintln(r.stdout, "")
	_, _ = fmt.Fprintln(r.stdout, "== PinchTab E2E overall summary ==")
	if r.overall.Suites == 0 {
		_, _ = fmt.Fprintln(r.stdout, "  no suite results emitted")
	} else {
		_, _ = fmt.Fprintf(r.stdout, "  Suites: %d\n", r.overall.Suites)
		_, _ = fmt.Fprintf(r.stdout, "  Tests: %d\n", total)
		_, _ = fmt.Fprintf(r.stdout, "  Passed: %d/%d\n", r.overall.Passed, total)
		_, _ = fmt.Fprintf(r.stdout, "  Failed: %d/%d\n", r.overall.Failed, total)
		_, _ = fmt.Fprintf(r.stdout, "  Test time: %dms\n", r.overall.TotalMs)
	}
	_, _ = fmt.Fprintf(r.stdout, "  Overall wall time: %s\n", formatDuration(duration))
	_, _ = fmt.Fprintln(r.stdout, "")
}

func renderSuiteReport(title string, results []suiteTestResult, passed, failed int, totalMs int64, suiteDuration time.Duration, timestamp string) string {
	var b strings.Builder
	total := passed + failed
	b.WriteString("## PinchTab E2E Test Report\n\n")
	b.WriteString("**Suite:** " + title + "\n\n")
	if failed == 0 {
		b.WriteString("**Status:** All tests passed\n\n")
	} else {
		_, _ = fmt.Fprintf(&b, "**Status:** %d test(s) failed\n\n", failed)
	}
	b.WriteString("| Test | Duration | Status |\n")
	b.WriteString("|------|----------|--------|\n")
	for _, result := range results {
		status := "✅"
		if result.Status == "failed" {
			status = "❌"
		}
		_, _ = fmt.Fprintf(&b, "| %s | %dms | %s |\n", markdownCell(result.Name), result.DurationMs, status)
	}
	b.WriteString("\n")
	_, _ = fmt.Fprintf(&b, "**Summary:** %d/%d passed in %dms\n\n", passed, total, totalMs)
	_, _ = fmt.Fprintf(&b, "**Suite Wall Time:** %s\n\n", formatDuration(suiteDuration))
	b.WriteString("<sub>Generated at " + timestamp + "</sub>\n")
	return b.String()
}

func markdownCell(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

func formatDuration(d time.Duration) string {
	ms := d.Milliseconds()
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	seconds := ms / 1000
	remMs := ms % 1000
	if seconds < 60 {
		return fmt.Sprintf("%d.%03ds", seconds, remMs)
	}
	minutes := seconds / 60
	remSec := seconds % 60
	return fmt.Sprintf("%dm%02d.%03ds", minutes, remSec, remMs)
}
