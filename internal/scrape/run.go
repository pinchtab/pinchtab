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
			Site:  seaportal.SiteInfo{BaseURL: originOf(clean[0])},
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
			BaseURL:            res.Site.BaseURL,
			Title:              res.Site.Title,
			SitemapFound:       res.Site.SitemapFound,
			TotalURLsInSitemap: res.Site.TotalURLsInSitemap,
			SampledPages:       len(pages),
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

// ThinContentChars is the post-enrichment thin-content threshold: a page whose final
// extraction is shorter than this has little usable text.
//
// PinchTab OWNS this number. seaportal applies the same 160 at the pinned version, but
// its constant is unexported AND inside another module's internal/ tree, so no import,
// linkname or reflection can reach it — claiming to mirror it would be a promise this
// code cannot keep. Owning it means a future upstream change becomes a visible decision
// here rather than a silent divergence with nothing failing.
const ThinContentChars = 160

// regeneratedRecommendations are the upstream recommendations this package rebuilds
// from the final pages, matched on the stable phrase in each — the errors/4xx line and
// the thin-content line.
//
// THE RULE, recorded where the choice is made, so the next upstream recommendation is
// classified rather than guessed at: a recommendation derived from PAGE CONTENT must be
// regenerated after enrichment, because the browser phase changes exactly that; one
// derived from CRAWL SCOPE is forwarded, because the browser phase changes nothing about
// which URLs were sampled. Both sitemap lines are scope and forward by matching nothing
// here; the errors and thin-content lines are content, and both are regenerated below
// from the final pages.
//
// An unrecognised recommendation is FORWARDED, since dropping advice that is still true
// is the worse failure — which is why this list names what is DROPPED, so a new upstream
// line takes the forwarding default until it is classified here.
var regeneratedRecommendations = []string{"extractable text", "returned errors"}

// summarize rolls the FINAL pages up. Every field including the prose comes from this one
// slice: the recommendations used to be carried in verbatim from the HTTP crawl, so the
// numbers were post-render and the sentences pre-render inside one object — a run that
// enriched every page still advised the reader to consider enrichment, and counted a page
// as having little text while reporting its 2012 characters two lines above.
// Failed is the one definition of a page that failed: a transport error, or a
// 4xx/5xx the browser did not go on to render. A recorded 3xx is a redirect
// artefact, not a page that failed to serve, and an absent status is unknown,
// not a failure. A browser render clears the failure because the page produced
// content, whatever the HTTP fetch answered.
func Failed(p Page) bool {
	return p.Error != "" || (p.StatusCode >= 400 && p.Source != SourceBrowser)
}

// summarize partitions the pages into failed, browser-rendered and http, summing
// to the page count, and counts content types over the successful ones only, so
// an error body is never reported as an ordinary page.
func summarize(pages []Page, inherited []string) Summary {
	s := Summary{ContentTypes: map[string]int{}}
	for _, p := range pages {
		switch {
		case Failed(p):
			s.FailedPages++
			continue
		case p.Source == SourceBrowser:
			s.BrowserPages++
		default:
			s.HTTPPages++
		}
		if p.ContentType != "" {
			s.ContentTypes[p.ContentType]++
		}
	}
	if len(s.ContentTypes) == 0 {
		s.ContentTypes = nil
	}
	s.Recommendations = recommend(pages, s, inherited)
	return s
}

// recommend keeps the crawl-scope advice the HTTP phase produced and regenerates the
// page-content advice from the pages as they finally stand.
func recommend(pages []Page, s Summary, inherited []string) []string {
	var recs []string
	if s.FailedPages > 0 {
		recs = append(recs, fmt.Sprintf("%d of %d pages returned errors or 4xx/5xx responses", s.FailedPages, len(pages)))
	}
	if thin, unenriched := thinPages(pages); thin > 0 {
		// The advice to enrich only survives while it is still actionable. A page the
		// browser already rendered — or tried to and failed — has had that remedy
		// applied, so repeating it sends the reader on a re-run that changes nothing.
		line := fmt.Sprintf("%d pages have little extractable text (possible SPA/JS-only)", thin)
		if unenriched > 0 {
			line += "; consider PinchTab enrichment"
		}
		recs = append(recs, line)
	}
	for _, rec := range inherited {
		if !regeneratedLocally(rec) {
			recs = append(recs, rec)
		}
	}
	return recs
}

// thinPages counts the pages whose FINAL content is thin, and how many of those never
// had a browser attempt — the second number is what decides whether advising enrichment
// is still useful. A failed page is not also thin: it has no content for a reason that
// the errors line already reports.
func thinPages(pages []Page) (thin, unenriched int) {
	for _, p := range pages {
		if Failed(p) {
			continue
		}
		if contentChars(p) >= ThinContentChars {
			continue
		}
		thin++
		if p.Source != SourceBrowser && p.BrowserError == "" {
			unenriched++
		}
	}
	return thin, unenriched
}

// contentChars is a page's extracted length, read from CharCount in preview mode where
// the body is deliberately withheld and Markdown is empty for every page.
func contentChars(p Page) int {
	if p.Markdown != "" {
		return utf8.RuneCountInString(strings.TrimSpace(p.Markdown))
	}
	return p.CharCount
}

func regeneratedLocally(rec string) bool {
	for _, phrase := range regeneratedRecommendations {
		if strings.Contains(rec, phrase) {
			return true
		}
	}
	return false
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
