package audit

import (
	"encoding/xml"
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
	// Page selects the collectors for every page.
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

type sitemapURLSet struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

// ParseSitemap extracts the <loc> URLs from a sitemap.xml document.
func ParseSitemap(data []byte) ([]string, error) {
	var set sitemapURLSet
	if err := xml.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("parse sitemap: %w", err)
	}
	urls := make([]string, 0, len(set.URLs))
	for _, u := range set.URLs {
		if u.Loc != "" {
			urls = append(urls, u.Loc)
		}
	}
	return urls, nil
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

// RunAudit audits every planned URL and assembles the versioned AuditReport.
// The entry URL (first in the plan) is always audited first, synchronously;
// the rest run with bounded concurrency. Page failures are report data, not
// errors — RunAudit only errors when there is nothing to audit.
func RunAudit(input AuditInput, opts RunOptions, discover SitemapFetcher, auditPage PageAuditor) (AuditReport, error) {
	urls := input.URLs
	if len(urls) == 0 && input.SitemapURL != "" {
		if discover == nil {
			return AuditReport{}, errors.New("sitemap input requires a sitemap fetcher")
		}
		discovered, err := discover(input.SitemapURL)
		if err != nil {
			return AuditReport{}, fmt.Errorf("sitemap discovery: %w", err)
		}
		urls = discovered
	}

	plan := SamplePages(PlanURLs(urls), opts.SampleSize, nil)
	if len(plan) == 0 {
		return AuditReport{}, errors.New("no URLs to audit")
	}

	concurrency := clampConcurrency(opts.Concurrency)
	results := make([]PageAudit, len(plan))
	results[0] = auditPage(plan[0], opts.Page)

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := 1; i < len(plan); i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = auditPage(plan[i], opts.Page)
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
	report.Pages = make([]PageResult, len(results))
	for i, pa := range results {
		report.Pages[i] = pa.ToPageResult()
	}
	report.SummaryScore = summaryScore(results)
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

// summaryScore is the mean accessibility score across pages that were
// audited without error, 0 when none were.
func summaryScore(results []PageAudit) int {
	sum, n := 0, 0
	for _, pa := range results {
		if pa.Error != "" {
			continue
		}
		sum += pa.AccessibilityScore
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / n
}
