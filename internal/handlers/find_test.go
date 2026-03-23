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

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/semantic"
)

// findMockBridge implements the subset of bridge.BridgeAPI required by HandleFind.
type findMockBridge struct {
	bridge.BridgeAPI
	failTab  bool
	refCache *bridge.RefCache
}

func (m *findMockBridge) EnsureChrome(cfg *config.RuntimeConfig) error { return nil }

func (m *findMockBridge) TabContext(tabID string) (context.Context, string, error) {
	if m.failTab {
		return nil, "", fmt.Errorf("tab not found")
	}
	return context.Background(), "tab1", nil
}

func (m *findMockBridge) ListTargets() ([]*target.Info, error) {
	return []*target.Info{{TargetID: "tab1", Type: "page"}}, nil
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
func (m *findMockBridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	return task(ctx)
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

	// Verify new Phase 2 response fields.
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
