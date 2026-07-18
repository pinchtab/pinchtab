package scrape

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	seaportal "github.com/pinchtab/seaportal"
)

// DefaultConcurrency is the number of pages browser-rendered in parallel
// when the caller does not choose one; MaxConcurrency caps what a caller may
// request (each worker drives its own browser tab).
const (
	DefaultConcurrency = 2
	MaxConcurrency     = 8
)

// MinContentChars mirrors seaportal's static-ok confidence threshold: an
// HTTP extraction shorter than this is treated as a probable JS shell and
// the page is routed to the browser.
const MinContentChars = 500

// Crawler produces the seaportal site crawl for Run. Isolated behind a
// function type so tests can fake the crawl and handlers can bake in
// timeouts without Run knowing about seaportal options.
type Crawler func(ctx context.Context) (*seaportal.ScrapeResult, error)

// BrowserRenderer returns the rendered HTML for url from a real browser
// tab. Failures come back as errors and are recorded per page, never
// aborting the run.
type BrowserRenderer func(url string) (string, error)

// RunOptions configures a scrape run.
type RunOptions struct {
	// Concurrency is the number of pages browser-rendered in parallel,
	// clamped to [1, MaxConcurrency].
	Concurrency int
	// EnrichAll browser-renders every reachable page, overriding routing.
	EnrichAll bool
	// NoBrowser skips browser enrichment entirely (HTTP crawl only);
	// routing verdicts are still recorded on each page.
	NoBrowser bool
	// Preview produces an outline: the HTTP crawl and routing verdicts, but
	// no browser enrichment and no full page bodies. Each page's Markdown is
	// withheld and replaced by CharCount plus a leading Snippet, so a caller
	// can survey a large site cheaply and then expand chosen pages (Input.URL
	// list) at full fidelity.
	Preview bool
}

// SnippetChars is how many characters of a page's content the preview keeps
// as a stand-in for the withheld body.
const SnippetChars = 240

// CrawlGuard applies pinchtab's navigation security stack to the HTTP crawl
// so seaportal's fetches (robots.txt, sitemaps, discovered links, pages) go
// through the same URL vetting as browser navigation.
type CrawlGuard struct {
	// ValidateURL is pinchtab's per-URL gate (navguard resolution checks +
	// IDPI domain rules). It runs before every crawl fetch and on every
	// redirect hop, and must be safe for concurrent use.
	ValidateURL func(url string) error
	// TrustedResolveCIDRs mirrors the runtime config escape hatch for hosts
	// that legitimately resolve to non-public addresses.
	TrustedResolveCIDRs []string
	// MaxRedirects caps redirect hops when > 0; otherwise seaportal's
	// default cap applies (a crawl never inherits "unlimited").
	MaxRedirects int
}

// Policy renders the guard as a seaportal SecurityPolicy: the secure
// defaults (scheme allowlist, redirect cap + per-hop revalidation, body and
// decompression caps) with private-IP enforcement delegated to ValidateURL,
// which owns pinchtab's richer semantics (IDPI-allowed internal domains,
// trusted CIDRs).
func (g CrawlGuard) Policy() *seaportal.SecurityPolicy {
	p := seaportal.DefaultSecurityPolicy()
	p.TrustedResolveCIDRs = g.TrustedResolveCIDRs
	if g.MaxRedirects > 0 {
		p.MaxRedirects = g.MaxRedirects
	}
	if g.ValidateURL != nil {
		p.BlockPrivateIPs = false
		p.URLFilter = func(_ context.Context, rawURL string) error {
			return g.ValidateURL(rawURL)
		}
	}
	return p
}

// SiteCrawler is the default Crawler: seaportal.ScrapeSite over input with
// every fetch gated by guard. timeout <= 0 keeps seaportal's default
// overall deadline.
func SiteCrawler(input Input, timeout time.Duration, guard CrawlGuard) Crawler {
	return func(ctx context.Context) (*seaportal.ScrapeResult, error) {
		opts := &seaportal.ScrapeOptions{
			BaseURL:         input.URL,
			MaxPages:        input.MaxPages,
			MaxPerPattern:   input.MaxPerPattern,
			IncludePatterns: input.IncludePatterns,
			ExcludePatterns: input.ExcludePatterns,
			Timeout:         timeout,
			Security:        guard.Policy(),
		}
		return seaportal.ScrapeSite(ctx, opts)
	}
}

// URLListCrawler is the Crawler for expand mode: instead of discovering a
// site, it fetches an explicit set of URLs over HTTP and extracts each, so
// Run can route and browser-render exactly the pages a caller chose from a
// prior preview. Every fetch goes through guard.Policy() — the same SSRF and
// redirect protection as the crawl. Per-URL fetch failures become failed
// pages, never aborting the run; the crawl only errors when the list is empty.
func URLListCrawler(urls []string, timeout time.Duration, guard CrawlGuard) Crawler {
	return func(ctx context.Context) (*seaportal.ScrapeResult, error) {
		clean := dedupeURLs(urls)
		if len(clean) == 0 {
			return nil, errors.New("no urls to expand")
		}
		policy := guard.Policy()
		pages := make([]seaportal.PageObject, 0, len(clean))
		for _, u := range clean {
			pages = append(pages, fetchOne(ctx, u, timeout, policy))
		}
		return &seaportal.ScrapeResult{
			Site:  seaportal.SiteInfo{BaseURL: originOf(clean[0]), TotalURLsInSitemap: len(clean)},
			Pages: pages,
		}, nil
	}
}

// fetchOne HTTP-fetches url through the security policy and extracts it into
// a PageObject. Fetch or extraction failures are recorded on the page's Error
// so the run continues and the page can still route to the browser.
func fetchOne(ctx context.Context, url string, timeout time.Duration, policy *seaportal.SecurityPolicy) seaportal.PageObject {
	body, hdr, status, err := seaportal.FetchBytes(ctx, url, seaportal.FetchBytesOptions{
		Security: policy,
		Timeout:  timeout,
	})
	if err != nil {
		return seaportal.PageObject{URL: url, Status: status, Error: err.Error()}
	}
	r := seaportal.FromHTML(string(body), url)
	p := seaportal.PageObject{
		URL:         url,
		Status:      status,
		ContentType: hdr.Get("Content-Type"),
		Title:       r.Title,
		Markdown:    r.Content,
		Error:       r.Error,
	}
	if r.Description != "" {
		p.Meta = map[string]string{"description": r.Description}
	}
	return p
}

// blockedStatuses are HTTP statuses where a real browser (with its stealth
// and challenge handling) may succeed where a plain HTTP client was refused.
var blockedStatuses = map[int]bool{401: true, 403: true, 407: true, 429: true, 503: true}

// NeedsBrowser is the routing verdict for one HTTP-extracted page: whether
// the browser should re-render it, and why. Not-found pages never route to
// the browser — re-rendering a 404 cannot recover content.
func NeedsBrowser(p Page) (bool, []string) {
	if p.StatusCode == 404 || p.StatusCode == 410 {
		return false, nil
	}
	var reasons []string
	if p.Error != "" {
		reasons = append(reasons, "fetch-error")
	}
	if blockedStatuses[p.StatusCode] {
		reasons = append(reasons, fmt.Sprintf("blocked-status:%d", p.StatusCode))
	}
	if len(reasons) == 0 && len(strings.TrimSpace(p.Markdown)) < MinContentChars {
		reasons = append(reasons, "thin-content")
	}
	return len(reasons) > 0, reasons
}

// Run executes the scrape pipeline: crawl the site over HTTP, route each
// page, then browser-render the routed pages with bounded concurrency and
// re-extract their content from the rendered HTML. Page failures are report
// data, not errors — Run only errors when the crawl itself fails or finds
// nothing.
func Run(ctx context.Context, input Input, opts RunOptions, crawl Crawler, render BrowserRenderer) (Report, error) {
	res, err := crawl(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("site crawl: %w", err)
	}
	if len(res.Pages) == 0 {
		return Report{}, errors.New("no pages discovered")
	}

	pages := make([]Page, len(res.Pages))
	enrich := make([]bool, len(res.Pages))
	for i, sp := range res.Pages {
		p := fromSeaportal(sp)
		needs, reasons := NeedsBrowser(p)
		p.BrowserRecommended = needs
		p.BrowserReasons = reasons
		if opts.EnrichAll && !needs && p.StatusCode != 404 && p.StatusCode != 410 {
			needs, p.BrowserReasons = true, []string{"enrich-all"}
		}
		if opts.Preview {
			p.CharCount = utf8.RuneCountInString(p.Markdown)
			p.Snippet = snippet(p.Markdown, SnippetChars)
			p.Markdown = ""
		}
		pages[i] = p
		enrich[i] = needs && !opts.NoBrowser && !opts.Preview && render != nil
	}

	sem := make(chan struct{}, clampConcurrency(opts.Concurrency))
	var wg sync.WaitGroup
	for i := range pages {
		if !enrich[i] {
			continue
		}
		wg.Add(1)
		go func(p *Page) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			enrichPage(p, render)
		}(&pages[i])
	}
	wg.Wait()

	return Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Input:         input,
		Site: SiteInfo{
			BaseURL:         res.Site.BaseURL,
			Title:           res.Site.Title,
			SitemapFound:    res.Site.SitemapFound,
			TotalDiscovered: res.Site.TotalURLsInSitemap,
			SampledPages:    len(pages),
		},
		PageGroups: fromSeaportalGroups(res.PageGroups),
		Pages:      pages,
		Summary:    summarize(pages, res.Summary.Recommendations),
	}, nil
}

// enrichPage renders p in the browser and replaces its content with the
// extraction over the rendered HTML. The HTTP extraction is kept whenever
// the browser path fails or yields nothing.
func enrichPage(p *Page, render BrowserRenderer) {
	html, err := render(p.URL)
	if err != nil {
		p.BrowserError = err.Error()
		return
	}
	r := seaportal.FromHTML(html, p.URL)
	if r.Error != "" {
		p.BrowserError = r.Error
		return
	}
	if strings.TrimSpace(r.Content) == "" {
		p.BrowserError = "browser extraction produced no content"
		return
	}
	p.Markdown = r.Content
	p.Source = SourceBrowser
	// The browser reached the page and produced content, so an HTTP fetch
	// failure no longer marks the page as failed.
	p.Error = ""
	if r.Title != "" {
		p.Title = r.Title
	}
	if p.Meta == nil && r.Description != "" {
		p.Meta = map[string]string{"description": r.Description}
	}
}

func fromSeaportal(sp seaportal.PageObject) Page {
	return Page{
		URL:           sp.URL,
		Title:         sp.Title,
		StatusCode:    sp.Status,
		ContentType:   sp.ContentType,
		Markdown:      sp.Markdown,
		Meta:          sp.Meta,
		Schema:        sp.Schema,
		InternalLinks: sp.InternalLinks,
		ExternalLinks: sp.ExternalLinks,
		Source:        SourceHTTP,
		Error:         sp.Error,
	}
}

// fromSeaportalGroups keeps the site tree as URL references; page content
// lives once in Report.Pages.
func fromSeaportalGroups(groups []seaportal.PageGroup) []PageGroup {
	out := make([]PageGroup, 0, len(groups))
	for _, g := range groups {
		urls := make([]string, 0, len(g.Pages))
		for _, p := range g.Pages {
			urls = append(urls, p.URL)
		}
		out = append(out, PageGroup{
			Pattern: g.Pattern,
			Total:   g.TotalInSitemap,
			Sampled: g.Sampled,
			URLs:    urls,
		})
	}
	return out
}

func summarize(pages []Page, recommendations []string) Summary {
	s := Summary{ContentTypes: map[string]int{}, Recommendations: recommendations}
	for _, p := range pages {
		if p.ContentType != "" {
			s.ContentTypes[p.ContentType]++
		}
		switch {
		case p.Error != "":
			s.FailedPages++
		case p.Source == SourceBrowser:
			s.BrowserPages++
		default:
			s.HTTPPages++
		}
	}
	if len(s.ContentTypes) == 0 {
		s.ContentTypes = nil
	}
	return s
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

// snippet returns the first max characters of s with runs of whitespace
// collapsed to single spaces, appending an ellipsis when it truncated.
func snippet(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:max])) + "…"
}

// dedupeURLs trims, drops blanks, and removes duplicate URLs while keeping
// first-seen order.
func dedupeURLs(urls []string) []string {
	seen := make(map[string]bool, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// originOf returns the scheme://host of a URL, or the raw string when it does
// not parse — used only to label the expand result's base URL.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Scheme + "://" + u.Host
}
