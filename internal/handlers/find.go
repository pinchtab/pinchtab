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
	"github.com/pinchtab/pinchtab/internal/semantic"
	"github.com/pinchtab/pinchtab/internal/web"
)

type findRequest struct {
	Query           string  `json:"query"`
	TabID           string  `json:"tabId,omitempty"`
	URL             string  `json:"url,omitempty"`
	WaitFor         string  `json:"waitFor,omitempty"`
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
	TabID        string                  `json:"tabId"`
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
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req findRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		web.Error(w, 400, fmt.Errorf("missing required field 'query'"))
		return
	}
	if req.Threshold <= 0 {
		req.Threshold = 0.3
	}
	if req.TopK <= 0 {
		req.TopK = 3
	}

	// Resolve tab context to get the resolved ID for cache lookup.
	ctx, resolvedTabID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}

	// Navigate if URL provided.
	if req.URL != "" {
		tCtx, tCancel := context.WithTimeout(ctx, h.Config.NavigateTimeout)
		defer tCancel()
		if err := h.ensureNavigated(tCtx, req.URL, req.WaitFor, WaitDOM); err != nil {
			web.Error(w, 500, fmt.Errorf("navigate: %w", err))
			return
		}
	}

	// Always take a fresh snapshot for find — ensures cache is populated.
	h.refreshSnapshot(ctx, resolvedTabID)

	nodes := h.resolveSnapshotNodes(resolvedTabID)
	if len(nodes) == 0 {
		web.Error(w, 500, fmt.Errorf("no elements found in snapshot for tab %s", resolvedTabID))
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

	start := time.Now()
	result, err := h.Matcher.Find(r.Context(), req.Query, descs, semantic.FindOptions{
		Threshold:       req.Threshold,
		TopK:            req.TopK,
		LexicalWeight:   req.LexicalWeight,
		EmbeddingWeight: req.EmbeddingWeight,
		Explain:         req.Explain,
	})
	if err != nil {
		web.Error(w, 500, fmt.Errorf("matcher error: %w", err))
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
		TabID:        resolvedTabID,
	}
	if resp.Matches == nil {
		resp.Matches = []semantic.ElementMatch{}
	}

	web.JSON(w, 200, resp)
}

// resolveSnapshotNodes returns cached A11yNodes for the tab.
func (h *Handlers) resolveSnapshotNodes(tabID string) []bridge.A11yNode {
	cache := h.Bridge.GetRefCache(tabID)
	if cache != nil && len(cache.Nodes) > 0 {
		return cache.Nodes
	}
	return nil
}

// refreshSnapshot takes a fresh accessibility snapshot and caches it.
// Used by /find to ensure the ref cache is populated before searching.
func (h *Handlers) refreshSnapshot(ctx context.Context, tabID string) {
	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()

	var rawResult json.RawMessage
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx,
				"Accessibility.getFullAXTree", nil, &rawResult)
		}),
	); err != nil {
		return
	}

	var treeResp struct {
		Nodes []bridge.RawAXNode `json:"nodes"`
	}
	if err := json.Unmarshal(rawResult, &treeResp); err != nil {
		return
	}

	flat, refs := bridge.BuildSnapshot(treeResp.Nodes, "", -1)
	h.Bridge.SetRefCache(tabID, &bridge.RefCache{Refs: refs, Nodes: flat})
}
