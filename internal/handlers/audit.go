package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/audit"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type auditRequest struct {
	URLs             []string              `json:"urls"`
	SitemapURL       string                `json:"sitemapUrl"`
	SeaportalResults json.RawMessage       `json:"seaportalResults"`
	SeaportalFile    string                `json:"seaportalFile"`
	EnrichAll        bool                  `json:"enrichAll"`
	Options          *auditPageOptionsBody `json:"options"`
	Concurrency      int                   `json:"concurrency"`
	SampleSize       int                   `json:"sampleSize"`
}

// @Endpoint POST /audit
func (h *Handlers) HandleAudit(w http.ResponseWriter, r *http.Request) {
	var req auditRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	if len(req.URLs) == 0 && req.SitemapURL == "" && len(req.SeaportalResults) == 0 {
		httpx.Error(w, 400, fmt.Errorf("urls, sitemapUrl, or seaportalResults required"))
		return
	}

	var seaportalPages []audit.SeaportalPage
	if len(req.SeaportalResults) > 0 {
		var err error
		if seaportalPages, err = audit.ParseSeaportalReport(req.SeaportalResults); err != nil {
			httpx.Error(w, 400, err)
			return
		}
	}

	routing, ok := h.resolveNavigateBrowser(w, r, "", "")
	if !ok {
		return
	}
	if !h.ensureBrowserOrRespond(w, routing.EffectiveCfg) {
		return
	}

	// A multi-page audit legitimately outlives the server's WriteTimeout.
	httpx.ExtendWriteDeadline(w, auditRunTimeout)

	auditor := func(url string, opts audit.PageOptions) audit.PageAudit {
		targets, err := h.validateAuditTarget(url, routing.EffectiveCfg)
		if err != nil {
			return audit.NewPageAuditError(url, err)
		}
		return h.auditPage(r.Context(), url, opts, routing.EffectiveCfg, targets)
	}

	report, err := audit.RunAudit(
		audit.AuditInput{URLs: req.URLs, SitemapURL: req.SitemapURL, SeaportalFile: req.SeaportalFile},
		seaportalPages,
		audit.RunOptions{
			SampleSize:  req.SampleSize,
			Concurrency: req.Concurrency,
			EnrichAll:   req.EnrichAll,
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
// validation as page navigation, then discovers pages through
// seaportal.FlattenSitemap (recursive sitemap-index support). The crawl
// guard also gates every child-sitemap fetch inside the flattening.
func (h *Handlers) fetchSitemap(cfg *config.RuntimeConfig) audit.SitemapFetcher {
	return func(sitemapURL string) ([]string, error) {
		if _, err := h.validateAuditTarget(sitemapURL, cfg); err != nil {
			return nil, err
		}
		return audit.FlattenSitemapURLs(context.Background(), sitemapURL, h.crawlGuard(cfg).Policy())
	}
}
