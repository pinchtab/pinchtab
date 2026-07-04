package audit

import (
	"reflect"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

var cleanFacts = PageFacts{Title: "Page", Lang: "en", HeadingLevels: []int{1, 2}}

func findingByRule(t *testing.T, report A11yReport, rule string) A11yFinding {
	t.Helper()
	for _, f := range report.Findings {
		if f.Rule == rule {
			return f
		}
	}
	t.Fatalf("finding %q not present in %+v", rule, report.Findings)
	return A11yFinding{}
}

func TestCleanTreeScoresPerfect(t *testing.T) {
	nodes := []observe.A11yNode{
		{Role: "image", Name: "logo"},
		{Role: "textbox", Name: "Search"},
		{Role: "link", Name: "Home"},
		{Role: "button", Name: "Submit"},
	}
	report := EvaluateA11y(nodes, cleanFacts)
	if report.Score != 100 {
		t.Errorf("Score = %d, want 100", report.Score)
	}
	if len(report.Findings) != 0 {
		t.Errorf("Findings = %+v, want none", report.Findings)
	}
	if report.Findings == nil {
		t.Error("Findings should be an empty slice, not nil (JSON [])")
	}
}

func TestMissingAltRule(t *testing.T) {
	report := EvaluateA11y([]observe.A11yNode{{Role: "image", Ref: "e3"}}, cleanFacts)
	f := findingByRule(t, report, "missing-alt")
	if f.Severity != SeveritySerious || f.Count != 1 {
		t.Errorf("missing-alt = %+v", f)
	}
	if report.Score != 90 {
		t.Errorf("Score = %d, want 90 (100 - 10)", report.Score)
	}
	named := EvaluateA11y([]observe.A11yNode{{Role: "image", Name: "logo"}}, cleanFacts)
	altOnly := EvaluateA11y([]observe.A11yNode{{Role: "img", Alt: "logo"}}, cleanFacts)
	if len(named.Findings) != 0 || len(altOnly.Findings) != 0 {
		t.Error("image with name or alt should not violate missing-alt")
	}
}

func TestMissingLabelRule(t *testing.T) {
	for _, role := range []string{"textbox", "searchbox", "combobox", "checkbox", "radio", "spinbutton", "slider", "switch"} {
		report := EvaluateA11y([]observe.A11yNode{{Role: role}}, cleanFacts)
		f := findingByRule(t, report, "missing-label")
		if f.Severity != SeveritySerious || f.Count != 1 {
			t.Errorf("%s: missing-label = %+v", role, f)
		}
	}
	labeled := EvaluateA11y([]observe.A11yNode{{Role: "textbox", Label: "Search"}}, cleanFacts)
	if len(labeled.Findings) != 0 {
		t.Error("labeled textbox should not violate missing-label")
	}
}

func TestEmptyLinkAndButtonRules(t *testing.T) {
	report := EvaluateA11y([]observe.A11yNode{{Role: "link"}, {Role: "button"}}, cleanFacts)
	if f := findingByRule(t, report, "empty-link"); f.Severity != SeverityModerate {
		t.Errorf("empty-link = %+v", f)
	}
	if f := findingByRule(t, report, "empty-button"); f.Severity != SeverityModerate {
		t.Errorf("empty-button = %+v", f)
	}
	if report.Score != 90 {
		t.Errorf("Score = %d, want 90 (100 - 5 - 5)", report.Score)
	}
	withText := EvaluateA11y([]observe.A11yNode{{Role: "link", Text: "Home"}, {Role: "button", Name: "Go"}}, cleanFacts)
	if len(withText.Findings) != 0 {
		t.Error("link with text / named button should not violate")
	}
}

func TestHiddenNodesSkipped(t *testing.T) {
	report := EvaluateA11y([]observe.A11yNode{{Role: "image", Hidden: true}}, cleanFacts)
	if len(report.Findings) != 0 {
		t.Errorf("hidden node should be skipped, got %+v", report.Findings)
	}
}

func TestPageFactRules(t *testing.T) {
	report := EvaluateA11y(nil, PageFacts{HeadingLevels: []int{1, 3, 4, 6}})
	if f := findingByRule(t, report, "missing-title"); f.Severity != SeverityModerate {
		t.Errorf("missing-title = %+v", f)
	}
	if f := findingByRule(t, report, "missing-lang"); f.Severity != SeverityModerate {
		t.Errorf("missing-lang = %+v", f)
	}
	skip := findingByRule(t, report, "heading-skip")
	if skip.Severity != SeverityMinor || skip.Count != 2 {
		t.Errorf("heading-skip = %+v, want count 2 (h1→h3, h4→h6)", skip)
	}
	if skip.Samples[0] != "h3 follows h1" {
		t.Errorf("heading-skip sample = %q", skip.Samples[0])
	}
	if report.Score != 100-5-5-2*2 {
		t.Errorf("Score = %d, want %d", report.Score, 100-5-5-2*2)
	}
}

func TestScoreFlooredAtZero(t *testing.T) {
	var nodes []observe.A11yNode
	for range 20 {
		nodes = append(nodes, observe.A11yNode{Role: "image"})
	}
	report := EvaluateA11y(nodes, cleanFacts)
	if report.Score != 0 {
		t.Errorf("Score = %d, want 0 (floored)", report.Score)
	}
	if f := findingByRule(t, report, "missing-alt"); f.Count != 20 || len(f.Samples) != maxA11ySamples {
		t.Errorf("missing-alt count/samples = %d/%d, want 20/%d", f.Count, len(f.Samples), maxA11ySamples)
	}
}

func TestDeterministicOrderingAndScore(t *testing.T) {
	nodes := []observe.A11yNode{
		{Role: "button"},
		{Role: "image", Ref: "e1"},
		{Role: "link"},
		{Role: "textbox"},
	}
	first := EvaluateA11y(nodes, cleanFacts)
	second := EvaluateA11y(nodes, cleanFacts)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("repeated evaluation differs:\n%+v\n%+v", first, second)
	}
	var rules []string
	for _, f := range first.Findings {
		rules = append(rules, f.Rule)
	}
	want := []string{"empty-button", "empty-link", "missing-alt", "missing-label"}
	if !reflect.DeepEqual(rules, want) {
		t.Errorf("rules order = %v, want %v", rules, want)
	}
	if first.Score != 100-10-10-5-5 {
		t.Errorf("Score = %d, want %d", first.Score, 100-10-10-5-5)
	}
}
