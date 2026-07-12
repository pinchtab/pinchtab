package e2e

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDryRunBasicSuitePlan(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--suite", "basic", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"runner e2e (Go) - resolved plan",
		"docker compose -f tests/e2e/docker-compose.yml build",
		"docker compose -f tests/e2e/docker-compose.yml up -d pinchtab fixtures",
		"E2E_HELPER=api",
		"E2E_SCENARIO_DIR=scenarios/api",
		"E2E_SUMMARY_TITLE=PinchTab E2E API Suite",
		"runner-api /bin/bash /e2e/run.sh scenario=actions-basic.sh",
		"E2E_HELPER=cli",
		"E2E_SCENARIO_DIR=scenarios/cli",
		"E2E_SUMMARY_TITLE=PinchTab E2E CLI Suite",
		"runner-cli /bin/bash /e2e/run.sh scenario=actions-basic.sh",
		"E2E_SCENARIO_DIR=scenarios/infra",
		"E2E_SUMMARY_TITLE=PinchTab E2E Infra Suite",
		"scenario=network-basic.sh",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	assertNoLegacyReportEnv(t, out)
}

func TestDryRunExtendedPlan(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--suite", "extended", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"suite:    extended",
		"docker compose -f tests/e2e/docker-compose-multi.yml up -d pinchtab pinchtab-secure pinchtab-medium pinchtab-full pinchtab-retain pinchtab-ghostchrome pinchtab-bridge fixtures",
		"run --rm --no-deps",
		"E2E_READY_TARGETS=E2E_SERVER E2E_SECURE_SERVER",
		"E2E_SUMMARY_TITLE=PinchTab E2E API Extended Suite",
		"runner-api /bin/bash /e2e/run.sh scenario=actions-basic.sh",
		"scenario=actions-extended.sh",
		"scenario=network-retain-body.sh",
		"E2E_SUMMARY_TITLE=PinchTab E2E CLI Extended Suite",
		"runner-cli /bin/bash /e2e/run.sh scenario=actions-basic.sh",
		"scenario=actions-extended.sh",
		"E2E_SUMMARY_TITLE=PinchTab E2E Infra Extended Suite",
		"E2E_READY_TARGETS=E2E_SERVER E2E_SECURE_SERVER E2E_MEDIUM_SERVER E2E_FULL_SERVER E2E_SERVER_GHOSTCHROME E2E_BRIDGE_URL|60|E2E_BRIDGE_TOKEN",
		"scenario=browser-config-basic.sh",
		"scenario=network-basic.sh",
		"scenario=orchestrator-extended.sh",
		"E2E_SUMMARY_TITLE=PinchTab E2E Plugin Suite",
		"runner-api /bin/bash /e2e/run.sh scenario=plugin-basic.sh",
		"docker compose -f tests/e2e/docker-compose-multi.yml restart pinchtab",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	assertNoLegacyReportEnv(t, out)
}

func TestDryRunSingleSuiteWithFilterAndLogs(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--suite", "infra-extended", "--filter", "orchestrator", "--logs", "hide", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"suite:    infra-extended",
		"logs:     hide",
		"filter: orchestrator",
		"docker compose -f tests/e2e/docker-compose-multi.yml up -d pinchtab pinchtab-bridge fixtures",
		"run --rm --no-deps",
		"E2E_SCENARIO_DIR=scenarios/infra",
		"E2E_READY_TARGETS=E2E_SERVER E2E_BRIDGE_URL|60|E2E_BRIDGE_TOKEN",
		"E2E_SUMMARY_TITLE=PinchTab E2E Infra Extended Suite",
		"runner-api /bin/bash /e2e/run.sh scenario=orchestrator-extended.sh",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "pinchtab-secure pinchtab-autoclose") {
		t.Fatalf("filtered single-suite run should not start the full multi-instance stack:\n%s", out)
	}
	assertNoLegacyReportEnv(t, out)
}

func TestDryRunExtendedFilterSkipsUnmatchedSuites(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--suite", "extended", "--filter", "orchestrator", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		`Skipping api-extended: filter "orchestrator" has no matching scenarios`,
		`Skipping cli-extended: filter "orchestrator" has no matching scenarios`,
		`Skipping plugin: filter "orchestrator" has no matching scenarios`,
		"docker compose -f tests/e2e/docker-compose-multi.yml up -d pinchtab pinchtab-bridge fixtures",
		"run --rm --no-deps",
		"E2E_READY_TARGETS=E2E_SERVER E2E_BRIDGE_URL|60|E2E_BRIDGE_TOKEN",
		"runner-api /bin/bash /e2e/run.sh scenario=orchestrator-extended.sh",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "E2E_SCENARIO_DIR=scenarios/plugin") {
		t.Fatalf("filtered extended run should not run plugin:\n%s", out)
	}
	if strings.Contains(out, "pinchtab-secure pinchtab-autoclose") {
		t.Fatalf("filtered extended run should not start the full multi-instance stack:\n%s", out)
	}
	assertNoLegacyReportEnv(t, out)
}

func TestScenarioMetadataDefaultsAndManifestOverrides(t *testing.T) {
	r := &Runner{repoRoot: resolveRepoRoot()}
	catalog, err := r.loadScenarioCatalog()
	if err != nil {
		t.Fatal(err)
	}

	actions, ok := catalog.find("api", "actions-basic.sh")
	if !ok {
		t.Fatal("api/actions-basic.sh missing from scenario catalog")
	}
	if actions.Tier != tierBasic || actions.Helper != "api" {
		t.Fatalf("unexpected actions metadata: tier=%s helper=%s", actions.Tier, actions.Helper)
	}
	if got := strings.Join(actions.Services, " "); got != "pinchtab fixtures" {
		t.Fatalf("unexpected default services: %s", got)
	}
	if !hasString(actions.Tags, "actions") || !hasString(actions.Tags, "pr") {
		t.Fatalf("expected default/manifest tags on actions-basic: %#v", actions.Tags)
	}

	orchestrator, ok := catalog.find("infra", "orchestrator-extended.sh")
	if !ok {
		t.Fatal("infra/orchestrator-extended.sh missing from scenario catalog")
	}
	if got := strings.Join(orchestrator.Services, " "); got != "pinchtab pinchtab-bridge fixtures" {
		t.Fatalf("unexpected orchestrator services: %s", got)
	}
	if got := strings.Join(orchestrator.Ready, " "); got != "E2E_SERVER E2E_BRIDGE_URL|60|E2E_BRIDGE_TOKEN" {
		t.Fatalf("unexpected orchestrator ready targets: %s", got)
	}
	if !hasString(orchestrator.Tags, "multiinstance") || !hasString(orchestrator.Tags, "bridge") {
		t.Fatalf("expected orchestrator tags, got: %#v", orchestrator.Tags)
	}

	retain, ok := catalog.find("api", "network-retain-body.sh")
	if !ok {
		t.Fatal("api/network-retain-body.sh missing from scenario catalog")
	}
	if got := strings.Join(retain.Services, " "); got != "pinchtab pinchtab-retain fixtures" {
		t.Fatalf("unexpected retained-body services: %s", got)
	}
	if got := strings.Join(retain.Ready, " "); got != "E2E_SERVER E2E_RETAIN_SERVER" {
		t.Fatalf("unexpected retained-body ready targets: %s", got)
	}
	if !hasString(retain.Tags, "retain") || !hasString(retain.Tags, "smoke") {
		t.Fatalf("expected retained-body tags, got: %#v", retain.Tags)
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertNoLegacyReportEnv(t *testing.T, out string) {
	t.Helper()
	for _, forbidden := range []string{
		"E2E_SUMMARY_FILE",
		"E2E_REPORT_FILE",
		"E2E_PROGRESS_FILE",
		"E2E_GENERATE_MARKDOWN_REPORT",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("dry-run output should not pass legacy report env %q:\n%s", forbidden, out)
		}
	}
}

func TestRejectsUnknownSuite(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"--suite", "nightly", "--dry-run"}, &stdout, &stderr); code == 0 {
		t.Fatalf("Run should reject unknown suite, stdout: %s", stdout.String())
	}
}

func TestWriteSuiteReportsFromShellResultLines(t *testing.T) {
	tmp := t.TempDir()
	resultsDir := filepath.Join(tmp, "tests/e2e/results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(resultsDir, "output-api.log")
	if err := os.WriteFile(output, []byte(strings.Join([]string{
		"normal log line",
		"E2E_RESULT\tpassed\t12\t✅ [browser-basic] browser: health",
		"E2E_RESULT\tfailed\t34\t❌ [browser-basic] browser: bad",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	r := &Runner{repoRoot: tmp, stderr: &stderr}
	def := suiteDef{
		Name:     "api",
		RunSuite: "api",
		Output:   "tests/e2e/results/output-api.log",
		Summary:  "tests/e2e/results/summary-api.txt",
		Report:   "tests/e2e/results/report-api.md",
	}
	r.writeSuiteReports(def, 1500*time.Millisecond, 1)

	summary, err := os.ReadFile(filepath.Join(resultsDir, "summary-api.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"passed=1", "failed=1", "total_time=46ms", "suite_wall_time=1500ms"} {
		if !strings.Contains(string(summary), want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}

	report, err := os.ReadFile(filepath.Join(resultsDir, "report-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[browser-basic] browser: health", "| [browser-basic] browser: bad | 34ms | ❌ |", "**Suite Wall Time:** 1.500s"} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestReportDataAddsFailureWhenSuiteExitsAfterPassedResults(t *testing.T) {
	tmp := t.TempDir()
	resultsDir := filepath.Join(tmp, "tests/e2e/results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logContent := "E2E_RESULT\tpassed\t12\t[browser-basic] browser: health\n\x1b[0;34m▶ [sessions-basic] pinchtab session create\x1b[0m\njq: parse error: Invalid numeric literal at line 2, column 0\n"
	if err := os.WriteFile(filepath.Join(resultsDir, "output-api.log"), []byte(logContent), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{repoRoot: tmp}
	data := r.buildSuiteReportData(suiteDef{Output: "tests/e2e/results/output-api.log"}, 1500*time.Millisecond, 1)
	if data.Passed != 1 || data.Failed != 1 || len(data.Results) != 2 {
		t.Fatalf("unexpected summary: passed=%d failed=%d results=%d", data.Passed, data.Failed, len(data.Results))
	}
	if got := data.Results[1].Name; got != "Suite exited with an error after the last emitted test result" {
		t.Fatalf("unexpected synthetic failure name: %q", got)
	}
	if !strings.Contains(data.ErrorTail, "jq: parse error") {
		t.Fatalf("expected ErrorTail to contain jq error, got: %q", data.ErrorTail)
	}
	if strings.Contains(data.ErrorTail, "\x1b") {
		t.Fatalf("ErrorTail should not contain ANSI escapes: %q", data.ErrorTail)
	}
}

func TestParseSuiteResultsHandlesLargeLogLines(t *testing.T) {
	tmp := t.TempDir()
	resultsDir := filepath.Join(tmp, "tests/e2e/results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	output := strings.Join([]string{
		"E2E_RESULT\tpassed\t12\t[browser-basic] browser: health",
		strings.Repeat("x", 128*1024),
		"E2E_RESULT\tfailed\t34\t[actions-extended] iframe: hover",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(resultsDir, "output-api.log"), []byte(output), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{repoRoot: tmp}
	results := r.parseSuiteResults(suiteDef{Output: "tests/e2e/results/output-api.log"})
	if len(results) != 2 {
		t.Fatalf("expected 2 parsed results, got %d", len(results))
	}
	if results[1].Status != "failed" || results[1].Name != "[actions-extended] iframe: hover" {
		t.Fatalf("unexpected second result: %+v", results[1])
	}
}

func TestPrintSuiteSummaryFromGoReportData(t *testing.T) {
	var stdout bytes.Buffer
	r := &Runner{stdout: &stdout}
	data := suiteReportData{
		Results: []suiteTestResult{
			{Name: "[browser-basic] browser: health", Status: "passed", DurationMs: 12},
			{Name: "[browser-basic] browser: bad", Status: "failed", DurationMs: 34},
		},
		Passed:  1,
		Failed:  1,
		TotalMs: 46,
	}

	r.printSuiteSummary(suiteDef{Name: "api", RunSuite: "api"}, data, 1500*time.Millisecond)

	out := stdout.String()
	for _, want := range []string{
		"== PinchTab E2E API Suite summary ==",
		"Passed: 1/2 | Failed: 1 | Wall time: 1.500s",
		"Failed tests:",
		"✗ [browser-basic] browser: bad (34ms)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintSuiteSummaryShowsErrorTail(t *testing.T) {
	var stdout bytes.Buffer
	r := &Runner{stdout: &stdout}
	data := suiteReportData{
		Results: []suiteTestResult{
			{Name: "[browser-basic] browser: health", Status: "passed", DurationMs: 12},
			{Name: "Suite exited with an error after the last emitted test result", Status: "failed", DurationMs: 1500},
		},
		Passed:    1,
		Failed:    1,
		TotalMs:   1512,
		ErrorTail: "▶ [sessions-basic] pinchtab session create\njq: parse error: Invalid numeric literal at line 2, column 0",
	}

	r.printSuiteSummary(suiteDef{Name: "cli-extended", RunSuite: "cli-extended"}, data, 1500*time.Millisecond)

	out := stdout.String()
	for _, want := range []string{
		"Error output (last lines):",
		"jq: parse error: Invalid numeric literal at line 2, column 0",
		"[sessions-basic] pinchtab session create",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintOverallSummaryFromRecordedSuites(t *testing.T) {
	var stdout bytes.Buffer
	r := &Runner{stdout: &stdout}
	r.recordOverallSummary(suiteReportData{Passed: 3, Failed: 1, TotalMs: 1234})
	r.recordOverallSummary(suiteReportData{Passed: 2, Failed: 0, TotalMs: 200})

	r.printOverallSummary(2500 * time.Millisecond)

	out := stdout.String()
	for _, want := range []string{
		"== PinchTab E2E overall summary ==",
		"Suites: 2",
		"Tests: 6",
		"Passed: 5/6",
		"Failed: 1/6",
		"Test time: 1434ms",
		"Overall wall time: 2.500s",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("overall summary output missing %q:\n%s", want, out)
		}
	}
}

func TestWriteGitHubActionsMetadata(t *testing.T) {
	tmp := t.TempDir()
	outputPath := filepath.Join(tmp, "github-output")
	summaryPath := filepath.Join(tmp, "github-summary")
	t.Setenv("GITHUB_OUTPUT", outputPath)
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	var stderr bytes.Buffer
	r := &Runner{
		suite:  "api",
		stderr: &stderr,
		overall: overallReportData{
			Suites:   1,
			Passed:   2,
			Failed:   1,
			TotalMs:  123,
			Failures: []string{"[actions-basic] click failed"},
		},
	}
	r.writeGitHubActionsMetadata(1500*time.Millisecond, 1)
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"status=failed",
		"passed=2",
		"failed=1",
		"tests=3",
		"suites=1",
		"test_time=123ms",
		"test_time_ms=123",
		"overall_wall_time=1.500s",
		"overall_wall_time_ms=1500",
		"failures<<PINCHTAB_E2E_",
		"[actions-basic] click failed",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("GitHub output missing %q:\n%s", want, output)
		}
	}

	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## PinchTab E2E Summary",
		"- Suite: `api`",
		"- Status: failed",
		"- Passed: 2/3",
		"- Failed: 1/3",
		"### Failed Tests",
		"- [actions-basic] click failed",
		"Artifacts: `tests/e2e/results/`",
	} {
		if !strings.Contains(string(summary), want) {
			t.Fatalf("GitHub summary missing %q:\n%s", want, summary)
		}
	}
}

func TestWriteGitHubActionsMetadataAddsRunnerFailureWithoutSuiteResults(t *testing.T) {
	tmp := t.TempDir()
	outputPath := filepath.Join(tmp, "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	r := &Runner{suite: "api", stderr: io.Discard}
	r.writeGitHubActionsMetadata(250*time.Millisecond, 1)

	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"status=failed",
		"passed=0",
		"failed=1",
		"tests=1",
		"Runner failed before suite results were emitted",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("GitHub output missing %q:\n%s", want, output)
		}
	}
}

func TestBuildSharedStackRetriesNoCacheOnBuildKitSnapshotFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("compose retry test uses a POSIX shell script stub")
	}
	tmp := t.TempDir()
	callsPath := filepath.Join(tmp, "calls.txt")
	scriptPath := filepath.Join(tmp, "compose.sh")
	t.Setenv("CALLS_FILE", callsPath)

	script := `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS_FILE"
case "$*" in
  *"build --no-cache"*)
    exit 0
    ;;
  *"build"*)
    echo "failed to solve: failed to stat active key during commit: snapshot abc does not exist: not found" >&2
    exit 17
    ;;
esac
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	r := &Runner{
		stdout:   &stdout,
		stderr:   &stderr,
		repoRoot: tmp,
		compose:  []string{scriptPath},
		logsMode: "hide",
	}

	if code := r.buildSharedStack("compose.yml"); code != 0 {
		t.Fatalf("buildSharedStack returned %d, stderr: %s", code, stderr.String())
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"-f compose.yml build",
		"-f compose.yml build --no-cache",
	} {
		if !strings.Contains(string(calls), want) {
			t.Fatalf("expected compose call %q, got:\n%s", want, calls)
		}
	}
	if !strings.Contains(stdout.String(), "retrying shared-stack build with --no-cache") {
		t.Fatalf("stdout should mention retry, got:\n%s", stdout.String())
	}
}

func TestBuildSharedStackDoesNotRetryNonCacheFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("compose retry test uses a POSIX shell script stub")
	}
	tmp := t.TempDir()
	callsPath := filepath.Join(tmp, "calls.txt")
	scriptPath := filepath.Join(tmp, "compose.sh")
	t.Setenv("CALLS_FILE", callsPath)

	script := `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS_FILE"
echo "real build error" >&2
exit 23
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	r := &Runner{
		stdout:   &stdout,
		stderr:   &stderr,
		repoRoot: tmp,
		compose:  []string{scriptPath},
		logsMode: "hide",
	}

	if code := r.buildSharedStack("compose.yml"); code != 23 {
		t.Fatalf("buildSharedStack returned %d, want 23; stderr: %s", code, stderr.String())
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "build") != 1 {
		t.Fatalf("non-cache failure should not retry, calls:\n%s", calls)
	}
	if strings.Contains(stdout.String(), "--no-cache") {
		t.Fatalf("stdout should not mention no-cache retry, got:\n%s", stdout.String())
	}
}

func TestBringUpSharedStackCloakBuildsSupportImagesOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := &Runner{
		args:     Args{DryRun: true},
		stdout:   &stdout,
		stderr:   &stderr,
		compose:  []string{"docker", "compose"},
		logsMode: "hide",
		overrides: &providerOverrides{
			provider:     "cloak",
			image:        defaultCloakImage,
			composeFiles: []string{"/tmp/docker-compose.cloak.yml"},
		},
	}

	if code := r.bringUpSharedStack("compose.yml", []string{"pinchtab", "fixtures"}); code != 0 {
		t.Fatalf("bringUpSharedStack returned %d, stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"docker compose -f compose.yml -f /tmp/docker-compose.cloak.yml build fixtures runner-api runner-cli",
		"docker compose -f compose.yml -f /tmp/docker-compose.cloak.yml up -d --no-build --force-recreate pinchtab fixtures",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, " build ") && strings.Contains(line, " pinchtab") {
			t.Fatalf("cloak provider must not rebuild pinchtab services:\n%s", out)
		}
	}
}

func TestBringUpSharedStackCloakBuildsStockImageForGhostChrome(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := &Runner{
		args:     Args{DryRun: true},
		stdout:   &stdout,
		stderr:   &stderr,
		compose:  []string{"docker", "compose"},
		logsMode: "hide",
		overrides: &providerOverrides{
			provider:     "cloak",
			image:        defaultCloakImage,
			composeFiles: []string{"/tmp/docker-compose.cloak.yml"},
		},
	}

	// pinchtab-ghostchrome is keepStockProvider: it stays on e2e-pinchtab:latest
	// even under cloak, so the stock pinchtab image must be built or the
	// `up --no-build` fails with "No such image: e2e-pinchtab:latest".
	services := []string{"pinchtab", "pinchtab-ghostchrome", "fixtures"}
	if code := r.bringUpSharedStack("compose.yml", services); code != 0 {
		t.Fatalf("bringUpSharedStack returned %d, stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"docker compose -f compose.yml build pinchtab",
		"docker compose -f compose.yml -f /tmp/docker-compose.cloak.yml build fixtures runner-api runner-cli",
		"docker compose -f compose.yml -f /tmp/docker-compose.cloak.yml up -d --no-build --force-recreate pinchtab pinchtab-ghostchrome fixtures",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("cloak ghostchrome flow missing %q:\n%s", want, out)
		}
	}
	bad := "docker compose -f compose.yml -f /tmp/docker-compose.cloak.yml build fixtures runner-api runner-cli pinchtab"
	if strings.Contains(out, bad) {
		t.Fatalf("stock pinchtab image must be built without cloak override:\n%s", out)
	}
}

func TestDryRunCloakProviderDoesNotRequireImage(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--suite", "api", "--browser", "cloak", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}

	out := stdout.String()
	quotedOverride := "'" + dryRunCloakComposeOverride + "'"
	for _, want := range []string{
		"browser: cloak",
		"docker compose -f tests/e2e/docker-compose.yml -f " + quotedOverride + " build fixtures runner-api runner-cli",
		"docker compose -f tests/e2e/docker-compose.yml -f " + quotedOverride + " up -d --no-build --force-recreate pinchtab fixtures",
		"PINCHTAB_E2E_BROWSER=cloak",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(stderr.String(), "image not found") {
		t.Fatalf("dry-run should not inspect the cloak image, stderr:\n%s", stderr.String())
	}
}

func TestStructuredEventTeeFiltersHumanOutputOnly(t *testing.T) {
	var human, log bytes.Buffer
	tee := &structuredEventTee{human: &human, log: &log}

	if _, err := tee.Write([]byte("visible\nE2E_RESULT\tpassed\t12\tname\nstill visible\n")); err != nil {
		t.Fatal(err)
	}
	if err := tee.Flush(); err != nil {
		t.Fatal(err)
	}

	if got := human.String(); got != "visible\nstill visible\n" {
		t.Fatalf("unexpected human output: %q", got)
	}
	if got := log.String(); !strings.Contains(got, "E2E_RESULT\tpassed\t12\tname") {
		t.Fatalf("structured event missing from log: %q", got)
	}
}

func TestRejectsRemovedLegacyAliases(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	for _, suite := range []string{"pr", "release", "all", "smoke-docker"} {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"--suite", suite, "--dry-run"}, &stdout, &stderr); code == 0 {
			t.Fatalf("Run should reject legacy alias %q, stdout: %s", suite, stdout.String())
		}
	}
}

func TestRejectsUnknownLogsMode(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"--suite", "basic", "--logs", "quiet", "--dry-run"}, &stdout, &stderr); code == 0 {
		t.Fatalf("Run should reject unknown logs mode, stdout: %s", stdout.String())
	}
}

func TestRejectsUnknownLogsModeFromEnvironment(t *testing.T) {
	t.Setenv("E2E_LOGS", "quiet")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"--suite", "basic", "--dry-run"}, &stdout, &stderr); code == 0 {
		t.Fatalf("Run should reject unknown E2E_LOGS mode, stdout: %s", stdout.String())
	}
}

func TestDryRunSmokePlan(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"--suite", "smoke", "--dry-run"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"suite:    smoke",
		`Skipping plugin-smoke: filter "" has no matching scenarios`,
		"docker compose -f tests/e2e/docker-compose-multi.yml up -d pinchtab pinchtab-secure pinchtab-autoclose pinchtab-medium pinchtab-full fixtures",
		"E2E_SUMMARY_TITLE=PinchTab E2E API Smoke Suite",
		"runner-api /bin/bash /e2e/run.sh scenario=auto-switch-smoke.sh scenario=network-route-smoke.sh scenario=recording-smoke.sh scenario=tabs-autoclose-smoke.sh",
		"E2E_SUMMARY_TITLE=PinchTab E2E CLI Smoke Suite",
		"runner-cli /bin/bash /e2e/run.sh scenario=system-smoke.sh scenario=tabs-smoke.sh",
		"docker compose -f tests/e2e/docker-compose-multi.yml restart pinchtab",
		"E2E_SUMMARY_TITLE=PinchTab E2E Infra Smoke Suite",
		"runner-api /bin/bash /e2e/run.sh scenario=autosolver-smoke.sh scenario=browser-routing-smoke.sh scenario=dashboard-smoke.sh scenario=orchestrator-smoke.sh scenario=security-smoke.sh",
		"== E2E Docker Smoke tests (host) ==",
		"docker build --load -t pinchtab-release-smoke:dry-run .",
		"docker build --load --platform linux/amd64 -f tests/tools/docker/chrome-cft-smoke.Dockerfile -t pinchtab-chrome-cft-smoke:dry-run .",
		"bash scripts/docker-smoke.sh pinchtab-release-smoke:dry-run",
		"bash scripts/docker-chrome-cft-smoke.sh pinchtab-chrome-cft-smoke:dry-run",
		"bash scripts/docker-port-conflict-smoke.sh pinchtab-chrome-cft-smoke:dry-run",
		"bash scripts/docker-mcp-smoke.sh pinchtab-release-smoke:dry-run",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{
		"scenario=actions-basic.sh",
		"scenario=orchestrator-extended.sh",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("smoke dry-run should not include %q:\n%s", forbidden, out)
		}
	}
}

func TestDryRunSmokeFilterDockerPlan(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"--suite", "smoke", "--filter", "docker", "--dry-run"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"suite:    smoke",
		"filter: docker",
		"== E2E Docker Smoke tests (host) ==",
		"docker build --load -t pinchtab-release-smoke:dry-run .",
		"docker build --load --platform linux/amd64 -f tests/tools/docker/chrome-cft-smoke.Dockerfile -t pinchtab-chrome-cft-smoke:dry-run .",
		"bash scripts/docker-smoke.sh pinchtab-release-smoke:dry-run",
		"bash scripts/docker-chrome-cft-smoke.sh pinchtab-chrome-cft-smoke:dry-run",
		"bash scripts/docker-port-conflict-smoke.sh pinchtab-chrome-cft-smoke:dry-run",
		"bash scripts/docker-mcp-smoke.sh pinchtab-release-smoke:dry-run",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "docker compose -f tests/e2e/docker-compose-multi.yml up") {
		t.Fatalf("smoke --filter docker should not start the compose stack:\n%s", out)
	}
}

func TestDryRunSmokeFilterMCPAddsImageBuildDependency(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"--suite", "smoke", "--filter", "mcp", "--dry-run"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"docker build --load -t pinchtab-release-smoke:dry-run .",
		"bash scripts/docker-mcp-smoke.sh pinchtab-release-smoke:dry-run",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{
		"scripts/docker-smoke.sh",
		"scripts/docker-chrome-cft-smoke.sh",
		"scripts/docker-port-conflict-smoke.sh",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("filtered docker smoke should not include %q:\n%s", forbidden, out)
		}
	}
}

func TestDryRunSmokeOrchestratorDoesNotRunDockerSmoke(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"--suite", "smoke-orchestrator", "--dry-run"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"suite:    smoke-orchestrator",
		"runner-api /bin/bash /e2e/run.sh scenario=orchestrator-smoke.sh",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "E2E Docker Smoke tests") {
		t.Fatalf("smoke-orchestrator should not run Docker smoke steps:\n%s", out)
	}
}

func TestDryRunChromeProviderShowsChromeInPlan(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--suite", "api", "--browser", "chrome", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"browser:  chrome",
		"PINCHTAB_E2E_BROWSER=chrome",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestDryRunProviderMatrixShowsBothProviders(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--suite", "api", "--browser", "all", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"browser matrix: chrome, cloak, ghost-chrome",
		"browser matrix [1/3]: chrome",
		"browser matrix [2/3]: cloak",
		"browser matrix [3/3]: ghost-chrome",
		"PINCHTAB_E2E_BROWSER=chrome",
		"PINCHTAB_E2E_BROWSER=cloak",
		"PINCHTAB_E2E_BROWSER=ghost-chrome",
		"Browser matrix completed: all 3 browsers passed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("matrix dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestDryRunProviderMatrixCommaSeparated(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--suite", "api", "--browser", "chrome,cloak", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"browser matrix: chrome, cloak",
		"browser matrix [1/2]: chrome",
		"browser matrix [2/2]: cloak",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("comma-separated matrix dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestRejectsUnknownProvider(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"--suite", "basic", "--browser", "firefox", "--dry-run"}, &stdout, &stderr); code == 0 {
		t.Fatalf("Run should reject unknown provider, stdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown browser") {
		t.Fatalf("stderr should mention unknown provider: %s", stderr.String())
	}
}

func TestRejectsPartiallyUnknownProviderList(t *testing.T) {
	t.Setenv("E2E_LOGS", "")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"--suite", "basic", "--browser", "chrome,firefox", "--dry-run"}, &stdout, &stderr); code == 0 {
		t.Fatalf("Run should reject unknown provider in list, stdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown browser") {
		t.Fatalf("stderr should mention unknown provider: %s", stderr.String())
	}
}

func TestResolveProviderList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"chrome", []string{"chrome"}},
		{"cloak", []string{"cloak"}},
		{"all", []string{"chrome", "cloak", "ghost-chrome"}},
		{"chrome,cloak", []string{"chrome", "cloak"}},
		{"cloak,chrome", []string{"cloak", "chrome"}},
		{"chrome,chrome", []string{"chrome"}},
		{"", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolveProviderList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("resolveProviderList(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Fatalf("resolveProviderList(%q)[%d] = %q, want %q", tt.input, i, v, tt.want[i])
				}
			}
		})
	}
}

func TestParseArgsProviderDefaults(t *testing.T) {
	args, err := ParseArgs([]string{"--suite", "basic"})
	if err != nil {
		t.Fatal(err)
	}
	if args.Provider != "chrome" {
		t.Fatalf("default provider = %q, want chrome", args.Provider)
	}
	if len(args.Providers) != 1 || args.Providers[0] != "chrome" {
		t.Fatalf("default providers = %v, want [chrome]", args.Providers)
	}
}

func TestParseArgsProviderAll(t *testing.T) {
	args, err := ParseArgs([]string{"--suite", "basic", "--browser", "all"})
	if err != nil {
		t.Fatal(err)
	}
	if args.Provider != "all" {
		t.Fatalf("provider = %q, want all", args.Provider)
	}
	if len(args.Providers) != 3 || args.Providers[0] != "chrome" || args.Providers[1] != "cloak" || args.Providers[2] != "ghost-chrome" {
		t.Fatalf("providers = %v, want [chrome cloak ghost-chrome]", args.Providers)
	}
}

func TestParseArgsProviderEqualsFormat(t *testing.T) {
	args, err := ParseArgs([]string{"--suite", "basic", "--browser=cloak"})
	if err != nil {
		t.Fatal(err)
	}
	if args.Provider != "cloak" {
		t.Fatalf("provider = %q, want cloak", args.Provider)
	}
	if len(args.Providers) != 1 || args.Providers[0] != "cloak" {
		t.Fatalf("providers = %v, want [cloak]", args.Providers)
	}
}

func TestNewBrowserScenariosInCatalog(t *testing.T) {
	r := &Runner{repoRoot: resolveRepoRoot()}
	catalog, err := r.loadScenarioCatalog()
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{
		"api/browser-route-basic.sh",
		"api/browser-selection-basic.sh",
		"api/browser-activity-basic.sh",
		"infra/browser-instance-basic.sh",
		"infra/browser-config-basic.sh",
	} {
		parts := strings.SplitN(key, "/", 2)
		meta, ok := catalog.find(parts[0], parts[1])
		if !ok {
			t.Fatalf("scenario %s missing from catalog", key)
		}
		if meta.Tier != tierBasic {
			t.Fatalf("scenario %s has tier %q, want basic", key, meta.Tier)
		}
		if !hasString(meta.Tags, "browser") {
			t.Fatalf("scenario %s missing 'browser' tag: %v", key, meta.Tags)
		}
		if !hasString(meta.Tags, "pr") {
			t.Fatalf("scenario %s missing 'pr' tag: %v", key, meta.Tags)
		}
	}
}
