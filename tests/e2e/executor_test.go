package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runProbeSuite drives the real tests/e2e/run.sh over throwaway scenario files. It needs
// no Docker and no server: the readiness loop skips a target whose variable is unset, so
// the executor's own accounting is exercised in isolation from everything it usually
// coordinates.
func runProbeSuite(t *testing.T, scenarios map[string]string, order ...string) (string, int) {
	t.Helper()

	dir := t.TempDir()
	for name, body := range scenarios {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	args := []string{"run.sh"}
	for _, name := range order {
		args = append(args, "scenario="+name)
	}
	cmd := exec.Command("bash", args...)
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"E2E_HELPER=api",
		"E2E_SCENARIO_DIR="+dir,
		"E2E_REQUIRED_COMMANDS=echo",
		"E2E_READY_TARGETS=PROBE_TARGET_DELIBERATELY_UNSET",
		"E2E_SUMMARY_TITLE=probe",
		"E2E_TEST_FILTER=",
		"E2E_LOGS=hide",
	)
	out, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running run.sh: %v\n%s", err, out)
	}
	return string(out), code
}

const passingScenario = `
start_test "probe passes"
pass_assert "ok"
end_test
`

// The executor decides a scenario's verdict from TESTS_FAILED, which the subshell cannot
// write back to the parent. Every way a scenario file can end has to reach that verdict —
// including the ways that skip the last statement.
func TestScenarioVerdictSurvivesEveryWayAFileCanEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executor under test is a bash script")
	}

	for _, tc := range []struct {
		name     string
		body     string
		wantFail bool
	}{
		{
			name:     "passing file",
			body:     passingScenario,
			wantFail: false,
		},
		{
			name:     "failed assertion, file ends normally",
			body:     "\nstart_test \"probe fails\"\nfail_assert \"deliberate\"\nend_test\n",
			wantFail: true,
		},
		{
			// The shape that shipped broken: a guard that records a failure and then
			// exits 0 to stop the file. cli/library-mode-smoke.sh does exactly this when
			// its binary is missing from the runner image, so a lane could print a failed
			// assertion and still go green.
			name:     "failed assertion then an explicit exit 0",
			body:     "\nstart_test \"probe fails then bails\"\nfail_assert \"deliberate\"\nend_test\nexit 0\n",
			wantFail: true,
		},
		{
			name:     "explicit non-zero exit with nothing recorded",
			body:     "\nstart_test \"probe bails hard\"\npass_assert \"ok\"\nend_test\nexit 1\n",
			wantFail: true,
		},
		{
			// A scenario whose last statement happens to return non-zero has not failed
			// a test; only recorded failures decide the verdict.
			name:     "last statement returns non-zero but nothing failed",
			body:     passingScenario + "\ngrep -q nothing-here /dev/null\n",
			wantFail: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runProbeSuite(t, map[string]string{"probe.sh": tc.body}, "probe.sh")
			if failed := code != 0; failed != tc.wantFail {
				t.Errorf("suite exit = %d (failed=%v), want failed=%v; a scenario's recorded failures must decide its verdict\n%s",
					code, failed, tc.wantFail, out)
			}
		})
	}
}

// A file that dies mid-way must take nothing with it: not the files after it, not its
// cleanup, and not its variables.
func TestOneScenarioDyingLeavesTheRestOfTheSuiteIntact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executor under test is a bash script")
	}

	dying := `
leaked_var="from-first"
leaked_fn() { echo leaked; }
scenario_cleanup() { echo PROBE-CLEANUP-RAN; }

start_test "first scenario fails then dies"
fail_assert "deliberate"
end_test

exit 1

start_test "never reached"
pass_assert "unreachable"
end_test
`
	following := `
start_test "second scenario runs anyway"
if [ -n "${leaked_var:-}" ]; then
  fail_assert "leaked_var crossed the file boundary: ${leaked_var}"
else
  pass_assert "no variable crossed the file boundary"
fi
if declare -F leaked_fn >/dev/null 2>&1; then
  fail_assert "leaked_fn crossed the file boundary"
else
  pass_assert "no function crossed the file boundary"
fi
end_test
`
	out, code := runProbeSuite(t,
		map[string]string{"first.sh": dying, "second.sh": following},
		"first.sh", "second.sh")

	if code == 0 {
		t.Errorf("suite passed despite a failed scenario:\n%s", out)
	}
	if !strings.Contains(out, "PROBE-CLEANUP-RAN") {
		t.Errorf("scenario_cleanup did not run for a file that exited early:\n%s", out)
	}
	if !strings.Contains(out, "second scenario runs anyway") {
		t.Errorf("the scenario after the dying one never ran, so one file still amputates the suite:\n%s", out)
	}
	if strings.Contains(out, "never reached") {
		t.Errorf("execution continued past the exit inside the dying file:\n%s", out)
	}
	// The second scenario asserts the isolation itself, so its own verdict is the
	// signal: a leak makes it record a failure rather than a pass.
	if !hasResultLine(out, "passed", "[second] second scenario runs anyway") {
		t.Errorf("the second scenario did not pass, so state crossed the subshell boundary:\n%s", out)
	}
}

// hasResultLine looks for one machine-readable result line by status and test name. The
// duration between them varies per run, so the two halves are matched separately rather
// than as one literal.
func hasResultLine(out, status, name string) bool {
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "E2E_RESULT\t"+status+"\t") && strings.HasSuffix(strings.TrimRight(line, "\r"), name) {
			return true
		}
	}
	return false
}
