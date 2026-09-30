package devtools

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	escalationScript   = "scripts/ci/detect-e2e-suites.sh"
	escalationMap      = "scripts/ci/e2e-escalation.map"
	escalationWorkflow = ".github/workflows/ci-e2e.yml"
	smokeWorkflow      = ".github/workflows/ci-smoke.yml"
)

func TestEscalationFollowsProductPathsAndTestPaths(t *testing.T) {
	root := repoRoot(t)

	cases := []struct {
		name    string
		changed []string
		want    []string
	}{
		{
			name:    "audit implementation with no scenario file",
			changed: []string{"internal/audit/run.go", "pkg/pinchtabaudit/client.go", "internal/handlers/audit.go", "cmd/pinchtab/cmd_audit.go"},
			want:    []string{"run_api_extended", "run_cli_extended"},
		},
		{
			name:    "audit CLI implementation rather than its command wrapper",
			changed: []string{"internal/cli/actions/actions_audit.go"},
			want:    []string{"run_api_extended", "run_cli_extended"},
		},
		{
			name:    "audit implementation alongside a basic scenario",
			changed: []string{"internal/audit/enrich.go", "tests/e2e/scenarios/api/audit-page-basic.sh"},
			want:    []string{"run_api_extended", "run_cli_extended"},
		},
		{
			name:    "compare implementation, which no API suite covers",
			changed: []string{"internal/cli/actions/actions_compare.go", "cmd/pinchtab/cmd_compare.go"},
			want:    []string{"run_cli_extended"},
		},
		{
			name:    "api extended scenario churn",
			changed: []string{"tests/e2e/scenarios/api/tabs-extended.sh"},
			want:    []string{"run_api_extended"},
		},
		{
			name:    "cli extended scenario churn",
			changed: []string{"tests/e2e/scenarios/cli/audit-pdf-extended.sh"},
			want:    []string{"run_cli_extended"},
		},
		{
			name:    "infra extended scenario churn",
			changed: []string{"tests/e2e/scenarios/infra/auth-extended.sh"},
			want:    []string{"run_infra_extended"},
		},
		{
			name:    "standalone scenario churn",
			changed: []string{"tests/e2e/scenarios/api/network-retain-body.sh"},
			want:    []string{"run_api_extended"},
		},
		{
			name:    "smoke inputs",
			changed: []string{"tests/e2e/scenarios/cli/tabs-smoke.sh", "Dockerfile"},
			want:    []string{"run_smoke"},
		},
		{
			name:    "plugin implementation and packaging",
			changed: []string{"plugins/openclaw/tools/browser.ts", "plugins/grok/.mcp.json", ".grok-plugin/marketplace.json"},
			want:    []string{"run_smoke"},
		},
		{
			name:    "basic scenario churn only",
			changed: []string{"tests/e2e/scenarios/api/tabs-basic.sh", "tests/e2e/scenarios/infra/network-basic.sh"},
			want:    nil,
		},
		{
			name:    "product paths no extended suite covers",
			changed: []string{"internal/dashboard/dashboard.go", "README.md", "internal/scroll/scroll.go"},
			want:    nil,
		},
		{
			name:    "empty diff",
			changed: nil,
			want:    nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectSuites(t, root, tc.changed)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("%s escalated %v, want %v", strings.Join(tc.changed, ", "), got, tc.want)
			}
		})
	}
}

func TestPluginPathsReachTheE2EWorkflow(t *testing.T) {
	root := repoRoot(t)
	var workflow struct {
		On map[string]struct {
			PathsIgnore []string `yaml:"paths-ignore"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, root, escalationWorkflow)), &workflow); err != nil {
		t.Fatalf("cannot parse %s: %v", escalationWorkflow, err)
	}

	for _, event := range []string{"pull_request", "push"} {
		trigger, ok := workflow.On[event]
		if !ok {
			t.Errorf("%s has no %s trigger", escalationWorkflow, event)
			continue
		}
		for _, ignored := range trigger.PathsIgnore {
			if ignored == "plugins/**" || ignored == ".grok-plugin/**" {
				t.Errorf("%s ignores %s on %s, so plugin changes cannot reach smoke coverage", escalationWorkflow, ignored, event)
			}
		}
	}
}

func TestEveryEscalationRuleMatchesATrackedPath(t *testing.T) {
	root := repoRoot(t)
	tracked := trackedPaths(t, root)

	for _, rule := range escalationRules(t, root) {
		pattern := rule.compile(t)
		matched := false
		for _, path := range tracked {
			if pattern.MatchString(path) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("rule %q matches no tracked file, so the coverage it claims is not wired to anything; re-point it at the path that moved rather than leaving it", rule.pattern)
		}
	}
}

func TestTheWorkflowConsumesExactlyTheEmittedOutputs(t *testing.T) {
	root := repoRoot(t)
	body := readRepoFile(t, root, escalationWorkflow)

	if !strings.Contains(body, escalationScript) {
		t.Fatalf("%s does not invoke %s, so the mapping decides nothing", escalationWorkflow, escalationScript)
	}

	emitted := map[string]bool{}
	for _, name := range detectSuiteNames(t, root) {
		emitted[name] = true
		if strings.HasSuffix(name, "_extended") {
			continue
		}
		if !strings.Contains(body, "steps.changes.outputs."+name) {
			t.Errorf("%s emits %s but %s never exports it, so the suite it gates can never run", escalationScript, name, escalationWorkflow)
		}
	}

	consumed := regexp.MustCompile(`detect-changes\.outputs\.([a-z_]+)`)
	for _, match := range consumed.FindAllStringSubmatch(body, -1) {
		if !emitted[match[1]] {
			t.Errorf("%s gates a job on %s, which %s never emits, so the job never runs", escalationWorkflow, match[1], escalationScript)
		}
	}
}

func TestEveryEscalatableSuiteAlsoRunsWithoutADiffToClaimIt(t *testing.T) {
	root := repoRoot(t)
	automatic := suitesOnAutomaticTriggers(t, root)

	for _, name := range detectSuiteNames(t, root) {
		if !strings.HasPrefix(name, "run_") {
			continue
		}
		suite := strings.ReplaceAll(strings.TrimPrefix(name, "run_"), "_", "-")
		if !automatic[suite] {
			t.Errorf("the %s suite runs only when a diff escalates it or a human dispatches it, so a merge the map does not claim never exercises the coverage it holds; give it a job gated on github.event_name == '<event>' under an automatic trigger in %s or %s", suite, escalationWorkflow, smokeWorkflow)
		}
	}
}

type ciWorkflow struct {
	On   map[string]any   `yaml:"on"`
	Jobs map[string]ciJob `yaml:"jobs"`
}

type ciJob struct {
	If   string `yaml:"if"`
	Uses string `yaml:"uses"`
	With struct {
		Suite string `yaml:"suite"`
	} `yaml:"with"`
}

func suitesOnAutomaticTriggers(t *testing.T, root string) map[string]bool {
	t.Helper()
	automatic := map[string]bool{}
	for _, event := range automaticEvents(parseWorkflow(t, root, escalationWorkflow)) {
		for _, suite := range plannedSuites(t, root, nil, "E2E_EVENT="+event) {
			automatic[suite] = true
		}
	}
	for _, rel := range []string{escalationWorkflow, smokeWorkflow} {
		workflow := parseWorkflow(t, root, rel)
		for _, event := range automaticEvents(workflow) {
			for _, job := range workflow.Jobs {
				if suite := jobSuite(job); suite != "" && jobRunsOnEvent(job, event) {
					automatic[suite] = true
				}
			}
		}
	}
	return automatic
}

func automaticEvents(workflow ciWorkflow) []string {
	var events []string
	for event := range workflow.On {
		if event != "pull_request" && event != "workflow_dispatch" {
			events = append(events, event)
		}
	}
	sort.Strings(events)
	return events
}

func jobSuite(job ciJob) string {
	if job.With.Suite != "" {
		return job.With.Suite
	}
	if strings.Contains(job.Uses, "reusable-smoke") {
		return "smoke"
	}
	return ""
}

func jobRunsOnEvent(job ciJob, event string) bool {
	return job.If == "" || strings.Contains(job.If, "github.event_name == '"+event+"'")
}

func parseWorkflow(t *testing.T, root, rel string) ciWorkflow {
	t.Helper()
	var workflow ciWorkflow
	if err := yaml.Unmarshal([]byte(readRepoFile(t, root, rel)), &workflow); err != nil {
		t.Fatalf("cannot parse %s: %v", rel, err)
	}
	if len(workflow.On) == 0 || len(workflow.Jobs) == 0 {
		t.Fatalf("%s declares %d triggers and %d jobs, so every trigger guard over it would pass vacuously", rel, len(workflow.On), len(workflow.Jobs))
	}
	return workflow
}

func detectSuites(t *testing.T, root string, changed []string) []string {
	t.Helper()
	var escalated []string
	for _, line := range runDetect(t, root, changed) {
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("%s emitted %q, which is not a name=value pair", escalationScript, line)
		}
		if value == "true" {
			escalated = append(escalated, name)
		}
	}
	sort.Strings(escalated)
	return escalated
}

func detectSuiteNames(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	for _, line := range runDetect(t, root, nil) {
		name, _, _ := strings.Cut(line, "=")
		names = append(names, name)
	}
	if len(names) == 0 {
		t.Fatalf("%s emitted no output, so every guard over it is vacuous", escalationScript)
	}
	return names
}

// plannedSuites is what the basic and extended matrices of the E2E workflow run
// for the given diff and event environment.
func plannedSuites(t *testing.T, root string, changed []string, env ...string) []string {
	t.Helper()
	var suites []string
	for _, line := range runDetect(t, root, changed, env...) {
		name, value, _ := strings.Cut(line, "=")
		if name != "basic_suites" && name != "extended_suites" {
			continue
		}
		var list []string
		if err := json.Unmarshal([]byte(value), &list); err != nil {
			t.Fatalf("%s emitted %s=%s, which is not a JSON list: %v", escalationScript, name, value, err)
		}
		suites = append(suites, list...)
	}
	sort.Strings(suites)
	return suites
}

func runDetect(t *testing.T, root string, changed []string, env ...string) []string {
	t.Helper()
	if readRepoFile(t, root, escalationScript) == "" || readRepoFile(t, root, escalationMap) == "" {
		t.Fatalf("%s and %s must both hold content, and reading them here is also what keeps a cached green from surviving an edit to either", escalationScript, escalationMap)
	}
	cmd := exec.Command("bash", filepath.Join(root, escalationScript))
	cmd.Stdin = strings.NewReader(strings.Join(changed, "\n"))
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s failed on %v: %v\n%s", escalationScript, changed, err, stderr.String())
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

type escalationRule struct {
	pattern string
	suites  []string
}

func escalationRules(t *testing.T, root string) []escalationRule {
	t.Helper()
	var rules []escalationRule
	for _, line := range strings.Split(readRepoFile(t, root, escalationMap), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		rules = append(rules, escalationRule{pattern: fields[0], suites: fields[1:]})
	}
	if len(rules) == 0 {
		t.Fatalf("%s holds no rule", escalationMap)
	}
	return rules
}

func (r escalationRule) claimsASuite() bool {
	return len(r.suites) > 0 && r.suites[0] != "-"
}

func (r escalationRule) compile(t *testing.T) *regexp.Regexp {
	t.Helper()
	pattern, err := regexp.Compile(r.pattern)
	if err != nil {
		t.Fatalf("rule %q in %s is not a valid expression: %v", r.pattern, escalationMap, err)
	}
	return pattern
}

func trackedPaths(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("cannot list tracked files, so staleness is unprovable: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func readRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("cannot read %s: %v", rel, err)
	}
	return string(body)
}

type enrolledArea struct {
	name     string
	match    string
	suites   []string
	excluded map[string]string
}

var enrolledAreas = []enrolledArea{
	{
		name:   "audit",
		match:  `(?i)audit`,
		suites: []string{"run_api_extended", "run_cli_extended"},
		excluded: map[string]string{
			"internal/authn/audit.go": "security audit logging — AuditLog writes slog entries, a different sense of the word, covered by no audit scenario",
		},
	},
	{
		name:   "compare",
		match:  `(?i)compare`,
		suites: []string{"run_cli_extended"},
		excluded: map[string]string{
			"internal/heapsnap/compare.go": "heap snapshot diffing — the memory compare route, a different sense of the word, covered by the memory scenarios rather than the compare ones",
		},
	},
}

func TestEveryEnrolledAreaIsCoveredByTheMap(t *testing.T) {
	root := repoRoot(t)
	tracked := trackedPaths(t, root)

	for _, area := range enrolledAreas {
		t.Run(area.name, func(t *testing.T) {
			pattern, err := regexp.Compile(area.match)
			if err != nil {
				t.Fatalf("area %q has an unusable match %q: %v", area.name, area.match, err)
			}

			var sources []string
			for _, path := range tracked {
				if isAreaSource(path) && pattern.MatchString(path) {
					sources = append(sources, path)
				}
			}
			if len(sources) < 2 {
				t.Fatalf("area %q matches %d source files; the match has stopped finding the area and this guard would pass vacuously", area.name, len(sources))
			}

			usedExclusion := map[string]bool{}
			for _, path := range sources {
				escalated := detectSuites(t, root, []string{path})
				if why, excluded := area.excluded[path]; excluded {
					usedExclusion[path] = true
					if len(escalated) > 0 {
						t.Errorf("%s is excluded from the %s area (%s) yet escalates %v; drop the exclusion rather than keeping a claim the map contradicts", path, area.name, why, escalated)
					}
					continue
				}
				for _, want := range area.suites {
					if !containsString(escalated, want) {
						t.Errorf("%s is %s implementation but escalates %v, not %s; the extended scenarios covering %s never fire on the diff most likely to break them",
							path, area.name, escalated, want, area.name)
					}
				}
			}

			for path, why := range area.excluded {
				if !usedExclusion[path] {
					t.Errorf("%s is excluded from the %s area (%s) but is no longer a source file the area matches; drop the exclusion or re-point it", path, area.name, why)
				}
			}
		})
	}
}

func isAreaSource(path string) bool {
	if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	return !strings.HasPrefix(path, "tests/")
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func TestEveryAutomaticTriggerCanActuallyFire(t *testing.T) {
	root := repoRoot(t)
	probes := escalatablePaths(t, root)

	for _, rel := range []string{escalationWorkflow, smokeWorkflow} {
		workflow := parseWorkflow(t, root, rel)
		reviewed, hasPullRequest := pullRequestFilter(t, rel, workflow)

		for _, event := range automaticEvents(workflow) {
			if crons, ok := scheduleCrons(workflow.On[event]); ok {
				if len(crons) == 0 {
					t.Errorf("%s declares a %s trigger holding no cron entry, so the lane it is meant to start never runs and every guard asserting the trigger exists passes on a name alone", rel, event)
				}
				continue
			}

			filter, ok := triggerFilterFor(workflow.On[event])
			if !ok || !hasPullRequest {
				continue
			}

			for _, branch := range reviewed.Branches {
				if !matchesAnyGlob(filter.Branches, branch) {
					t.Errorf("%s runs pull requests targeting %q but its %s trigger is limited to %v, so coverage retiered off the pull request lands on a branch nothing merges into", rel, branch, event, filter.Branches)
				}
			}

			for _, path := range probes {
				if triggerAcceptsPath(reviewed, path) && !triggerAcceptsPath(filter, path) {
					t.Errorf("%s claims %s escalates a suite and the pull_request trigger in %s fires on it, yet the %s trigger filters it out (paths=%v paths-ignore=%v), so the merge lane is blind to a change the review lane tests", escalationMap, path, rel, event, filter.Paths, filter.PathsIgnore)
				}
			}
		}
	}
}

func TestTheTriggerPathMatcherFollowsGitHubGlobSemantics(t *testing.T) {
	cases := []struct {
		glob string
		path string
		want bool
	}{
		{"**.md", "README.md", true},
		{"**.md", "docs/guides/contributing.md", true},
		{"**.md", "cmd/pinchtab/main.go", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/guides/contributing.md", false},
		{"docs/**", "docs/guides/contributing.md", true},
		{"docs/**", "docsite/index.html", false},
		{"docs/**", "internal/docs/render.go", false},
		{"LICENSE", "LICENSE", true},
		{"LICENSE", "LICENSE.md", false},
		{"skills/**", "skills/pinchtab/SKILL.md", true},
		{"skills/**", "internal/skills/load.go", false},
	}

	for _, tc := range cases {
		if got := matchesAnyGlob([]string{tc.glob}, tc.path); got != tc.want {
			t.Errorf("glob %q against %q returned %t, want %t", tc.glob, tc.path, got, tc.want)
		}
	}

	allowlist := triggerFilter{Paths: []string{"internal/**"}}
	if triggerAcceptsPath(allowlist, "tests/e2e/scenarios/api/audit-extended.sh") {
		t.Error("a paths allowlist that names only internal/** accepted a scenario file, so the allowlist arm of the matcher cannot reject anything")
	}
	if !triggerAcceptsPath(allowlist, "internal/audit/run.go") {
		t.Error("a paths allowlist that names internal/** rejected a file under internal/, so the allowlist arm rejects everything")
	}

	ignore := triggerFilter{PathsIgnore: []string{"**.md"}}
	if triggerAcceptsPath(ignore, "TESTING.md") {
		t.Error("a paths-ignore of **.md accepted a markdown file, so the ignore arm of the matcher cannot reject anything")
	}
	if !triggerAcceptsPath(ignore, "internal/audit/run.go") {
		t.Error("a paths-ignore of **.md rejected a Go file, so the ignore arm rejects everything")
	}
}

type triggerFilter struct {
	Branches    []string `yaml:"branches"`
	Paths       []string `yaml:"paths"`
	PathsIgnore []string `yaml:"paths-ignore"`
}

func triggerFilterFor(node any) (triggerFilter, bool) {
	encoded, err := yaml.Marshal(node)
	if err != nil {
		return triggerFilter{}, false
	}
	var filter triggerFilter
	if err := yaml.Unmarshal(encoded, &filter); err != nil {
		return triggerFilter{}, false
	}
	return filter, true
}

func scheduleCrons(node any) ([]string, bool) {
	entries, ok := node.([]any)
	if !ok {
		return nil, false
	}
	var crons []string
	for _, entry := range entries {
		mapping, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if cron, ok := mapping["cron"].(string); ok && strings.TrimSpace(cron) != "" {
			crons = append(crons, cron)
		}
	}
	return crons, true
}

func pullRequestFilter(t *testing.T, rel string, workflow ciWorkflow) (triggerFilter, bool) {
	t.Helper()
	node, declared := workflow.On["pull_request"]
	if !declared {
		return triggerFilter{}, false
	}
	filter, ok := triggerFilterFor(node)
	if !ok {
		t.Fatalf("%s declares a pull_request trigger this guard cannot decode, so the coverage the merge lane has to match is unknown", rel)
	}
	return filter, true
}

func triggerAcceptsPath(filter triggerFilter, path string) bool {
	if len(filter.Paths) > 0 && !matchesAnyGlob(filter.Paths, path) {
		return false
	}
	return !matchesAnyGlob(filter.PathsIgnore, path)
}

func matchesAnyGlob(globs []string, value string) bool {
	for _, glob := range globs {
		if regexp.MustCompile(globToPattern(glob)).MatchString(value) {
			return true
		}
	}
	return false
}

func globToPattern(glob string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(glob[i : i+1]))
		}
	}
	b.WriteString("$")
	return b.String()
}

func escalatablePaths(t *testing.T, root string) []string {
	t.Helper()
	tracked := trackedPaths(t, root)

	var claiming []*regexp.Regexp
	for _, rule := range escalationRules(t, root) {
		if rule.claimsASuite() {
			claiming = append(claiming, rule.compile(t))
		}
	}

	var paths []string
	for _, path := range tracked {
		for _, pattern := range claiming {
			if pattern.MatchString(path) {
				paths = append(paths, path)
				break
			}
		}
	}
	if len(paths) < len(claiming) {
		t.Fatalf("%s names %d suite-claiming rules but they match only %d tracked files; the probe set has stopped representing the map and this guard would pass on a handful of paths", escalationMap, len(claiming), len(paths))
	}
	return paths
}

func TestTheMatricesRunWhatEachTriggerRanBeforeTheCollapse(t *testing.T) {
	root := repoRoot(t)
	cases := []struct {
		name    string
		changed []string
		env     []string
		want    []string
	}{
		{"pull request with no escalation", []string{"README.md"}, []string{"E2E_EVENT=pull_request"}, []string{"api", "cli", "infra"}},
		{"pull request escalating api", []string{"tests/e2e/scenarios/api/x.sh"}, []string{"E2E_EVENT=pull_request"}, []string{"api", "api-extended", "cli", "infra"}},
		{"push", nil, []string{"E2E_EVENT=push"}, []string{"api-extended", "cli-extended", "infra-extended"}},
		{"dispatch all", nil, []string{"E2E_EVENT=workflow_dispatch", "E2E_DISPATCH_SUITE=all"}, []string{"api", "api-extended", "cli", "cli-extended", "infra", "infra-extended"}},
		{"dispatch cli", nil, []string{"E2E_EVENT=workflow_dispatch", "E2E_DISPATCH_SUITE=cli"}, []string{"cli", "cli-extended"}},
		{"dispatch smoke", nil, []string{"E2E_EVENT=workflow_dispatch", "E2E_DISPATCH_SUITE=smoke"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := plannedSuites(t, root, tc.changed, tc.env...); strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("planned %v, want %v", got, tc.want)
			}
		})
	}
}
