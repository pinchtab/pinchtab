package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var errNoMatchingScenarios = errors.New("no matching scenarios")

type noMatchingScenariosError struct {
	message string
}

func (e noMatchingScenariosError) Error() string {
	return e.message
}

func (e noMatchingScenariosError) Is(target error) bool {
	return target == errNoMatchingScenarios
}

// suiteGroup captures the scenario-group metadata shared by every suite that
// runs that group (api/cli/infra/plugin). Suites in the same group differ only
// by mode (basic/extended/smoke), compose stack, ready targets and log services.
type suiteGroup struct {
	label       string // human label used in titles, e.g. "API"
	dir         string // scenario group dir name, e.g. "api"
	helper      string
	commands    []string
	runner      string
	runSuiteKey string // RunSuite key; defaults to dir when empty
}

func (g suiteGroup) runSuite() string {
	if g.runSuiteKey != "" {
		return g.runSuiteKey
	}
	return g.dir
}

var (
	groupAPI    = suiteGroup{label: "API", dir: "api", helper: "api", commands: apiCommands(), runner: "runner-api"}
	groupCLI    = suiteGroup{label: "CLI", dir: "cli", helper: "cli", commands: cliCommands(), runner: "runner-cli"}
	groupInfra  = suiteGroup{label: "Infra", dir: "infra", helper: "api", commands: apiCommands(), runner: "runner-api"}
	groupPlugin = suiteGroup{label: "Plugin", dir: "plugin", helper: "api", commands: apiCommands(), runner: "runner-api"}
)

// suiteDescriptor is the single source of truth for one suite. Adding or
// renaming a suite is one table entry; all artifact/log paths and the title are
// derived from Name so they cannot drift by typo. Group is nil for host-only
// suites such as docker-smoke.
type suiteDescriptor struct {
	Name        string
	Group       *suiteGroup
	TitleSuffix string // overrides the derived "tests (Docker)" suffix when set
	Compose     string
	Extended    bool
	Smoke       bool
	Ready       []string
	LogServices []string
}

func resultsPath(prefix, name, ext string) string {
	return "tests/e2e/results/" + prefix + "-" + name + "." + ext
}

func (d suiteDescriptor) build() suiteDef {
	def := suiteDef{
		Name:        d.Name,
		Title:       d.title(),
		Compose:     d.Compose,
		Ready:       d.Ready,
		Extended:    d.Extended,
		Smoke:       d.Smoke,
		Summary:     resultsPath("summary", d.Name, "txt"),
		Report:      resultsPath("report", d.Name, "md"),
		Output:      resultsPath("output", d.Name, "log"),
		LogPrefix:   "logs-" + d.Name,
		LogServices: d.LogServices,
	}
	if d.Group != nil {
		g := d.Group
		def.GroupDir = "tests/e2e/scenarios/" + g.dir
		def.ScenarioDir = "scenarios/" + g.dir
		def.Helper = g.helper
		def.Commands = g.commands
		def.Runner = g.runner
		def.RunSuite = g.runSuite()
	}
	return def
}

func (d suiteDescriptor) title() string {
	suffix := d.TitleSuffix
	if suffix == "" {
		suffix = "tests (Docker)"
	}
	label := "Docker"
	if d.Group != nil {
		label = d.Group.label
	}
	mode := ""
	switch {
	case d.Smoke:
		mode = " Smoke"
	case d.Extended:
		mode = " Extended"
	}
	return "E2E " + label + mode + " " + suffix
}

var suiteDescriptors = []suiteDescriptor{
	{Name: "api", Group: &groupAPI, Compose: singleCompose, Ready: primaryReady(),
		LogServices: []string{"runner-api", "pinchtab"}},
	{Name: "api-extended", Group: &groupAPI, Compose: multiCompose, Extended: true, Ready: extendedReady(),
		LogServices: []string{"runner-api", "pinchtab", "pinchtab-secure", "pinchtab-medium", "pinchtab-full", "pinchtab-ghostchrome", "pinchtab-bridge"}},
	{Name: "cli", Group: &groupCLI, Compose: singleCompose, Ready: primaryReady(),
		LogServices: []string{"runner-cli", "pinchtab"}},
	{Name: "cli-extended", Group: &groupCLI, Compose: singleCompose, Extended: true, Ready: primaryReady(),
		LogServices: []string{"runner-cli", "pinchtab"}},
	{Name: "infra", Group: &groupInfra, Compose: singleCompose, Ready: primaryReady(),
		LogServices: []string{"runner-api", "pinchtab"}},
	{Name: "infra-extended", Group: &groupInfra, Compose: multiCompose, Extended: true, Ready: extendedReady(),
		LogServices: []string{"runner-api", "pinchtab", "pinchtab-secure", "pinchtab-medium", "pinchtab-full", "pinchtab-ghostchrome", "pinchtab-bridge"}},
	{Name: "plugin", Group: &groupPlugin, Compose: singleCompose, Ready: primaryReady(),
		LogServices: []string{"runner-api", "pinchtab"}},
	{Name: "api-smoke", Group: &groupAPI, Compose: multiCompose, Smoke: true, Ready: extendedReady(),
		LogServices: []string{"runner-api", "pinchtab", "pinchtab-secure", "pinchtab-autoclose", "pinchtab-medium", "pinchtab-full", "pinchtab-ghostchrome", "pinchtab-bridge"}},
	{Name: "cli-smoke", Group: &groupCLI, Compose: multiCompose, Smoke: true, Ready: primaryReady(),
		LogServices: []string{"runner-cli", "pinchtab"}},
	{Name: "infra-smoke", Group: &groupInfra, Compose: multiCompose, Smoke: true, Ready: extendedReady(),
		LogServices: []string{"runner-api", "pinchtab", "pinchtab-secure", "pinchtab-medium", "pinchtab-full", "pinchtab-ghostchrome", "pinchtab-bridge"}},
	{Name: "plugin-smoke", Group: &groupPlugin, Compose: multiCompose, Smoke: true, Ready: primaryReady(),
		LogServices: []string{"runner-api", "pinchtab"}},
	{Name: "docker-smoke", Group: nil, TitleSuffix: "tests (host)", Smoke: true},
}

func suiteByName(name string) suiteDef {
	for _, d := range suiteDescriptors {
		if d.Name == name {
			return d.build()
		}
	}
	panic("e2e: unknown suite descriptor " + name)
}

func apiSuite() suiteDef           { return suiteByName("api") }
func apiExtendedSuite() suiteDef   { return suiteByName("api-extended") }
func cliSuite() suiteDef           { return suiteByName("cli") }
func cliExtendedSuite() suiteDef   { return suiteByName("cli-extended") }
func infraSuite() suiteDef         { return suiteByName("infra") }
func infraExtendedSuite() suiteDef { return suiteByName("infra-extended") }
func pluginSuite() suiteDef        { return suiteByName("plugin") }
func apiSmokeSuite() suiteDef      { return suiteByName("api-smoke") }
func cliSmokeSuite() suiteDef      { return suiteByName("cli-smoke") }
func infraSmokeSuite() suiteDef    { return suiteByName("infra-smoke") }
func pluginSmokeSuite() suiteDef   { return suiteByName("plugin-smoke") }
func dockerSmokeSuite() suiteDef   { return suiteByName("docker-smoke") }

func apiCommands() []string {
	return []string{"curl", "jq", "grep", "sed", "awk", "seq", "mktemp"}
}

func cliCommands() []string {
	return []string{"pinchtab", "curl", "jq", "grep", "sed", "awk", "seq", "mktemp"}
}

func primaryReady() []string {
	return []string{"E2E_SERVER"}
}

func extendedReady() []string {
	return []string{
		"E2E_SERVER",
		"E2E_SECURE_SERVER",
		"E2E_AUTOCLOSE_SERVER",
		"E2E_MEDIUM_SERVER",
		"E2E_FULL_SERVER",
		"E2E_SERVER_GHOSTCHROME",
		"E2E_BRIDGE_URL|60|E2E_BRIDGE_TOKEN",
	}
}

func (r *Runner) selectedScenarioMeta(def suiteDef) ([]scenarioMeta, error) {
	catalog, err := r.loadScenarioCatalog()
	if err != nil {
		return nil, err
	}
	group := filepath.Base(def.ScenarioDir)
	seen := map[string]bool{}
	var scenarios []scenarioMeta

	add := func(meta scenarioMeta) error {
		if meta.Key == "" || seen[meta.Key] {
			return nil
		}
		if meta.Helper != def.Helper {
			return fmt.Errorf("scenario %s declares helper %q but suite %s uses helper %q", meta.Key, meta.Helper, def.Name, def.Helper)
		}
		seen[meta.Key] = true
		scenarios = append(scenarios, meta)
		return nil
	}

	if def.Smoke {
		for _, meta := range catalog.group(group) {
			if meta.Tier != tierSmoke {
				continue
			}
			if err := add(meta); err != nil {
				return nil, err
			}
		}
	} else {
		for _, meta := range catalog.group(group) {
			if meta.Tier != tierBasic {
				continue
			}
			if err := add(meta); err != nil {
				return nil, err
			}
		}

		if def.Extended {
			for _, meta := range catalog.group(group) {
				if meta.Tier != tierExtended {
					continue
				}
				if err := add(meta); err != nil {
					return nil, err
				}
			}
		}
	}

	if r.args.Extra != "" {
		for _, extra := range strings.Fields(r.args.Extra) {
			name := filepath.Base(extra)
			if meta, ok := catalog.find(group, name); ok {
				if err := add(meta); err != nil {
					return nil, err
				}
			}
		}
	}

	if r.args.Filter != "" {
		filtered := scenarios[:0]
		for _, meta := range scenarios {
			if scenarioMatchesFilter(meta, r.args.Filter) {
				filtered = append(filtered, meta)
			}
		}
		scenarios = filtered
	}

	if len(scenarios) == 0 {
		if r.args.Filter != "" {
			return nil, noMatchingScenariosError{message: fmt.Sprintf("no scenario files matched filter %q in %s", r.args.Filter, def.GroupDir)}
		}
		return nil, noMatchingScenariosError{message: fmt.Sprintf("no scenario files found in %s", def.GroupDir)}
	}
	return scenarios, nil
}

func servicesForPlans(plans []suitePlan, fallback []string) []string {
	var services []string
	for _, plan := range plans {
		services = append(services, servicesForScenarios(plan.scenarios)...)
	}
	out := orderedUnion(composeServiceOrder, services)
	if len(out) == 0 {
		return fallback
	}
	return out
}

func (r *Runner) showSuiteSkip(suite string) {
	_, _ = fmt.Fprintf(r.stdout, "Skipping %s: filter %q has no matching scenarios\n", suite, r.args.Filter)
}

func (r *Runner) prepareSuiteResults(def suiteDef) {
	if r.args.DryRun {
		_, _ = fmt.Fprintf(r.stdout, "# prepare results for %s\n", def.Name)
		return
	}
	for _, path := range []string{def.Summary, def.Report, def.Output} {
		_ = os.Remove(filepath.Join(r.repoRoot, path))
	}
	for _, path := range []string{
		"tests/e2e/results/summary.txt",
		"tests/e2e/results/report.md",
	} {
		_ = os.Remove(filepath.Join(r.repoRoot, path))
	}
	logs, _ := filepath.Glob(filepath.Join(r.repoRoot, "tests/e2e/results", def.LogPrefix+"-*.log"))
	for _, path := range logs {
		_ = os.Remove(path)
	}
}

func (r *Runner) dumpComposeFailure(composeFile string, def suiteDef) {
	if r.args.DryRun {
		return
	}
	for _, service := range def.LogServices {
		out := filepath.Join(r.repoRoot, "tests/e2e/results", def.LogPrefix+"-"+service+".log")
		if err := writeCommandOutput(out, r.repoRoot, r.composeArgs(composeFile, "logs", service)); err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to capture logs for %s: %v\n", service, err)
		}
	}
}

func writeCommandOutput(path, dir string, command []string) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()

	cmd := execCommand(command, dir)
	cmd.Stdout = file
	cmd.Stderr = file
	err = cmd.Run()
	return
}

func execCommand(command []string, dir string) *exec.Cmd {
	cmd := exec.Command(command[0], command[1:]...) // #nosec G204 -- commands are fixed compose invocations.
	cmd.Dir = dir
	cmd.Env = os.Environ()
	return cmd
}

func (r *Runner) showFailureArtifacts(def suiteDef, duration time.Duration) {
	paths := []string{def.Summary, def.Report, def.Output}
	for _, path := range paths {
		if fileExists(filepath.Join(r.repoRoot, path)) {
			_, _ = fmt.Fprintf(r.stdout, "  artifact: %s\n", path)
		}
	}
	for _, service := range def.LogServices {
		path := filepath.Join("tests/e2e/results", def.LogPrefix+"-"+service+".log")
		if fileExists(filepath.Join(r.repoRoot, path)) {
			_, _ = fmt.Fprintf(r.stdout, "  logs:     %s\n", path)
		}
	}
	if fileExists(filepath.Join(r.repoRoot, stackOutput)) {
		_, _ = fmt.Fprintf(r.stdout, "  logs:     %s\n", stackOutput)
	}
	if duration > 0 {
		_, _ = fmt.Fprintf(r.stdout, "  duration: %s\n", formatDuration(duration))
	}
}

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
	for _, line := range strings.Split(string(data), "\n") {
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
		for _, line := range strings.Split(data.ErrorTail, "\n") {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
