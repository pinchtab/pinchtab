package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/semantic"
)

// findMockBridge implements the subset of bridge.BridgeAPI required by HandleFind.
type findMockBridge struct {
	bridge.BridgeAPI
	failTab  bool
	refCache *bridge.RefCache
}

func (m *findMockBridge) EnsureBrowser(cfg *config.RuntimeConfig) error  { return nil }
func (m *findMockBridge) RestartBrowser(cfg *config.RuntimeConfig) error { return nil }

func (m *findMockBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	if m.failTab {
		return nil, "", fmt.Errorf("tab not found")
	}
	return bridge.NewTabHandle(context.Background()), "tab1", nil
}

func (m *findMockBridge) ListTargets() ([]bridge.TabTarget, error) {
	return []bridge.TabTarget{{TargetID: "tab1", Type: "page"}}, nil
}

func (m *findMockBridge) GetRefCache(tabID string) *bridge.RefCache {
	return m.refCache
}

func (m *findMockBridge) SetRefCache(tabID string, cache *bridge.RefCache) {}
func (m *findMockBridge) DeleteRefCache(tabID string)                      {}
func (m *findMockBridge) AvailableActions() []string                       { return nil }
func (m *findMockBridge) TabLockInfo(tabID string) *bridge.LockInfo        { return nil }
func (m *findMockBridge) GetCrashLogs() []string                           { return nil }
func (m *findMockBridge) NetworkMonitor() *bridge.NetworkMonitor           { return nil }

func (m *findMockBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	return nil, nil
}
func (m *findMockBridge) GetMemoryMetrics(tabID string) (*bridge.MemoryMetrics, error) {
	return &bridge.MemoryMetrics{}, nil
}
func (m *findMockBridge) GetBrowserMemoryMetrics() (*bridge.MemoryMetrics, error) {
	return &bridge.MemoryMetrics{}, nil
}
func (m *findMockBridge) GetAggregatedMemoryMetrics() (*bridge.MemoryMetrics, error) {
	return &bridge.MemoryMetrics{}, nil
}
func (m *findMockBridge) Evaluate(ctx context.Context, expression string, result any, opts bridge.EvalOpts) error {
	return nil
}

func (m *findMockBridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	return task(ctx)
}

func (m *findMockBridge) CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return fmt.Errorf("not implemented")
}

func (m *findMockBridge) EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts bridge.EvalOpts) error {
	return fmt.Errorf("not implemented")
}

func (m *findMockBridge) DescribeNode(ctx context.Context, backendNodeID int64) (*bridge.NodeInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func newFindTestHandler(cache *bridge.RefCache, failTab bool) *Handlers {
	mb := &findMockBridge{
		failTab:  failTab,
		refCache: cache,
	}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: 10 * time.Second}, nil, nil, nil)
	h.Matcher = semantic.NewLexicalMatcher()
	return h
}

func TestHandleFind_BasicMatch(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Log In"},
			{Ref: "e1", Role: "link", Name: "Sign Up"},
			{Ref: "e2", Role: "textbox", Name: "Email"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2, "e2": 3},
	}

	h := newFindTestHandler(cache, false)

	body := `{"query": "log in button", "threshold": 0.1, "topK": 3}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp findResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.BestRef != "e0" {
		t.Errorf("expected best_ref=e0, got %s", resp.BestRef)
	}
	if resp.Score <= 0 {
		t.Errorf("expected positive score, got %f", resp.Score)
	}
	if resp.Strategy != "lexical" {
		t.Errorf("expected strategy=lexical, got %s", resp.Strategy)
	}
	if len(resp.Matches) == 0 {
		t.Error("expected at least one match")
	}
}

func TestHandleFind_NoStrongMatch(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Log In"},
			{Ref: "e1", Role: "link", Name: "Sign Up"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2},
	}

	h := newFindTestHandler(cache, false)

	// Query with no semantic overlap to existing elements.
	body := `{"query": "download pdf report", "threshold": 0.3, "topK": 3}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp findResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// With a high-enough threshold, no matches should survive.
	if resp.Confidence != "low" {
		t.Errorf("expected confidence=low, got %s", resp.Confidence)
	}
}

func TestHandleFind_ThresholdFiltering(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Submit"},
			{Ref: "e1", Role: "link", Name: "Home"},
			{Ref: "e2", Role: "textbox", Name: "Search"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2, "e2": 3},
	}

	h := newFindTestHandler(cache, false)

	// High threshold should filter out weak matches.
	body := `{"query": "submit button", "threshold": 0.9, "topK": 5}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp findResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// All matches must meet the threshold.
	for _, m := range resp.Matches {
		if m.Score < 0.9 {
			t.Errorf("match %s has score %f below threshold 0.9", m.Ref, m.Score)
		}
	}
}

func TestHandleFind_MissingQuery(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{{Ref: "e0", Role: "button", Name: "OK"}},
		Refs:  map[string]int64{"e0": 1},
	}

	h := newFindTestHandler(cache, false)

	body := `{"threshold": 0.5}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing query, got %d", w.Code)
	}
}

func TestHandleFind_NoSnapshot(t *testing.T) {
	h := newFindTestHandler(nil, false) // nil cache

	body := `{"query": "login"}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for missing snapshot, got %d", w.Code)
	}
}

func TestHandleFind_TabNotFound(t *testing.T) {
	h := newFindTestHandler(nil, true) // failTab = true

	body := `{"query": "login"}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing tab, got %d", w.Code)
	}
}

func TestHandleFind_RouteRegistered(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{{Ref: "e0", Role: "button", Name: "OK"}},
		Refs:  map[string]int64{"e0": 1},
	}
	h := newFindTestHandler(cache, false)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	body := `{"query": "button"}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 from registered /find route, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleFind_ResponseMetrics(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Submit"},
			{Ref: "e1", Role: "link", Name: "Home"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2},
	}
	h := newFindTestHandler(cache, false)

	body := `{"query": "submit button", "threshold": 0.2, "topK": 3}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp findResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.ElementCount != 2 {
		t.Errorf("expected element_count=2, got %d", resp.ElementCount)
	}
	if resp.Threshold != 0.2 {
		t.Errorf("expected threshold=0.2, got %f", resp.Threshold)
	}
	if resp.LatencyMs < 0 {
		t.Errorf("expected non-negative latency_ms, got %d", resp.LatencyMs)
	}
	if resp.Strategy != "lexical" {
		t.Errorf("expected strategy=lexical, got %s", resp.Strategy)
	}
}

func TestHandleFind_NegativeQuery(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Submit"},
			{Ref: "e1", Role: "button", Name: "Cancel"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2},
	}

	h := newFindTestHandler(cache, false)

	body := `{"query": "button not submit", "threshold": 0.0, "topK": 3}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp findResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.BestRef != "e1" {
		t.Errorf("expected best_ref=e1 (Cancel) for negative query, got %s", resp.BestRef)
	}
}

func TestHandleFind_VisualQuery_BottomButton(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Action"},
			{Ref: "e1", Role: "button", Name: "Action"},
			{Ref: "e2", Role: "button", Name: "Action"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2, "e2": 3},
	}

	mb := &findMockBridge{refCache: cache}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: 10 * time.Second}, nil, nil, nil)
	h.Matcher = semantic.NewCombinedMatcher(semantic.NewHashingEmbedder(128))

	body := `{"query": "bottom button", "threshold": 0.0, "topK": 3}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp findResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.BestRef != "e2" {
		t.Errorf("expected best_ref=e2 (last in document order) for 'bottom button', got %s", resp.BestRef)
	}
}

func TestHandleFind_OrdinalQuery_LastButton(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Action"},
			{Ref: "e1", Role: "button", Name: "Action"},
			{Ref: "e2", Role: "button", Name: "Action"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2, "e2": 3},
	}

	mb := &findMockBridge{refCache: cache}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: 10 * time.Second}, nil, nil, nil)
	h.Matcher = semantic.NewCombinedMatcher(semantic.NewHashingEmbedder(128))

	body := `{"query": "last button", "threshold": 0.0, "topK": 3}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp findResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.BestRef != "e2" {
		t.Errorf("expected best_ref=e2 for 'last button', got %s", resp.BestRef)
	}
}

func TestSemanticDescriptorsFromNodes_EnrichesContext(t *testing.T) {
	nodes := []bridge.A11yNode{
		{Ref: "e0", Role: "form", Name: "Login", Depth: 0},
		{Ref: "e1", Role: "textbox", Name: "Email", Depth: 1, Label: "Work Email", Placeholder: "Email address", TestID: "email-input", Text: "Email", Tag: "input"},
		{Ref: "e2", Role: "button", Name: "Submit", Depth: 1},
	}

	descs := semanticDescriptorsFromNodes(nodes)
	if len(descs) != 3 {
		t.Fatalf("expected 3 descriptors, got %d", len(descs))
	}
	if !descs[1].Interactive {
		t.Error("expected textbox descriptor to be interactive")
	}
	if descs[1].DocumentIdx != 1 {
		t.Errorf("DocumentIdx = %d, want 1", descs[1].DocumentIdx)
	}
	if descs[1].Parent != "form: Login" {
		t.Errorf("Parent = %q, want form context", descs[1].Parent)
	}
	if descs[1].Section != "form: Login" {
		t.Errorf("Section = %q, want form context", descs[1].Section)
	}
	if descs[1].Positional.Depth != 1 {
		t.Errorf("Positional.Depth = %d, want 1", descs[1].Positional.Depth)
	}
	if descs[1].Positional.SiblingIndex != 0 || descs[1].Positional.SiblingCount != 2 {
		t.Errorf("sibling metadata = %d/%d, want 0/2", descs[1].Positional.SiblingIndex, descs[1].Positional.SiblingCount)
	}
	if descs[1].Positional.LabelledBy != "Login" {
		t.Errorf("LabelledBy = %q, want Login", descs[1].Positional.LabelledBy)
	}
	if descs[1].Label != "Work Email" || descs[1].Placeholder != "Email address" || descs[1].TestID != "email-input" || descs[1].Text != "Email" || descs[1].Tag != "input" {
		t.Errorf("DOM-backed fields were not copied into semantic descriptor: %+v", descs[1])
	}
}

func TestApplySemanticActionSelector_UsesStructuredLocator(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "textbox", Name: "Input", Label: "Work Email", NodeID: 42},
		},
		Targets: map[string]bridge.RefTarget{
			"e0": {BackendNodeID: 42},
		},
	}
	mb := &findMockBridge{refCache: cache}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: 10 * time.Second}, nil, nil, nil)
	h.Matcher = semantic.NewCombinedMatcher(semantic.NewHashingEmbedder(128))

	req := bridge.ActionRequest{Selector: "label:Work Email"}
	handled, err := h.applySemanticActionSelector(context.Background(), "tab1", selector.Parse(req.Selector), &req)
	if err != nil {
		t.Fatalf("applySemanticActionSelector returned error: %v", err)
	}
	if !handled {
		t.Fatal("expected structured locator to be handled by semantic")
	}
	if req.Ref != "e0" || req.NodeID != 42 || req.Selector != "" {
		t.Fatalf("resolved request = %+v, want ref e0 node 42 and cleared selector", req)
	}
}

func TestHandleFind_EmbeddingMatcher(t *testing.T) {
	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Login"},
			{Ref: "e1", Role: "textbox", Name: "Username"},
			{Ref: "e2", Role: "link", Name: "Forgot Password"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2, "e2": 3},
	}

	mb := &findMockBridge{refCache: cache}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: 10 * time.Second}, nil, nil, nil)
	h.Matcher = semantic.NewEmbeddingMatcher(semantic.NewHashingEmbedder(64))

	body := `{"query": "login button", "threshold": 0.0, "topK": 3}`
	req := httptest.NewRequest("POST", "/find", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleFind(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp findResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !strings.HasPrefix(resp.Strategy, "embedding:") {
		t.Errorf("expected strategy prefix 'embedding:', got %s", resp.Strategy)
	}
	if resp.ElementCount != 3 {
		t.Errorf("expected element_count=3, got %d", resp.ElementCount)
	}
	if len(resp.Matches) == 0 {
		t.Error("expected at least one match from embedding matcher")
	}
}
