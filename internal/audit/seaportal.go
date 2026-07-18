package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	seaportal "github.com/pinchtab/seaportal"
)

// SeaportalReportFormat versions the interim seaportal input format: a JSON
// array of seaportal Result objects (seaportal has no site-level SiteReport
// yet). Recorded on AuditInput so report consumers can detect format changes.
const SeaportalReportFormat = "seaportal-results/v0"

// SeaportalPage is the audit-facing view of one seaportal Result: the
// summary fields merged into report pages plus the browser-routing decision.
type SeaportalPage struct {
	URL                string
	Title              string
	StatusCode         int
	BrowserRecommended bool
	// Summary is embedded verbatim into PageResult.Seaportal.
	Summary map[string]any
}

// ParseSeaportalReport parses the interim JSON array of seaportal Results
// into audit pages. Entries without a URL are dropped.
func ParseSeaportalReport(data []byte) ([]SeaportalPage, error) {
	var results []seaportal.Result
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("parse seaportal report: %w", err)
	}
	pages := make([]SeaportalPage, 0, len(results))
	for _, r := range results {
		if r.URL == "" {
			continue
		}
		pages = append(pages, SeaportalPage{
			URL:                r.URL,
			Title:              r.Title,
			StatusCode:         r.StatusCode,
			BrowserRecommended: r.Profile.BrowserRecommended,
			Summary: map[string]any{
				"title":              r.Title,
				"description":        r.Description,
				"confidence":         r.Confidence,
				"quality":            r.Quality,
				"decision":           string(r.Profile.Decision),
				"browserRecommended": r.Profile.BrowserRecommended,
			},
		})
	}
	return pages, nil
}

// FlattenSitemapURLs discovers page URLs from a sitemap (recursively for
// sitemap indexes) via seaportal.FlattenSitemap. security gates every
// sitemap fetch — child sitemaps of an index are attacker-controlled URLs,
// so only the root sitemap URL being pre-validated is not enough.
func FlattenSitemapURLs(ctx context.Context, sitemapURL string, security *seaportal.SecurityPolicy) ([]string, error) {
	entries, err := seaportal.FlattenSitemap(ctx, sitemapURL, seaportal.FlattenSitemapOptions{
		Timeout:  15 * time.Second,
		Security: security,
	})
	if err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Loc != "" {
			urls = append(urls, e.Loc)
		}
	}
	return urls, nil
}
