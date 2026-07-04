package report

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/pinchtab/pinchtab/internal/audit"
)

// reportCSS keeps the HTML output self-contained: no external stylesheets,
// fonts, or scripts.
const reportCSS = `
body { font-family: -apple-system, "Segoe UI", Roboto, sans-serif; margin: 2rem auto; max-width: 60rem; padding: 0 1rem; color: #1a1a2e; }
h1 { border-bottom: 2px solid #0a6cff; padding-bottom: .3rem; }
h2 { margin-top: 2rem; border-bottom: 1px solid #d0d4dd; padding-bottom: .2rem; }
table { border-collapse: collapse; width: 100%; margin: .5rem 0 1rem; }
th, td { border: 1px solid #d0d4dd; padding: .35rem .6rem; text-align: left; font-size: .92rem; }
th { background: #f2f4f8; }
.score { font-size: 1.3rem; font-weight: 700; }
.error { color: #c0182b; }
img.screenshot { max-width: 100%; border: 1px solid #d0d4dd; margin: .3rem 0 1rem; }
code { background: #f2f4f8; padding: .1rem .3rem; border-radius: 3px; }
`

var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"pageLabel":       pageLabel,
	"consoleProblems": consoleProblems,
	"formatMs":        formatMs,
	"formatCLS":       formatCLS,
	"sortedKeys":      sortedKeys,
	"pageErrors":      pageConsoleErrors,
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Site Audit Report</title>
<style>{{.CSS}}</style>
</head>
<body>
<h1>Site Audit Report</h1>
<p>Schema version {{.R.SchemaVersion}} · generated {{.R.GeneratedAt.Format "2006-01-02 15:04:05 UTC"}} · {{len .R.Pages}} page(s)</p>

<h2>Summary</h2>
<p class="score">Summary score: {{.R.SummaryScore}}/100</p>
<table>
<tr><th>Page</th><th>Score</th><th>Load</th><th>Broken assets</th><th>Console errors</th><th>Status</th></tr>
{{range .R.Pages}}<tr><td>{{pageLabel .}}</td><td>{{.Browser.AccessibilityScore}}</td><td>{{formatMs .Browser.TimingMetrics.Load}}</td><td>{{len .Browser.BrokenAssets}}</td><td>{{pageErrors .}}</td><td>{{if .Error}}<span class="error">{{.Error}}</span>{{else}}ok{{end}}</td></tr>
{{end}}</table>

{{if .HasSeaportal}}<h2>SEO &amp; Metadata</h2>
{{range .R.Pages}}{{if .Seaportal}}<h3>{{pageLabel .}}</h3><table>
{{$p := .}}{{range sortedKeys .Seaportal}}<tr><th>{{.}}</th><td>{{index $p.Seaportal .}}</td></tr>
{{end}}</table>{{end}}{{end}}{{end}}

{{if .HasElements}}<h2>Content &amp; Functionality</h2>
<table><tr><th>Page</th><th>Title</th><th>Interactive elements</th></tr>
{{range .R.Pages}}<tr><td>{{.URL}}</td><td>{{.Title}}</td><td>{{len .Browser.InteractiveElements}}</td></tr>
{{end}}</table>{{end}}

{{if .HasVisualDiff}}<h2>Visual Differences</h2>
<table><tr><th>Page</th><th>Changed</th><th>Diff pixels</th><th>Diff ratio</th><th>Diff image</th></tr>
{{range .R.Pages}}{{if .Browser.VisualDiff}}<tr><td>{{pageLabel .}}</td><td>{{.Browser.VisualDiff.Changed}}</td><td>{{.Browser.VisualDiff.DiffPixels}}</td><td>{{printf "%.4f" .Browser.VisualDiff.DiffRatio}}</td><td>{{.Browser.VisualDiff.DiffPath}}</td></tr>{{end}}
{{end}}</table>{{end}}

{{if .HasTiming}}<h2>Performance</h2>
<table><tr><th>Page</th><th>TTFB</th><th>FCP</th><th>LCP</th><th>CLS</th><th>Load</th></tr>
{{range .R.Pages}}<tr><td>{{pageLabel .}}</td><td>{{formatMs .Browser.TimingMetrics.TimeToFirstByte}}</td><td>{{formatMs .Browser.TimingMetrics.FirstContentfulPaint}}</td><td>{{formatMs .Browser.TimingMetrics.LargestContentfulPaint}}</td><td>{{formatCLS .Browser.TimingMetrics.CumulativeLayoutShift}}</td><td>{{formatMs .Browser.TimingMetrics.Load}}</td></tr>
{{end}}</table>{{end}}

{{if .HasConsole}}<h2>Console &amp; JS Errors</h2>
{{range .R.Pages}}{{$problems := consoleProblems .}}{{if $problems}}<h3>{{pageLabel .}}</h3><ul>
{{range $problems}}<li><code>{{.Level}}</code> {{.Message}}</li>
{{end}}</ul>{{end}}{{end}}{{end}}

{{if .HasBroken}}<h2>Broken Assets</h2>
<table><tr><th>Page</th><th>Asset</th><th>Type</th><th>Status</th></tr>
{{range .R.Pages}}{{$p := .}}{{range .Browser.BrokenAssets}}<tr><td>{{pageLabel $p}}</td><td>{{.URL}}</td><td>{{.ResourceType}}</td><td>{{if .Status}}{{.Status}}{{else}}{{.Error}}{{end}}</td></tr>
{{end}}{{end}}</table>{{end}}

{{if .Usability}}<h2>Usability Issues</h2><ul>
{{range .Usability}}<li>{{pageLabel .}}: accessibility score {{.Browser.AccessibilityScore}}/100</li>
{{end}}</ul>{{end}}

{{if .R.SecurityFindings}}<h2>Security Findings</h2><ul>
{{range .R.SecurityFindings}}<li><strong>{{.RuleID}}</strong> [{{.Severity}}]: {{.Detail}}{{if .URL}} ({{.URL}}){{end}}</li>
{{end}}</ul>{{end}}

{{if .Recommendations}}<h2>Recommendations</h2><ol>
{{range .Recommendations}}<li>{{.}}</li>
{{end}}</ol>{{end}}

{{if .Screenshots}}<h2>Screenshots</h2>
{{range .R.Pages}}{{if .Browser.ScreenshotPath}}<h3>{{pageLabel .}}</h3><img class="screenshot" src="{{.Browser.ScreenshotPath}}" alt="Screenshot of {{pageLabel .}}">{{end}}{{end}}{{end}}
</body>
</html>
`))

type htmlContext struct {
	R               audit.AuditReport
	CSS             template.CSS
	HasSeaportal    bool
	HasElements     bool
	HasVisualDiff   bool
	HasTiming       bool
	HasConsole      bool
	HasBroken       bool
	Usability       []audit.PageResult
	Recommendations []string
	Screenshots     bool
}

func renderHTML(r audit.AuditReport) ([]byte, error) {
	screenshots := false
	for _, p := range r.Pages {
		if p.Browser.ScreenshotPath != "" {
			screenshots = true
			break
		}
	}
	var buf bytes.Buffer
	err := htmlTemplate.Execute(&buf, htmlContext{
		R:               r,
		CSS:             template.CSS(reportCSS),
		HasSeaportal:    hasSeaportal(r),
		HasElements:     hasElements(r),
		HasVisualDiff:   hasVisualDiff(r),
		HasTiming:       hasTiming(r),
		HasConsole:      hasConsoleProblems(r),
		HasBroken:       hasBrokenAssets(r),
		Usability:       usabilityPages(r),
		Recommendations: recommendations(r),
		Screenshots:     screenshots,
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var comparisonHTMLTemplate = template.Must(template.New("comparison").Funcs(template.FuncMap{
	"pct": func(p *float64) string {
		if p == nil {
			return "-"
		}
		return fmt.Sprintf("%.2f%%", *p)
	},
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Site Comparison Report</title>
<style>{{.CSS}}</style>
</head>
<body>
<h1>Site Comparison Report</h1>
<p>Schema version {{.R.SchemaVersion}} · generated {{.R.GeneratedAt.Format "2006-01-02 15:04:05 UTC"}}</p>
<p>Live: {{.R.LiveBase}}<br>Staging: {{.R.StagingBase}}<br>Differences found: {{.R.HasDiffs}}</p>

<h2>Summary</h2>
<table><tr><th>Page</th><th>Status</th><th>Visual diff</th><th>Data drift</th><th>Diff image</th></tr>
{{range .R.Pages}}<tr><td>{{if .Path}}{{.Path}}{{else}}(base){{end}}</td><td>{{.Status}}</td><td>{{pct .DiffPercentage}}</td><td>{{len .Drift}}</td><td>{{if .DiffImagePath}}<img class="screenshot" src="{{.DiffImagePath}}" alt="Diff for {{.Path}}">{{end}}</td></tr>
{{end}}</table>
</body>
</html>
`))

func renderComparisonHTML(r audit.ComparisonReport) ([]byte, error) {
	var buf bytes.Buffer
	err := comparisonHTMLTemplate.Execute(&buf, struct {
		R   audit.ComparisonReport
		CSS template.CSS
	}{r, template.CSS(reportCSS)})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
