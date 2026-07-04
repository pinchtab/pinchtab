package audit

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefaultConcurrency is the number of pages audited in parallel when the
// caller does not choose one; MaxConcurrency caps what a caller may request
// (each worker drives its own browser tab).
const (
	DefaultConcurrency = 2
	MaxConcurrency     = 8
)

// RunOptions configures a multi-page audit run.
type RunOptions struct {
	// SampleSize caps how many pages each template group contributes
	// (see SamplePages), 0 = no cap.
	SampleSize int
	// Concurrency is the number of pages audited in parallel, clamped to
	// [1, MaxConcurrency].
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

// clampConcurrency normalizes a requested concurrency into [1, MaxConcurrency].
func clampConcurrency(n int) int {
	switch {
	case n < 1:
		return DefaultConcurrency
	case n > MaxConcurrency:
		return MaxConcurrency
	default:
		return n
	}
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

	concurrency := clampConcurrency(opts.Concurrency)
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
	return report, nil
}

// ToPageResult converts a single-page audit into the report page shape.
func (pa PageAudit) ToPageResult() PageResult {
	return PageResult{
		URL:        pa.URL,
		Title:      pa.Title,
		Error:      pa.Error,
		Screenshot: pa.Screenshot,
		Browser:    pa.BrowserPageData,
	}
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

// summaryScore is the mean accessibility score across browser-enriched
// pages that were audited without error, 0 when none were.
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
