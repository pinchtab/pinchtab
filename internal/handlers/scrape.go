package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/scrape"
)

const (
	scrapeRenderTimeout = 60 * time.Second
	// scrapeRunTimeout bounds a whole scrape run (HTTP crawl plus every
	// browser-rendered page) and is how far the response write deadline is
	// extended past the server's default WriteTimeout. Matches the CLI's
	// client-side scrape timeout.
	scrapeRunTimeout = 15 * time.Minute
)

type scrapeRequest struct {
	URL             string   `json:"url"`
	MaxPages        int      `json:"maxPages"`
	MaxPerPattern   int      `json:"maxPerPattern"`
	IncludePatterns []string `json:"includePatterns"`
	ExcludePatterns []string `json:"excludePatterns"`
	Concurrency     int      `json:"concurrency"`
	EnrichAll       bool     `json:"enrichAll"`
	NoBrowser       bool     `json:"noBrowser"`
	TimeoutSeconds  int      `json:"timeoutSeconds"`
	// Preview returns an outline (routing verdicts, per-page snippet and
	// charCount) without browser rendering or full page bodies.
	Preview bool `json:"preview"`
	// Only, when set, expands exactly these URLs at full fidelity instead of
	// discovering the site — the drill-down half of a preview.
	Only []string `json:"only"`
}

// @Endpoint POST /scrape
func (h *Handlers) HandleScrape(w http.ResponseWriter, r *http.Request) {
	var req scrapeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		httpx.Error(w, 400, fmt.Errorf("url required"))
		return
	}

	routing, ok := h.resolveNavigateBrowser(w, r, "", "")
	if !ok {
		return
	}
	if _, err := h.validateAuditTarget(req.URL, routing.EffectiveCfg); err != nil {
		httpx.Error(w, 400, err)
		return
	}
	for _, u := range req.Only {
		if strings.TrimSpace(u) == "" {
			continue
		}
		if _, err := h.validateAuditTarget(u, routing.EffectiveCfg); err != nil {
			httpx.Error(w, 400, fmt.Errorf("expand target %q: %w", u, err))
			return
		}
	}
	// A preview never renders, so it needs no browser; expand and full runs do.
	if !req.NoBrowser && !req.Preview && !h.ensureBrowserOrRespond(w, routing.EffectiveCfg) {
		return
	}

	// A multi-page run legitimately outlives the server's WriteTimeout;
	// extend the write deadline and bound the whole run to the same window.
	httpx.ExtendWriteDeadline(w, scrapeRunTimeout)
	runCtx, runCancel := context.WithTimeout(r.Context(), scrapeRunTimeout)
	defer runCancel()

	input := scrape.Input{
		URL:             req.URL,
		MaxPages:        req.MaxPages,
		MaxPerPattern:   req.MaxPerPattern,
		IncludePatterns: req.IncludePatterns,
		ExcludePatterns: req.ExcludePatterns,
	}
	guard := h.crawlGuard(routing.EffectiveCfg)
	renderer := func(url string) (string, error) {
		targets, err := h.validateAuditTarget(url, routing.EffectiveCfg)
		if err != nil {
			return "", err
		}
		return h.renderPageHTML(runCtx, url, routing.EffectiveCfg, targets)
	}

	// Expand a chosen URL set instead of discovering the site when --only is
	// given; otherwise crawl from the base URL.
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	crawler := scrape.SiteCrawler(input, timeout, guard)
	if len(req.Only) > 0 {
		crawler = scrape.URLListCrawler(req.Only, timeout, guard)
	}

	report, err := scrape.Run(
		runCtx,
		input,
		scrape.RunOptions{
			Concurrency: req.Concurrency,
			EnrichAll:   req.EnrichAll,
			NoBrowser:   req.NoBrowser,
			Preview:     req.Preview,
		},
		crawler,
		renderer,
	)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}
	httpx.JSON(w, 200, report)
}

// crawlGuard adapts this instance's navigation security stack (navguard
// resolution checks + IDPI domain rules + trusted CIDRs) into the guard the
// seaportal HTTP crawl applies to every fetch and redirect hop.
func (h *Handlers) crawlGuard(cfg *config.RuntimeConfig) scrape.CrawlGuard {
	return scrape.CrawlGuard{
		ValidateURL: func(url string) error {
			_, err := h.validateAuditTarget(url, cfg)
			return err
		},
		TrustedResolveCIDRs: cfg.TrustedResolveCIDRs,
		MaxRedirects:        cfg.MaxRedirects,
	}
}

// renderPageHTML navigates a fresh tab to url and returns the rendered
// document HTML after the page settles. The same navigation guard and
// error-page detection as auditPage apply; the tab is always closed.
func (h *Handlers) renderPageHTML(clientCtx context.Context, url string, cfg *config.RuntimeConfig, targets navTargets) (string, error) {
	tabID, tabCtx, _, err := h.Bridge.CreateTab("")
	if err != nil {
		return "", fmt.Errorf("new tab: %w", err)
	}
	defer func() { _ = h.Bridge.CloseTab(tabID) }()

	navTimeout := cfg.NavigateTimeout
	if navTimeout <= 0 {
		navTimeout = 30 * time.Second
	}
	navCtx, navCancel := context.WithTimeout(tabCtx, navTimeout)
	defer navCancel()

	navGuard, err := installNavigateRuntimeGuardWithBridge(h.Bridge, navCtx, navCancel, targets.target, targets.trustedCIDRs)
	if err != nil {
		return "", fmt.Errorf("navigation guard: %w", err)
	}

	if _, navErr := h.Bridge.Navigate(navCtx, url, bridge.NavigateParams{MaxRedirects: cfg.MaxRedirects}); navErr != nil {
		if navGuard != nil {
			if blockedErr := navGuard.blocked(); blockedErr != nil {
				navErr = blockedErr
			}
		}
		return "", navErr
	}

	if cur, urlErr := h.Bridge.CurrentURL(navCtx); urlErr == nil && strings.HasPrefix(cur, "chrome-error://") {
		return "", h.documentNetError(tabID, url)
	}

	cCtx, cCancel := context.WithTimeout(tabCtx, scrapeRenderTimeout)
	defer cCancel()
	go httpx.CancelOnClientDone(clientCtx, cCancel)

	// Let JS rendering and late subresources land before reading the DOM.
	_, _ = observe.WaitForQuietWindow(cCtx, 500*time.Millisecond, 5*time.Second)

	var html string
	if err := h.Bridge.Evaluate(cCtx, "document.documentElement.outerHTML", &html, bridge.EvalOpts{}); err != nil {
		return "", fmt.Errorf("read rendered html: %w", err)
	}
	return html, nil
}
