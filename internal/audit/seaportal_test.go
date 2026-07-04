package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

func TestParseSeaportalReport(t *testing.T) {
	data := []byte(`[
		{"url":"http://fixtures/audit-site/index.html","title":"Home","description":"Landing page","confidence":90,"statusCode":200,
		 "profile":{"decision":"browser-needed","browserRecommended":true}},
		{"url":"http://fixtures/audit-site/clean.html","title":"Clean","description":"Static page","confidence":95,"statusCode":200,
		 "profile":{"decision":"static-high-confidence","browserRecommended":false}},
		{"url":"","title":"dropped"}
	]`)
	pages, err := ParseSeaportalReport(data)
	if err != nil {
		t.Fatalf("ParseSeaportalReport: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2 (empty URL dropped)", len(pages))
	}
	if !pages[0].BrowserRecommended || pages[1].BrowserRecommended {
		t.Errorf("BrowserRecommended routing wrong: %+v", pages)
	}
	if pages[1].Summary["title"] != "Clean" || pages[1].Summary["description"] != "Static page" {
		t.Errorf("Summary = %+v", pages[1].Summary)
	}
	if pages[1].Summary["decision"] != "static-high-confidence" {
		t.Errorf("decision = %v", pages[1].Summary["decision"])
	}

	if _, err := ParseSeaportalReport([]byte("{not an array}")); err == nil {
		t.Error("ParseSeaportalReport should error on malformed input")
	}
}

func TestFlattenSitemapURLsAgainstFixture(t *testing.T) {
	data, err := os.ReadFile("../../tests/e2e/fixtures/audit-site/sitemap.xml")
	if err != nil {
		t.Fatalf("read fixture sitemap: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	urls, err := FlattenSitemapURLs(context.Background(), srv.URL+"/sitemap.xml")
	if err != nil {
		t.Fatalf("FlattenSitemapURLs: %v", err)
	}
	if len(urls) != 12 {
		t.Errorf("urls = %d, want 12", len(urls))
	}
	if urls[0] != "http://fixtures/audit-site/index.html" {
		t.Errorf("first url = %q", urls[0])
	}
}

func TestRunAuditSeaportalRouting(t *testing.T) {
	pages := []SeaportalPage{
		{URL: "http://x/spa", Title: "SPA", BrowserRecommended: true, Summary: map[string]any{"title": "SPA"}},
		{URL: "http://x/static", Title: "Static", StatusCode: 200, BrowserRecommended: false, Summary: map[string]any{"title": "Static", "description": "d"}},
	}
	var audited []string
	auditor := func(url string, opts PageOptions) PageAudit {
		audited = append(audited, url)
		return PageAudit{URL: url, Title: "browser " + url, BrowserPageData: BrowserPageData{AccessibilityScore: 100}}
	}

	report, err := RunAudit(AuditInput{SeaportalFile: "report.json"}, pages, RunOptions{Concurrency: 1}, nil, auditor)
	if err != nil {
		t.Fatalf("RunAudit: %v", err)
	}
	if !reflect.DeepEqual(audited, []string{"http://x/spa"}) {
		t.Errorf("audited = %v, want only the browserRecommended page", audited)
	}
	if report.Input.SeaportalFormat != SeaportalReportFormat {
		t.Errorf("SeaportalFormat = %q", report.Input.SeaportalFormat)
	}
	if len(report.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(report.Pages))
	}

	static := report.Pages[1]
	if static.URL != "http://x/static" || static.Title != "Static" || static.StatusCode != 200 {
		t.Errorf("static page = %+v", static)
	}
	if static.Seaportal["description"] != "d" {
		t.Errorf("static Seaportal = %+v", static.Seaportal)
	}
	if static.Browser.AccessibilityScore != 0 || static.Screenshot != "" {
		t.Errorf("static page should not be browser-enriched: %+v", static)
	}

	enriched := report.Pages[0]
	if enriched.Seaportal["title"] != "SPA" {
		t.Errorf("enriched page should carry seaportal summary: %+v", enriched)
	}

	audited = nil
	if _, err := RunAudit(AuditInput{}, pages, RunOptions{EnrichAll: true, Concurrency: 1}, nil, auditor); err != nil {
		t.Fatalf("RunAudit enrichAll: %v", err)
	}
	if len(audited) != 2 {
		t.Errorf("enrichAll audited = %v, want both pages", audited)
	}
}
