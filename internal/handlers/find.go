package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
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
	if err := h.ensureBrowser(h.Config); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("browser initialization: %w", err))
		return
	}

	req, err := parseFindRequest(w, r)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}

	// Resolve tab context to get the resolved ID for cache lookup.
	// Keep ctxTab so we can reuse it for CDP operations (e.g. auto-refresh).
	ctxTab, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctxTab, resolvedTabID); !ok {
		return
	}

	// Bound the snapshot/match work with the shared action timeout + client
	// disconnect cancel, matching the other read handlers (this path previously
	// ran the matcher on the raw request context with no ActionTimeout).
	tCtx, cancel := context.WithTimeout(ctxTab, h.Config.ActionTimeout)
	defer cancel()
	go httpx.CancelOnClientDone(r.Context(), cancel)

	nodes, serr := h.acquireFindNodes(tCtx, resolvedTabID)
	if serr != nil {
		httpx.Error(w, serr.status, serr.err)
		return
	}

	descs := semanticDescriptorsFromNodes(nodes)

	idpiWarning, blocked := h.scanFindCorpusForIDPI(w, tCtx, nodes)
	if blocked {
		return
	}

	start := time.Now()
	result, err := h.Matcher.Find(tCtx, req.Query, descs, semantic.FindOptions{
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

	resp := buildFindResponse(req, result, idpiWarning, start)

	h.recordActivity(r, activity.Update{Action: "find"})
	h.recordFindIntent(resolvedTabID, req, result, descs, resp.Confidence)

	httpx.JSON(w, 200, resp)
}

// parseFindRequest decodes and normalizes the find request: the {id} path
// override, a trimmed non-empty query, and Threshold/TopK defaults. Returns an
// error (mapped to 400 by the caller) on a decode failure or a missing query.
func parseFindRequest(w http.ResponseWriter, r *http.Request) (findRequest, error) {
	var req findRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		return findRequest{}, fmt.Errorf("decode: %w", err)
	}

	if pathID := r.PathValue("id"); pathID != "" {
		req.TabID = pathID
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return findRequest{}, fmt.Errorf("missing required field 'query'")
	}
	if req.Threshold <= 0 {
		req.Threshold = 0.3
	}
	if req.TopK <= 0 {
		req.TopK = 3
	}
	return req, nil
}

// resolveOrRefreshSnapshotNodes returns the tab's cached snapshot AX nodes,
// taking one CDP ref-cache refresh when the cache is empty. It is the shared
// resolve-or-refresh prefix of /find (acquireFindNodes) and semantic-selector
// descriptor acquisition (semanticDescriptorsForTabInFrame); callers apply their
// own empty-handling, DOM-metadata enrichment, and frame scoping on top.
func (h *Handlers) resolveOrRefreshSnapshotNodes(ctx context.Context, tabID string) []bridge.A11yNode {
	nodes := h.resolveSnapshotNodes(tabID)
	if len(nodes) == 0 {
		// Auto-refresh: take a fresh snapshot via CDP using the context obtained
		// from the initial TabContext call (tabID is the raw CDPID and cannot be
		// passed to TabContext again).
		h.refreshRefCache(ctx, tabID)
		nodes = h.resolveSnapshotNodes(tabID)
	}
	return nodes
}

// acquireFindNodes returns the tab's snapshot AX nodes (auto-refreshing the ref
// cache once via CDP if the cache is empty) enriched with DOM metadata, or a 500
// status error when the page still has no elements.
func (h *Handlers) acquireFindNodes(ctx context.Context, resolvedTabID string) ([]bridge.A11yNode, *statusError) {
	nodes := h.resolveOrRefreshSnapshotNodes(ctx, resolvedTabID)
	if len(nodes) == 0 {
		return nil, &statusError{500, fmt.Errorf("no elements found in snapshot for tab %s — navigate first", resolvedTabID)}
	}
	_ = bridge.EnrichA11yNodesWithDOMMetadata(ctx, nodes)
	return nodes, nil
}

// scanFindCorpusForIDPI scans the AX-node text corpus plus full page body text
// for injection patterns before semantic matching. The interactive AX filter
// omits non-interactive elements (<p>, headings, etc.), so body.innerText is
// fetched as a bounded sub-operation to cover the full visible page. In strict
// mode a detected threat blocks the request (writes HTTP 403 and returns
// blocked=true); in warn mode the response headers and the returned warning
// carry the advisory. A no-op (returns "",false) when IDPI content scanning is
// disabled.
func (h *Handlers) scanFindCorpusForIDPI(w http.ResponseWriter, ctx context.Context, nodes []bridge.A11yNode) (warning string, blocked bool) {
	if !h.Config.IDPI.Enabled || !h.Config.IDPI.ScanContent {
		return "", false
	}

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
	scanTimeout := time.Duration(h.Config.IDPI.ScanTimeoutSec) * time.Second
	if scanTimeout <= 0 {
		scanTimeout = 5 * time.Second
	}
	var bodyText string
	scanCtx, scanCancel := context.WithTimeout(ctx, scanTimeout)
	_ = h.Bridge.Evaluate(scanCtx, `document.body ? document.body.innerText : ""`, &bodyText, bridge.EvalOpts{})
	scanCancel()
	sb.WriteString(bodyText)

	corpus := sb.String()
	if corpus == "" {
		return "", false
	}
	scanResult := h.ContentGuard.ScanOnly(corpus)
	if scanResult.Blocked {
		httpx.Error(w, http.StatusForbidden, fmt.Errorf("idpi: %s", scanResult.BlockReason))
		return "", true
	}
	scanResult.SetHeaders(w)
	return scanResult.Warning, false
}

func buildFindResponse(req findRequest, result semantic.FindResult, idpiWarning string, start time.Time) findResponse {
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
	return resp
}

// recordFindIntent caches the query + best-match descriptor so the recovery
// engine can reconstruct a search if the ref later goes stale. No-op when there
// is no best ref or no recovery engine.
func (h *Handlers) recordFindIntent(resolvedTabID string, req findRequest, result semantic.FindResult, descs []semantic.ElementDescriptor, confidence string) {
	if result.BestRef == "" || h.Recovery == nil {
		return
	}
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
		Confidence: confidence,
		Strategy:   result.Strategy,
		CachedAt:   time.Now(),
	})
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
