package e2e

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type serviceState struct {
	ID       string `json:"ID"`
	Service  string `json:"Service"`
	State    string `json:"State"`
	ExitCode int    `json:"ExitCode"`

	oomKilled bool
}

// parseComposePS reads `compose ps --format json`, which emits one object per
// line on modern Compose and a single array on older versions.
func parseComposePS(raw []byte) []serviceState {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var states []serviceState
		if err := json.Unmarshal([]byte(trimmed), &states); err != nil {
			return nil
		}
		return states
	}
	var states []serviceState
	for line := range strings.SplitSeq(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var state serviceState
		if err := json.Unmarshal([]byte(line), &state); err != nil {
			continue
		}
		states = append(states, state)
	}
	return states
}

// describeServiceDeath reports the loud line for a service that is no longer
// running, and "" for one that is. Exit 137 is SIGKILL, which is what a starved
// container looks like from the outside, so the verdict is stated here rather
// than left for a reader to infer from a client-side EOF.
func describeServiceDeath(state serviceState) string {
	if strings.EqualFold(state.State, "running") {
		return ""
	}
	line := fmt.Sprintf("  container: %s is %s (exit %d)", state.Service, strings.ToLower(state.State), state.ExitCode)
	switch {
	case state.oomKilled:
		return line + " — OOM-KILLED: the container ran out of memory. Raise the service limit or the Docker VM's memory."
	case state.ExitCode == 137:
		return line + " — SIGKILL: killed from outside the process; the kernel recorded no OOM, so suspect an external stop or a starved Docker VM."
	case state.ExitCode != 0:
		return line + " — the process exited on its own; read its captured log for the reason."
	}
	return line
}

func serviceDeathReport(states []serviceState) []string {
	var lines []string
	for _, state := range states {
		if line := describeServiceDeath(state); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// reportServiceDeaths prints the state of every named service, so a run that
// failed because a container died says so instead of leaving the reader with a
// bare connection error. Both the suite-failure and the stack-bring-up paths
// call it: a starved container dies during bring-up too, where the only symptom
// is a refused connection on the readiness probe.
func (r *Runner) reportServiceDeaths(composeFile string, services []string) {
	if r.args.DryRun || len(services) == 0 {
		return
	}
	args := append([]string{"ps", "-a", "--format", "json"}, services...)
	raw, err := commandOutput(r.repoRoot, r.composeArgs(composeFile, args...))
	if len(raw) == 0 {
		if err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to read container state: %v\n", err)
		}
		return
	}
	states := parseComposePS(raw)
	for i := range states {
		states[i].oomKilled = r.containerWasOOMKilled(states[i].ID)
	}
	for _, state := range states {
		line := describeServiceDeath(state)
		if line == "" {
			continue
		}
		_, _ = fmt.Fprintln(r.stdout, line)
		_, _ = fmt.Fprintf(r.stdout, "             log: %s\n", r.captureServiceLog(composeFile, state.Service))
	}
}

// captureServiceLog writes the dead service's own log where the reader can find
// it, since the stack-bring-up path has no suite artifacts to fall back on.
func (r *Runner) captureServiceLog(composeFile, service string) string {
	rel := filepath.Join(resultsDir, "logs-stack-"+service+".log")
	if err := writeCommandOutput(filepath.Join(r.repoRoot, rel), r.repoRoot, r.composeArgs(composeFile, "logs", "--no-color", service)); err != nil {
		return "unavailable: " + err.Error()
	}
	return rel
}

func (r *Runner) containerWasOOMKilled(containerID string) bool {
	if strings.TrimSpace(containerID) == "" {
		return false
	}
	out, err := commandOutput(r.repoRoot, []string{dockerBinary(r.compose), "inspect", containerID, "--format", "{{.State.OOMKilled}}"})
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// isOutOfDiskLog spots the Docker VM running out of disk, which reaches the
// reader as an ordinary build failure with the reason buried in the build log.
func isOutOfDiskLog(log string) bool {
	return strings.Contains(strings.ToLower(log), "no space left on device")
}

const outOfDiskRemedy = "  the Docker VM is OUT OF DISK — reclaim space before re-running: docker system df, then docker builder prune -f (and docker image prune -a)"

func commandOutput(dir string, command []string) ([]byte, error) {
	return execCommand(command, dir).Output()
}

func dockerBinary(compose []string) string {
	if len(compose) > 0 && filepath.Base(compose[0]) == "docker" {
		return compose[0]
	}
	return "docker"
}
