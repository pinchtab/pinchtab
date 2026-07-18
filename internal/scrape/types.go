// Package scrape implements the site scrape pipeline: a fast seaportal HTTP
// crawl builds the page tree and markdown content, then pages whose HTTP
// extraction came back thin, blocked, or failed are re-rendered in a real
// browser (whatever provider the instance runs — chrome, cloak, ghost-chrome)
// and re-extracted from the rendered HTML.
package scrape

import "time"

// SchemaVersion is the current scrape report schema version, embedded in
// every report so consumers can detect format changes.
const SchemaVersion = "1.0"

// Page sources: how a page's content was obtained.
const (
	SourceHTTP    = "http"
	SourceBrowser = "browser"
)

// Input records what was asked for, echoed back in the report.
type Input struct {
	URL             string   `json:"url"`
	MaxPages        int      `json:"maxPages,omitempty"`
	MaxPerPattern   int      `json:"maxPerPattern,omitempty"`
	IncludePatterns []string `json:"includePatterns,omitempty"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
}

// Page is one scraped page: seaportal's HTTP extraction, possibly replaced
// by a browser-rendered extraction when routing decided HTTP was not enough.
type Page struct {
	URL         string            `json:"url"`
	Title       string            `json:"title,omitempty"`
	StatusCode  int               `json:"statusCode,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Markdown    string            `json:"markdown,omitempty"`
	Meta        map[string]string `json:"meta,omitempty"`

	// CharCount is the length in characters of the extracted content. It is
	// set in preview mode (where Markdown is withheld) so a caller can gauge
	// how heavy a full expand of this page would be.
	CharCount int `json:"charCount,omitempty"`
	// Snippet is the leading, whitespace-collapsed slice of the extracted
	// content, set only in preview mode as a stand-in for the withheld
	// Markdown so a caller can tell what the page is about.
	Snippet       string           `json:"snippet,omitempty"`
	Schema        []map[string]any `json:"schema,omitempty"`
	InternalLinks int              `json:"internalLinks,omitempty"`
	ExternalLinks int              `json:"externalLinks,omitempty"`

	// Source is where Markdown came from: SourceHTTP or SourceBrowser.
	Source string `json:"source"`
	// BrowserRecommended is the routing verdict for this page, recorded
	// even when enrichment was skipped (--no-browser) or forced (--enrich-all).
	BrowserRecommended bool `json:"browserRecommended,omitempty"`
	// BrowserReasons explains why the page was routed to the browser.
	BrowserReasons []string `json:"browserReasons,omitempty"`
	// BrowserError is a failed browser enrichment; the HTTP extraction is
	// kept and the run does not fail.
	BrowserError string `json:"browserError,omitempty"`
	// Error is the page-level failure when no engine produced content.
	Error string `json:"error,omitempty"`
}

// PageGroup is one URL-pattern cluster of the site tree. Pages appear once
// in Report.Pages; groups reference them by URL.
type PageGroup struct {
	Pattern string   `json:"pattern"`
	Total   int      `json:"total"`
	Sampled int      `json:"sampled"`
	URLs    []string `json:"urls"`
}

// SiteInfo describes the crawled site.
type SiteInfo struct {
	BaseURL         string `json:"baseUrl"`
	Title           string `json:"title,omitempty"`
	SitemapFound    bool   `json:"sitemapFound"`
	TotalDiscovered int    `json:"totalDiscovered"`
	SampledPages    int    `json:"sampledPages"`
}

// Summary is the run roll-up.
type Summary struct {
	ContentTypes    map[string]int `json:"contentTypes,omitempty"`
	HTTPPages       int            `json:"httpPages"`
	BrowserPages    int            `json:"browserPages"`
	FailedPages     int            `json:"failedPages"`
	Recommendations []string       `json:"recommendations,omitempty"`
}

// Report is the versioned scrape output.
type Report struct {
	SchemaVersion string      `json:"schemaVersion"`
	GeneratedAt   time.Time   `json:"generatedAt"`
	Input         Input       `json:"input"`
	Site          SiteInfo    `json:"site"`
	PageGroups    []PageGroup `json:"pageGroups"`
	Pages         []Page      `json:"pages"`
	Summary       Summary     `json:"summary"`
}
