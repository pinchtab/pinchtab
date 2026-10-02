package e2e

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	inlineTabIDParse   = regexp.MustCompile(`jq\s+-r\s+['"]\.tabId\b`)
	tabIDParserOpens   = regexp.MustCompile(`^get_tab_id\(\)\s*\{`)
	functionClosesAtC0 = regexp.MustCompile(`^\}`)
	counterWrite       = regexp.MustCompile(`(\+\+|--)\s*ASSERTIONS_(PASSED|FAILED|SKIPPED)\b|\bASSERTIONS_(PASSED|FAILED|SKIPPED)\s*(\+\+|--|[-+*/%]?=([^=]|$))`)
)

func TestScenariosReadTabIDThroughTheHelper(t *testing.T) {
	files := loadScenarioFiles(t)
	for name, body := range loadHelperFiles(t) {
		files = append(files, scenarioFile{Name: name, Body: body})
	}
	var offenders []string
	for _, f := range files {
		offenders = append(offenders, inlineTabIDParsesOutsideTheParser(f)...)
	}
	if len(offenders) > 0 {
		t.Fatalf("read a tab id with get_tab_id (or get_tab_id \"$JSON\") from helpers/base.sh, not an inline jq:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

func TestTabIDRuleCatchesAnInlineParseInAHelper(t *testing.T) {
	helper := scenarioFile{
		Name: "helpers/example.sh",
		Body: "get_tab_id() {\n  jq -r '.tabId // empty' <<<\"$1\"\n}\n\nassert_it() {\n  TAB_ID=$(echo \"$RESULT\" | jq -r '.tabId')\n}\n",
	}
	got := inlineTabIDParsesOutsideTheParser(helper)
	want := []string{`helpers/example.sh:6: TAB_ID=$(echo "$RESULT" | jq -r '.tabId')`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("offenders = %q, want %q", got, want)
	}
}

func inlineTabIDParsesOutsideTheParser(f scenarioFile) []string {
	var offenders []string
	inParser := false
	for i, line := range strings.Split(f.Body, "\n") {
		switch {
		case tabIDParserOpens.MatchString(line):
			inParser = true
		case inParser && functionClosesAtC0.MatchString(line):
			inParser = false
		case !inParser && inlineTabIDParse.MatchString(line):
			offenders = append(offenders, f.Name+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
		}
	}
	return offenders
}

func TestScenariosBumpAssertionCountersThroughTheHelpers(t *testing.T) {
	var offenders []string
	for _, f := range loadScenarioFiles(t) {
		offenders = append(offenders, inlineCounterWrites(f)...)
	}
	if len(offenders) > 0 {
		t.Fatalf("count assertions with pass_assert/fail_assert/skip_assert from helpers/, not an inline ASSERTIONS_* write:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

func TestCounterRuleCatchesEveryInlineWriteForm(t *testing.T) {
	scenario := scenarioFile{
		Name: "scenarios/example.sh",
		Body: strings.Join([]string{
			`((ASSERTIONS_PASSED++))`,
			`if [ "$ASSERTIONS_FAILED" == 0 ]; then`,
			`  (( ASSERTIONS_FAILED += 1 ))`,
			`fi`,
			`ASSERTIONS_SKIPPED=$((ASSERTIONS_SKIPPED+1))`,
			`pass_assert "ok"`,
			`(( ++ASSERTIONS_PASSED ))`,
			`((ASSERTIONS_FAILED-=1))`,
			`echo "$ASSERTIONS_PASSED"`,
			`((ASSERTIONS_SKIPPED--))`,
			`[ "$ASSERTIONS_FAILED" != 0 ] && exit 1`,
			`ASSERTIONS_PASSED=`,
		}, "\n"),
	}
	got := inlineCounterWrites(scenario)
	want := []string{
		"scenarios/example.sh:1: ((ASSERTIONS_PASSED++))",
		"scenarios/example.sh:3: (( ASSERTIONS_FAILED += 1 ))",
		"scenarios/example.sh:5: ASSERTIONS_SKIPPED=$((ASSERTIONS_SKIPPED+1))",
		"scenarios/example.sh:7: (( ++ASSERTIONS_PASSED ))",
		"scenarios/example.sh:8: ((ASSERTIONS_FAILED-=1))",
		"scenarios/example.sh:10: ((ASSERTIONS_SKIPPED--))",
		"scenarios/example.sh:12: ASSERTIONS_PASSED=",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("offenders = %q, want %q", got, want)
	}
}

func inlineCounterWrites(f scenarioFile) []string {
	var offenders []string
	for i, line := range strings.Split(f.Body, "\n") {
		if counterWrite.MatchString(line) {
			offenders = append(offenders, f.Name+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
		}
	}
	return offenders
}
