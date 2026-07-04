// Package pinchtabaudit is the public Go client for pinchtab's audit API —
// the spec's library mode. It talks to a running pinchtab server over HTTP
// and exposes typed wrappers for single-page enrichment (POST /audit/page)
// and multi-page audit runs (POST /audit), so Go programs can embed browser
// enrichment without shelling out to the CLI.
//
// The exported types mirror the server's JSON contract and depend on no
// pinchtab internal packages.
package pinchtabaudit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout bounds a whole enrichment call when the caller's context
// carries no deadline. Multi-page runs can legitimately take minutes.
const DefaultTimeout = 10 * time.Minute

// Client is a typed client for a running pinchtab server.
type Client struct {
	baseURL string
	token   string
	// HTTPClient may be replaced before first use; it defaults to a client
	// with DefaultTimeout.
	HTTPClient *http.Client
}

// New returns a Client for the pinchtab server at baseURL. token may be
// empty when the server runs without authentication.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
		HTTPClient: &http.Client{Timeout: DefaultTimeout},
	}
}

// EnrichPage audits a single URL with full browser enrichment and returns
// its BrowserPageData. A page that fails to load is not an error: the
// returned PageAudit carries the failure in its Error field.
func (c *Client) EnrichPage(ctx context.Context, url string, opts *PageOptions) (*PageAudit, error) {
	body := map[string]any{"url": url}
	if opts != nil {
		body["options"] = opts
	}
	var page PageAudit
	if err := c.post(ctx, "/audit/page", body, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// EnrichWithBrowser runs a multi-page audit over the given input — a URL
// list, a sitemap, or seaportal results (routed via browserRecommended) —
// and returns the versioned AuditReport.
func (c *Client) EnrichWithBrowser(ctx context.Context, input AuditInput, opts *RunOptions) (*AuditReport, error) {
	body := map[string]any{}
	switch {
	case len(input.URLs) > 0:
		body["urls"] = input.URLs
	case input.SitemapURL != "":
		body["sitemapUrl"] = input.SitemapURL
	case len(input.SeaportalResults) > 0:
		body["seaportalResults"] = json.RawMessage(input.SeaportalResults)
	default:
		return nil, fmt.Errorf("pinchtabaudit: input needs urls, a sitemap URL, or seaportal results")
	}
	if opts != nil {
		if opts.Concurrency > 0 {
			body["concurrency"] = opts.Concurrency
		}
		if opts.SampleSize > 0 {
			body["sampleSize"] = opts.SampleSize
		}
		if opts.EnrichAll {
			body["enrichAll"] = true
		}
		if opts.Page != nil {
			body["options"] = opts.Page
		}
	}
	var report AuditReport
	if err := c.post(ctx, "/audit", body, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("pinchtabaudit: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("pinchtabaudit: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("pinchtabaudit: %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("pinchtabaudit: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pinchtabaudit: %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("pinchtabaudit: decode response: %w", err)
	}
	return nil
}
