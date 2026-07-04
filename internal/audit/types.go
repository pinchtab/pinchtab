// Package audit defines the shared data model and versioned report schema
// for the PinchTab scraping & audit pipeline (docs/pinchtab-scrape-audit-spec.md).
// Types here are audit-local with camelCase JSON tags; they map from bridge
// and observe shapes without importing handler types, so downstream consumers
// (LLM pipelines, CI diffs) can rely on a stable contract.
package audit

import "time"

// SchemaVersion is the current audit report schema version, embedded in
// every AuditReport so consumers can detect incompatible changes.
const SchemaVersion = "1.0"

// ConsoleLogEntry is a single console message captured during page load.
// It mirrors the bridge console log shape.
type ConsoleLogEntry struct {
	// Timestamp is when the message was emitted.
	Timestamp time.Time `json:"timestamp"`
	// Level is the console level (log, info, warn, error).
	Level string `json:"level"`
	// Message is the console message text.
	Message string `json:"message"`
	// Source identifies where the message originated (URL or subsystem).
	Source string `json:"source,omitempty"`
}

// NetworkRequest is a single resource request observed while loading a page.
// It mirrors the observe network entry shape, trimmed to audit-relevant fields.
type NetworkRequest struct {
	// URL is the requested resource URL.
	URL string `json:"url"`
	// Method is the HTTP method.
	Method string `json:"method"`
	// Status is the HTTP response status code, zero if the request never completed.
	Status int `json:"status,omitempty"`
	// StatusText is the HTTP status text.
	StatusText string `json:"statusText,omitempty"`
	// ResourceType classifies the resource (Document, Image, Script, ...).
	ResourceType string `json:"resourceType,omitempty"`
	// MimeType is the response MIME type.
	MimeType string `json:"mimeType,omitempty"`
	// StartTime is when the request started.
	StartTime time.Time `json:"startTime"`
	// Duration is the request duration in milliseconds.
	Duration float64 `json:"duration,omitempty"`
	// Size is the response body size in bytes.
	Size int64 `json:"size,omitempty"`
	// Failed reports whether the request failed (network error or HTTP >= 400).
	Failed bool `json:"failed,omitempty"`
	// Error is the network error text when the request failed before a response.
	Error string `json:"error,omitempty"`
}

// BrokenAsset is a page resource that failed to load (404 images, missing
// scripts or stylesheets, failed API calls).
type BrokenAsset struct {
	// URL is the asset URL that failed.
	URL string `json:"url"`
	// ResourceType classifies the asset (Image, Script, Stylesheet, XHR, ...).
	ResourceType string `json:"resourceType,omitempty"`
	// Status is the HTTP status code, zero when the failure was a network error.
	Status int `json:"status,omitempty"`
	// Error describes the failure when no HTTP status is available.
	Error string `json:"error,omitempty"`
}

// InteractiveElement is an actionable element discovered on the page,
// derived from the accessibility snapshot.
type InteractiveElement struct {
	// Ref is the stable snapshot reference (e.g. "e5").
	Ref string `json:"ref"`
	// Role is the accessibility role (button, link, textbox, ...).
	Role string `json:"role"`
	// Name is the accessible name.
	Name string `json:"name,omitempty"`
	// Tag is the underlying HTML tag.
	Tag string `json:"tag,omitempty"`
	// Label is the associated form label, when present.
	Label string `json:"label,omitempty"`
	// Disabled reports whether the element is disabled.
	Disabled bool `json:"disabled,omitempty"`
	// Visible reports whether the element is visible in the layout.
	Visible bool `json:"visible,omitempty"`
}

// BrowserTimingMetrics holds browser-level performance timings for a page,
// in milliseconds unless noted otherwise. JSON field names match the
// GET /timing endpoint.
type BrowserTimingMetrics struct {
	// TimeToFirstByte is the navigation time-to-first-byte.
	TimeToFirstByte float64 `json:"ttfbMs,omitempty"`
	// DOMContentLoaded is the DOMContentLoaded event time.
	DOMContentLoaded float64 `json:"domContentLoadedMs,omitempty"`
	// Load is the load event time.
	Load float64 `json:"loadMs,omitempty"`
	// FirstContentfulPaint is the FCP Core Web Vital.
	FirstContentfulPaint float64 `json:"fcpMs,omitempty"`
	// LargestContentfulPaint is the LCP Core Web Vital.
	LargestContentfulPaint float64 `json:"lcpMs,omitempty"`
	// CumulativeLayoutShift is the CLS Core Web Vital (unitless score).
	CumulativeLayoutShift float64 `json:"cls,omitempty"`
}

// VisualDiffResult is the outcome of comparing a page screenshot against a
// baseline in compare mode.
type VisualDiffResult struct {
	// BaselinePath is the baseline screenshot file path.
	BaselinePath string `json:"baselinePath"`
	// CurrentPath is the current screenshot file path.
	CurrentPath string `json:"currentPath"`
	// DiffPath is the annotated diff image path, when a diff was rendered.
	DiffPath string `json:"diffPath,omitempty"`
	// DiffPixels is the number of pixels that differ.
	DiffPixels int `json:"diffPixels"`
	// DiffRatio is the fraction of pixels that differ, in [0,1].
	DiffRatio float64 `json:"diffRatio"`
	// Changed reports whether the diff exceeds the change threshold.
	Changed bool `json:"changed"`
}

// SecurityFinding is a single security-surface issue detected during an audit
// (mixed content, exposed endpoint, scanner rule hit).
type SecurityFinding struct {
	// RuleID identifies the rule that produced the finding.
	RuleID string `json:"ruleId"`
	// Severity is the finding severity (info, low, medium, high, critical).
	Severity string `json:"severity"`
	// Detail is a human-readable description of the finding.
	Detail string `json:"detail"`
	// URL is the page or resource the finding applies to.
	URL string `json:"url,omitempty"`
}

// BrowserPageData is the browser-enriched data captured for a single page.
type BrowserPageData struct {
	// ScreenshotPath is the saved screenshot file path.
	ScreenshotPath string `json:"screenshotPath,omitempty"`
	// FullPageScreenshot reports whether the screenshot covers the full page.
	FullPageScreenshot bool `json:"fullPageScreenshot,omitempty"`
	// ConsoleLogs are the console messages captured during load.
	ConsoleLogs []ConsoleLogEntry `json:"consoleLogs,omitempty"`
	// NetworkRequests are the resource requests observed during load.
	NetworkRequests []NetworkRequest `json:"networkRequests,omitempty"`
	// BrokenAssets are resources that failed to load, especially 404 images.
	BrokenAssets []BrokenAsset `json:"brokenAssets,omitempty"`
	// InteractiveElements are the actionable elements found on the page.
	InteractiveElements []InteractiveElement `json:"interactiveElements,omitempty"`
	// AccessibilityScore is the basic a11y score for the page, in [0,100].
	AccessibilityScore int `json:"accessibilityScore,omitempty"`
	// VisualDiff is the visual regression result; set only in compare mode.
	VisualDiff *VisualDiffResult `json:"visualDiff,omitempty"`
	// TimingMetrics are the browser-level performance timings.
	TimingMetrics BrowserTimingMetrics `json:"timingMetrics"`
}

// PageResult is the audit outcome for a single page: its URL, any SeaPortal
// summary carried over from the input, and the browser-enriched data.
type PageResult struct {
	// URL is the audited page URL.
	URL string `json:"url"`
	// Title is the page title, when known.
	Title string `json:"title,omitempty"`
	// StatusCode is the HTTP status of the page navigation, when known.
	StatusCode int `json:"statusCode,omitempty"`
	// Error describes a navigation or collection failure; empty on success.
	Error string `json:"error,omitempty"`
	// Screenshot is the base64-encoded PNG when captured inline; consumers
	// writing artifacts move it to Browser.ScreenshotPath.
	Screenshot string `json:"screenshot,omitempty"`
	// Seaportal holds SeaPortal per-page summary fields passed through
	// verbatim when the input was a SeaPortal results file.
	Seaportal map[string]any `json:"seaportal,omitempty"`
	// Browser is the browser-enriched data for the page.
	Browser BrowserPageData `json:"browser"`
}

// AuditInput describes where the audited pages came from. Exactly one source
// is expected to be set.
type AuditInput struct {
	// URLs is a direct list of page URLs to audit.
	URLs []string `json:"urls,omitempty"`
	// SitemapURL is a sitemap to discover pages from.
	SitemapURL string `json:"sitemapUrl,omitempty"`
	// SeaportalFile is the path to a SeaPortal results JSON file.
	SeaportalFile string `json:"seaportalFile,omitempty"`
	// SeaportalFormat versions the ingested seaportal payload
	// (SeaportalReportFormat); empty for non-seaportal inputs.
	SeaportalFormat string `json:"seaportalFormat,omitempty"`
}

// AuditOptions are the run options an audit was executed with.
type AuditOptions struct {
	// SampleSize caps how many pages are audited per page group.
	SampleSize int `json:"sampleSize,omitempty"`
	// Screenshot enables screenshot capture.
	Screenshot bool `json:"screenshot,omitempty"`
	// NetworkMonitor enables network request capture.
	NetworkMonitor bool `json:"networkMonitor,omitempty"`
	// VisualDiff enables visual regression against a baseline.
	VisualDiff bool `json:"visualDiff,omitempty"`
	// Concurrency is the number of pages audited in parallel.
	Concurrency int `json:"concurrency,omitempty"`
	// OutputDir is where screenshots and report artifacts are written.
	OutputDir string `json:"outputDir,omitempty"`
}

// AuditReport is the site-level audit result: the versioned top-level schema
// every audit, compare, and report consumer builds on.
type AuditReport struct {
	// SchemaVersion is the report schema version; always SchemaVersion.
	SchemaVersion string `json:"schemaVersion"`
	// GeneratedAt is when the report was produced.
	GeneratedAt time.Time `json:"generatedAt"`
	// Input describes where the audited pages came from.
	Input AuditInput `json:"input"`
	// Options are the run options the audit was executed with.
	Options AuditOptions `json:"options"`
	// Pages are the per-page audit results.
	Pages []PageResult `json:"pages"`
	// SummaryScore is the overall site score, in [0,100].
	SummaryScore int `json:"summaryScore"`
	// SecurityFindings are site-wide security-surface findings.
	SecurityFindings []SecurityFinding `json:"securityFindings,omitempty"`
	// Recommendations are human-readable follow-up suggestions.
	Recommendations []string `json:"recommendations,omitempty"`
}

// NewAuditReport returns an AuditReport stamped with the current SchemaVersion.
func NewAuditReport() AuditReport {
	return AuditReport{SchemaVersion: SchemaVersion}
}
