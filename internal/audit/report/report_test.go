package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/audit"
)

var update = flag.Bool("update", false, "regenerate golden files")

// sampleReport exercises every section: seaportal metadata, elements,
// visual diff, timing, console errors, broken assets, usability, security
// findings, and recommendations. GeneratedAt is fixed for determinism.
func sampleReport() audit.AuditReport {
	r := audit.NewAuditReport()
	r.GeneratedAt = time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	r.Input = audit.AuditInput{URLs: []string{"http://fixtures/audit-site/index.html"}}
	r.Options = audit.AuditOptions{Screenshot: true, NetworkMonitor: true, Concurrency: 2}
	r.SummaryScore = 85
	r.SecurityFindings = []audit.SecurityFinding{
		{RuleID: "insecure-form-action", Severity: "medium", Detail: "password posts over http", URL: "http://fixtures/audit-site/forms.html"},
	}
	r.Recommendations = []string{"Serve the site over https."}
	r.Pages = []audit.PageResult{
		{
			URL:   "http://fixtures/audit-site/index.html",
			Title: "Audit Fixture Site",
			Seaportal: map[string]any{
				"title":       "Audit Fixture Site",
				"description": "Deterministic audit fixture site.",
				"confidence":  90,
			},
			Browser: audit.BrowserPageData{
				ScreenshotPath: "screenshots/page-001.png",
				ConsoleLogs: []audit.ConsoleLogEntry{
					{Level: "error", Message: "boom"},
					{Level: "warning", Message: "careful"},
					{Level: "log", Message: "ignored in report"},
				},
				BrokenAssets: []audit.BrokenAsset{
					{URL: "http://fixtures/audit-site/assets/missing.png", ResourceType: "image", Status: 404},
					{URL: "http://fixtures/api", ResourceType: "xhr", Error: "net::ERR_FAILED"},
				},
				InteractiveElements: []audit.InteractiveElement{{Ref: "e1", Role: "link", Name: "Home"}},
				AccessibilityScore:  70,
				VisualDiff:          &audit.VisualDiffResult{BaselinePath: "base.png", CurrentPath: "cur.png", DiffPath: "diffs/index.diff.png", DiffPixels: 120, DiffRatio: 0.02, Changed: true},
				TimingMetrics:       audit.BrowserTimingMetrics{TimeToFirstByte: 5, FirstContentfulPaint: 120, LargestContentfulPaint: 300, CumulativeLayoutShift: 0.05, DOMContentLoaded: 90, Load: 200},
			},
		},
		{
			URL:   "http://fixtures/audit-site/down.html",
			Error: "navigation failed: net::ERR_CONNECTION_REFUSED",
		},
	}
	return r
}

func goldenCompare(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test -update`)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from golden (got %d bytes, want %d)", name, len(got), len(want))
	}
}

func TestRenderMarkdownGolden(t *testing.T) {
	got, err := Render(sampleReport(), FormatMarkdown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	goldenCompare(t, "report.golden.md", got)
}

func TestRenderHTMLGolden(t *testing.T) {
	got, err := Render(sampleReport(), FormatHTML)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	goldenCompare(t, "report.golden.html", got)
}

func TestAllSectionsPresent(t *testing.T) {
	md, err := Render(sampleReport(), FormatMarkdown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, heading := range []string{
		"## Summary",
		"## SEO & Metadata",
		"## Content & Functionality",
		"## Visual Differences",
		"## Performance",
		"## Console & JS Errors",
		"## Broken Assets",
		"## Usability Issues",
		"## Security Findings",
		"## Recommendations",
	} {
		if !strings.Contains(string(md), heading) {
			t.Errorf("markdown missing heading %q", heading)
		}
	}
}

func TestEmptySectionsOmitted(t *testing.T) {
	r := audit.NewAuditReport()
	r.GeneratedAt = time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	r.SummaryScore = 100
	r.Pages = []audit.PageResult{{URL: "http://x/clean.html", Title: "Clean", Browser: audit.BrowserPageData{AccessibilityScore: 100}}}

	md, err := Render(r, FormatMarkdown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(md), "## Summary") {
		t.Error("Summary must always render")
	}
	for _, absent := range []string{"## Broken Assets", "## Security Findings", "## Console", "## Visual Differences", "## SEO", "## Usability"} {
		if strings.Contains(string(md), absent) {
			t.Errorf("empty section %q should be omitted", absent)
		}
	}
}

func TestRenderDeterministic(t *testing.T) {
	for _, format := range []string{FormatMarkdown, FormatHTML} {
		first, err := Render(sampleReport(), format)
		if err != nil {
			t.Fatalf("Render(%s): %v", format, err)
		}
		for range 5 {
			again, err := Render(sampleReport(), format)
			if err != nil {
				t.Fatalf("Render(%s): %v", format, err)
			}
			if !bytes.Equal(first, again) {
				t.Fatalf("%s rendering is not byte-identical across runs", format)
			}
		}
	}
}

func TestHTMLSelfContained(t *testing.T) {
	html, err := Render(sampleReport(), FormatHTML)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	s := string(html)
	if !strings.Contains(s, "<style>") {
		t.Error("HTML must inline its CSS")
	}
	for _, external := range []string{"<link ", "<script src", "https://cdn.", "@import"} {
		if strings.Contains(s, external) {
			t.Errorf("HTML must be self-contained, found %q", external)
		}
	}
	if !strings.Contains(s, `src="screenshots/page-001.png"`) {
		t.Error("screenshot links must stay relative")
	}
}

func TestRenderRejectsUnknownFormat(t *testing.T) {
	if _, err := Render(sampleReport(), "pdf"); err == nil {
		t.Error("Render should reject unsupported formats")
	}
	if _, err := RenderComparison(audit.ComparisonReport{}, "json"); err == nil {
		t.Error("RenderComparison should reject unsupported formats")
	}
}

func TestRenderComparisonMarkdown(t *testing.T) {
	pct := 2.5
	cr := audit.ComparisonReport{
		SchemaVersion: audit.SchemaVersion,
		GeneratedAt:   time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		LiveBase:      "http://x/live/",
		StagingBase:   "http://x/stage/",
		HasDiffs:      true,
		Pages: []audit.PageComparison{
			{Path: "index.html", Status: audit.CompareStatusCompared, DiffPercentage: &pct, DiffImagePath: "diffs/index.html.diff.png",
				Drift: []audit.DataDrift{{Field: "accessibilityScore", Live: "100", Staging: "90"}}},
			{Path: "gone.html", Status: audit.CompareStatusRemoved, Drift: []audit.DataDrift{}},
		},
	}
	md, err := RenderComparison(cr, FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderComparison: %v", err)
	}
	for _, want := range []string{"# Site Comparison Report", "## Summary", "## Data Drift", "2.50%", "removed"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("comparison markdown missing %q", want)
		}
	}

	if _, err := RenderComparison(cr, FormatHTML); err != nil {
		t.Errorf("RenderComparison html: %v", err)
	}
}
