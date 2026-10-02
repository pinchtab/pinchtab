package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/selector"
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
	TiedRefs     []string                `json:"tied_refs,omitempty"`
	RetryQuery   string                  `json:"retry_query,omitempty"`
	Score        float64                 `json:"score"`
	Matches      []semantic.ElementMatch `json:"matches"`
	Strategy     string                  `json:"strategy"`
	Threshold    float64                 `json:"threshold"`
	LatencyMs    int64                   `json:"latency_ms"`
	ElementCount int                     `json:"element_count"`
	IDPIWarning  string                  `json:"idpiWarning,omitempty"`
}

// HandleFind performs semantic element matching against the tab's live
// accessibility tree, re-read on every call.
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

	req, ok := parseFindRequest(w, r)
	if !ok {
		return
	}

	ctxTab, resolvedTabID, ok := h.guardedTabContext(w, r, req.TabID, guardDialogBlocked|guardDomainPolicy)
	if !ok {
		return
	}

	// Bound the snapshot/match work with the shared action timeout + client
	// disconnect cancel, matching the other read handlers (this path previously
	// ran the matcher on the raw request context with no ActionTimeout).
	tCtx, cancel := httpx.WithClientTimeout(r.Context(), ctxTab, h.Config.ActionTimeout)
	defer cancel()

	nodes, serr := h.acquireFindNodes(tCtx, resolvedTabID)
	if serr != nil {
		httpx.Error(w, serr.status, serr.err)
		return
	}
	h.publishTabVocab(w, resolvedTabID)

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
// override, a trimmed non-empty query, and Threshold/TopK defaults.
func parseFindRequest(w http.ResponseWriter, r *http.Request) (findRequest, bool) {
	var req findRequest
	if !decodeRequiredJSON(w, r, &req) {
		return findRequest{}, false
	}

	if pathID := r.PathValue("id"); pathID != "" {
		req.TabID = pathID
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		httpx.Error(w, 400, fmt.Errorf("missing required field 'query'"))
		return findRequest{}, false
	}
	if req.Threshold <= 0 {
		req.Threshold = 0.3
	}
	if req.TopK <= 0 {
		req.TopK = 3
	}
	return req, true
}

// resolveOrRefreshSnapshotNodes re-reads the live AX tree before every match, so
// a control an action just revealed is matchable and one it hid is not; a failed
// read falls back to the cached snapshot. EpochRefs keeps each surviving node's
// ref, so the refresh never renumbers a ref the caller already holds.
func (h *Handlers) resolveOrRefreshSnapshotNodes(ctx context.Context, tabID string) []bridge.A11yNode {
	h.refreshRefCache(ctx, tabID)
	return h.resolveSnapshotNodes(tabID)
}

func (h *Handlers) acquireFindNodes(ctx context.Context, resolvedTabID string) ([]bridge.A11yNode, *statusError) {
	nodes := h.resolveOrRefreshSnapshotNodes(ctx, resolvedTabID)
	if len(nodes) == 0 {
		return nil, &statusError{500, fmt.Errorf("no elements found in snapshot for tab %s — navigate first", resolvedTabID)}
	}
	return nodes, nil
}

// scanFindCorpusForIDPI scans the AX-node text corpus, full page body text and
// any extra caller-supplied strings for injection patterns before semantic matching. The interactive AX filter
// omits non-interactive elements (<p>, headings, etc.), so body.innerText is
// fetched as a bounded sub-operation to cover the full visible page. In strict
// mode a detected threat blocks the request (writes HTTP 403 and returns
// blocked=true); in warn mode the response headers and the returned warning
// carry the advisory. A no-op (returns "",false) when IDPI content scanning is
// disabled.
func (h *Handlers) scanFindCorpusForIDPI(w http.ResponseWriter, ctx context.Context, nodes []bridge.A11yNode, extra ...string) (warning string, blocked bool) {
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
	for _, s := range extra {
		sb.WriteByte('\n')
		sb.WriteString(s)
	}

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

const (
	confidenceAmbiguous = "ambiguous"
	tieScoreMargin      = 0.01
)

func tiedTopMatches(matches []semantic.ElementMatch) []semantic.ElementMatch {
	if len(matches) < 2 {
		return nil
	}
	top := matches[0]
	tied := []semantic.ElementMatch{top}
	for _, m := range matches[1:] {
		if sameRoleAndName(m, top) || top.Score-m.Score <= tieScoreMargin {
			tied = append(tied, m)
		}
	}
	if len(tied) < 2 {
		return nil
	}
	return tied
}

func sameRoleAndName(a, b semantic.ElementMatch) bool {
	return a.Role == b.Role && a.Name == b.Name
}

func matchRefs(matches []semantic.ElementMatch) []string {
	if matches == nil {
		return nil
	}
	refs := make([]string, len(matches))
	for i, m := range matches {
		refs[i] = m.Ref
	}
	return refs
}

func positionalRetryQuery(tied []semantic.ElementMatch) string {
	top := tied[0]
	for _, m := range tied[1:] {
		if !sameRoleAndName(m, top) {
			return ""
		}
	}
	role := selector.Selector{Kind: selector.KindRole, Value: top.Role + " " + top.Name}
	return selector.Selector{Kind: selector.KindNth, Value: "<i>:" + role.String()}.String()
}

func buildFindResponse(req findRequest, result semantic.FindResult, idpiWarning string, start time.Time) findResponse {
	confidence := result.ConfidenceLabel()
	retryQuery := ""
	tied := tiedTopMatches(result.Matches)
	if tied != nil {
		confidence = confidenceAmbiguous
		retryQuery = positionalRetryQuery(tied)
	}
	resp := findResponse{
		BestRef:      result.BestRef,
		Confidence:   confidence,
		TiedRefs:     matchRefs(tied),
		RetryQuery:   retryQuery,
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

func (h *Handlers) resolveSnapshotNodes(tabID string) []bridge.A11yNode {
	cache := h.Bridge.GetRefCache(tabID)
	if cache != nil && len(cache.Nodes) > 0 {
		return cache.Nodes
	}
	return nil
}
