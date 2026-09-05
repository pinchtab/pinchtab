package audit

import (
	"encoding/base64"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

// PageOptions selects which collectors run when enriching a page.
type PageOptions struct {
	Screenshot bool `json:"screenshot"`
	Network    bool `json:"network"`
	// Console captures both page-message channels: console calls and
	// uncaught JavaScript exceptions.
	Console  bool `json:"console"`
	A11y     bool `json:"a11y"`
	Timing   bool `json:"timing"`
	Elements bool `json:"elements"`
	Security bool `json:"security"`
}

// DefaultPageOptions enables every collector.
func DefaultPageOptions() PageOptions {
	return PageOptions{Screenshot: true, Network: true, Console: true, A11y: true, Timing: true, Elements: true, Security: true}
}

// Collectors are the browser-backed data sources EnrichPage composes. Hooks
// for disabled options may be nil.
type Collectors struct {
	Title      func() (string, error)
	Screenshot func() ([]byte, error)
	Console    func() ([]bridge.LogEntry, error)
	JSErrors   func() ([]bridge.ErrorEntry, error)
	Network    func() ([]observe.NetworkEntry, error)
	Snapshot   func() ([]observe.A11yNode, error)
	PageFacts  func() (PageFacts, error)
	Timing     func() (*observe.TimingMetrics, error)
	Forms      func() ([]FormFact, error)
}

// PageAudit is the audit result for one page. Collector failures are data,
// not errors: they land in Error while the remaining fields stay populated.
type PageAudit struct {
	// URL is the audited page URL as requested.
	URL string `json:"url"`
	// Title is the page title after navigation.
	Title string `json:"title,omitempty"`
	// Error describes navigation or collector failures, empty on success.
	Error string `json:"error,omitempty"`
	// StatusCode is the HTTP status of the main document's response, read from
	// the network log; zero when the log was not collected or has no document.
	StatusCode int `json:"statusCode,omitempty"`
	// Screenshot is the base64-encoded PNG when screenshot capture is on.
	Screenshot string `json:"screenshot,omitempty"`
	// A11yFindings are the accessibility rule violations behind AccessibilityScore.
	A11yFindings []A11yFinding `json:"a11yFindings,omitempty"`
	// SecurityFindings are the page's rule-based security-surface findings.
	SecurityFindings []SecurityFinding `json:"securityFindings,omitempty"`
	BrowserPageData
}

// NewPageAuditError returns the structured entry for a page that could not
// be navigated at all.
func NewPageAuditError(url string, err error) PageAudit {
	return PageAudit{URL: url, Error: err.Error()}
}

// EnrichPage assembles a PageAudit from an already-navigated page via the
// given collectors, honoring opts. Individual collector failures never abort
// the audit; they are recorded in the Error field.
func EnrichPage(url string, opts PageOptions, c Collectors) PageAudit {
	pa := PageAudit{URL: url}
	var errs []string
	fail := func(stage string, err error) { errs = append(errs, stage+": "+err.Error()) }

	if c.Title != nil {
		if title, err := c.Title(); err == nil {
			pa.Title = title
		}
	}

	if opts.Console && c.Console != nil {
		if logs, err := c.Console(); err != nil {
			fail("console", err)
		} else {
			pa.ConsoleLogs = MapConsoleLogs(logs)
		}
	}

	if opts.Console && c.JSErrors != nil {
		if errors, err := c.JSErrors(); err != nil {
			fail("jsErrors", err)
		} else {
			pa.JSErrors = MapJSErrors(errors)
		}
	}

	if opts.Network && c.Network != nil {
		if entries, err := c.Network(); err != nil {
			fail("network", err)
		} else {
			pa.NetworkRequests = MapNetworkRequests(entries)
			pa.BrokenAssets = MapBrokenAssets(observe.BrokenAssets(entries))
			pa.StatusCode = mainDocumentStatus(url, entries)
		}
	}

	var nodes []observe.A11yNode
	if (opts.Elements || opts.A11y) && c.Snapshot != nil {
		var err error
		if nodes, err = c.Snapshot(); err != nil {
			fail("snapshot", err)
			nodes = nil
		}
	}
	if opts.Elements {
		pa.InteractiveElements = MapInteractiveElements(nodes)
	}
	if opts.A11y && c.PageFacts != nil {
		if facts, err := c.PageFacts(); err != nil {
			fail("a11y", err)
		} else {
			report := EvaluateA11y(nodes, facts)
			pa.AccessibilityScore = report.Score
			pa.A11yFindings = report.Findings
		}
	}

	if opts.Timing && c.Timing != nil {
		if timing, err := c.Timing(); err != nil {
			fail("timing", err)
		} else if timing != nil {
			pa.TimingMetrics = MapTimingMetrics(*timing)
		}
	}

	if opts.Screenshot && c.Screenshot != nil {
		if png, err := c.Screenshot(); err != nil {
			fail("screenshot", err)
		} else {
			pa.Screenshot = base64.StdEncoding.EncodeToString(png)
		}
	}

	if opts.Security {
		var forms []FormFact
		if c.Forms != nil {
			var err error
			if forms, err = c.Forms(); err != nil {
				fail("security", err)
				forms = nil
			}
		}
		pa.SecurityFindings = EvaluateSecurity(url, pa.Title, pa.NetworkRequests, forms)
	}

	pa.Error = strings.Join(errs, "; ")
	return pa
}

// mainDocumentStatus is the status of the page's own navigation. Once the
// requested document redirects, successive document responses belong to that
// redirect chain until its first non-redirect response. Later document entries
// may be frames and must not replace the terminal navigation status.
func mainDocumentStatus(url string, entries []observe.NetworkEntry) int {
	status := 0
	followingRedirect := false
	for _, e := range entries {
		if !strings.EqualFold(e.ResourceType, "document") {
			continue
		}
		if e.URL == url || followingRedirect {
			status = e.Status
			followingRedirect = e.Status >= 300 && e.Status < 400
			if e.URL != url && !followingRedirect {
				return status
			}
		}
	}
	return status
}

// MapConsoleLogs converts bridge console entries to the audit schema.
func MapConsoleLogs(logs []bridge.LogEntry) []ConsoleLogEntry {
	out := make([]ConsoleLogEntry, 0, len(logs))
	for _, l := range logs {
		out = append(out, ConsoleLogEntry{
			Timestamp: l.Timestamp,
			Level:     l.Level,
			Message:   l.Message,
			Source:    l.Source,
		})
	}
	return out
}

// MapJSErrors converts bridge uncaught-exception entries to the audit schema.
func MapJSErrors(errors []bridge.ErrorEntry) []JSError {
	out := make([]JSError, 0, len(errors))
	for _, e := range errors {
		out = append(out, JSError{
			Timestamp: e.Timestamp,
			Message:   e.Message,
			Type:      e.Type,
			URL:       e.URL,
			Line:      e.Line,
			Column:    e.Column,
			Stack:     e.Stack,
		})
	}
	return out
}

// MapNetworkRequests converts observed network entries to the audit schema.
func MapNetworkRequests(entries []observe.NetworkEntry) []NetworkRequest {
	out := make([]NetworkRequest, 0, len(entries))
	for _, e := range entries {
		out = append(out, NetworkRequest{
			URL:          e.URL,
			Method:       e.Method,
			Status:       e.Status,
			StatusText:   e.StatusText,
			ResourceType: e.ResourceType,
			MimeType:     e.MimeType,
			StartTime:    e.StartTime,
			Duration:     e.Duration,
			Size:         e.Size,
			Failed:       e.Failed || e.Status >= 400,
			Error:        e.Error,
		})
	}
	return out
}

// MapBrokenAssets converts observe broken-asset entries to the audit schema.
func MapBrokenAssets(assets []observe.BrokenAsset) []BrokenAsset {
	out := make([]BrokenAsset, 0, len(assets))
	for _, a := range assets {
		out = append(out, BrokenAsset{
			URL:          a.URL,
			ResourceType: a.ResourceType,
			Status:       a.StatusCode,
			Error:        a.Error,
		})
	}
	return out
}

// MapInteractiveElements extracts the visible interactive elements from a
// full accessibility snapshot.
func MapInteractiveElements(nodes []observe.A11yNode) []InteractiveElement {
	out := make([]InteractiveElement, 0)
	for _, n := range nodes {
		if n.Hidden || !observe.InteractiveRoles[n.Role] {
			continue
		}
		out = append(out, InteractiveElement{
			Ref:      n.Ref,
			Role:     n.Role,
			Name:     n.Name,
			Tag:      n.Tag,
			Label:    n.Label,
			Disabled: n.Disabled,
		})
	}
	return out
}

// MapTimingMetrics converts observe timing metrics to the audit schema.
func MapTimingMetrics(t observe.TimingMetrics) BrowserTimingMetrics {
	return BrowserTimingMetrics{
		TimeToFirstByte:        t.TTFBMs,
		DOMContentLoaded:       t.DOMContentLoadedMs,
		Load:                   t.LoadMs,
		FirstContentfulPaint:   t.FCPMs,
		LargestContentfulPaint: t.LCPMs,
		CumulativeLayoutShift:  t.CLS,
	}
}
