package e2e

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	singleCompose = "tests/e2e/docker-compose.yml"
	multiCompose  = "tests/e2e/docker-compose-multi.yml"
	resultsDir    = "tests/e2e/results"
	stackOutput   = "tests/e2e/results/output-e2e-stack.log"
)

type Runner struct {
	args      Args
	suite     string
	stdout    io.Writer
	stderr    io.Writer
	repoRoot  string
	compose   []string
	logsMode  string
	overall   overallReportData
	overrides *providerOverrides
}

type overallReportData struct {
	Suites   int
	Passed   int
	Failed   int
	TotalMs  int64
	Failures []string
}

type suiteDef struct {
	Name        string
	Title       string
	Compose     string
	GroupDir    string
	Helper      string
	ScenarioDir string
	Commands    []string
	Ready       []string
	Runner      string
	RunSuite    string
	Extended    bool
	Smoke       bool
	Summary     string
	Report      string
	LogPrefix   string
	Output      string
	LogServices []string
}

type suitePlan struct {
	def       suiteDef
	scenarios []scenarioMeta
}

type dockerSmokeStep struct {
	Name                 string
	Tags                 []string
	Command              []string
	ProvidesReleaseImage bool
	ProvidesChromeImage  bool
	RequiresReleaseImage bool
	RequiresChromeImage  bool
}

func NewRunner(args Args, stdout, stderr io.Writer) (*Runner, error) {
	suite, err := normalizeSuite(args.Suite)
	if err != nil {
		return nil, err
	}

	compose, err := resolveCompose(args.DryRun)
	if err != nil {
		return nil, err
	}

	logsMode := args.Logs
	if logsMode == "" {
		logsMode = strings.TrimSpace(os.Getenv("E2E_LOGS"))
	}
	if logsMode == "" {
		logsMode = "compact"
	}
	switch logsMode {
	case "show", "hide", "compact":
	default:
		return nil, fmt.Errorf("--logs must be show, hide, or compact")
	}

	return &Runner{
		args:     args,
		suite:    suite,
		stdout:   stdout,
		stderr:   stderr,
		repoRoot: resolveRepoRoot(),
		compose:  compose,
		logsMode: logsMode,
	}, nil
}

func (r *Runner) Run() int {
	started := time.Now()
	overrides, err := r.prepareProviderOverrides()
	if err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: %v\n", err)
		return 1
	}
	r.overrides = overrides
	if overrides != nil {
		defer overrides.cleanup()
		_, _ = fmt.Fprintf(r.stdout, "  browser: %s (image: %s)\n", overrides.provider, overrides.image)
	}
	code := r.run()
	duration := time.Since(started)
	r.printOverallSummary(duration)
	r.writeGitHubActionsMetadata(duration, code)
	return code
}

func (r *Runner) run() int {
	if r.args.DryRun {
		r.printPlanHeader()
	}

	if err := os.MkdirAll(filepath.Join(r.repoRoot, resultsDir), 0o755); err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to create results directory: %v\n", err)
		return 1
	}
	if !r.args.DryRun {
		_ = os.Remove(filepath.Join(r.repoRoot, stackOutput))
	}

	switch r.suite {
	case "basic":
		return r.runBasic()
	case "extended":
		return r.runExtended()
	case "smoke":
		return r.runSmoke()
	case "smoke-orchestrator":
		return r.runSmokeFiltered("orchestrator")
	case "smoke-security":
		return r.runSmokeFiltered("security")
	case "smoke-lifecycle":
		return r.runSmokeFiltered("lifecycle")
	case "api":
		return r.runSingle(apiSuite())
	case "cli":
		return r.runSingle(cliSuite())
	case "infra":
		return r.runSingle(infraSuite())
	case "plugin":
		return r.runSingle(pluginSuite())
	case "api-extended":
		return r.runSingle(apiExtendedSuite())
	case "cli-extended":
		return r.runSingle(cliExtendedSuite())
	case "infra-extended":
		return r.runSingle(infraExtendedSuite())
	default:
		_, _ = fmt.Fprintf(r.stderr, "e2e: unknown suite %q\n", r.suite)
		return 1
	}
}

func (r *Runner) printPlanHeader() {
	_, _ = fmt.Fprintln(r.stdout, "runner e2e (Go) - resolved plan")
	_, _ = fmt.Fprintf(r.stdout, "  suite:    %s\n", r.suite)
	_, _ = fmt.Fprintf(r.stdout, "  browser:  %s\n", r.args.Provider)
	_, _ = fmt.Fprintf(r.stdout, "  logs:     %s\n", r.logsMode)
	if r.args.Filter != "" {
		_, _ = fmt.Fprintf(r.stdout, "  filter:   %s\n", r.args.Filter)
	}
	if r.args.Test != "" {
		_, _ = fmt.Fprintf(r.stdout, "  test:     %s\n", r.args.Test)
	}
	if r.args.Extra != "" {
		_, _ = fmt.Fprintf(r.stdout, "  extra:    %s\n", r.args.Extra)
	}
	_, _ = fmt.Fprintln(r.stdout, "")
}

// stackPlanRun configures a shared-stack suite run for bringUpAndRunPlans.
type stackPlanRun struct {
	stack string
	plans []suitePlan
	// restartAfter returns the services to restart after the plan at index i of
	// n (nil/empty = no restart), letting each caller encode its own timing.
	restartAfter func(plan suitePlan, index, total int) []string
}

// bringUpAndRunPlans builds+starts the shared stack for the plans' services,
// then runs each plan in order, restarting services per cfg.restartAfter
// between plans and printing a blank line after each. It returns per-plan exit
// codes keyed by def.Name (only non-zero codes are stored), whether any restart
// returned non-zero, and a non-zero setupCode if bringup failed. It does NOT
// tear down the stack — the caller owns teardown semantics.
func (r *Runner) bringUpAndRunPlans(cfg stackPlanRun) (codes map[string]int, restartFailed bool, setupCode int) {
	services := servicesForPlans(cfg.plans, []string{"pinchtab", "fixtures"})
	if code := r.bringUpSharedStack(cfg.stack, services); code != 0 {
		return nil, false, code
	}

	codes = map[string]int{}
	for i, plan := range cfg.plans {
		if code := r.runSinglePlanWithCompose(plan, cfg.stack); code != 0 {
			codes[plan.def.Name] = code
		}
		if svcs := cfg.restartAfter(plan, i, len(cfg.plans)); len(svcs) > 0 {
			if rc := r.restartSharedStack(cfg.stack, svcs); rc != 0 {
				restartFailed = true
			}
		}
		_, _ = fmt.Fprintln(r.stdout, "")
	}
	return codes, restartFailed, 0
}

func (r *Runner) runBasic() int {
	stack := singleCompose
	exitCodes := map[string]int{"api": 0, "cli": 0, "infra": 0}
	plans, code := r.planSuites([]suiteDef{apiSuite(), cliSuite(), infraSuite()})
	if code != 0 {
		return code
	}

	if len(plans) == 0 {
		_, _ = fmt.Fprintf(r.stderr, "e2e: no basic suites matched filter %q\n", r.args.Filter)
		return 1
	}

	codes, _, setupCode := r.bringUpAndRunPlans(stackPlanRun{
		stack: stack,
		plans: plans,
		restartAfter: func(suitePlan, int, int) []string {
			return nil
		},
	})
	if setupCode != 0 {
		_ = r.composeDown(stack)
		return setupCode
	}
	defer r.composeDown(stack)

	for name, code := range codes {
		exitCodes[name] = code
	}

	if exitCodes["api"] != 0 || exitCodes["cli"] != 0 || exitCodes["infra"] != 0 {
		_, _ = fmt.Fprintln(r.stderr, "e2e: basic suites failed")
		_, _ = fmt.Fprintf(r.stderr, "e2e: exit codes: api=%d, cli=%d, infra=%d\n", exitCodes["api"], exitCodes["cli"], exitCodes["infra"])
		return 1
	}
	if !r.args.DryRun {
		_, _ = fmt.Fprintln(r.stdout, "E2E basic suites passed")
	}
	return 0
}

func (r *Runner) runExtended() int {
	stack := multiCompose
	exitCodes := map[string]int{
		"api-extended":   0,
		"cli-extended":   0,
		"infra-extended": 0,
		"plugin":         0,
	}
	defs := []suiteDef{apiExtendedSuite(), cliExtendedSuite(), infraExtendedSuite(), pluginSuite()}
	plans, code := r.planSuites(defs)
	if code != 0 {
		return code
	}
	if len(plans) == 0 {
		_, _ = fmt.Fprintf(r.stderr, "e2e: no extended suites matched filter %q\n", r.args.Filter)
		return 1
	}

	codes, _, setupCode := r.bringUpAndRunPlans(stackPlanRun{
		stack: stack,
		plans: plans,
		restartAfter: func(plan suitePlan, _, _ int) []string {
			if plan.def.Name == "cli-extended" || plan.def.Name == "infra-extended" {
				return []string{"pinchtab"}
			}
			return nil
		},
	})
	if setupCode != 0 {
		_ = r.composeDown(stack)
		return setupCode
	}
	defer r.composeDown(stack)

	for name, code := range codes {
		exitCodes[name] = code
	}

	if exitCodes["api-extended"] != 0 || exitCodes["cli-extended"] != 0 || exitCodes["infra-extended"] != 0 || exitCodes["plugin"] != 0 {
		_, _ = fmt.Fprintln(r.stderr, "e2e: extended suites failed")
		_, _ = fmt.Fprintf(r.stderr, "e2e: exit codes: api-extended=%d, cli-extended=%d, infra-extended=%d, plugin=%d\n",
			exitCodes["api-extended"], exitCodes["cli-extended"], exitCodes["infra-extended"], exitCodes["plugin"])
		return 1
	}
	if !r.args.DryRun {
		_, _ = fmt.Fprintln(r.stdout, "E2E extended suites passed")
	}
	return 0
}

func (r *Runner) runSmokeFiltered(filter string) int {
	if r.args.Filter == "" {
		r.args.Filter = filter
	}
	return r.runSmoke()
}

func (r *Runner) runSmoke() int {
	stack := multiCompose
	defs := []suiteDef{apiSmokeSuite(), cliSmokeSuite(), infraSmokeSuite(), pluginSmokeSuite()}
	plans, code := r.planSuites(defs)
	if code != 0 {
		return code
	}
	dockerSteps := r.selectedDockerSmokeSteps()
	if len(plans) == 0 && len(dockerSteps) == 0 {
		_, _ = fmt.Fprintf(r.stderr, "e2e: no smoke suites matched filter %q\n", r.args.Filter)
		return 1
	}

	failed := false
	if len(plans) > 0 {
		codes, restartFailed, setupCode := r.bringUpAndRunPlans(stackPlanRun{
			stack: stack,
			plans: plans,
			restartAfter: func(plan suitePlan, i, n int) []string {
				if plan.def.Name == "cli-smoke" && i < n-1 {
					return []string{"pinchtab"}
				}
				return nil
			},
		})
		if setupCode != 0 {
			_ = r.composeDown(stack)
			return setupCode
		}
		if len(codes) > 0 {
			failed = true
		}
		if restartFailed {
			failed = true
		}
		if code := r.composeDown(stack); code != 0 {
			failed = true
		}
	}

	if len(dockerSteps) > 0 {
		if code := r.runDockerSmokeSteps(dockerSteps); code != 0 {
			failed = true
		}
		_, _ = fmt.Fprintln(r.stdout, "")
	}

	if failed {
		_, _ = fmt.Fprintln(r.stderr, "e2e: smoke suites failed")
		return 1
	}
	if !r.args.DryRun {
		_, _ = fmt.Fprintln(r.stdout, "E2E smoke suites passed")
	}
	return 0
}

func (r *Runner) runDockerSmokeSteps(steps []dockerSmokeStep) int {
	def := dockerSmokeSuite()
	r.printSuiteStart(def)
	r.prepareSuiteResults(def)
	started := time.Now()

	exitCode := 0
	for _, step := range steps {
		stepStarted := time.Now()
		code := r.runLoggedCommand("running "+step.Name, def.Output, step.Command)
		status := "passed"
		if code != 0 {
			status = "failed"
			exitCode = code
		}
		if err := r.appendSuiteResult(def, status, time.Since(stepStarted), "["+def.Name+"] "+step.Name); err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to record docker smoke result: %v\n", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
		if code != 0 {
			break
		}
	}

	duration := time.Since(started)
	summary := r.writeSuiteReports(def, duration, exitCode)
	r.recordOverallSummary(summary)
	r.printSuiteSummary(def, summary, duration)
	if exitCode != 0 {
		r.showFailureArtifacts(def, duration)
	}
	return exitCode
}

func (r *Runner) selectedDockerSmokeSteps() []dockerSmokeStep {
	return selectDockerSmokeSteps(r.dockerSmokeSteps(), r.args.Filter)
}

func (r *Runner) dockerSmokeSteps() []dockerSmokeStep {
	releaseImage, chromeImage, customReleaseImage, customChromeImage := r.dockerSmokeImages()
	steps := []dockerSmokeStep{}
	if !customReleaseImage {
		steps = append(steps, dockerSmokeStep{
			Name:                 "docker: build release image",
			Tags:                 []string{"docker", "build", "release", "image"},
			Command:              []string{"docker", "build", "--load", "-t", releaseImage, "."},
			ProvidesReleaseImage: true,
		})
	}
	if !customChromeImage {
		steps = append(steps, dockerSmokeStep{
			Name:                "docker: build Chrome for Testing image",
			Tags:                []string{"docker", "build", "chrome", "cft", "image"},
			Command:             []string{"docker", "build", "--load", "--platform", "linux/amd64", "-f", "tests/tools/docker/chrome-cft-smoke.Dockerfile", "-t", chromeImage, "."},
			ProvidesChromeImage: true,
		})
	}
	steps = append(steps,
		dockerSmokeStep{
			Name:                 "docker: bootstrap path in container",
			Tags:                 []string{"docker", "bootstrap", "release", "container", "config"},
			Command:              []string{"bash", "scripts/docker-smoke.sh", releaseImage},
			RequiresReleaseImage: true,
		},
		dockerSmokeStep{
			Name:                "docker: Chrome for Testing startup",
			Tags:                []string{"docker", "chrome", "cft", "startup", "health"},
			Command:             []string{"bash", "scripts/docker-chrome-cft-smoke.sh", chromeImage},
			RequiresChromeImage: true,
		},
		dockerSmokeStep{
			Name:                "docker: instance port conflict detection",
			Tags:                []string{"docker", "chrome", "cft", "ports", "conflict"},
			Command:             []string{"bash", "scripts/docker-port-conflict-smoke.sh", chromeImage},
			RequiresChromeImage: true,
		},
		dockerSmokeStep{
			Name:                 "docker: MCP stdio in container",
			Tags:                 []string{"docker", "mcp", "stdio", "release", "container"},
			Command:              []string{"bash", "scripts/docker-mcp-smoke.sh", releaseImage},
			RequiresReleaseImage: true,
		},
	)
	return steps
}

func (r *Runner) dockerSmokeImages() (releaseImage, chromeImage string, customReleaseImage, customChromeImage bool) {
	suffix := strings.TrimSpace(os.Getenv("PINCHTAB_DOCKER_SMOKE_TAG_SUFFIX"))
	if suffix == "" {
		if r.args.DryRun {
			suffix = "dry-run"
		} else {
			suffix = fmt.Sprintf("%d", time.Now().UnixNano())
		}
	}

	releaseImage = strings.TrimSpace(os.Getenv("PINCHTAB_DOCKER_SMOKE_RELEASE_IMAGE"))
	if releaseImage == "" {
		releaseImage = "pinchtab-release-smoke:" + suffix
	} else {
		customReleaseImage = true
	}

	chromeImage = strings.TrimSpace(os.Getenv("PINCHTAB_DOCKER_SMOKE_CHROME_IMAGE"))
	if chromeImage == "" {
		chromeImage = "pinchtab-chrome-cft-smoke:" + suffix
	} else {
		customChromeImage = true
	}
	return releaseImage, chromeImage, customReleaseImage, customChromeImage
}

func selectDockerSmokeSteps(steps []dockerSmokeStep, filter string) []dockerSmokeStep {
	if filter == "" {
		return steps
	}

	selected := map[int]bool{}
	for i, step := range steps {
		if dockerSmokeStepMatchesFilter(step, filter) {
			selected[i] = true
		}
	}
	if len(selected) == 0 {
		return nil
	}

	needsReleaseImage := false
	needsChromeImage := false
	for i := range selected {
		needsReleaseImage = needsReleaseImage || steps[i].RequiresReleaseImage
		needsChromeImage = needsChromeImage || steps[i].RequiresChromeImage
	}
	if needsReleaseImage {
		for i, step := range steps {
			if step.ProvidesReleaseImage {
				selected[i] = true
				break
			}
		}
	}
	if needsChromeImage {
		for i, step := range steps {
			if step.ProvidesChromeImage {
				selected[i] = true
				break
			}
		}
	}

	out := make([]dockerSmokeStep, 0, len(selected))
	for i, step := range steps {
		if selected[i] {
			out = append(out, step)
		}
	}
	return out
}

func dockerSmokeStepMatchesFilter(step dockerSmokeStep, filter string) bool {
	for _, value := range append([]string{step.Name}, step.Tags...) {
		if strings.Contains(value, filter) {
			return true
		}
	}
	return false
}

func (r *Runner) runSingle(def suiteDef) int {
	scenarios, err := r.selectedScenarioMeta(def)
	if err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: %v\n", err)
		return 1
	}
	plan := suitePlan{def: def, scenarios: scenarios}
	if code := r.bringUpSharedStack(def.Compose, servicesForPlans([]suitePlan{plan}, []string{"pinchtab", "fixtures"})); code != 0 {
		_ = r.composeDown(def.Compose)
		return code
	}
	defer r.composeDown(def.Compose)
	return r.runSinglePlanWithCompose(plan, def.Compose)
}

func (r *Runner) runSinglePlanWithCompose(plan suitePlan, composeFile string) int {
	def := plan.def
	r.printSuiteStart(def)
	r.prepareSuiteResults(def)
	started := time.Now()

	command, err := r.suiteRunCommand(composeFile, def, plan.scenarios)
	if err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: %v\n", err)
		return 1
	}

	code := r.runLoggedCommand("running "+def.Name+" suite", def.Output, command)
	duration := time.Since(started)
	summary := r.writeSuiteReports(def, duration, code)
	r.recordOverallSummary(summary)
	r.printSuiteSummary(def, summary, duration)
	if code != 0 {
		r.dumpComposeFailure(composeFile, def)
		r.showFailureArtifacts(def, duration)
	}
	return code
}

func (r *Runner) planSuites(defs []suiteDef) ([]suitePlan, int) {
	var plans []suitePlan
	for _, def := range defs {
		scenarios, err := r.selectedScenarioMeta(def)
		if err != nil {
			if errors.Is(err, errNoMatchingScenarios) {
				r.showSuiteSkip(def.Name)
				continue
			}
			_, _ = fmt.Fprintf(r.stderr, "e2e: %v\n", err)
			return nil, 1
		}
		plans = append(plans, suitePlan{def: def, scenarios: scenarios})
	}
	return plans, 0
}

func (r *Runner) bringUpSharedStack(composeFile string, services []string) int {
	// Cloak pinchtab services are supplied by the provider override image.
	// Build support images such as fixtures and runners, but keep compose from
	// rebuilding the overridden pinchtab services.
	skipPinchtabBuild := r.overrides != nil && r.overrides.provider == "cloak"
	if skipPinchtabBuild {
		buildServices := cloakSupportBuildServices()
		// keepStockProvider services (e.g. pinchtab-ghostchrome) stay pinned to
		// the stock e2e-pinchtab:latest image even in the cloak lane. If the
		// suite brings any of them up, that image must exist or `up --no-build`
		// fails with "No such image", so build the stock pinchtab service too.
		if needsStockPinchtabImage(services) {
			buildServices = append(buildServices, "pinchtab")
		}
		if code := r.buildSharedStack(composeFile, buildServices...); code != 0 {
			return code
		}
	} else {
		if code := r.buildSharedStack(composeFile); code != 0 {
			return code
		}
	}
	args := []string{"up", "-d"}
	if skipPinchtabBuild {
		args = append(args, "--no-build", "--force-recreate")
	}
	args = append(args, services...)
	if code := r.runLoggedCommand("starting shared stack", stackOutput, r.composeArgs(composeFile, args...)); code != 0 {
		return code
	}
	if err := r.assertStealthStatus(composeFile); err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: pre-suite stealth assertion failed: %v\n", err)
		return 1
	}
	return 0
}

func (r *Runner) buildSharedStack(composeFile string, services ...string) int {
	args := append([]string{"build"}, services...)
	code := r.runLoggedCommand("building shared-stack images", stackOutput, r.composeArgs(composeFile, args...))
	if code == 0 {
		return 0
	}
	if !r.stackOutputHasBuildKitCacheFailure() {
		return code
	}
	_, _ = fmt.Fprintln(r.stdout, "  build cache looked stale; retrying shared-stack build with --no-cache...")
	retryArgs := append([]string{"build", "--no-cache"}, services...)
	return r.runLoggedCommand("rebuilding shared-stack images without cache", stackOutput, r.composeArgs(composeFile, retryArgs...))
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
	data, err := os.ReadFile(filepath.Join(r.repoRoot, stackOutput))
	if err != nil {
		return false
	}
	return isBuildKitCacheFailureLog(string(data))
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

func (r *Runner) suiteRunCommand(composeFile string, def suiteDef, scenarios []scenarioMeta) ([]string, error) {
	cmd := r.composeArgs(composeFile, "run", "--rm", "--no-deps")
	for _, env := range r.suiteEnvironment(def, scenarios) {
		cmd = append(cmd, "-e", env)
	}
	cmd = append(cmd, def.Runner, "/bin/bash", "/e2e/run.sh")
	for _, scenario := range scenarios {
		cmd = append(cmd, "scenario="+scenario.File)
	}
	return cmd, nil
}

func (r *Runner) suiteEnvironment(def suiteDef, scenarios []scenarioMeta) []string {
	provider := r.args.Provider
	if provider == "" {
		provider = "chrome"
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
	out := append([]string{}, r.compose...)
	out = append(out, "-f", composeFile)
	if r.overrides != nil {
		for _, override := range r.overrides.composeFiles {
			out = append(out, "-f", override)
		}
	}
	out = append(out, args...)
	return out
}

func (r *Runner) printSuiteStart(def suiteDef) {
	_, _ = fmt.Fprintf(r.stdout, "== %s ==\n", def.Title)
	if r.args.Filter != "" {
		_, _ = fmt.Fprintf(r.stdout, "  filter: %s\n", r.args.Filter)
	} else {
		_, _ = fmt.Fprintln(r.stdout, "  filter: none")
	}
	if r.args.Test != "" {
		_, _ = fmt.Fprintf(r.stdout, "  test:   %s\n", r.args.Test)
	}
	if r.args.Extra != "" {
		_, _ = fmt.Fprintf(r.stdout, "  extra:  %s\n", r.args.Extra)
	}
	_, _ = fmt.Fprintf(r.stdout, "  logs:   %s\n\n", r.logsMode)
}

func (r *Runner) runLoggedCommand(label, outputFile string, command []string) int {
	if r.args.DryRun {
		_, _ = fmt.Fprintf(r.stdout, "# %s\n%s\n", label, shellQuoteArgs(command))
		return 0
	}

	if r.logsMode == "show" {
		_, _ = fmt.Fprintf(r.stdout, "%s\n", label)
		return r.runStreamingCommand(command, outputFile)
	}
	if outputFile == "" {
		outputFile = stackOutput
	}
	return r.runCompactCommand(label, outputFile, command)
}

func (r *Runner) appendSuiteResult(def suiteDef, status string, duration time.Duration, name string) (err error) {
	if r.args.DryRun {
		return nil
	}
	outputPath := filepath.Join(r.repoRoot, def.Output)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	if stat, err := file.Stat(); err == nil && stat.Size() > 0 {
		var last [1]byte
		if _, err := file.ReadAt(last[:], stat.Size()-1); err == nil && last[0] != '\n' {
			if _, err := fmt.Fprintln(file); err != nil {
				return err
			}
		}
	}
	_, err = fmt.Fprintf(file, "E2E_RESULT\t%s\t%d\t%s\n", status, duration.Milliseconds(), name)
	return
}

func (r *Runner) runStreamingCommand(command []string, outputFile string) int {
	cmd := exec.Command(command[0], command[1:]...) // #nosec G204 -- commands are constructed from fixed compose/script inputs.
	cmd.Dir = r.repoRoot
	var logFile *os.File
	var stdoutFilter *structuredEventTee
	if outputFile != "" {
		outputPath := filepath.Join(r.repoRoot, outputFile)
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to prepare output path: %v\n", err)
			return 1
		}
		file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to open %s: %v\n", outputFile, err)
			return 1
		}
		logFile = file
		defer func() {
			if closeErr := logFile.Close(); closeErr != nil {
				_, _ = fmt.Fprintf(r.stderr, "e2e: failed to close %s: %v\n", outputFile, closeErr)
			}
		}()
		stdoutFilter = &structuredEventTee{human: r.stdout, log: logFile}
		cmd.Stdout = stdoutFilter
		cmd.Stderr = io.MultiWriter(r.stderr, logFile)
	} else {
		cmd.Stdout = r.stdout
		cmd.Stderr = r.stderr
	}
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	err := cmd.Run()
	if stdoutFilter != nil {
		if flushErr := stdoutFilter.Flush(); flushErr != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to write output: %v\n", flushErr)
			return 1
		}
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to run %s: %v\n", shellQuoteArgs(command), err)
		return 1
	}
	return 0
}

type structuredEventTee struct {
	human   io.Writer
	log     io.Writer
	pending []byte
}

func (w *structuredEventTee) Write(p []byte) (int, error) {
	if _, err := w.log.Write(p); err != nil {
		return 0, err
	}

	remaining := p
	for len(remaining) > 0 {
		idx := bytes.IndexByte(remaining, '\n')
		if idx < 0 {
			w.pending = append(w.pending, remaining...)
			break
		}
		line := append(w.pending, remaining[:idx+1]...)
		w.pending = w.pending[:0]
		if err := w.writeHumanLine(line); err != nil {
			return 0, err
		}
		remaining = remaining[idx+1:]
	}
	return len(p), nil
}

func (w *structuredEventTee) Flush() error {
	if len(w.pending) == 0 {
		return nil
	}
	line := w.pending
	w.pending = nil
	return w.writeHumanLine(line)
}

func (w *structuredEventTee) writeHumanLine(line []byte) error {
	text := strings.TrimRight(string(line), "\r\n")
	if strings.HasPrefix(text, "E2E_RESULT\t") || strings.HasPrefix(text, "E2E_RESULT_SUMMARY\t") {
		return nil
	}
	_, err := w.human.Write(line)
	return err
}

func (r *Runner) runCompactCommand(label, outputFile string, command []string) int {
	outputPath := filepath.Join(r.repoRoot, outputFile)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to prepare output path: %v\n", err)
		return 1
	}
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to open %s: %v\n", outputFile, err)
		return 1
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to close %s: %v\n", outputFile, closeErr)
		}
	}()

	prog := newProgressLine(r.stdout)
	prog.Update(fmt.Sprintf("  %s...", label))

	cmd := exec.Command(command[0], command[1:]...) // #nosec G204 -- commands are constructed from fixed compose/script inputs.
	cmd.Dir = r.repoRoot
	cmd.Stdout = file
	cmd.Stderr = file
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		prog.Clear()
		_, _ = fmt.Fprintf(r.stderr, "e2e: failed to start %s: %v\n", shellQuoteArgs(command), err)
		return 1
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	lastRunning := ""
	passed := 0
	failed := 0
	for {
		select {
		case err := <-done:
			ticker.Stop()
			passed, failed = r.countResults(outputFile)
			if err == nil {
				if passed+failed > 0 {
					prog.Complete(fmt.Sprintf("  %s: %d passed", label, passed))
				} else {
					prog.Complete(fmt.Sprintf("  %s: done", label))
				}
				return 0
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				if passed+failed > 0 {
					prog.Complete(fmt.Sprintf("  %s: %d passed, %d failed", label, passed, failed))
				} else {
					prog.Complete(fmt.Sprintf("  %s: failed (exit %d)", label, exitErr.ExitCode()))
				}
				return exitErr.ExitCode()
			}
			prog.Clear()
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed while running %s: %v\n", shellQuoteArgs(command), err)
			return 1
		case <-ticker.C:
			p, f := r.countResults(outputFile)
			passed, failed = p, f
			name := r.readLastRunningName(outputFile)
			if name != "" {
				name = strings.TrimSuffix(name, ".sh")
			}
			if name != "" && name != lastRunning {
				lastRunning = name
			}
			total := passed + failed
			if total > 0 && lastRunning != "" {
				prog.Update(fmt.Sprintf("  %s [%d done] %s", label, total, lastRunning))
			} else if lastRunning != "" {
				prog.Update(fmt.Sprintf("  %s: %s", label, lastRunning))
			}
		}
	}
}

func (r *Runner) countResults(outputFile string) (passed, failed int) {
	file, err := os.Open(filepath.Join(r.repoRoot, outputFile))
	if err != nil {
		return 0, 0
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "E2E_RESULT\tpassed\t") {
			passed++
		} else if strings.HasPrefix(line, "E2E_RESULT\tfailed\t") {
			failed++
		}
	}
	return passed, failed
}

func (r *Runner) readLastRunningName(outputFile string) string {
	file, err := os.Open(filepath.Join(r.repoRoot, outputFile))
	if err != nil {
		return ""
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	last := ""
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "Running:"); idx >= 0 {
			name := strings.TrimSpace(line[idx+len("Running:"):])
			name = strings.TrimSuffix(name, "\x1b[0m")
			if name != "" {
				last = name
			}
		}
	}
	return last
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
