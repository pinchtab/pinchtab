package pinchtabaudit

import "time"

// The types in this file mirror the pinchtab audit API's JSON contract.
// They are defined here rather than aliased so the public surface never
// depends on pinchtab internal packages.

// ConsoleLogEntry is a single console message captured during page load.
type ConsoleLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Source    string    `json:"source,omitempty"`
}

// JSError is an uncaught JavaScript exception raised while the page loaded.
// It is a separate channel from ConsoleLogEntry: a console.error call lands
// there, a thrown exception lands here.
type JSError struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Type      string    `json:"type,omitempty"`
	URL       string    `json:"url,omitempty"`
	Line      int64     `json:"line,omitempty"`
	Column    int64     `json:"column,omitempty"`
	Stack     string    `json:"stack,omitempty"`
}

// NetworkRequest is a resource request observed while loading a page.
type NetworkRequest struct {
	URL          string    `json:"url"`
	Method       string    `json:"method"`
	Status       int       `json:"status,omitempty"`
	StatusText   string    `json:"statusText,omitempty"`
	ResourceType string    `json:"resourceType,omitempty"`
	MimeType     string    `json:"mimeType,omitempty"`
	StartTime    time.Time `json:"startTime"`
	Duration     float64   `json:"duration,omitempty"`
	Size         int64     `json:"size,omitempty"`
	Failed       bool      `json:"failed,omitempty"`
	Error        string    `json:"error,omitempty"`
}

// BrokenAsset is a page resource that failed to load.
type BrokenAsset struct {
	URL          string `json:"url"`
	ResourceType string `json:"resourceType,omitempty"`
	Status       int    `json:"status,omitempty"`
	Error        string `json:"error,omitempty"`
}

// InteractiveElement is an actionable element discovered on the page.
type InteractiveElement struct {
	Ref      string `json:"ref"`
	Role     string `json:"role"`
	Name     string `json:"name,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Label    string `json:"label,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

// TimingMetrics holds browser-level performance timings in milliseconds.
type TimingMetrics struct {
	TimeToFirstByte        float64 `json:"ttfbMs,omitempty"`
	DOMContentLoaded       float64 `json:"domContentLoadedMs,omitempty"`
	Load                   float64 `json:"loadMs,omitempty"`
	FirstContentfulPaint   float64 `json:"fcpMs,omitempty"`
	LargestContentfulPaint float64 `json:"lcpMs,omitempty"`
	CumulativeLayoutShift  float64 `json:"cls,omitempty"`
}

// VisualDiffResult is the outcome of a compare-mode screenshot diff.
type VisualDiffResult struct {
	BaselinePath string  `json:"baselinePath"`
	CurrentPath  string  `json:"currentPath"`
	DiffPath     string  `json:"diffPath,omitempty"`
	DiffPixels   int     `json:"diffPixels"`
	DiffRatio    float64 `json:"diffRatio"`
	Changed      bool    `json:"changed"`
}

// SecurityFinding is a rule-based security-surface issue.
type SecurityFinding struct {
	RuleID   string `json:"ruleId"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	URL      string `json:"url,omitempty"`
}

// A11yFinding is one accessibility rule violation aggregated for a page.
type A11yFinding struct {
	Rule     string   `json:"rule"`
	Severity string   `json:"severity"`
	Count    int      `json:"count"`
	Samples  []string `json:"samples"`
}

// BrowserPageData is the browser-enriched data captured for a single page.
type BrowserPageData struct {
	ScreenshotPath      string               `json:"screenshotPath,omitempty"`
	FullPageScreenshot  bool                 `json:"fullPageScreenshot,omitempty"`
	ConsoleLogs         []ConsoleLogEntry    `json:"consoleLogs,omitempty"`
	JSErrors            []JSError            `json:"jsErrors,omitempty"`
	NetworkRequests     []NetworkRequest     `json:"networkRequests,omitempty"`
	BrokenAssets        []BrokenAsset        `json:"brokenAssets,omitempty"`
	InteractiveElements []InteractiveElement `json:"interactiveElements,omitempty"`
	AccessibilityScore  int                  `json:"accessibilityScore,omitempty"`
	VisualDiff          *VisualDiffResult    `json:"visualDiff,omitempty"`
	TimingMetrics       TimingMetrics        `json:"timingMetrics"`
}

// PageAudit is the POST /audit/page response: one page's audit with the
// BrowserPageData fields inlined.
type PageAudit struct {
	URL              string            `json:"url"`
	Title            string            `json:"title,omitempty"`
	Error            string            `json:"error,omitempty"`
	StatusCode       int               `json:"statusCode,omitempty"`
	Screenshot       string            `json:"screenshot,omitempty"`
	A11yFindings     []A11yFinding     `json:"a11yFindings,omitempty"`
	SecurityFindings []SecurityFinding `json:"securityFindings,omitempty"`
	BrowserPageData
}

// PageResult is one page entry of an AuditReport.
type PageResult struct {
	URL              string            `json:"url"`
	Title            string            `json:"title,omitempty"`
	StatusCode       int               `json:"statusCode,omitempty"`
	Error            string            `json:"error,omitempty"`
	Screenshot       string            `json:"screenshot,omitempty"`
	Seaportal        map[string]any    `json:"seaportal,omitempty"`
	SecurityFindings []SecurityFinding `json:"securityFindings,omitempty"`
	Browser          BrowserPageData   `json:"browser"`
}

// AuditInput describes where a run's pages come from, on the way IN. Exactly one
// source should be set. EnrichWithBrowser builds the request body from it field by
// field, so this type is never marshalled whole and is not a mirror of the server's
// report input — that is AuditReportInput, which carries the fields the server
// echoes back instead of the raw results this one sends.
type AuditInput struct {
	// URLs is a direct list of page URLs to audit.
	URLs []string `json:"urls,omitempty"`
	// SitemapURL discovers pages from a sitemap (recursively).
	SitemapURL string `json:"sitemapUrl,omitempty"`
	// SeaportalResults is a raw seaportal results array (the interim
	// seaportal-results/v0 format); pages route through browserRecommended.
	SeaportalResults []byte `json:"seaportalResults,omitempty"`
}

// AuditReportInput is the input block a report carries back, mirroring
// internal/audit.AuditInput field for field. It is a separate type from AuditInput
// because the two describe opposite directions: a caller SENDS seaportal results as
// raw bytes, while the server REPORTS the file it read them from and the format it
// parsed. Decoding a report into AuditInput would silently drop both of those.
type AuditReportInput struct {
	// URLs is a direct list of page URLs that were audited.
	URLs []string `json:"urls,omitempty"`
	// SitemapURL is the sitemap pages were discovered from.
	SitemapURL string `json:"sitemapUrl,omitempty"`
	// SeaportalFile is the path to the SeaPortal results JSON file read.
	SeaportalFile string `json:"seaportalFile,omitempty"`
	// SeaportalFormat versions the ingested seaportal payload; empty for
	// non-seaportal inputs.
	SeaportalFormat string `json:"seaportalFormat,omitempty"`
}

// AuditOptions echoes the options a run was executed with.
type AuditOptions struct {
	SampleSize     int    `json:"sampleSize,omitempty"`
	Screenshot     bool   `json:"screenshot,omitempty"`
	NetworkMonitor bool   `json:"networkMonitor,omitempty"`
	VisualDiff     bool   `json:"visualDiff,omitempty"`
	Concurrency    int    `json:"concurrency,omitempty"`
	OutputDir      string `json:"outputDir,omitempty"`
}

// AuditReport is the POST /audit response: the versioned site-level report.
// Input carries no omitempty: the server emits the key unconditionally, and
// omitempty is a no-op on a non-pointer struct field anyway, so keeping it would
// only make the two sides of the mirror read as if they disagreed.
type AuditReport struct {
	SchemaVersion string           `json:"schemaVersion"`
	GeneratedAt   time.Time        `json:"generatedAt"`
	Input         AuditReportInput `json:"input"`
	Options       AuditOptions     `json:"options"`
	Pages         []PageResult     `json:"pages"`
	// SummaryScore is the mean accessibility score of enriched pages, in
	// [0,100]; broken assets, failed requests and uncaught JS errors do not
	// move it.
	SummaryScore     int               `json:"summaryScore"`
	SecurityFindings []SecurityFinding `json:"securityFindings,omitempty"`
	Recommendations  []string          `json:"recommendations,omitempty"`
}

// PageOptions toggles the per-page collectors. Nil fields keep the server
// default (all collectors on); use Bool to set one explicitly.
type PageOptions struct {
	Screenshot *bool `json:"screenshot,omitempty"`
	Network    *bool `json:"network,omitempty"`
	Console    *bool `json:"console,omitempty"`
	A11y       *bool `json:"a11y,omitempty"`
	Timing     *bool `json:"timing,omitempty"`
	Elements   *bool `json:"elements,omitempty"`
	Security   *bool `json:"security,omitempty"`
}

// RunOptions configures a multi-page EnrichWithBrowser run.
type RunOptions struct {
	// Concurrency is the number of pages audited in parallel (server caps apply).
	Concurrency int
	// SampleSize caps pages per URL template group, 0 = no cap.
	SampleSize int
	// EnrichAll browser-enriches every seaportal page regardless of
	// browserRecommended routing.
	EnrichAll bool
	// Page toggles the per-page collectors.
	Page *PageOptions
}

// Bool returns a pointer to v, for PageOptions fields.
func Bool(v bool) *bool { return &v }
