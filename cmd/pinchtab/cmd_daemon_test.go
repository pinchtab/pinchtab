package main

import (
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/daemon"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

type recordingDaemonManager struct {
	calls []string
}

func (m *recordingDaemonManager) record(name string) { m.calls = append(m.calls, name) }

func (m *recordingDaemonManager) Preflight() error { m.record("Preflight"); return nil }

func (m *recordingDaemonManager) Install(configPath string) (string, error) {
	m.record("Install " + configPath)
	return "Installed service", nil
}

func (m *recordingDaemonManager) ServicePath() string { m.record("ServicePath"); return "/tmp/service" }

func (m *recordingDaemonManager) Start() (string, error) {
	m.record("Start")
	return "Pinchtab daemon started.", nil
}

func (m *recordingDaemonManager) Restart() (string, error) {
	m.record("Restart")
	return "Pinchtab daemon restarted.", nil
}

func (m *recordingDaemonManager) Status() (string, error) { m.record("Status"); return "", nil }

func (m *recordingDaemonManager) Stop() (string, error) {
	m.record("Stop")
	return "Pinchtab daemon stopped.", nil
}

func (m *recordingDaemonManager) Uninstall() (string, error) {
	m.record("Uninstall")
	return "Removed service", nil
}

func (m *recordingDaemonManager) ManualInstructions() string {
	m.record("ManualInstructions")
	return ""
}

func (m *recordingDaemonManager) Pid() (string, error) { m.record("Pid"); return "", nil }

func (m *recordingDaemonManager) Logs(n int) (string, error) { m.record("Logs"); return "", nil }

func stubDaemonSeams(t *testing.T, installed bool, statusErr error) (*recordingDaemonManager, *int) {
	t.Helper()

	manager := &recordingDaemonManager{}
	constructed := 0

	origStatus := daemonInstallationStatus
	origManager := daemonCurrentManager
	daemonInstallationStatus = func() (bool, error) { return installed, statusErr }
	daemonCurrentManager = func() (daemon.Manager, error) {
		constructed++
		return manager, nil
	}
	t.Cleanup(func() {
		daemonInstallationStatus = origStatus
		daemonCurrentManager = origManager
	})
	return manager, &constructed
}

func TestDaemonStartAndRestartRefuseWhenNotInstalled(t *testing.T) {
	for _, sub := range []string{"start", "restart"} {
		t.Run(sub, func(t *testing.T) {
			manager, constructed := stubDaemonSeams(t, false, nil)

			var code int
			output := captureStderr(t, func() {
				code = dispatchDaemonCommand(sub, false)
			})

			if code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			if !strings.Contains(output, "pinchtab daemon install") {
				t.Fatalf("expected remedy in output, got %q", output)
			}
			if !strings.Contains(output, "not installed") {
				t.Fatalf("expected not-installed wording, got %q", output)
			}
			for _, forbidden := range []string{"launchctl", "systemctl", "as root"} {
				if strings.Contains(output, forbidden) {
					t.Fatalf("output leaks %q: %q", forbidden, output)
				}
			}
			if len(manager.calls) != 0 {
				t.Fatalf("manager was asked to act: %v", manager.calls)
			}
			if *constructed != 0 {
				t.Fatalf("manager constructed %d times, want 0", *constructed)
			}
		})
	}
}

func TestDaemonStartRefusesWhenInstallationStateUnknown(t *testing.T) {
	manager, _ := stubDaemonSeams(t, false, errors.New("permission denied"))

	var code int
	output := captureStderr(t, func() {
		code = dispatchDaemonCommand("start", false)
	})

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(output, "cannot determine") || !strings.Contains(output, "permission denied") {
		t.Fatalf("expected cannot-determine wording, got %q", output)
	}
	if strings.Contains(output, "not installed") {
		t.Fatalf("output must not claim not installed: %q", output)
	}
	if len(manager.calls) != 0 {
		t.Fatalf("manager was asked to act: %v", manager.calls)
	}
}

func TestDaemonActionsReachManager(t *testing.T) {
	cases := []struct {
		sub       string
		installed bool
		want      []string
	}{
		{sub: "start", installed: true, want: []string{"Start"}},
		{sub: "restart", installed: true, want: []string{"Restart"}},
		{sub: "stop", installed: true, want: []string{"Stop"}},
		{sub: "uninstall", installed: true, want: []string{"Uninstall"}},
	}
	for _, tc := range cases {
		t.Run(tc.sub, func(t *testing.T) {
			manager, _ := stubDaemonSeams(t, tc.installed, nil)

			var code int
			captureStdout(t, func() {
				code = dispatchDaemonCommand(tc.sub, false)
			})

			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if strings.Join(manager.calls, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("manager calls = %v, want %v", manager.calls, tc.want)
			}
		})
	}
}

func TestDaemonStatusMatchesBareForm(t *testing.T) {
	// An empty HOME keeps both captures off the live service path and log dir:
	// the overview's log tail would otherwise read ~/.pinchtab/logs between the
	// two captures and differ by whatever the running daemon logged in between.
	t.Setenv("HOME", t.TempDir())

	for _, jsonOut := range []bool{false, true} {
		bare := captureStdout(t, func() { dispatchDaemonCommand("", jsonOut) })
		status := captureStdout(t, func() { dispatchDaemonCommand("status", jsonOut) })
		if bare != status {
			t.Fatalf("json=%v: status output differs from bare form\nbare:   %q\nstatus: %q", jsonOut, bare, status)
		}
	}
}

func TestDaemonUsageListsStatusForm(t *testing.T) {
	stubDaemonSeams(t, true, nil)

	var code int
	output := captureStderr(t, func() {
		code = dispatchDaemonCommand("bogus", false)
	})

	if code != unknownSubcommandExitCode {
		t.Fatalf("exit code = %d, want the one unknown-subcommand code %d used across the CLI", code, unknownSubcommandExitCode)
	}
	if !strings.Contains(output, "pinchtab daemon <status|install|start|restart|stop|uninstall>") {
		t.Fatalf("expected usage line listing status, got %q", output)
	}
}

func TestPrintDaemonStatusJSONShape(t *testing.T) {
	output := captureStdout(t, func() {
		printDaemonStatusJSON()
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}
	for _, key := range []string{"installed", "running"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("expected key %q in JSON output, got %s", key, output)
		}
	}
}

func TestCollectDaemonStatusInvariants(t *testing.T) {
	st := collectDaemonStatus()

	// PID and ServicePath are only meaningful when running/installed; they must
	// stay empty otherwise so the renderers don't print stale rows.
	if !st.Running && st.PID != "" {
		t.Fatalf("PID should be empty when not running, got %q", st.PID)
	}
	if !st.Installed && st.ServicePath != "" {
		t.Fatalf("ServicePath should be empty when not installed, got %q", st.ServicePath)
	}
	// A manager error short-circuits collection before any probing fields are set.
	if st.ManagerError != "" {
		if st.PID != "" || st.ServicePath != "" || st.PreflightError != "" {
			t.Fatalf("expected no probe fields when ManagerError set, got %+v", st)
		}
	}
}

func TestPrintDaemonOverviewIncludesStatusAndHints(t *testing.T) {
	output := captureStdout(t, func() {
		printDaemonOverview()
	})
	if runtime.GOOS == "windows" {
		for _, needle := range []string{"Daemon", "supported on macOS and Linux", "current OS is windows"} {
			if !strings.Contains(output, needle) {
				t.Fatalf("expected output to contain %q\n%s", needle, output)
			}
		}
		return
	}

	for _, needle := range []string{
		"Daemon",
		"service",
		"state",
		"Manage daemon:",
		"pinchtab daemon --json",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("expected output to contain %q\n%s", needle, output)
		}
	}
}

func TestDaemonStopAndUninstallReportAnHonestNoOpWhenNotInstalled(t *testing.T) {
	for _, tc := range []struct{ sub, claim string }{
		{sub: "stop", claim: "stopped"},
		{sub: "uninstall", claim: "uninstalled"},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			stubDaemonSeams(t, false, nil)

			var code int
			output := captureStdout(t, func() {
				code = dispatchDaemonCommand(tc.sub, false)
			})

			if code != 0 {
				t.Fatalf("exit code = %d, want 0 — the contract is idempotent", code)
			}
			if strings.Contains(output, tc.claim) {
				t.Errorf("output claims the service was %s when nothing was installed: %q", tc.claim, output)
			}
			if strings.Contains(strings.ToLower(output), "nothing to") {
				t.Errorf("output %q predicts there was nothing to do; the installation probe cannot support that claim about a running job", output)
			}
			if !strings.Contains(strings.ToLower(output), "not installed") {
				t.Errorf("output %q does not state the observed fact", output)
			}
		})
	}
}

func TestDaemonNoOpVerbsStillActWhenNotInstalled(t *testing.T) {
	for _, tc := range []struct{ sub, want string }{
		{sub: "stop", want: "Stop"},
		{sub: "uninstall", want: "Uninstall"},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			manager, constructed := stubDaemonSeams(t, false, nil)

			var code int
			captureStdout(t, func() {
				code = dispatchDaemonCommand(tc.sub, false)
			})

			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if *constructed != 1 {
				t.Fatalf("manager constructed %d times, want 1 — an absent service file is not an absent job", *constructed)
			}
			if strings.Join(manager.calls, ",") != tc.want {
				t.Errorf("manager calls = %v, want [%s] — a job left loaded without its service file has no other way out", manager.calls, tc.want)
			}
		})
	}
}

func TestDaemonStopAndUninstallDoNotClaimNothingToDoWhenTheStateIsUnknown(t *testing.T) {
	for _, tc := range []struct{ sub, want string }{
		{sub: "stop", want: "Stop"},
		{sub: "uninstall", want: "Uninstall"},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			manager, _ := stubDaemonSeams(t, false, errors.New("permission denied"))

			var code int
			output := captureStdout(t, func() {
				code = dispatchDaemonCommand(tc.sub, false)
			})

			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			for _, forbidden := range []string{"nothing to", "not installed"} {
				if strings.Contains(strings.ToLower(output), forbidden) {
					t.Errorf("output %q claims %q from a state that could not be read", output, forbidden)
				}
			}
			if strings.Join(manager.calls, ",") != tc.want {
				t.Errorf("manager calls = %v, want [%s] — an unknown state must be attempted, not assumed absent", manager.calls, tc.want)
			}
		})
	}
}

func TestDaemonStopStillReportsTheRealResultWhenInstalled(t *testing.T) {
	manager, _ := stubDaemonSeams(t, true, nil)

	var code int
	output := captureStdout(t, func() {
		code = dispatchDaemonCommand("stop", false)
	})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(output, "Pinchtab daemon stopped.") {
		t.Errorf("output %q is not the manager's real result", output)
	}
	if strings.Contains(strings.ToLower(output), "nothing to stop") {
		t.Errorf("the no-op wording is unconditional: %q", output)
	}
	if strings.Join(manager.calls, ",") != "Stop" {
		t.Errorf("manager calls = %v, want [Stop]", manager.calls)
	}
}

func TestEveryDispatchedDaemonVerbDeclaresItsNotInstalledPolicy(t *testing.T) {
	pkg := srccensus.Load(t, ".", 40)
	body := pkg.Source(pkg.MustFunc(t, "dispatchDaemonCommand"))

	var verbs []string
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `case "`) {
			continue
		}
		verbs = append(verbs, strings.TrimSuffix(strings.TrimPrefix(line, `case "`), `":`))
	}
	if len(verbs) == 0 {
		t.Fatal("found no dispatched verbs; this census would pass vacuously")
	}

	for _, verb := range verbs {
		if _, declared := daemonNotInstalledPolicies[verb]; !declared {
			t.Errorf("dispatchDaemonCommand handles %q but it declares no not-installed policy, so it inherits no guard", verb)
		}
	}
	for verb, policy := range daemonNotInstalledPolicies {
		if policy == daemonNoOp && daemonNotInstalledResults[verb] == "" {
			t.Errorf("%q is a no-op verb with no honest wording, so it would print an empty claim", verb)
		}
		if policy != daemonNoOp && daemonNotInstalledResults[verb] != "" {
			t.Errorf("%q is not a no-op verb but carries no-op wording", verb)
		}
	}
}
