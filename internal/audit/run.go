package audit

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/scrape"
)

// RunOptions configures a multi-page audit run.
type RunOptions struct {
	// SampleSize caps how many pages each template group contributes
	// (see SamplePages), 0 = no cap.
	SampleSize int
	// Concurrency is the number of pages audited in parallel, clamped to
	// [1, scrape.MaxConcurrency].
	Concurrency int
	// EnrichAll browser-enriches every seaportal page, overriding the
	// per-page BrowserRecommended routing.
	EnrichAll bool
	// Page selects the collectors for every enriched page.
	Page PageOptions
}

// SitemapFetcher discovers page URLs from a sitemap URL. Isolated behind a
// function type so a richer discovery (e.g. seaportal.FlattenSitemap) can be
// swapped in.
type SitemapFetcher func(sitemapURL string) ([]string, error)

// PageAuditor audits a single URL. Failures must come back as a PageAudit
// with the Error field set, never as a panic.
type PageAuditor func(url string, opts PageOptions) PageAudit

// PlanURLs dedupes the URL list preserving first-occurrence order, so the
// entry URL stays first. Empty strings are dropped.
func PlanURLs(urls []string) []string {
	seen := make(map[string]bool, len(urls))
	plan := make([]string, 0, len(urls))
	for _, u := range urls {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		plan = append(plan, u)
	}
	return plan
}

// pagePlan is one planned page of a run: where to go, whether the browser
// enriches it, and the seaportal metadata to merge into its report entry.
type pagePlan struct {
	url    string
	enrich bool
	sp     *SeaportalPage
}

// planRun resolves the input into the ordered page plan. Seaportal pages
// route through BrowserRecommended (overridden by EnrichAll); URL-list and
// sitemap inputs enrich everything.
func planRun(input *AuditInput, seaportalPages []SeaportalPage, opts RunOptions, discover SitemapFetcher) ([]pagePlan, error) {
	if len(seaportalPages) > 0 {
		input.SeaportalFormat = SeaportalReportFormat
		meta := make(map[string]*SeaportalPage, len(seaportalPages))
		urls := make([]string, 0, len(seaportalPages))
		for i := range seaportalPages {
			sp := &seaportalPages[i]
			if meta[sp.URL] == nil {
				meta[sp.URL] = sp
				urls = append(urls, sp.URL)
			}
		}
		plan := SamplePages(urls, opts.SampleSize, nil)
		plans := make([]pagePlan, len(plan))
		for i, u := range plan {
			sp := meta[u]
			plans[i] = pagePlan{url: u, enrich: sp.BrowserRecommended || opts.EnrichAll, sp: sp}
		}
		return plans, nil
	}

	urls := input.URLs
	if len(urls) == 0 && input.SitemapURL != "" {
		if discover == nil {
			return nil, errors.New("sitemap input requires a sitemap fetcher")
		}
		discovered, err := discover(input.SitemapURL)
		if err != nil {
			return nil, fmt.Errorf("sitemap discovery: %w", err)
		}
		urls = discovered
	}

	plan := SamplePages(PlanURLs(urls), opts.SampleSize, nil)
	plans := make([]pagePlan, len(plan))
	for i, u := range plan {
		plans[i] = pagePlan{url: u, enrich: true}
	}
	return plans, nil
}

// RunAudit audits every planned page and assembles the versioned
// AuditReport. The entry page (first in the plan) is always processed first,
// synchronously; the rest run with bounded concurrency. Page failures are
// report data, not errors — RunAudit only errors when there is nothing to
// audit.
func RunAudit(input AuditInput, seaportalPages []SeaportalPage, opts RunOptions, discover SitemapFetcher, auditPage PageAuditor) (AuditReport, error) {
	plans, err := planRun(&input, seaportalPages, opts, discover)
	if err != nil {
		return AuditReport{}, err
	}
	if len(plans) == 0 {
		return AuditReport{}, errors.New("no URLs to audit")
	}

	concurrency := scrape.ClampConcurrency(opts.Concurrency)
	results := make([]PageResult, len(plans))
	exec := func(i int) {
		p := plans[i]
		if !p.enrich {
			results[i] = seaportalOnlyResult(p)
			return
		}
		results[i] = mergeSeaportal(auditPage(p.url, opts.Page).ToPageResult(), p.sp)
	}

	exec(0)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := 1; i < len(plans); i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			exec(i)
		}(i)
	}
	wg.Wait()

	report := NewAuditReport()
	report.GeneratedAt = time.Now().UTC()
	report.Input = input
	report.Options = AuditOptions{
		SampleSize:     opts.SampleSize,
		Screenshot:     opts.Page.Screenshot,
		NetworkMonitor: opts.Page.Network,
		Concurrency:    concurrency,
	}
	report.Pages = results
	report.SummaryScore = summaryScore(plans, results)
	for _, pr := range results {
		report.SecurityFindings = append(report.SecurityFindings, pr.SecurityFindings...)
	}
	return report, nil
}

// ToPageResult converts a single-page audit into the report page shape.
func (pa PageAudit) ToPageResult() PageResult {
	pr := PageResult{
		URL:              pa.URL,
		Title:            pa.Title,
		Error:            pa.Error,
		Screenshot:       pa.Screenshot,
		SecurityFindings: pa.SecurityFindings,
		Browser:          pa.BrowserPageData,
	}
	// The page's own HTTP status is a first-class field, read from the main
	// document's network request. When that document failed, it IS the page —
	// not a sub-resource — so it is removed from BrokenAssets, which counts only
	// failed sub-resources (a 404 image, script, or nested iframe still counts).
	if status, docURL, ok := mainDocumentStatus(pr.Browser, pr.URL); ok {
		pr.StatusCode = status
		pr.Browser.BrokenAssets = withoutAsset(pr.Browser.BrokenAssets, "document", docURL)
	}
	return pr
}

// mainDocumentStatus reports the HTTP status and URL of the page's own document,
// read from the observed network requests: the request for the audited URL
// itself, or failing that the first document request (the main-frame navigation,
// for the redirect case where no request carries the exact audited URL). ok is
// false when the network collector did not run, so StatusCode stays unset rather
// than a misleading zero.
func mainDocumentStatus(browser BrowserPageData, pageURL string) (status int, url string, ok bool) {
	var first *NetworkRequest
	for i := range browser.NetworkRequests {
		req := &browser.NetworkRequests[i]
		if !strings.EqualFold(req.ResourceType, "document") {
			continue
		}
		if first == nil {
			first = req
		}
		if req.URL == pageURL {
			return req.Status, req.URL, true
		}
	}
	if first != nil {
		return first.Status, first.URL, true
	}
	return 0, "", false
}

// withoutAsset returns the broken assets with the one that is the main document
// itself removed (matched on the lowercased resourceType and the document URL),
// so the page failure is not double-counted as a broken sub-resource. A
// same-typed asset at another URL — a 404 iframe document — is kept.
func withoutAsset(assets []BrokenAsset, resourceType, url string) []BrokenAsset {
	out := make([]BrokenAsset, 0, len(assets))
	for _, a := range assets {
		if strings.EqualFold(a.ResourceType, resourceType) && a.URL == url {
			continue
		}
		out = append(out, a)
	}
	return out
}

// seaportalOnlyResult is the report entry for a page seaportal marked as not
// needing the browser: its HTTP-extraction summary without browser data.
func seaportalOnlyResult(p pagePlan) PageResult {
	return PageResult{
		URL:        p.url,
		Title:      p.sp.Title,
		StatusCode: p.sp.StatusCode,
		Seaportal:  p.sp.Summary,
	}
}

// mergeSeaportal embeds the seaportal summary into an enriched page entry,
// filling title/status where the browser did not provide them.
func mergeSeaportal(pr PageResult, sp *SeaportalPage) PageResult {
	if sp == nil {
		return pr
	}
	pr.Seaportal = sp.Summary
	if pr.Title == "" {
		pr.Title = sp.Title
	}
	if pr.StatusCode == 0 {
		pr.StatusCode = sp.StatusCode
	}
	return pr
}

// PageStatus is the one-line health verdict every report surface prints for
// a page: the audit failure when it could not be collected, otherwise the page's
// own 4xx/5xx HTTP status, otherwise the uncaught-exception count, otherwise ok.
// A page whose own document is an error response is a failed page, not an "ok"
// page with a broken asset. An uncaught exception halts the script that raised
// it, so a page carrying one is never ok either. (3xx and an unset status are
// not failures.)
func PageStatus(p PageResult) string {
	switch {
	case p.Error != "":
		return "error: " + p.Error
	case p.StatusCode >= 400:
		return fmt.Sprintf("HTTP %d", p.StatusCode)
	case len(p.Browser.JSErrors) > 0:
		return fmt.Sprintf("%d uncaught JS error(s)", len(p.Browser.JSErrors))
	default:
		return "ok"
	}
}

// summaryScore is the mean accessibility score of enriched pages that were
// audited without error, 0 when none were. It deliberately ignores broken
// assets, failed requests and uncaught JS errors; see AuditReport.SummaryScore.
func summaryScore(plans []pagePlan, results []PageResult) int {
	sum, n := 0, 0
	for i, pr := range results {
		if !plans[i].enrich || pr.Error != "" {
			continue
		}
		sum += pr.Browser.AccessibilityScore
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / n
}
