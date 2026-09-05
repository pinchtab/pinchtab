package scrape

import (
	"context"
	"encoding/json"
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

// crawlWithoutSitemap is the shape every site without a usable sitemap
// produces: pages found by crawling, no sitemap total to report.
func crawlWithoutSitemap(pages ...seaportal.PageObject) Crawler {
	return func(context.Context) (*seaportal.ScrapeResult, error) {
		return &seaportal.ScrapeResult{
			Site:  seaportal.SiteInfo{BaseURL: "https://example.com", SitemapFound: false},
			Pages: pages,
		}, nil
	}
}

func TestRunWithoutSitemapNeverReportsATotalBelowTheSampledPages(t *testing.T) {
	crawl := crawlWithoutSitemap(
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
		seaportal.PageObject{URL: "https://example.com/docs", Status: 200, Markdown: longMarkdown},
	)

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{Preview: true}, crawl, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Site.TotalURLsInSitemap != 0 {
		t.Fatalf("TotalURLsInSitemap = %d, want 0 without a sitemap", report.Site.TotalURLsInSitemap)
	}
	summary := SampledPagesSummary(len(report.Pages), report.Site.TotalURLsInSitemap)
	if strings.Contains(summary, " of ") {
		t.Fatalf("summary %q claims a total the crawl never had", summary)
	}
	if !strings.Contains(summary, fmt.Sprintf("%d page(s) sampled", len(report.Pages))) {
		t.Fatalf("summary %q does not report the %d pages it lists", summary, len(report.Pages))
	}
	if md := string(RenderMarkdown(report)); !strings.Contains(md, summary) || strings.Contains(md, "of 0") {
		t.Fatalf("markdown report disagrees with the preview line: %q", firstLines(md, 4))
	}
}

func TestRunWithSitemapStillReportsTheSitemapTotal(t *testing.T) {
	pages := []seaportal.PageObject{
		{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
		{URL: "https://example.com/docs", Status: 200, Markdown: longMarkdown},
	}
	crawl := func(context.Context) (*seaportal.ScrapeResult, error) {
		return &seaportal.ScrapeResult{
			Site:  seaportal.SiteInfo{BaseURL: "https://example.com", SitemapFound: true, TotalURLsInSitemap: 42},
			Pages: pages,
		}, nil
	}

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{Preview: true}, crawl, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !report.Site.SitemapFound || report.Site.TotalURLsInSitemap != 42 {
		t.Fatalf("sitemap info lost: found=%v total=%d", report.Site.SitemapFound, report.Site.TotalURLsInSitemap)
	}
	summary := SampledPagesSummary(len(report.Pages), report.Site.TotalURLsInSitemap)
	if !strings.Contains(summary, "42 in sitemap") {
		t.Fatalf("summary %q drops the sitemap total", summary)
	}
	if md := string(RenderMarkdown(report)); !strings.Contains(md, summary) {
		t.Fatalf("markdown report disagrees with the preview line: %q", firstLines(md, 4))
	}
}

func TestURLListCrawlerLeavesTheSitemapTotalUnset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, renderedHTML)
	}))
	defer srv.Close()
	guard := CrawlGuard{ValidateURL: func(string) error { return nil }}
	crawl := URLListCrawler([]string{srv.URL + "/a", srv.URL + "/b"}, 5*time.Second, guard)

	res, err := crawl(context.Background())
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}
	if res.Site.TotalURLsInSitemap != 0 {
		t.Fatalf("TotalURLsInSitemap = %d, want 0: caller-supplied URLs never came from a sitemap", res.Site.TotalURLsInSitemap)
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// The upstream crawl's own recommendations, which PinchTab used to forward verbatim.
// They are written for a caller who has NOT yet run a browser, and PinchTab is a caller
// who just did.
const (
	inheritedThinRec    = "2 pages have little extractable text (possible SPA/JS-only); consider PinchTab enrichment"
	inheritedErrorsRec  = "1 of 4 pages returned errors or 4xx/5xx responses"
	inheritedSitemapRec = "sitemap lists 400 URLs across 12 patterns but only 4 were sampled; sampling recommended"
	inheritedCoverRec   = "only 4 of 400 sitemap URLs were sampled (1.0% coverage); raise --max-pages for broader coverage"
)

func crawlAdvising(recs []string, pages ...seaportal.PageObject) Crawler {
	return func(context.Context) (*seaportal.ScrapeResult, error) {
		return &seaportal.ScrapeResult{
			Site:    seaportal.SiteInfo{BaseURL: "https://example.com", TotalURLsInSitemap: len(pages)},
			Pages:   pages,
			Summary: seaportal.ScrapeSummary{Recommendations: recs},
			PageGroups: []seaportal.PageGroup{
				{Pattern: "/*", TotalInSitemap: len(pages), Sampled: len(pages), Pages: pages},
			},
		}, nil
	}
}

func recommendationNaming(t *testing.T, report Report, substr string) string {
	t.Helper()
	for _, rec := range report.Summary.Recommendations {
		if strings.Contains(rec, substr) {
			return rec
		}
	}
	return ""
}

// The discriminating fixture: a JS-only page that extracts to nothing over HTTP and to
// plenty after rendering, beside a page that is thin either way. The pre-render count is
// 2 and the post-render count is 1, so a recommendation carried over from the HTTP phase
// says 2 while the report's own per-page records say otherwise.
func TestThinRecommendationCountsPagesAsTheyFinallyStand(t *testing.T) {
	crawl := crawlAdvising([]string{inheritedThinRec},
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown, ContentType: "page"},
		seaportal.PageObject{URL: "https://example.com/about", Status: 200, Markdown: longMarkdown, ContentType: "page"},
		seaportal.PageObject{URL: "https://example.com/js", Status: 200, Markdown: "", ContentType: "page"},
		seaportal.PageObject{URL: "https://example.com/thin", Status: 200, Markdown: "hi", ContentType: "page"},
	)
	render := func(url string) (string, error) {
		if strings.HasSuffix(url, "/thin") {
			return `<html><body><article><p>hi</p></article></body></html>`, nil
		}
		return renderedHTML, nil
	}

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	js := report.Pages[2]
	if js.Source != SourceBrowser || utf8.RuneCountInString(js.Markdown) < ThinContentChars {
		t.Fatalf("the JS page was not filled by the browser, so this fixture cannot tell the two counts apart: source=%q chars=%d", js.Source, utf8.RuneCountInString(js.Markdown))
	}

	thinRec := recommendationNaming(t, report, "little extractable text")
	if thinRec == "" {
		t.Fatalf("the still-thin page produced no thin-content recommendation: %v", report.Summary.Recommendations)
	}
	if !strings.HasPrefix(thinRec, "1 pages") {
		t.Errorf("thin count is pre-render: %q — the browser filled one of the two pages the HTTP phase called thin", thinRec)
	}
	if strings.Contains(thinRec, "consider PinchTab enrichment") {
		t.Errorf("the report advises the enrichment it already performed: %q", thinRec)
	}
}

// --enrich-all renders every page by definition, so advising enrichment can never be
// actionable — a reader who follows it re-runs and gets the same sentence back.
func TestEnrichAllNeverAdvisesEnrichment(t *testing.T) {
	crawl := crawlAdvising([]string{inheritedThinRec},
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
		seaportal.PageObject{URL: "https://example.com/thin", Status: 200, Markdown: "hi"},
	)
	render := func(url string) (string, error) {
		if strings.HasSuffix(url, "/thin") {
			return `<html><body><article><p>hi</p></article></body></html>`, nil
		}
		return renderedHTML, nil
	}

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{EnrichAll: true}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Summary.BrowserPages == 0 {
		t.Fatal("no page was browser-rendered, so --enrich-all is not being exercised")
	}
	for _, rec := range report.Summary.Recommendations {
		if strings.Contains(rec, "consider PinchTab enrichment") {
			t.Errorf("--enrich-all still advises enrichment: %q", rec)
		}
	}
	if recommendationNaming(t, report, "little extractable text") == "" {
		t.Error("the page that is thin even after rendering stopped being reported; that half is still worth saying")
	}
}

// A page that is thin and was never offered to the browser keeps the advice, because
// there the remedy has not been applied yet.
func TestThinPagesTheBrowserNeverSawKeepTheEnrichmentAdvice(t *testing.T) {
	crawl := crawlAdvising([]string{inheritedThinRec},
		seaportal.PageObject{URL: "https://example.com/thin", Status: 200, Markdown: "hi"},
	)

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{NoBrowser: true}, crawl, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	thinRec := recommendationNaming(t, report, "little extractable text")
	if !strings.Contains(thinRec, "consider PinchTab enrichment") {
		t.Errorf("with no browser phase the advice is still actionable and must survive: %q", thinRec)
	}
}

// The second live instance of the same defect: the browser can recover a page that failed
// over HTTP, and summarize already counts FailedPages from the final pages — so the
// inherited errors line contradicts the report's own number.
func TestErrorsRecommendationCountsPagesAsTheyFinallyStand(t *testing.T) {
	crawl := crawlAdvising([]string{inheritedErrorsRec},
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
		seaportal.PageObject{URL: "https://example.com/flaky", Status: 200, Error: "tls handshake failure"},
	)
	render := func(string) (string, error) { return renderedHTML, nil }

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	recovered := report.Pages[1]
	if recovered.Error != "" || recovered.Source != SourceBrowser {
		t.Fatalf("the browser did not recover the failed page, so this fixture proves nothing: error=%q source=%q", recovered.Error, recovered.Source)
	}
	if report.Summary.FailedPages != 0 {
		t.Fatalf("FailedPages = %d, want 0 after recovery", report.Summary.FailedPages)
	}
	if rec := recommendationNaming(t, report, "returned errors"); rec != "" {
		t.Errorf("the report says %q while its own failedPages is 0", rec)
	}
}

func TestAGenuinelyFailedPageIsStillReported(t *testing.T) {
	crawl := crawlAdvising(nil,
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
		seaportal.PageObject{URL: "https://example.com/dead", Status: 200, Error: "tls handshake failure"},
	)
	render := func(string) (string, error) { return "", errors.New("tab crashed") }

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	rec := recommendationNaming(t, report, "returned errors")
	if rec == "" {
		t.Fatalf("a page that failed and stayed failed produced no recommendation: %v", report.Summary.Recommendations)
	}
	if !strings.HasPrefix(rec, "1 of 2 pages") {
		t.Errorf("the errors line does not match the report's own failedPages=%d: %q", report.Summary.FailedPages, rec)
	}
}

// Crawl-scope advice describes which URLs were SAMPLED, which no amount of browser
// rendering changes, so it must survive verbatim. Dropping advice that is still true is
// the failure mode on the other side of this fix.
func TestSitemapCoverageAdviceIsForwardedUnchanged(t *testing.T) {
	crawl := crawlAdvising([]string{inheritedSitemapRec, inheritedCoverRec},
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
	)

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{NoBrowser: true}, crawl, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, want := range []string{inheritedSitemapRec, inheritedCoverRec} {
		found := false
		for _, rec := range report.Summary.Recommendations {
			if rec == want {
				found = true
			}
		}
		if !found {
			t.Errorf("crawl-scope advice was dropped: %q\n got %v", want, report.Summary.Recommendations)
		}
	}
}

// A clean run says nothing rather than saying nothing under a heading: an empty
// "## Recommendations" section reads as a report with no conclusions.
func TestACleanRunOmitsTheRecommendationsBlockEntirely(t *testing.T) {
	crawl := crawlAdvising([]string{inheritedThinRec, inheritedErrorsRec},
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
		seaportal.PageObject{URL: "https://example.com/js", Status: 200, Markdown: ""},
	)
	render := func(string) (string, error) { return renderedHTML, nil }

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{EnrichAll: true}, crawl, render)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(report.Summary.Recommendations) != 0 {
		t.Fatalf("a run with nothing left to advise still carries %v", report.Summary.Recommendations)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "recommendations") {
		t.Error("the JSON report carries an empty recommendations key; omitempty needs a nil slice, not an allocated one")
	}
	if strings.Contains(string(RenderMarkdown(report)), "## Recommendations") {
		t.Error("the markdown report renders a Recommendations heading with nothing under it")
	}
}

// The threshold is PinchTab's own. seaportal applies the same number at the pinned
// version, but its constant is unexported and inside another module's internal/ tree, so
// nothing here can read it — this test is the pin that makes a future divergence a
// decision someone takes rather than one that happens.
func TestThinContentThresholdIsPinned(t *testing.T) {
	if ThinContentChars != 160 {
		t.Errorf("ThinContentChars = %d, want 160 — the value matches seaportal's own thin-content threshold at the pinned version. Changing it is allowed, but say why in the constant's comment, because the two notions of thin then differ", ThinContentChars)
	}
}

// Preview withholds every page body, so a thin count read from Markdown would call the
// whole site thin.
func TestPreviewCountsWithheldBodiesByTheirRecordedLength(t *testing.T) {
	crawl := crawlAdvising(nil,
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
		seaportal.PageObject{URL: "https://example.com/thin", Status: 200, Markdown: "hi"},
	)

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{Preview: true}, crawl, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Pages[0].Markdown != "" || report.Pages[0].CharCount == 0 {
		t.Fatalf("preview did not withhold the body, so this fixture proves nothing: %+v", report.Pages[0])
	}
	thinRec := recommendationNaming(t, report, "little extractable text")
	if !strings.HasPrefix(thinRec, "1 pages") {
		t.Errorf("preview counted withheld bodies as thin: %q", thinRec)
	}
}

func TestAnUnrecognisedInheritedRecommendationIsForwardedNotDropped(t *testing.T) {
	unknown := "robots.txt disallows 3 of the discovered paths; consider --ignore-robots"
	crawl := crawlAdvising([]string{unknown},
		seaportal.PageObject{URL: "https://example.com/", Status: 200, Markdown: longMarkdown},
	)

	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{NoBrowser: true}, crawl, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	found := false
	for _, rec := range report.Summary.Recommendations {
		if rec == unknown {
			found = true
		}
	}
	if !found {
		t.Errorf("an inherited recommendation matching neither the regenerated phrases nor the sitemap lines was silently swallowed; the recorded rule says unrecognised advice FORWARDS, since dropping advice that is still true is the worse failure\n got %v", report.Summary.Recommendations)
	}
}

// A reachable server answering 404 is a page that failed, and the one the
// transport-error fixtures cannot see: it has a status code, an error body, and no
// error. The summary must count it as failed, keep it out of the content taxonomy,
// and make the errors line fire.
func TestA404WithABodyIsAFailedPageNotASuccessfulHTTPPage(t *testing.T) {
	crawl := fakeCrawl(
		seaportal.PageObject{URL: "https://example.com/", Title: "Home", Status: 200, Markdown: longMarkdown, ContentType: "page"},
		seaportal.PageObject{URL: "https://example.com/missing", Title: "Error response", Status: 404, Markdown: "Error code: 404\n\nMessage: File not found.", ContentType: "page"},
		seaportal.PageObject{URL: "https://example.com/moved", Title: "Moved", Status: 301, Markdown: longMarkdown, ContentType: "page"},
		seaportal.PageObject{URL: "https://example.com/unknown", Title: "No status", Markdown: longMarkdown, ContentType: "page"},
	)
	report, err := Run(context.Background(), Input{URL: "https://example.com"}, RunOptions{NoBrowser: true}, crawl, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := report.Summary
	if s.FailedPages != 1 || s.HTTPPages != 3 || s.BrowserPages != 0 {
		t.Fatalf("summary = %+v, want the 404 alone failed and the 200, 3xx and status-less pages http", s)
	}
	if s.FailedPages+s.HTTPPages+s.BrowserPages != len(report.Pages) {
		t.Fatalf("summary %+v does not partition %d pages", s, len(report.Pages))
	}
	if s.ContentTypes["page"] != 3 {
		t.Fatalf("contentTypes = %v; the 404's error body is counted as an ordinary page", s.ContentTypes)
	}
	if missing := report.Pages[1]; missing.Markdown == "" || missing.StatusCode != 404 {
		t.Fatalf("the failed page lost its own body or status: %+v", missing)
	}
	rec := recommendationNaming(t, report, "returned errors")
	if !strings.HasPrefix(rec, "1 of 4 pages") {
		t.Fatalf("the 4xx/5xx recommendation did not fire for a 404: %v", s.Recommendations)
	}
	if thin := recommendationNaming(t, report, "little extractable text"); thin != "" {
		t.Fatalf("the failed page was also reported thin: %q", thin)
	}
}
