package actions

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/audit"
)

// The headline that exists to surface failures must count a dead URL, and must
// not also count that page's own document among the broken assets.
func TestAuditSummaryCountsA404PageAsFailedOnce(t *testing.T) {
	report := audit.NewAuditReport()
	report.SummaryScore = 90
	report.Pages = []audit.PageResult{
		{URL: "http://x/ok", StatusCode: 200},
		{URL: "http://x/missing", StatusCode: 404, Browser: audit.BrowserPageData{BrokenAssets: []audit.BrokenAsset{
			{URL: "http://x/missing", ResourceType: "document", Status: 404},
			{URL: "http://x/favicon.ico", ResourceType: "other", Status: 404},
		}}},
	}
	lines := auditSummaryLines(report)
	if !strings.Contains(lines[0], "1 broken asset(s)") || !strings.Contains(lines[0], "1 failed page(s)") {
		t.Fatalf("headline = %q, want one broken asset and one failed page", lines[0])
	}
	if !strings.Contains(lines[0], "mean accessibility score 90") {
		t.Fatalf("headline moved the score: %q", lines[0])
	}
	if !strings.HasSuffix(lines[2], "http 404") {
		t.Fatalf("the 404 page line = %q, want http 404", lines[2])
	}
	if !strings.HasSuffix(lines[1], "ok") {
		t.Fatalf("the healthy page line = %q", lines[1])
	}
}
