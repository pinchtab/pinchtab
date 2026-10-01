package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	Name         string
	Group        *suiteGroup
	TitleSuffix  string // overrides the derived "tests (Docker)" suffix when set
	Compose      string
	Extended     bool
	Smoke        bool
	Ready        []string
	LogServices  []string
	RestartAfter []string
}

func resultsPath(prefix, name, ext string) string {
	return "tests/e2e/results/" + prefix + "-" + name + "." + ext
}

func (d suiteDescriptor) build() suiteDef {
	def := suiteDef{
		Name:         d.Name,
		Title:        d.title(),
		Compose:      d.Compose,
		Ready:        d.Ready,
		Extended:     d.Extended,
		Smoke:        d.Smoke,
		Summary:      resultsPath("summary", d.Name, "txt"),
		Report:       resultsPath("report", d.Name, "md"),
		Timings:      resultsPath("timings", d.Name, "json"),
		Output:       resultsPath("output", d.Name, "log"),
		LogPrefix:    "logs-" + d.Name,
		LogServices:  d.LogServices,
		RestartAfter: d.RestartAfter,
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
		LogServices: []string{"runner-cli", "pinchtab"}, RestartAfter: []string{"pinchtab"}},
	{Name: "infra", Group: &groupInfra, Compose: singleCompose, Ready: primaryReady(),
		LogServices: []string{"runner-api", "pinchtab"}},
	{Name: "infra-extended", Group: &groupInfra, Compose: multiCompose, Extended: true, Ready: extendedReady(),
		LogServices:  []string{"runner-api", "pinchtab", "pinchtab-secure", "pinchtab-medium", "pinchtab-full", "pinchtab-ghostchrome", "pinchtab-bridge"},
		RestartAfter: []string{"pinchtab"}},
	{Name: "plugin", Group: &groupPlugin, Compose: singleCompose, Ready: primaryReady(),
		LogServices: []string{"runner-api", "pinchtab"}},
	{Name: "api-smoke", Group: &groupAPI, Compose: multiCompose, Smoke: true, Ready: extendedReady(),
		LogServices: []string{"runner-api", "pinchtab", "pinchtab-secure", "pinchtab-autoclose", "pinchtab-medium", "pinchtab-full", "pinchtab-ghostchrome", "pinchtab-bridge"}},
	{Name: "cli-smoke", Group: &groupCLI, Compose: multiCompose, Smoke: true, Ready: primaryReady(),
		LogServices: []string{"runner-cli", "pinchtab"}, RestartAfter: []string{"pinchtab"}},
	{Name: "infra-smoke", Group: &groupInfra, Compose: multiCompose, Smoke: true, Ready: extendedReady(),
		LogServices: []string{"runner-api", "pinchtab", "pinchtab-secure", "pinchtab-medium", "pinchtab-full", "pinchtab-ghostchrome", "pinchtab-bridge"}},
	{Name: "plugin-smoke", Group: &groupPlugin, Compose: multiCompose, Smoke: true, Ready: primaryReady(),
		LogServices: []string{"runner-api", "pinchtab"}},
	{Name: "docker-smoke", Group: nil, TitleSuffix: "tests (host)", Smoke: true},
}

func suiteDefByName(name string) (suiteDef, bool) {
	for _, d := range suiteDescriptors {
		if d.Name == name {
			return d.build(), true
		}
	}
	return suiteDef{}, false
}

func suiteByName(name string) suiteDef {
	def, ok := suiteDefByName(name)
	if !ok {
		panic("e2e: unknown suite descriptor " + name)
	}
	return def
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
		for extra := range strings.FieldsSeq(r.args.Extra) {
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

const pinchtabImageBuilder = "pinchtab"

func servicesToBuild(plans []suitePlan, fallback []string) []string {
	services := append([]string(nil), servicesForPlans(plans, fallback)...)
	seen := map[string]bool{}
	needsImageBuilder := false
	for _, svc := range services {
		seen[svc] = true
		if strings.HasPrefix(svc, pinchtabImageBuilder+"-") {
			needsImageBuilder = true
		}
	}
	if needsImageBuilder && !seen[pinchtabImageBuilder] {
		seen[pinchtabImageBuilder] = true
		services = append([]string{pinchtabImageBuilder}, services...)
	}
	for _, plan := range plans {
		if runner := plan.def.Runner; runner != "" && !seen[runner] {
			seen[runner] = true
			services = append(services, runner)
		}
	}
	return services
}

func (r *Runner) showSuiteSkip(suite string) {
	_, _ = fmt.Fprintf(r.stdout, "Skipping %s: filter %q has no matching scenarios\n", suite, r.args.Filter)
}

func (r *Runner) prepareSuiteResults(def suiteDef) {
	if r.args.DryRun {
		_, _ = fmt.Fprintf(r.stdout, "# prepare results for %s\n", def.Name)
		return
	}
	for _, path := range []string{def.Summary, def.Report, def.Timings, def.Output} {
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
	paths := []string{def.Summary, def.Report, def.Timings, def.Output}
	for _, path := range paths {
		if fileExists(filepath.Join(r.repoRoot, path)) {
			_, _ = fmt.Fprintf(r.stdout, "  artifact: %s\n", path)
		}
	}
	for _, service := range def.LogServices {
		path := filepath.Join("tests/e2e/results", def.LogPrefix+"-"+service+".log")
		if fileExists(filepath.Join(r.repoRoot, path)) {
			_, _ = fmt.Fprintf(r.stdout, "  logs:     %s%s\n", path, emptyLogSuffix(filepath.Join(r.repoRoot, path)))
		}
	}
	if fileExists(filepath.Join(r.repoRoot, stackOutput)) {
		_, _ = fmt.Fprintf(r.stdout, "  logs:     %s%s\n", stackOutput, emptyLogSuffix(filepath.Join(r.repoRoot, stackOutput)))
	}
	if duration > 0 {
		_, _ = fmt.Fprintf(r.stdout, "  duration: %s\n", formatDuration(duration))
	}
}

// emptyLogSuffix marks a log artifact that has nothing in it. An empty file
// listed as `logs:` is worse than no listing at all — it sends whoever is
// diagnosing a failure to a file that cannot explain it — so say so on the line
// that advertises it. The named causes have to be ones that can still happen: a
// server discarding its logs was the original suspect and is no longer possible,
// so pointing at it would send a maintainer after a removed cause.
func emptyLogSuffix(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 0 {
		return ""
	}
	return "  (EMPTY — captured nothing; the service may never have started, or its --log-level excludes everything it logged)"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
