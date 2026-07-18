package scrape

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	seaportal "github.com/pinchtab/seaportal"
)

var longMarkdown = strings.Repeat("Plenty of extracted article prose. ", 30)

// renderedHTML is rich enough for seaportal's readability extraction to
// produce non-empty markdown in enrichPage.
const renderedHTML = `<html><head><title>Rendered Title</title></head><body><article>
<h1>Rendered Title</h1>
<p>This paragraph only exists after JavaScript rendered the application shell into real content.</p>
<p>It repeats enough prose that the readability extractor keeps it as the main article body of the page.</p>
<p>Browser rendering recovered this content where the plain HTTP fetch saw an empty root element only.</p>
</article></body></html>`

func fakeCrawl(pages ...seaportal.PageObject) Crawler {
	return func(context.Context) (*seaportal.ScrapeResult, error) {
		return &seaportal.ScrapeResult{
			Site:  seaportal.SiteInfo{BaseURL: "https://example.com", TotalURLsInSitemap: len(pages)},
			Pages: pages,
			PageGroups: []seaportal.PageGroup{
				{Pattern: "/*", TotalInSitemap: len(pages), Sampled: len(pages), Pages: pages},
			},
		}, nil
	}
}

func TestNeedsBrowser(t *testing.T) {
	tests := []struct {
		name   string
		page   Page
		want   bool
		reason string
	}{
		{"rich static page", Page{StatusCode: 200, Markdown: longMarkdown}, false, ""},
		{"thin spa shell", Page{StatusCode: 200, Markdown: "Loading…"}, true, "thin-content"},
		{"fetch error", Page{Error: "tls handshake failure", Markdown: longMarkdown}, true, "fetch-error"},
		{"blocked status", Page{StatusCode: 403, Markdown: longMarkdown}, true, "blocked-status:403"},
		{"not found never routes", Page{StatusCode: 404, Markdown: ""}, false, ""},
		{"gone never routes", Page{StatusCode: 410, Error: "x"}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reasons := NeedsBrowser(tt.page)
			if got != tt.want {
				t.Fatalf("NeedsBrowser() = %v, want %v (reasons %v)", got, tt.want, reasons)
			}
			if tt.reason != "" && (len(reasons) == 0 || reasons[0] != tt.reason) {
				t.Fatalf("reasons = %v, want first %q", reasons, tt.reason)
			}
		})
	}
}

func TestRunRoutesThinPagesThroughBrowser(t *testing.T) {
	crawl := fakeCrawl(
		seaportal.PageObject{URL: "https://example.com/", Title: "Home", Status: 200, Markdown: longMarkdown, ContentType: "page"},
		seaportal.PageObject{URL: "https://example.com/app", Title: "App", Status: 200, Markdown: "Loading…", ContentType: "page"},
		seaportal.PageObject{URL: "https://example.com/missing", Status: 404, Error: "not found"},
	)
	var rendered []string
	render := func(url string) (string, error) {
		rendered = append(rendered, url)
		return renderedHTML, nil
	}

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(rendered) != 1 || rendered[0] != "https://example.com/app" {
		t.Fatalf("browser rendered %v, want only the thin page", rendered)
	}

	home, app, missing := report.Pages[0], report.Pages[1], report.Pages[2]
	if home.Source != SourceHTTP || home.Markdown != longMarkdown {
		t.Errorf("rich page should keep http content, got source=%q", home.Source)
	}
	if app.Source != SourceBrowser {
		t.Errorf("thin page source = %q, want browser (browserError=%q)", app.Source, app.BrowserError)
	}
	if !strings.Contains(app.Markdown, "JavaScript rendered") {
		t.Errorf("thin page markdown not replaced by rendered extraction: %q", app.Markdown)
	}
	if app.Title != "Rendered Title" {
		t.Errorf("thin page title = %q, want rendered title", app.Title)
	}
	if !app.BrowserRecommended || len(app.BrowserReasons) == 0 {
		t.Errorf("thin page should record routing verdict, got %v %v", app.BrowserRecommended, app.BrowserReasons)
	}
	if missing.Source != SourceHTTP || missing.Error == "" {
		t.Errorf("404 page must stay http-sourced with its error, got source=%q error=%q", missing.Source, missing.Error)
	}

	if report.Summary.HTTPPages != 1 || report.Summary.BrowserPages != 1 || report.Summary.FailedPages != 1 {
		t.Errorf("summary = %+v, want 1 http / 1 browser / 1 failed", report.Summary)
	}
	if report.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion = %q", report.SchemaVersion)
	}
	if len(report.PageGroups) != 1 || len(report.PageGroups[0].URLs) != 3 {
		t.Errorf("page tree not preserved: %+v", report.PageGroups)
	}
}

func TestRunBrowserFailureKeepsHTTPContent(t *testing.T) {
	crawl := fakeCrawl(seaportal.PageObject{URL: "https://example.com/app", Status: 200, Markdown: "Loading…"})
	render := func(string) (string, error) { return "", errors.New("tab crashed") }

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	p := report.Pages[0]
	if p.Source != SourceHTTP || p.Markdown != "Loading…" {
		t.Errorf("failed enrichment must keep http content, got source=%q markdown=%q", p.Source, p.Markdown)
	}
	if p.BrowserError != "tab crashed" {
		t.Errorf("browserError = %q", p.BrowserError)
	}
}

func TestRunBrowserSuccessClearsFetchError(t *testing.T) {
	crawl := fakeCrawl(seaportal.PageObject{URL: "https://example.com/blocked", Status: 403, Error: "403 Forbidden"})
	render := func(string) (string, error) { return renderedHTML, nil }

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	p := report.Pages[0]
	if p.Error != "" || p.Source != SourceBrowser {
		t.Errorf("browser recovery should clear the fetch error, got error=%q source=%q", p.Error, p.Source)
	}
	if report.Summary.FailedPages != 0 || report.Summary.BrowserPages != 1 {
		t.Errorf("summary = %+v", report.Summary)
	}
}

func TestRunNoBrowserSkipsEnrichmentButRecordsVerdict(t *testing.T) {
	crawl := fakeCrawl(seaportal.PageObject{URL: "https://example.com/app", Status: 200, Markdown: "Loading…"})
	render := func(string) (string, error) {
		t.Fatal("render must not be called with NoBrowser")
		return "", nil
	}

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{NoBrowser: true}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	p := report.Pages[0]
	if p.Source != SourceHTTP {
		t.Errorf("source = %q", p.Source)
	}
	if !p.BrowserRecommended {
		t.Error("routing verdict should still be recorded")
	}
}

func TestRunEnrichAllRendersRichPagesToo(t *testing.T) {
	crawl := fakeCrawl(
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
		seaportal.PageObject{URL: "https://example.com/missing", Status: 404},
	)
	var rendered []string
	render := func(url string) (string, error) {
		rendered = append(rendered, url)
		return renderedHTML, nil
	}

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{EnrichAll: true}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rendered) != 1 || rendered[0] != "https://example.com/" {
		t.Fatalf("enrich-all rendered %v, want the rich page but never the 404", rendered)
	}
	p := report.Pages[0]
	if p.BrowserRecommended {
		t.Error("enrich-all must not fake the routing verdict")
	}
	if len(p.BrowserReasons) != 1 || p.BrowserReasons[0] != "enrich-all" {
		t.Errorf("reasons = %v", p.BrowserReasons)
	}
}

func TestRunPreviewWithholdsBodiesAndSkipsBrowser(t *testing.T) {
	crawl := fakeCrawl(
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown, ContentType: "page"},
		seaportal.PageObject{URL: "https://example.com/app", Status: 200, Markdown: "Loading…"},
	)
	render := func(string) (string, error) {
		t.Fatal("preview must not render in the browser")
		return "", nil
	}

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{Preview: true}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	home := report.Pages[0]
	if home.Markdown != "" {
		t.Errorf("preview must withhold the body, got %q", home.Markdown)
	}
	if home.CharCount != utf8.RuneCountInString(longMarkdown) {
		t.Errorf("charCount = %d, want %d", home.CharCount, utf8.RuneCountInString(longMarkdown))
	}
	if !strings.HasPrefix(home.Snippet, "Plenty of extracted") {
		t.Errorf("snippet = %q", home.Snippet)
	}
	if app := report.Pages[1]; !app.BrowserRecommended || len(app.BrowserReasons) == 0 {
		t.Error("preview should still record the routing verdict per page")
	}
	if report.Summary.BrowserPages != 0 {
		t.Errorf("preview renders nothing, browserPages = %d", report.Summary.BrowserPages)
	}
}

func TestURLListCrawlerFetchesExplicitURLsSecurely(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, renderedHTML)
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var vetted []string
	guard := CrawlGuard{ValidateURL: func(u string) error { vetted = append(vetted, u); return nil }}
	// Duplicate /a proves dedupe; --no-browser keeps the test hermetic.
	crawl := URLListCrawler([]string{srv.URL + "/a", srv.URL + "/missing", srv.URL + "/a"}, 5*time.Second, guard)

	report, err := Run(context.Background(), Input{URL: srv.URL}, RunOptions{NoBrowser: true}, crawl, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Pages) != 2 {
		t.Fatalf("got %d pages, want 2 after dedupe", len(report.Pages))
	}
	if len(vetted) == 0 {
		t.Error("expand fetches must pass through the security URLFilter")
	}

	a := report.Pages[0]
	if a.Title != "Rendered Title" || !strings.Contains(a.Markdown, "JavaScript rendered") {
		t.Errorf("page /a not extracted: title=%q md=%q", a.Title, a.Markdown)
	}
	if missing := report.Pages[1]; missing.StatusCode != http.StatusNotFound && missing.Error == "" {
		t.Errorf("missing page should carry a 404 or error, got status=%d error=%q", missing.StatusCode, missing.Error)
	}
}

func TestURLListCrawlerEmptyErrors(t *testing.T) {
	crawl := URLListCrawler([]string{"  ", ""}, 0, CrawlGuard{})
	if _, err := crawl(context.Background()); err == nil {
		t.Error("an empty expand list must error")
	}
}

func TestSnippet(t *testing.T) {
	if got := snippet("  hello\n\n  world  ", 100); got != "hello world" {
		t.Errorf("whitespace collapse = %q, want %q", got, "hello world")
	}
	got := snippet(strings.Repeat("a", 300), 10)
	if utf8.RuneCountInString(got) != 11 || !strings.HasSuffix(got, "…") {
		t.Errorf("truncation = %q (len %d), want 10 chars + ellipsis", got, utf8.RuneCountInString(got))
	}
}

func TestRunErrors(t *testing.T) {
	failCrawl := func(context.Context) (*seaportal.ScrapeResult, error) { return nil, errors.New("dns failure") }
	if _, err := Run(context.Background(), Input{}, RunOptions{}, failCrawl, nil); err == nil || !strings.Contains(err.Error(), "site crawl") {
		t.Errorf("crawl failure must surface, got %v", err)
	}
	if _, err := Run(context.Background(), Input{}, RunOptions{}, fakeCrawl(), nil); err == nil || !strings.Contains(err.Error(), "no pages") {
		t.Errorf("empty crawl must error, got %v", err)
	}
}

func TestCrawlGuardPolicy(t *testing.T) {
	var checked []string
	guard := CrawlGuard{
		ValidateURL: func(url string) error {
			checked = append(checked, url)
			if strings.Contains(url, "blocked") {
				return errors.New("blocked by navguard")
			}
			return nil
		},
		TrustedResolveCIDRs: []string{"172.16.0.0/12"},
		MaxRedirects:        4,
	}
	p := guard.Policy()

	if p.BlockPrivateIPs {
		t.Error("private-IP enforcement must be delegated to ValidateURL")
	}
	if p.MaxRedirects != 4 || !p.RevalidateRedirects {
		t.Errorf("redirect policy = %d/%v, want 4/true", p.MaxRedirects, p.RevalidateRedirects)
	}
	if len(p.TrustedResolveCIDRs) != 1 {
		t.Errorf("trusted CIDRs not threaded: %v", p.TrustedResolveCIDRs)
	}
	if p.MaxResponseBytes == 0 || len(p.AllowedSchemes) == 0 {
		t.Error("secure defaults (size caps, scheme allowlist) must be kept")
	}

	if err := p.ValidateURL(context.Background(), "https://ok.example.com/"); err != nil {
		t.Errorf("allowed URL rejected: %v", err)
	}
	if err := p.ValidateURL(context.Background(), "https://blocked.example.com/"); err == nil {
		t.Error("navguard veto must propagate through the policy")
	}
	if len(checked) != 2 {
		t.Errorf("ValidateURL saw %v, want both URLs", checked)
	}
}

func TestCrawlGuardPolicyWithoutValidatorFailsClosed(t *testing.T) {
	p := CrawlGuard{}.Policy()
	if !p.BlockPrivateIPs {
		t.Error("without a ValidateURL hook the policy must keep BlockPrivateIPs")
	}
	if p.MaxRedirects <= 0 {
		t.Errorf("MaxRedirects = %d, a crawl must never be unlimited", p.MaxRedirects)
	}
}

func TestRunConcurrencyIsBounded(t *testing.T) {
	var pages []seaportal.PageObject
	for i := 0; i < 20; i++ {
		pages = append(pages, seaportal.PageObject{URL: fmt.Sprintf("https://example.com/p%d", i), Status: 200, Markdown: "x"})
	}
	active := make(chan struct{}, MaxConcurrency) // buffered at the cap: overflow panics via select default
	render := func(string) (string, error) {
		select {
		case active <- struct{}{}:
		default:
			t.Error("more concurrent renders than MaxConcurrency")
		}
		defer func() { <-active }()
		return renderedHTML, nil
	}
	if _, err := Run(context.Background(), Input{}, RunOptions{Concurrency: 99}, fakeCrawl(pages...), render); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
