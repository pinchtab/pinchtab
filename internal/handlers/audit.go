package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/audit"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const maxSitemapBytes = 5 << 20

type auditRequest struct {
	URLs        []string              `json:"urls"`
	SitemapURL  string                `json:"sitemapUrl"`
	Options     *auditPageOptionsBody `json:"options"`
	Concurrency int                   `json:"concurrency"`
	SampleSize  int                   `json:"sampleSize"`
}

// @Endpoint POST /audit
func (h *Handlers) HandleAudit(w http.ResponseWriter, r *http.Request) {
	var req auditRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if len(req.URLs) == 0 && req.SitemapURL == "" {
		httpx.Error(w, 400, fmt.Errorf("urls or sitemapUrl required"))
		return
	}

	routing, ok := h.resolveNavigateBrowser(w, r, "", "")
	if !ok {
		return
	}
	if !h.ensureBrowserOrRespond(w, routing.EffectiveCfg) {
		return
	}

	auditor := func(url string, opts audit.PageOptions) audit.PageAudit {
		targets, err := h.validateAuditTarget(url, routing.EffectiveCfg)
		if err != nil {
			return audit.NewPageAuditError(url, err)
		}
		return h.auditPage(r.Context(), url, opts, routing.EffectiveCfg, targets)
	}

	report, err := audit.RunAudit(
		audit.AuditInput{URLs: req.URLs, SitemapURL: req.SitemapURL},
		audit.RunOptions{
			SampleSize:  req.SampleSize,
			Concurrency: req.Concurrency,
			Page:        req.Options.pageOptions(),
		},
		h.fetchSitemap(routing.EffectiveCfg),
		auditor,
	)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}
	httpx.JSON(w, 200, report)
}

// fetchSitemap returns a SitemapFetcher that applies the same URL/IDPI/SSRF
// validation as page navigation before fetching over plain HTTP.
func (h *Handlers) fetchSitemap(cfg *config.RuntimeConfig) audit.SitemapFetcher {
	return func(sitemapURL string) ([]string, error) {
		if _, err := h.validateAuditTarget(sitemapURL, cfg); err != nil {
			return nil, err
		}
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(sitemapURL)
		if err != nil {
			return nil, fmt.Errorf("fetch sitemap: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch sitemap: HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxSitemapBytes))
		if err != nil {
			return nil, fmt.Errorf("read sitemap: %w", err)
		}
		return audit.ParseSitemap(data)
	}
}
