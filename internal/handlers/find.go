package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/semantic"
	"github.com/pinchtab/semantic/recovery"
)

type findRequest struct {
	Query           string  `json:"query"`
	TabID           string  `json:"tabId,omitempty"`
	Threshold       float64 `json:"threshold,omitempty"`
	TopK            int     `json:"topK,omitempty"`
	LexicalWeight   float64 `json:"lexicalWeight,omitempty"`
	EmbeddingWeight float64 `json:"embeddingWeight,omitempty"`
	Explain         bool    `json:"explain,omitempty"`
}

type findResponse struct {
	BestRef      string                  `json:"best_ref"`
	Confidence   string                  `json:"confidence"`
	Score        float64                 `json:"score"`
	Matches      []semantic.ElementMatch `json:"matches"`
	Strategy     string                  `json:"strategy"`
	Threshold    float64                 `json:"threshold"`
	LatencyMs    int64                   `json:"latency_ms"`
	ElementCount int                     `json:"element_count"`
	IDPIWarning  string                  `json:"idpiWarning,omitempty"`
}

// HandleFind performs semantic element matching against the accessibility
// snapshot for a tab. If no cached snapshot exists, it is fetched
// automatically via the existing snapshot infrastructure.
//
// @Endpoint POST /find
// @Description Find elements by natural language query
//
// @Param query string body Natural language description of the element (required)
// @Param tabId string body Tab ID (optional, defaults to active tab)
// @Param threshold float body Minimum similarity score (optional, default: 0.3)
// @Param topK int body Maximum results to return (optional, default: 3)
//
// @Response 200 application/json Returns matched elements with scores and metrics
// @Response 400 application/json Missing query
// @Response 404 application/json Tab not found
// @Response 500 application/json Snapshot or matching error
func (h *Handlers) HandleFind(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureChrome(); err != nil {
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req findRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if pathID := r.PathValue("id"); pathID != "" {
		req.TabID = pathID
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		httpx.Error(w, 400, fmt.Errorf("missing required field 'query'"))
		return
	}
	if req.Threshold <= 0 {
		req.Threshold = 0.3
	}
	if req.TopK <= 0 {
		req.TopK = 3
	}

	// Resolve tab context to get the resolved ID for cache lookup.
	// Keep ctxTab so we can reuse it for CDP operations (e.g. auto-refresh).
	ctxTab, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctxTab, resolvedTabID); !ok {
		return
	}

	// Try cached snapshot first; auto-fetch if not available.
	nodes := h.resolveSnapshotNodes(resolvedTabID)
	if len(nodes) == 0 {
		// Auto-refresh: take a fresh snapshot via CDP using the context
		// obtained from the initial TabContext call (resolvedTabID is the
		// raw CDPID and cannot be passed to TabContext again).
		h.refreshRefCache(ctxTab, resolvedTabID)
		nodes = h.resolveSnapshotNodes(resolvedTabID)
	}
	if len(nodes) == 0 {
		httpx.Error(w, 500, fmt.Errorf("no elements found in snapshot for tab %s — navigate first", resolvedTabID))
		return
	}

	// Build descriptors from A11yNodes.
	descs := make([]semantic.ElementDescriptor, len(nodes))
	for i, n := range nodes {
		descs[i] = semantic.ElementDescriptor{
			Ref:   n.Ref,
			Role:  n.Role,
			Name:  n.Name,
			Value: n.Value,
		}
	}

	// IDPI: scan AX-node text corpus and full page body text for injection
	// patterns before semantic matching. The interactive AX filter omits
	// non-interactive elements (<p>, headings, etc.), so body.innerText is
	// fetched as a 3-second sub-operation to cover the full visible page.
	// In strict mode a detected threat blocks the request (HTTP 403); in
	// warn mode the response headers and IDPIWarning field carry the advisory.
	var idpiWarning string
	if h.Config.IDPI.Enabled && h.Config.IDPI.ScanContent {
		var sb strings.Builder
		for _, n := range nodes {
			if n.Name != "" {
				sb.WriteString(n.Name)
				sb.WriteByte('\n')
			}
			if n.Value != "" {
				sb.WriteString(n.Value)
				sb.WriteByte('\n')
			}
		}
		// Augment with full body text to catch injection in non-interactive
		// content that the interactive AX filter omits (paragraphs, headings).
		scanTimeout := time.Duration(h.Config.IDPI.ScanTimeoutSec) * time.Second
		if scanTimeout <= 0 {
			scanTimeout = 5 * time.Second
		}
		var bodyText string
		scanCtx, scanCancel := context.WithTimeout(ctxTab, scanTimeout)
		_ = chromedp.Run(scanCtx, chromedp.Evaluate(`document.body ? document.body.innerText : ""`, &bodyText))
		scanCancel()
		sb.WriteString(bodyText)
		if corpus := sb.String(); corpus != "" {
			if ir := h.IDPIGuard.ScanContent(corpus); ir.Threat {
				if ir.Blocked {
					httpx.Error(w, http.StatusForbidden, fmt.Errorf("idpi: %s", ir.Reason))
					return
				}
				w.Header().Set("X-IDPI-Warning", ir.Reason)
				if ir.Pattern != "" {
					w.Header().Set("X-IDPI-Pattern", ir.Pattern)
				}
				idpiWarning = ir.Reason
			}
		}
	}

	start := time.Now()
	result, err := h.Matcher.Find(r.Context(), req.Query, descs, semantic.FindOptions{
		Threshold:       req.Threshold,
		TopK:            req.TopK,
		LexicalWeight:   req.LexicalWeight,
		EmbeddingWeight: req.EmbeddingWeight,
		Explain:         req.Explain,
	})
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("matcher error: %w", err))
		return
	}

	resp := findResponse{
		BestRef:      result.BestRef,
		Confidence:   result.ConfidenceLabel(),
		Score:        result.BestScore,
		Matches:      result.Matches,
		Strategy:     result.Strategy,
		Threshold:    req.Threshold,
		LatencyMs:    time.Since(start).Milliseconds(),
		ElementCount: result.ElementCount,
		IDPIWarning:  idpiWarning,
	}
	if resp.Matches == nil {
		resp.Matches = []semantic.ElementMatch{}
	}

	// Cache intent for recovery: store the query + best-match descriptor
	// so the recovery engine can reconstruct a search if the ref goes stale.
	if result.BestRef != "" && h.Recovery != nil {
		var bestDesc semantic.ElementDescriptor
		for _, d := range descs {
			if d.Ref == result.BestRef {
				bestDesc = d
				break
			}
		}
		h.Recovery.RecordIntent(resolvedTabID, result.BestRef, recovery.IntentEntry{
			Query:      req.Query,
			Descriptor: bestDesc,
			Score:      result.BestScore,
			Confidence: resp.Confidence,
			Strategy:   result.Strategy,
			CachedAt:   time.Now(),
		})
	}

	httpx.JSON(w, 200, resp)
}

// resolveSnapshotNodes returns cached A11yNodes for the tab, or an empty
// slice if no cache is available. The caller should use refreshRefCache
// to auto-fetch a fresh snapshot via CDP when this returns nil.
func (h *Handlers) resolveSnapshotNodes(tabID string) []bridge.A11yNode {
	cache := h.Bridge.GetRefCache(tabID)
	if cache != nil && len(cache.Nodes) > 0 {
		return cache.Nodes
	}
	return nil
}
