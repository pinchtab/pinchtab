package e2e

import (
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// tabCleanupHarness sources the real helpers, stubs the two I/O seams
// (_e2e_snapshot_tab_ids and e2e_curl), then drives _e2e_close_leaked_tabs and
// prints one "CLOSED <id>" line per tab it tried to close. Overriding the seams
// after the source is what keeps the function under test the real one.
// The close call inside _e2e_close_leaked_tabs redirects stdout/stderr to
// /dev/null, so the stub records to $CLOSE_LOG (which the redirect does not
// touch) rather than to stdout.
const tabCleanupHarness = `
set -uo pipefail
source helpers/base.sh >/dev/null 2>&1
source helpers/api-http.sh >/dev/null 2>&1
_e2e_snapshot_tab_ids() { local t; for t in ${STUB_CURRENT:-}; do echo "$t"; done; }
e2e_curl() {
  local a
  for a in "$@"; do
    case "$a" in
      *'"tabId"'*) printf 'CLOSED %s\n' "$(printf '%s' "$a" | sed -n 's/.*"tabId":"\([^"]*\)".*/\1/p')" >> "$CLOSE_LOG" ;;
    esac
  done
}
SCENARIO_TAB_BASELINE="${BASELINE:-}"
SCENARIO_TAB_BASELINE_OK="${OK:-0}"
_e2e_close_leaked_tabs
`

func runTabCleanup(t *testing.T, baseline, ok, current string) []string {
	t.Helper()
	closeLog := t.TempDir() + "/closed.log"
	cmd := exec.Command("bash", "-c", tabCleanupHarness)
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "BASELINE="+baseline, "OK="+ok, "STUB_CURRENT="+current, "CLOSE_LOG="+closeLog)
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("harness failed: %v\n%s", err, raw)
	}
	logged, err := os.ReadFile(closeLog)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read close log: %v", err)
	}
	var closed []string
	for line := range strings.SplitSeq(string(logged), "\n") {
		if rest, ok := strings.CutPrefix(line, "CLOSED "); ok {
			closed = append(closed, strings.TrimSpace(rest))
		}
	}
	sort.Strings(closed)
	return closed
}

func TestCloseLeakedTabsClosesOnlyNonBaselineTabs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper under test is a bash function")
	}

	t.Run("keeps the baseline tab and closes the scenario's, even when it is listed first", func(t *testing.T) {
		// GET /tabs lists the current tab first (PIN-370); after a scenario that
		// opened T1 the list is [T1, T0, T2] with baseline T0. Only T0 must survive.
		got := runTabCleanup(t, "T0", "1", "T1 T0 T2")
		want := []string{"T1", "T2"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("closed %v, want %v (baseline tab must survive, scenario tabs must close)", got, want)
		}
	})

	t.Run("a successful empty baseline closes every tab the scenario opened", func(t *testing.T) {
		// A scenario that starts with zero tabs (the normal state after a prior
		// scenario closed its own) opens T1 and T2; both are leaks and must close.
		// This is the case the empty-baseline skip got wrong.
		got := runTabCleanup(t, "", "1", "T1 T2")
		want := []string{"T1", "T2"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("closed %v, want %v (a successful empty baseline must close every open tab)", got, want)
		}
	})

	t.Run("a failed start snapshot skips cleanup because an unknown baseline cannot say which tabs the scenario opened", func(t *testing.T) {
		got := runTabCleanup(t, "", "0", "T1")
		if len(got) != 0 {
			t.Fatalf("closed %v, want none (a failed baseline snapshot must skip cleanup)", got)
		}
	})
}

const tabBaselineHarness = `
set -uo pipefail
source helpers/base.sh >/dev/null 2>&1
source helpers/api-http.sh >/dev/null 2>&1
e2e_curl() {
  [ "${STUB_CURL_FAILS:-0}" = "1" ] && return 7
  printf '%s\n%s' "${STUB_BODY:-}" "${STUB_STATUS:-}"
}
_e2e_record_tab_baseline
printf 'OK=%s BASELINE=[%s]' "${SCENARIO_TAB_BASELINE_OK}" "${SCENARIO_TAB_BASELINE% }"
`

func TestRecordTabBaselineSeparatesAnEmptySnapshotFromAFailedOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper under test is a bash function")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the helper parses tab ids with jq")
	}
	for _, tc := range []struct {
		name, body, status, fails, want string
	}{
		{"a 200 with tabs records them and marks the snapshot good", `{"tabs":[{"id":"T0"},{"id":"T1"}]}`, "200", "0", "OK=1 BASELINE=[T0 T1]"},
		{"a 200 with no tabs is a good, empty baseline", `{"tabs":[]}`, "200", "0", "OK=1 BASELINE=[]"},
		{"a non-200 answer is a failed snapshot", `{"error":"no instance"}`, "503", "0", "OK=0 BASELINE=[]"},
		{"a curl failure is a failed snapshot", "", "", "1", "OK=0 BASELINE=[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "-c", tabBaselineHarness)
			cmd.Dir = "."
			cmd.Env = append(os.Environ(), "STUB_BODY="+tc.body, "STUB_STATUS="+tc.status, "STUB_CURL_FAILS="+tc.fails)
			raw, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("harness failed: %v\n%s", err, raw)
			}
			if got := strings.TrimSpace(string(raw)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
