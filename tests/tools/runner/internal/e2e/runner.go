package e2e

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	mem       *suiteMemory
}

type overallReportData struct {
	Suites   int
	Passed   int
	Failed   int
	TotalMs  int64
	Failures []string
}

type suiteDef struct {
	Name         string
	Title        string
	Compose      string
	GroupDir     string
	Helper       string
	ScenarioDir  string
	Commands     []string
	Ready        []string
	Runner       string
	RunSuite     string
	Extended     bool
	Smoke        bool
	Summary      string
	Report       string
	Timings      string
	LogPrefix    string
	Output       string
	LogServices  []string
	RestartAfter []string
}

type suitePlan struct {
	def       suiteDef
	scenarios []scenarioMeta
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
		return r.runStackLane(basicLane())
	case "extended":
		return r.runStackLane(extendedLane())
	case "smoke":
		return r.runSmokeLane(smokeLane())
	case "smoke-orchestrator":
		return r.runSmokeFiltered("orchestrator")
	case "smoke-security":
		return r.runSmokeFiltered("security")
	case "smoke-lifecycle":
		return r.runSmokeFiltered("lifecycle")
	}
	if def, ok := suiteDefByName(r.suite); ok {
		return r.runSingle(def)
	}
	_, _ = fmt.Fprintf(r.stderr, "e2e: unknown suite %q\n", r.suite)
	return 1
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

type lane struct {
	name  string
	stack string
	defs  []suiteDef
}

func basicLane() lane {
	return lane{
		name:  "basic",
		stack: singleCompose,
		defs:  []suiteDef{apiSuite(), cliSuite(), infraSuite()},
	}
}

func extendedLane() lane {
	return lane{
		name:  "extended",
		stack: multiCompose,
		defs:  []suiteDef{apiExtendedSuite(), cliExtendedSuite(), infraExtendedSuite(), pluginSuite()},
	}
}

func smokeLane() lane {
	return lane{
		name:  "smoke",
		stack: multiCompose,
		defs:  []suiteDef{apiSmokeSuite(), cliSmokeSuite(), infraSmokeSuite(), pluginSmokeSuite()},
	}
}

func (r *Runner) bringUpAndRunPlans(stack string, plans []suitePlan) (codes map[string]int, restartFailed bool, setupCode int) {
	services := servicesForPlans(plans, []string{"pinchtab", "fixtures"})
	buildServices := servicesToBuild(plans, []string{"pinchtab", "fixtures"})
	if code := r.bringUpSharedStack(stack, services, buildServices); code != 0 {
		return nil, false, code
	}

	codes = map[string]int{}
	for i, plan := range plans {
		if code := r.runSinglePlanWithCompose(plan, stack); code != 0 {
			codes[plan.def.Name] = code
		}
		if svcs := plan.def.RestartAfter; len(svcs) > 0 && i < len(plans)-1 {
			if rc := r.restartSharedStack(stack, svcs); rc != 0 {
				restartFailed = true
			}
		}
		_, _ = fmt.Fprintln(r.stdout, "")
	}
	return codes, restartFailed, 0
}

func (r *Runner) reportNoSuitesMatched(l lane) int {
	_, _ = fmt.Fprintf(r.stderr, "e2e: no %s suites matched filter %q\n", l.name, r.args.Filter)
	return 1
}

func (r *Runner) reportLaneOutcome(l lane, codes map[string]int, otherFailure bool) int {
	if len(codes) == 0 && !otherFailure {
		if !r.args.DryRun {
			_, _ = fmt.Fprintf(r.stdout, "E2E %s suites passed\n", l.name)
		}
		return 0
	}
	_, _ = fmt.Fprintf(r.stderr, "e2e: %s suites failed\n", l.name)
	if len(codes) > 0 {
		_, _ = fmt.Fprintf(r.stderr, "e2e: exit codes: %s\n", formatSuiteExitCodes(codes))
	}
	return 1
}

func formatSuiteExitCodes(codes map[string]int) string {
	names := make([]string, 0, len(codes))
	for name := range codes {
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, fmt.Sprintf("%s=%d", name, codes[name]))
	}
	return strings.Join(pairs, ", ")
}

func (r *Runner) runStackLane(l lane) int {
	plans, code := r.planSuites(l.defs)
	if code != 0 {
		return code
	}
	if len(plans) == 0 {
		return r.reportNoSuitesMatched(l)
	}

	codes, restartFailed, setupCode := r.bringUpAndRunPlans(l.stack, plans)
	if setupCode != 0 {
		_ = r.composeDown(l.stack)
		return setupCode
	}
	defer r.composeDown(l.stack)

	return r.reportLaneOutcome(l, codes, restartFailed)
}

func (r *Runner) runSingle(def suiteDef) int {
	scenarios, err := r.selectedScenarioMeta(def)
	if err != nil {
		_, _ = fmt.Fprintf(r.stderr, "e2e: %v\n", err)
		return 1
	}
	plan := suitePlan{def: def, scenarios: scenarios}
	plans := []suitePlan{plan}
	fallback := []string{"pinchtab", "fixtures"}
	if code := r.bringUpSharedStack(def.Compose, servicesForPlans(plans, fallback), servicesToBuild(plans, fallback)); code != 0 {
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

	command := r.suiteRunCommand(composeFile, def, plan.scenarios)

	var probe *memoryProbe
	if !r.args.DryRun {
		probe = r.startMemorySampler(composeFile)
	}
	code := r.runLoggedCommand("running "+def.Name+" suite", def.Output, command)
	if probe != nil {
		r.applyMemoryResult(probe.finish())
	}
	duration := time.Since(started)
	summary := r.writeSuiteReports(def, duration, code)
	r.mem = nil
	r.recordOverallSummary(summary)
	r.printSuiteSummary(def, summary, duration)
	if code != 0 {
		r.dumpComposeFailure(composeFile, def)
		r.reportServiceDeaths(composeFile, def.LogServices)
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

func (r *Runner) provider() string {
	if r.args.Provider != "" {
		return r.args.Provider
	}
	return defaultProvider
}
