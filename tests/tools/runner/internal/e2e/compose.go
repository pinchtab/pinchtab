package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (r *Runner) bringUpSharedStack(composeFile string, services, buildServices []string) int {
	// Cloak pinchtab services are supplied by the provider override image.
	// Build support images such as fixtures and runners, but keep compose from
	// rebuilding the overridden pinchtab services.
	skipPinchtabBuild := r.overrides != nil && r.overrides.provider == "cloak"
	if skipPinchtabBuild {
		// keepStockProvider services (e.g. pinchtab-ghostchrome) stay pinned to
		// the stock e2e-pinchtab:latest image even in the cloak lane. If the
		// suite brings any of them up, that image must exist or `up --no-build`
		// fails with "No such image". Build that stock image via the base compose
		// only so the cloak override cannot retag it to the provider image.
		if needsStockPinchtabImage(services) {
			if code := r.buildSharedStackWithOverrides(composeFile, false, "pinchtab"); code != 0 {
				return code
			}
		}
		if code := r.buildSharedStack(composeFile, cloakSupportBuildServices()...); code != 0 {
			return code
		}
	} else {
		if code := r.buildSharedStack(composeFile, buildServices...); code != 0 {
			return code
		}
	}
	args := []string{"up", "-d"}
	if skipPinchtabBuild {
		args = append(args, "--no-build", "--force-recreate")
	}
	args = append(args, services...)
	if code := r.runLoggedCommand("starting shared stack", stackOutput, r.composeArgs(composeFile, args...)); code != 0 {
		r.reportServiceDeaths(composeFile, services)
		return code
	}
	if err := r.assertStealthStatus(composeFile); err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: pre-suite stealth assertion failed: %v\n", err)
		r.reportServiceDeaths(composeFile, services)
		return 1
	}
	return 0
}

func (r *Runner) buildSharedStack(composeFile string, services ...string) int {
	return r.buildSharedStackWithOverrides(composeFile, true, services...)
}

func (r *Runner) buildSharedStackWithOverrides(composeFile string, includeOverrides bool, services ...string) int {
	args := append([]string{"build"}, services...)
	code := r.runLoggedCommand("building shared-stack images", stackOutput, r.composeArgsWithOverrides(composeFile, includeOverrides, args...))
	if code == 0 {
		return 0
	}
	if r.stackOutputIsOutOfDisk() {
		_, _ = fmt.Fprintln(r.stdout, outOfDiskRemedy)
		return code
	}
	if !r.stackOutputHasBuildKitCacheFailure() {
		return code
	}
	_, _ = fmt.Fprintln(r.stdout, "  build cache looked stale; retrying shared-stack build with --no-cache...")
	retryArgs := append([]string{"build", "--no-cache"}, services...)
	return r.runLoggedCommand("rebuilding shared-stack images without cache", stackOutput, r.composeArgsWithOverrides(composeFile, includeOverrides, retryArgs...))
}

func cloakSupportBuildServices() []string {
	return []string{"fixtures", "runner-api", "runner-cli"}
}

// needsStockPinchtabImage reports whether any service being brought up is a
// keepStockProvider service that stays pinned to e2e-pinchtab:latest. Such
// services require the stock pinchtab image even in the cloak lane.
func needsStockPinchtabImage(services []string) bool {
	for _, svc := range services {
		for _, def := range pinchtabServiceTable {
			if def.name == svc && def.keepStockProvider {
				return true
			}
		}
	}
	return false
}

func (r *Runner) stackOutputHasBuildKitCacheFailure() bool {
	return isBuildKitCacheFailureLog(r.stackOutputLog())
}

func (r *Runner) stackOutputIsOutOfDisk() bool {
	return isOutOfDiskLog(r.stackOutputLog())
}

func (r *Runner) stackOutputLog() string {
	data, err := os.ReadFile(filepath.Join(r.repoRoot, stackOutput))
	if err != nil {
		return ""
	}
	return string(data)
}

func isBuildKitCacheFailureLog(log string) bool {
	log = strings.ToLower(log)
	return strings.Contains(log, "failed to stat active key during commit") ||
		(strings.Contains(log, "snapshot") && strings.Contains(log, "does not exist"))
}

func (r *Runner) restartSharedStack(composeFile string, services []string) int {
	args := append([]string{"restart"}, services...)
	return r.runLoggedCommand("restarting shared stack", stackOutput, r.composeArgs(composeFile, args...))
}

func (r *Runner) composeDown(composeFile string) int {
	return r.runLoggedCommand("stopping stack", stackOutput, r.composeArgs(composeFile, "down", "-v"))
}

func (r *Runner) suiteRunCommand(composeFile string, def suiteDef, scenarios []scenarioMeta) []string {
	cmd := r.composeArgs(composeFile, "run", "--rm", "--no-deps")
	for _, env := range r.suiteEnvironment(def, scenarios) {
		cmd = append(cmd, "-e", env)
	}
	cmd = append(cmd, def.Runner, "/bin/bash", "/e2e/run.sh")
	for _, scenario := range scenarios {
		cmd = append(cmd, "scenario="+scenario.File)
	}
	return cmd
}

func (r *Runner) suiteEnvironment(def suiteDef, scenarios []scenarioMeta) []string {
	provider := r.args.Provider
	if provider == "" {
		provider = defaultProvider
	}
	return []string{
		"E2E_HELPER=" + def.Helper,
		"E2E_SCENARIO_DIR=" + def.ScenarioDir,
		"E2E_REQUIRED_COMMANDS=" + strings.Join(def.Commands, " "),
		"E2E_READY_TARGETS=" + strings.Join(readyTargetsForScenarios(def, scenarios), " "),
		"E2E_TEST_FILTER=" + r.args.Test,
		"E2E_SUMMARY_TITLE=" + suiteReportTitle(def),
		"PINCHTAB_E2E_BROWSER=" + provider,
	}
}

func suiteReportTitle(def suiteDef) string {
	labels := map[string]string{
		"api":    "API",
		"cli":    "CLI",
		"infra":  "Infra",
		"plugin": "Plugin",
		"docker": "Docker",
	}
	label := labels[def.RunSuite]
	if label == "" {
		label = def.RunSuite
	}
	if def.Smoke {
		return "PinchTab E2E " + label + " Smoke Suite"
	}
	if def.Extended && def.RunSuite != "plugin" {
		return "PinchTab E2E " + label + " Extended Suite"
	}
	return "PinchTab E2E " + label + " Suite"
}

func (r *Runner) composeArgs(composeFile string, args ...string) []string {
	return r.composeArgsWithOverrides(composeFile, true, args...)
}

func (r *Runner) composeArgsWithOverrides(composeFile string, includeOverrides bool, args ...string) []string {
	out := append([]string{}, r.compose...)
	out = append(out, "-f", composeFile)
	if includeOverrides && r.overrides != nil {
		for _, override := range r.overrides.composeFiles {
			out = append(out, "-f", override)
		}
	}
	out = append(out, args...)
	return out
}

func resolveCompose(dryRun bool) ([]string, error) {
	if custom := strings.TrimSpace(os.Getenv("PINCHTAB_COMPOSE")); custom != "" {
		return strings.Fields(custom), nil
	}
	if dryRun {
		return []string{"docker", "compose"}, nil
	}
	if err := exec.Command("docker", "compose", "version").Run(); err == nil {
		return []string{"docker", "compose"}, nil
	}
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return []string{"docker-compose"}, nil
	}
	return nil, errors.New("neither 'docker compose' nor 'docker-compose' is available")
}
