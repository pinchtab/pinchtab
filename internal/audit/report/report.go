// Package report renders audit results into human-readable Markdown and
// self-contained HTML. Sections without data are omitted entirely (the
// documented empty-section policy); the JSON report remains the complete
// machine interface. Rendering is deterministic: stable section and item
// ordering, sorted map keys, and no wall-clock reads — the only timestamp
// is the report's own GeneratedAt.
package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/audit"
)

// Formats accepted by Render.
const (
	FormatJSON     = "json"
	FormatMarkdown = "md"
	FormatHTML     = "html"
)

// Render renders an AuditReport as Markdown or HTML. FormatJSON is the
// caller's business (the report already is JSON) and is rejected here.
func Render(r audit.AuditReport, format string) ([]byte, error) {
	switch format {
	case FormatMarkdown:
		return renderMarkdown(r), nil
	case FormatHTML:
		return renderHTML(r)
	default:
		return nil, fmt.Errorf("unsupported render format %q (md|html)", format)
	}
}

// RenderComparison renders a ComparisonReport as Markdown or HTML.
func RenderComparison(r audit.ComparisonReport, format string) ([]byte, error) {
	switch format {
	case FormatMarkdown:
		return renderComparisonMarkdown(r), nil
	case FormatHTML:
		return renderComparisonHTML(r)
	default:
		return nil, fmt.Errorf("unsupported render format %q (md|html)", format)
	}
}

func pageLabel(p audit.PageResult) string {
	if p.Title != "" {
		return p.Title
	}
	return p.URL
}

func hasSeaportal(r audit.AuditReport) bool {
	for _, p := range r.Pages {
		if len(p.Seaportal) > 0 {
			return true
		}
	}
	return false
}

func hasVisualDiff(r audit.AuditReport) bool {
	for _, p := range r.Pages {
		if p.Browser.VisualDiff != nil {
			return true
		}
	}
	return false
}

func hasTiming(r audit.AuditReport) bool {
	for _, p := range r.Pages {
		if p.Browser.TimingMetrics != (audit.BrowserTimingMetrics{}) {
			return true
		}
	}
	return false
}

func hasElements(r audit.AuditReport) bool {
	for _, p := range r.Pages {
		if len(p.Browser.InteractiveElements) > 0 {
			return true
		}
	}
	return false
}

// consoleProblems returns the error and warning console entries of a page.
func consoleProblems(p audit.PageResult) []audit.ConsoleLogEntry {
	var out []audit.ConsoleLogEntry
	for _, l := range p.Browser.ConsoleLogs {
		if l.Level == "error" || l.Level == "warning" || l.Level == "warn" {
			out = append(out, l)
		}
	}
	return out
}

func hasConsoleProblems(r audit.AuditReport) bool {
	for _, p := range r.Pages {
		if len(consoleProblems(p)) > 0 {
			return true
		}
	}
	return false
}

func hasBrokenAssets(r audit.AuditReport) bool {
	for _, p := range r.Pages {
		if len(p.Browser.BrokenAssets) > 0 {
			return true
		}
	}
	return false
}

// usabilityPages are the pages with a below-perfect accessibility score.
func usabilityPages(r audit.AuditReport) []audit.PageResult {
	var out []audit.PageResult
	for _, p := range r.Pages {
		if p.Error == "" && p.Browser.AccessibilityScore < 100 {
			out = append(out, p)
		}
	}
	return out
}

func totalBrokenAssets(r audit.AuditReport) int {
	n := 0
	for _, p := range r.Pages {
		n += len(p.Browser.BrokenAssets)
	}
	return n
}

func totalConsoleErrors(r audit.AuditReport) int {
	n := 0
	for _, p := range r.Pages {
		for _, l := range p.Browser.ConsoleLogs {
			if l.Level == "error" {
				n++
			}
		}
	}
	return n
}

// recommendations merges the report's explicit recommendations with
// rule-derived ones, in a stable order.
func recommendations(r audit.AuditReport) []string {
	recs := append([]string(nil), r.Recommendations...)
	if n := totalBrokenAssets(r); n > 0 {
		recs = append(recs, fmt.Sprintf("Fix %d broken asset reference(s) — see Broken Assets.", n))
	}
	if n := totalConsoleErrors(r); n > 0 {
		recs = append(recs, fmt.Sprintf("Investigate %d console error(s) — see Console & JS Errors.", n))
	}
	if pages := usabilityPages(r); len(pages) > 0 {
		recs = append(recs, fmt.Sprintf("Improve accessibility on %d page(s) scoring below 100 — see Usability Issues.", len(pages)))
	}
	if n := len(r.SecurityFindings); n > 0 {
		recs = append(recs, fmt.Sprintf("Address %d security finding(s) — see Security Findings.", n))
	}
	failed := 0
	for _, p := range r.Pages {
		if p.Error != "" {
			failed++
		}
	}
	if failed > 0 {
		recs = append(recs, fmt.Sprintf("Re-check %d page(s) that failed to audit.", failed))
	}
	return recs
}

// sortedKeys returns a map's keys in stable sorted order.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatMs(v float64) string {
	if v == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f ms", v)
}

func formatCLS(v float64) string {
	if v == 0 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", v), "0"), ".")
}
