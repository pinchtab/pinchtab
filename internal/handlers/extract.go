package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/extract"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type extractRequest struct {
	TabID     string          `json:"tabId,omitempty"`
	Schema    json.RawMessage `json:"schema"`
	Scope     string          `json:"scope,omitempty"`
	Threshold float64         `json:"threshold,omitempty"`
	MaxItems  int             `json:"maxItems,omitempty"`
}

type extractResponse struct {
	Data            map[string]any                 `json:"data"`
	Fields          map[string]extract.FieldResult `json:"fields"`
	Missing         []string                       `json:"missing"`
	Truncated       bool                           `json:"truncated"`
	LatencyMs       int64                          `json:"latency_ms"`
	ElementCount    int                            `json:"element_count"`
	VocabularyToken string                         `json:"vocabularyToken,omitempty"`
	IDPIWarning     string                         `json:"idpiWarning,omitempty"`
}

// @Endpoint POST /extract
// @Description Extract schema-typed data from the page
//
// @Param schema object body JSON schema (object with properties; array-of-object properties allowed) (required)
// @Param tabId string body Tab ID (optional, defaults to active tab)
// @Param scope string body Confine every field to the subtree of one element: a ref, role:, text: or plain query; CSS and XPath are refused (optional)
// @Param threshold float body Minimum match score per field (optional, default: 0.3)
// @Param maxItems int body Cap on array items (optional, default: 100)
//
// @Response 200 application/json Typed data plus per-field ref, score and confidence, the vocabularyToken for follow-up ref actions (also in X-PinchTab-Vocab), and X-PinchTab-Tab-Id naming the resolved tab
// @Response 400 application/json Missing or unsupported schema, naming the offending path
// @Response 403 application/json IDPI strict mode blocked injected content on the page or in the extracted values
// @Response 404 application/json Tab not found
// @Response 409 application/json A JavaScript dialog is blocking the tab
// @Response 500 application/json Snapshot error
func (h *Handlers) HandleExtract(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureBrowser(h.Config); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("browser initialization: %w", err))
		return
	}

	var req extractRequest
	if !decodeRequiredJSON(w, r, &req) {
		return
	}
	schema, err := parseExtractSchema(req)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}

	ctxTab, resolvedTabID, ok := h.guardedTabContextWithHeader(w, r, req.TabID, guardDialogBlocked|guardDomainPolicy)
	if !ok {
		return
	}

	tCtx, cancel := context.WithTimeout(ctxTab, h.Config.ActionTimeout)
	defer cancel()
	go httpx.CancelOnClientDone(r.Context(), cancel)

	nodes, vocab, serr := h.acquireExtractNodes(w, tCtx, resolvedTabID)
	if serr != nil {
		httpx.Error(w, serr.status, serr.err)
		return
	}

	start := time.Now()
	result := extract.ResolveContext(tCtx, schema, nodes, extract.Options{Threshold: req.Threshold, MaxItems: req.MaxItems, Matcher: h.Matcher})

	idpiWarning, blocked := h.scanFindCorpusForIDPI(w, tCtx, nodes, extractedStrings(result.Data)...)
	if blocked {
		return
	}

	h.recordActivity(r, activity.Update{Action: "extract"})
	resp := buildExtractResponse(result, len(nodes), idpiWarning, start)
	resp.VocabularyToken = vocab
	httpx.JSON(w, 200, resp)
}

func (h *Handlers) acquireExtractNodes(w http.ResponseWriter, ctx context.Context, tabID string) ([]bridge.A11yNode, string, *statusError) {
	result, err := h.Bridge.Snapshot(ctx, tabID, "", bridge.ContentParams{})
	if err != nil {
		return nil, "", &statusError{500, fmt.Errorf("snapshot: %w", err)}
	}
	if len(result.Nodes) == 0 {
		return nil, "", &statusError{500, fmt.Errorf("no elements found in snapshot for tab %s — navigate first", tabID)}
	}
	cache := bridge.EpochRefs(h.Bridge.GetRefCache(tabID), result.Nodes)
	h.Bridge.SetRefCache(tabID, cache)
	publishVocab(w, tabID, cache.DomEpoch)
	return result.Nodes, cache.DomEpoch, nil
}

func extractedStrings(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case map[string]any:
		var out []string
		for _, e := range t {
			out = append(out, extractedStrings(e)...)
		}
		return out
	case []map[string]any:
		var out []string
		for _, e := range t {
			out = append(out, extractedStrings(e)...)
		}
		return out
	case []any:
		var out []string
		for _, e := range t {
			out = append(out, extractedStrings(e)...)
		}
		return out
	}
	return nil
}

func (h *Handlers) HandleTabExtract(w http.ResponseWriter, r *http.Request) {
	h.withPathTabIDBody(w, r, h.HandleExtract)
}

func parseExtractSchema(req extractRequest) (extract.Schema, error) {
	if len(req.Schema) == 0 {
		return extract.Schema{}, fmt.Errorf("missing required field 'schema'")
	}
	var quoted string
	if json.Unmarshal(req.Schema, &quoted) == nil {
		req.Schema = json.RawMessage(quoted)
	}
	schema, err := extract.ParseSchema(req.Schema)
	if err != nil {
		var unsupported *extract.UnsupportedError
		if errors.As(err, &unsupported) {
			return extract.Schema{}, fmt.Errorf("schema %s", err)
		}
		return extract.Schema{}, fmt.Errorf("schema: %w", err)
	}
	if schema, err = schema.WithScope(req.Scope); err != nil {
		return extract.Schema{}, err
	}
	return schema, nil
}

func buildExtractResponse(result extract.Result, elementCount int, idpiWarning string, start time.Time) extractResponse {
	resp := extractResponse{
		Data:         result.Data,
		Fields:       result.Fields,
		Missing:      result.Missing,
		LatencyMs:    time.Since(start).Milliseconds(),
		ElementCount: elementCount,
		IDPIWarning:  idpiWarning,
	}
	if resp.Missing == nil {
		resp.Missing = []string{}
	}
	for _, fr := range result.Fields {
		if fr.Truncated {
			resp.Truncated = true
		}
	}
	return resp
}
