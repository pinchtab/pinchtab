package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/audit"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const auditCollectTimeout = 60 * time.Second

type auditPageRequest struct {
	URL     string `json:"url"`
	Options *struct {
		Screenshot *bool `json:"screenshot"`
		Network    *bool `json:"network"`
		Console    *bool `json:"console"`
		A11y       *bool `json:"a11y"`
		Timing     *bool `json:"timing"`
		Elements   *bool `json:"elements"`
	} `json:"options"`
}

func (req auditPageRequest) pageOptions() audit.PageOptions {
	opts := audit.DefaultPageOptions()
	if req.Options == nil {
		return opts
	}
	apply := func(dst *bool, src *bool) {
		if src != nil {
			*dst = *src
		}
	}
	apply(&opts.Screenshot, req.Options.Screenshot)
	apply(&opts.Network, req.Options.Network)
	apply(&opts.Console, req.Options.Console)
	apply(&opts.A11y, req.Options.A11y)
	apply(&opts.Timing, req.Options.Timing)
	apply(&opts.Elements, req.Options.Elements)
	return opts
}

// @Endpoint POST /audit/page
func (h *Handlers) HandleAuditPage(w http.ResponseWriter, r *http.Request) {
	var req auditPageRequest
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
	targets, ok := h.validateNavigateTargets(w, r, "", req.URL, routing.EffectiveCfg)
	if !ok {
		return
	}
	if !h.ensureBrowserOrRespond(w, routing.EffectiveCfg) {
		return
	}

	tabID, tabCtx, _, err := h.Bridge.CreateTab("")
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("new tab: %w", err))
		return
	}
	defer func() { _ = h.Bridge.CloseTab(tabID) }()

	navTimeout := routing.EffectiveCfg.NavigateTimeout
	if navTimeout <= 0 {
		navTimeout = 30 * time.Second
	}
	navCtx, navCancel := context.WithTimeout(tabCtx, navTimeout)
	defer navCancel()

	navGuard, err := installNavigateRuntimeGuardWithBridge(h.Bridge, navCtx, navCancel, targets.target, targets.trustedCIDRs)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("navigation guard: %w", err))
		return
	}

	// Per-page failures are data, not crashes: navigation errors come back
	// as a structured entry with the error field set, never a 5xx.
	if _, navErr := h.Bridge.Navigate(navCtx, req.URL, bridge.NavigateParams{MaxRedirects: routing.EffectiveCfg.MaxRedirects}); navErr != nil {
		if navGuard != nil {
			if blockedErr := navGuard.blocked(); blockedErr != nil {
				navErr = blockedErr
			}
		}
		httpx.JSON(w, 200, audit.NewPageAuditError(req.URL, navErr))
		return
	}

	// Chrome renders net-level failures (connection refused, DNS) as an
	// error page without failing the navigation; detect it and report the
	// underlying net error from the network capture as page data.
	if cur, urlErr := h.Bridge.CurrentURL(navCtx); urlErr == nil && strings.HasPrefix(cur, "chrome-error://") {
		httpx.JSON(w, 200, audit.NewPageAuditError(req.URL, h.documentNetError(tabID, req.URL)))
		return
	}

	// Paint metrics only fire on visible pages.
	_ = h.Bridge.FocusTab(tabID)

	cCtx, cCancel := context.WithTimeout(tabCtx, auditCollectTimeout)
	defer cCancel()
	go httpx.CancelOnClientDone(r.Context(), cCancel)

	// Let late subresources (async fetches, images) land before collecting.
	_, _ = observe.WaitForQuietWindow(cCtx, 500*time.Millisecond, 5*time.Second)

	pageAudit := audit.EnrichPage(req.URL, req.pageOptions(), h.auditCollectors(cCtx, tabID))
	httpx.JSON(w, 200, pageAudit)
}

// documentNetError recovers the document request's net error from the
// network capture, falling back to a generic message.
func (h *Handlers) documentNetError(tabID, url string) error {
	if nm := h.Bridge.NetworkMonitor(); nm != nil {
		if buf := nm.GetBuffer(tabID); buf != nil {
			for _, e := range buf.List(bridge.NetworkFilter{}) {
				if e.URL == url && e.Failed && e.Error != "" {
					return fmt.Errorf("navigation failed: %s", e.Error)
				}
			}
		}
	}
	return fmt.Errorf("navigation failed: %s could not be loaded", url)
}

// auditCollectors wires the audit collectors to this tab's bridge data.
func (h *Handlers) auditCollectors(tCtx context.Context, tabID string) audit.Collectors {
	return audit.Collectors{
		Title: func() (string, error) {
			return h.Bridge.CurrentTitle(tCtx)
		},
		Screenshot: func() ([]byte, error) {
			return h.Bridge.CaptureScreenshot(tCtx, "png", 0, nil)
		},
		Console: func() ([]bridge.LogEntry, error) {
			return h.Bridge.GetConsoleLogs(tabID, 0), nil
		},
		Network: func() ([]observe.NetworkEntry, error) {
			nm := h.Bridge.NetworkMonitor()
			if nm == nil {
				return nil, nil
			}
			buf := nm.GetBuffer(tabID)
			if buf == nil {
				return nil, nil
			}
			return buf.List(bridge.NetworkFilter{}), nil
		},
		Snapshot: func() ([]observe.A11yNode, error) {
			rawNodes, err := bridge.FetchAXTree(tCtx)
			if err != nil {
				return nil, err
			}
			nodes, _ := bridge.BuildSnapshot(rawNodes, "", -1)
			return nodes, nil
		},
		PageFacts: func() (audit.PageFacts, error) {
			var facts audit.PageFacts
			err := h.Bridge.Evaluate(tCtx, audit.PageFactsScript, &facts, bridge.EvalOpts{})
			return facts, err
		},
		Timing: func() (*observe.TimingMetrics, error) {
			return observe.CollectTiming(func(expression string, result any) error {
				return h.Bridge.Evaluate(tCtx, expression, result, bridge.EvalOpts{AwaitPromise: true})
			})
		},
	}
}
