package report

import (
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/audit"
)

func renderMarkdown(r audit.AuditReport) []byte {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	w("# Site Audit Report")
	w("")
	w("- Schema version: %s", r.SchemaVersion)
	w("- Generated: %s", r.GeneratedAt.Format("2006-01-02 15:04:05 UTC"))
	w("- Pages audited: %d", len(r.Pages))
	w("")

	w("## Summary")
	w("")
	w("**Summary score: %d/100**", r.SummaryScore)
	w("")
	w("| Page | Score | Load | Broken assets | Console errors | Status |")
	w("|---|---|---|---|---|---|")
	for _, p := range r.Pages {
		status := "ok"
		if p.Error != "" {
			status = "error: " + p.Error
		}
		w("| %s | %d | %s | %d | %d | %s |",
			pageLabel(p), p.Browser.AccessibilityScore, formatMs(p.Browser.TimingMetrics.Load),
			len(p.Browser.BrokenAssets), pageConsoleErrors(p), status)
	}
	w("")

	if hasSeaportal(r) {
		w("## SEO & Metadata")
		w("")
		for _, p := range r.Pages {
			if len(p.Seaportal) == 0 {
				continue
			}
			w("### %s", pageLabel(p))
			w("")
			for _, k := range sortedKeys(p.Seaportal) {
				w("- %s: %v", k, p.Seaportal[k])
			}
			w("")
		}
	}

	if hasElements(r) {
		w("## Content & Functionality")
		w("")
		w("| Page | Title | Interactive elements |")
		w("|---|---|---|")
		for _, p := range r.Pages {
			w("| %s | %s | %d |", p.URL, p.Title, len(p.Browser.InteractiveElements))
		}
		w("")
	}

	if hasVisualDiff(r) {
		w("## Visual Differences")
		w("")
		w("| Page | Changed | Diff pixels | Diff ratio | Diff image |")
		w("|---|---|---|---|---|")
		for _, p := range r.Pages {
			vd := p.Browser.VisualDiff
			if vd == nil {
				continue
			}
			w("| %s | %v | %d | %.4f | %s |", pageLabel(p), vd.Changed, vd.DiffPixels, vd.DiffRatio, vd.DiffPath)
		}
		w("")
	}

	if hasTiming(r) {
		w("## Performance")
		w("")
		w("| Page | TTFB | FCP | LCP | CLS | DOMContentLoaded | Load |")
		w("|---|---|---|---|---|---|---|")
		for _, p := range r.Pages {
			t := p.Browser.TimingMetrics
			if t == (audit.BrowserTimingMetrics{}) {
				continue
			}
			w("| %s | %s | %s | %s | %s | %s | %s |", pageLabel(p),
				formatMs(t.TimeToFirstByte), formatMs(t.FirstContentfulPaint), formatMs(t.LargestContentfulPaint),
				formatCLS(t.CumulativeLayoutShift), formatMs(t.DOMContentLoaded), formatMs(t.Load))
		}
		w("")
	}

	if hasConsoleProblems(r) {
		w("## Console & JS Errors")
		w("")
		for _, p := range r.Pages {
			problems := consoleProblems(p)
			if len(problems) == 0 {
				continue
			}
			w("### %s", pageLabel(p))
			w("")
			for _, l := range problems {
				w("- `%s` %s", l.Level, l.Message)
			}
			w("")
		}
	}

	if hasBrokenAssets(r) {
		w("## Broken Assets")
		w("")
		w("| Page | Asset | Type | Status |")
		w("|---|---|---|---|")
		for _, p := range r.Pages {
			for _, a := range p.Browser.BrokenAssets {
				status := fmt.Sprintf("%d", a.Status)
				if a.Status == 0 {
					status = a.Error
				}
				w("| %s | %s | %s | %s |", pageLabel(p), a.URL, a.ResourceType, status)
			}
		}
		w("")
	}

	if pages := usabilityPages(r); len(pages) > 0 {
		w("## Usability Issues")
		w("")
		for _, p := range pages {
			w("- %s: accessibility score %d/100", pageLabel(p), p.Browser.AccessibilityScore)
		}
		w("")
	}

	if len(r.SecurityFindings) > 0 {
		w("## Security Findings")
		w("")
		for _, f := range r.SecurityFindings {
			suffix := ""
			if f.URL != "" {
				suffix = " (" + f.URL + ")"
			}
			w("- **%s** [%s]: %s%s", f.RuleID, f.Severity, f.Detail, suffix)
		}
		w("")
	}

	if recs := recommendations(r); len(recs) > 0 {
		w("## Recommendations")
		w("")
		for i, rec := range recs {
			w("%d. %s", i+1, rec)
		}
		w("")
	}

	return []byte(b.String())
}

func pageConsoleErrors(p audit.PageResult) int {
	n := 0
	for _, l := range p.Browser.ConsoleLogs {
		if l.Level == "error" {
			n++
		}
	}
	return n
}

func renderComparisonMarkdown(r audit.ComparisonReport) []byte {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	w("# Site Comparison Report")
	w("")
	w("- Schema version: %s", r.SchemaVersion)
	w("- Generated: %s", r.GeneratedAt.Format("2006-01-02 15:04:05 UTC"))
	w("- Live: %s", r.LiveBase)
	w("- Staging: %s", r.StagingBase)
	w("- Differences found: %v", r.HasDiffs)
	w("")

	w("## Summary")
	w("")
	w("| Page | Status | Visual diff | Data drift | Diff image |")
	w("|---|---|---|---|---|")
	for _, p := range r.Pages {
		label := p.Path
		if label == "" {
			label = "(base)"
		}
		visual := "-"
		if p.DiffPercentage != nil {
			visual = fmt.Sprintf("%.2f%%", *p.DiffPercentage)
		}
		w("| %s | %s | %s | %d | %s |", label, p.Status, visual, len(p.Drift), p.DiffImagePath)
	}
	w("")

	drifted := false
	for _, p := range r.Pages {
		if len(p.Drift) > 0 {
			drifted = true
			break
		}
	}
	if drifted {
		w("## Data Drift")
		w("")
		w("| Page | Field | Live | Staging |")
		w("|---|---|---|---|")
		for _, p := range r.Pages {
			for _, d := range p.Drift {
				w("| %s | %s | %s | %s |", p.Path, d.Field, d.Live, d.Staging)
			}
		}
		w("")
	}

	return []byte(b.String())
}
