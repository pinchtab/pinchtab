package audit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

// Severity levels for accessibility findings.
const (
	SeveritySerious  = "serious"
	SeverityModerate = "moderate"
	SeverityMinor    = "minor"
)

// a11yWeights are the per-violation score deductions by severity. The score
// formula is: 100 - Σ weight(severity) × count, floored at 0.
var a11yWeights = map[string]int{
	SeveritySerious:  10,
	SeverityModerate: 5,
	SeverityMinor:    2,
}

// maxA11ySamples caps how many sample descriptions a finding carries.
const maxA11ySamples = 5

// A11yFinding is one accessibility rule violation aggregated across a page.
type A11yFinding struct {
	// Rule is the stable rule identifier (e.g. "missing-alt").
	Rule string `json:"rule"`
	// Severity is serious, moderate, or minor.
	Severity string `json:"severity"`
	// Count is how many times the rule was violated.
	Count int `json:"count"`
	// Samples describe up to maxA11ySamples offending elements.
	Samples []string `json:"samples"`
}

// A11yReport is the page-level accessibility audit result.
type A11yReport struct {
	// Score is 100 minus severity-weighted violation counts, floored at 0.
	Score int `json:"score"`
	// Findings are the rule violations, sorted by rule for determinism.
	Findings []A11yFinding `json:"findings"`
}

// PageFacts holds page-level facts the AX tree cannot provide, gathered by
// PageFactsScript.
type PageFacts struct {
	// Title is document.title.
	Title string `json:"title"`
	// Lang is the html element's lang attribute.
	Lang string `json:"lang"`
	// HeadingLevels are the h1–h6 levels in document order.
	HeadingLevels []int `json:"headingLevels"`
}

// PageFactsScript evaluates to the PageFacts JSON shape in the page.
const PageFactsScript = `(() => ({
  title: document.title || '',
  lang: document.documentElement.getAttribute('lang') || '',
  headingLevels: Array.from(document.querySelectorAll('h1,h2,h3,h4,h5,h6')).map((h) => Number(h.tagName[1])),
}))()`

// labelableRoles are AX roles that require an accessible name from a label.
var labelableRoles = map[string]bool{
	"textbox":    true,
	"searchbox":  true,
	"combobox":   true,
	"listbox":    true,
	"checkbox":   true,
	"radio":      true,
	"spinbutton": true,
	"slider":     true,
	"switch":     true,
}

// EvaluateA11y derives rule-based findings and a 0–100 score from the
// accessibility snapshot plus page facts. Deterministic for the same inputs:
// findings are keyed and sorted by rule, samples follow node order.
func EvaluateA11y(nodes []observe.A11yNode, facts PageFacts) A11yReport {
	collector := map[string]*A11yFinding{}
	record := func(rule, severity, sample string) {
		f, ok := collector[rule]
		if !ok {
			f = &A11yFinding{Rule: rule, Severity: severity, Samples: []string{}}
			collector[rule] = f
		}
		f.Count++
		if len(f.Samples) < maxA11ySamples {
			f.Samples = append(f.Samples, sample)
		}
	}

	for _, node := range nodes {
		if node.Hidden {
			continue
		}
		name := strings.TrimSpace(node.Name)
		switch {
		case (node.Role == "image" || node.Role == "img") && name == "" && strings.TrimSpace(node.Alt) == "":
			record("missing-alt", SeveritySerious, nodeSample(node))
		case labelableRoles[node.Role] && name == "" && strings.TrimSpace(node.Label) == "":
			record("missing-label", SeveritySerious, nodeSample(node))
		case node.Role == "link" && name == "" && strings.TrimSpace(node.Text) == "":
			record("empty-link", SeverityModerate, nodeSample(node))
		case node.Role == "button" && name == "" && strings.TrimSpace(node.Text) == "":
			record("empty-button", SeverityModerate, nodeSample(node))
		}
	}

	if strings.TrimSpace(facts.Title) == "" {
		record("missing-title", SeverityModerate, "document.title is empty")
	}
	if strings.TrimSpace(facts.Lang) == "" {
		record("missing-lang", SeverityModerate, "html element has no lang attribute")
	}
	prev := 0
	for _, level := range facts.HeadingLevels {
		if prev > 0 && level > prev+1 {
			record("heading-skip", SeverityMinor, fmt.Sprintf("h%d follows h%d", level, prev))
		}
		prev = level
	}

	report := A11yReport{Score: 100, Findings: []A11yFinding{}}
	for _, f := range collector {
		report.Findings = append(report.Findings, *f)
		report.Score -= a11yWeights[f.Severity] * f.Count
	}
	if report.Score < 0 {
		report.Score = 0
	}
	sort.Slice(report.Findings, func(i, j int) bool {
		return report.Findings[i].Rule < report.Findings[j].Rule
	})
	return report
}

func nodeSample(node observe.A11yNode) string {
	if node.Ref != "" {
		return node.Role + " " + node.Ref
	}
	if node.Tag != "" {
		return node.Role + " <" + node.Tag + ">"
	}
	return node.Role
}
