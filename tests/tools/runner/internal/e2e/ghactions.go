package e2e

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func (r *Runner) writeGitHubActionsMetadata(duration time.Duration, exitCode int) {
	if r.args.DryRun {
		return
	}

	passed := r.overall.Passed
	failed := r.overall.Failed
	failures := append([]string{}, r.overall.Failures...)
	if exitCode != 0 && failed == 0 {
		failed = 1
		failures = append(failures, "Runner failed before suite results were emitted")
	}

	if path := strings.TrimSpace(os.Getenv("GITHUB_OUTPUT")); path != "" {
		if err := appendGitHubActionsOutput(path, map[string]string{
			"status":               githubActionsStatus(failed),
			"passed":               fmt.Sprintf("%d", passed),
			"failed":               fmt.Sprintf("%d", failed),
			"tests":                fmt.Sprintf("%d", passed+failed),
			"suites":               fmt.Sprintf("%d", r.overall.Suites),
			"test_time":            formatDuration(time.Duration(r.overall.TotalMs) * time.Millisecond),
			"test_time_ms":         fmt.Sprintf("%d", r.overall.TotalMs),
			"overall_wall_time":    formatDuration(duration),
			"overall_wall_time_ms": fmt.Sprintf("%d", duration.Milliseconds()),
			"failures":             strings.Join(failures, "\n"),
		}); err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to write GitHub output: %v\n", err)
		}
	}

	if path := strings.TrimSpace(os.Getenv("GITHUB_STEP_SUMMARY")); path != "" {
		if err := appendGitHubActionsSummary(path, r.suite, passed, failed, r.overall.Suites, r.overall.TotalMs, duration, failures); err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to write GitHub summary: %v\n", err)
		}
	}
}

func githubActionsStatus(failed int) string {
	if failed > 0 {
		return "failed"
	}
	return "passed"
}

func appendGitHubActionsOutput(path string, values map[string]string) (err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()

	for _, key := range []string{"status", "passed", "failed", "tests", "suites", "test_time", "test_time_ms", "overall_wall_time", "overall_wall_time_ms"} {
		if _, err := fmt.Fprintf(file, "%s=%s\n", key, values[key]); err != nil {
			return err
		}
	}

	delimiter := fmt.Sprintf("PINCHTAB_E2E_%d", time.Now().UnixNano())
	if _, err := fmt.Fprintf(file, "failures<<%s\n%s\n%s\n", delimiter, values["failures"], delimiter); err != nil {
		return err
	}
	return nil
}

func appendGitHubActionsSummary(path, suite string, passed, failed, suites int, totalMs int64, duration time.Duration, failures []string) (err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()

	status := "passed"
	if failed > 0 {
		status = "failed"
	}
	total := passed + failed
	lines := []string{
		"## PinchTab E2E Summary",
		"",
		fmt.Sprintf("- Suite: `%s`", suite),
		fmt.Sprintf("- Status: %s", status),
		fmt.Sprintf("- Suites: %d", suites),
		fmt.Sprintf("- Tests: %d", total),
		fmt.Sprintf("- Passed: %d/%d", passed, total),
		fmt.Sprintf("- Failed: %d/%d", failed, total),
		fmt.Sprintf("- Test time: %dms", totalMs),
		fmt.Sprintf("- Overall wall time: %s", formatDuration(duration)),
		"",
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(file, line); err != nil {
			return err
		}
	}

	if len(failures) > 0 {
		if _, err := fmt.Fprintln(file, "### Failed Tests"); err != nil {
			return err
		}
		for _, failure := range failures {
			if _, err := fmt.Fprintf(file, "- %s\n", failure); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(file); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(file, "Artifacts: `tests/e2e/results/`"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(file); err != nil {
		return err
	}
	return nil
}
