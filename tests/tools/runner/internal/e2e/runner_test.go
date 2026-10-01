package e2e

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func planWithServices(def suiteDef, services ...string) suitePlan {
	return suitePlan{def: def, scenarios: []scenarioMeta{{Key: def.Name, Services: services}}}
}

func TestServicesToBuildAddsOnlyTheRunnersThePlansExecute(t *testing.T) {
	fallback := []string{"pinchtab", "fixtures"}
	for _, tc := range []struct {
		name  string
		plans []suitePlan
		want  []string
	}{
		{
			name:  "api builds its runner-api and no cli runner",
			plans: []suitePlan{planWithServices(apiSuite(), "pinchtab", "fixtures")},
			want:  []string{"pinchtab", "fixtures", "runner-api"},
		},
		{
			name:  "cli builds runner-cli and never runner-api",
			plans: []suitePlan{planWithServices(cliSuite(), "pinchtab", "fixtures")},
			want:  []string{"pinchtab", "fixtures", "runner-cli"},
		},
		{
			name: "extended builds both runners and every pinchtab variant the plans need",
			plans: []suitePlan{
				planWithServices(apiExtendedSuite(), "pinchtab", "pinchtab-secure", "fixtures"),
				planWithServices(cliExtendedSuite(), "pinchtab", "fixtures"),
			},
			want: []string{"pinchtab", "pinchtab-secure", "fixtures", "runner-api", "runner-cli"},
		},
		{
			name:  "an image-only variant pulls in the pinchtab service that builds the shared image",
			plans: []suitePlan{planWithServices(apiExtendedSuite(), "pinchtab-secure", "fixtures")},
			want:  []string{"pinchtab", "pinchtab-secure", "fixtures", "runner-api"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := servicesToBuild(tc.plans, fallback)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("servicesToBuild = %v, want %v", got, tc.want)
			}
			up := servicesForPlans(tc.plans, fallback)
			for _, runner := range []string{"runner-api", "runner-cli"} {
				if slices.Contains(up, runner) {
					t.Errorf("up service list must not start the runner %q; it is executed via compose run", runner)
				}
			}
		})
	}
}

func TestDryRunBuildsOnlyTheSuiteRunner(t *testing.T) {
	for _, tc := range []struct {
		name      string
		suite     string
		wantLine  string
		wantNoStr string
	}{
		{name: "api", suite: "api", wantLine: "build pinchtab fixtures runner-api", wantNoStr: "runner-cli"},
		{name: "cli", suite: "cli", wantLine: "build pinchtab fixtures runner-cli", wantNoStr: "runner-api"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run([]string{"--suite", tc.suite, "--dry-run"}, &stdout, &stderr); code != 0 {
				t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
			}
			out := stdout.String()
			buildLine := ""
			for line := range strings.SplitSeq(out, "\n") {
				if strings.Contains(line, "compose") && strings.Contains(line, " build ") {
					buildLine = line
					break
				}
			}
			if buildLine == "" {
				t.Fatalf("no shared-stack build line in dry run:\n%s", out)
			}
			if !strings.Contains(buildLine, tc.wantLine) {
				t.Errorf("build line = %q, want it to contain %q", buildLine, tc.wantLine)
			}
			if strings.Contains(buildLine, tc.wantNoStr) {
				t.Errorf("build line %q must not name %q for the %s suite", buildLine, tc.wantNoStr, tc.suite)
			}
		})
	}
}
