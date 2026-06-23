package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type countFrameBridge struct {
	mockBridge
	lastFrameID    string
	lastExpression string
}

func (b *countFrameBridge) EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts bridge.EvalOpts) error {
	b.lastFrameID = frameID
	b.lastExpression = expression
	if ptr, ok := result.(*int); ok {
		*ptr = 7
	}
	return nil
}

func TestHandleCount_MissingSelector(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/count", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "selector") {
		t.Fatalf("expected error about selector, got %s", w.Body.String())
	}
}

func TestHandleCount_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/count?selector=button", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCount_ValidCount(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	h.evalRuntime = func(ctx context.Context, expression string, out any, opts bridge.EvalOpts) error {
		if !strings.Contains(expression, "document.querySelectorAll") {
			t.Fatalf("expected querySelectorAll expression, got %s", expression)
		}
		if ptr, ok := out.(*int); ok {
			*ptr = 5
		}
		return nil
	}

	req := httptest.NewRequest("GET", "/count?selector=button.submit", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp countResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Selector != "button.submit" {
		t.Fatalf("expected selector button.submit, got %s", resp.Selector)
	}
	if resp.Count != 5 {
		t.Fatalf("expected count 5, got %d", resp.Count)
	}
}

func TestHandleCount_ExplicitCSSPrefixUsesCSSValue(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	h.evalRuntime = func(ctx context.Context, expression string, out any, opts bridge.EvalOpts) error {
		if strings.Contains(expression, "css:button.submit") {
			t.Fatalf("expected css: prefix to be stripped, got %s", expression)
		}
		if !strings.Contains(expression, "button.submit") {
			t.Fatalf("expected CSS selector in expression, got %s", expression)
		}
		if ptr, ok := out.(*int); ok {
			*ptr = 3
		}
		return nil
	}

	req := httptest.NewRequest("GET", "/count?selector=css:button.submit", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCount_UsesFrameScopedEvaluationWhenFrameSelected(t *testing.T) {
	b := &countFrameBridge{}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)
	h.evalRuntime = func(ctx context.Context, expression string, out any, opts bridge.EvalOpts) error {
		return fmt.Errorf("top-level eval should not run for frame-scoped count")
	}
	b.SetFrameScope("tab1", bridge.FrameScope{FrameID: "frame-123"})

	req := httptest.NewRequest("GET", "/count?selector=button.submit", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if b.lastFrameID != "frame-123" {
		t.Fatalf("expected frame-scoped evaluation, got frame %q", b.lastFrameID)
	}
	if !strings.Contains(b.lastExpression, "button.submit") {
		t.Fatalf("expected frame-scoped expression to contain selector, got %s", b.lastExpression)
	}
}

// captureExprBridge records the last JS expression evaluated and lets tests
// stub the returned count, for both the top-level and frame-scoped paths.
type captureExprBridge struct {
	mockBridge
	lastFrameID    string
	lastExpression string
	returnCount    int
}

func (b *captureExprBridge) EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts bridge.EvalOpts) error {
	b.lastFrameID = frameID
	b.lastExpression = expression
	if ptr, ok := result.(*int); ok {
		*ptr = b.returnCount
	}
	return nil
}

func newCountHarness(t *testing.T, returnCount int) (*Handlers, *string) {
	t.Helper()
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	var lastExpr string
	h.evalRuntime = func(ctx context.Context, expression string, out any, opts bridge.EvalOpts) error {
		lastExpr = expression
		if ptr, ok := out.(*int); ok {
			*ptr = returnCount
		}
		return nil
	}
	return h, &lastExpr
}

func TestHandleCount_XPathUsesSnapshotLength(t *testing.T) {
	h, lastExpr := newCountHarness(t, 4)
	req := httptest.NewRequest("GET", "/count?selector="+url.QueryEscape("xpath://div[@class='row']"), nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(*lastExpr, "document.evaluate(") || !strings.Contains(*lastExpr, "snapshotLength") {
		t.Fatalf("expected xpath snapshotLength expression, got %s", *lastExpr)
	}
	if strings.Contains(*lastExpr, "xpath:") {
		t.Fatalf("expected xpath: prefix stripped, got %s", *lastExpr)
	}
	if !strings.Contains(*lastExpr, "div[@class='row']") {
		t.Fatalf("expected xpath value in expression, got %s", *lastExpr)
	}
	var resp countResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 4 {
		t.Fatalf("expected count 4, got %d", resp.Count)
	}
}

func TestHandleCount_XPathHonorsFrameScope(t *testing.T) {
	b := &captureExprBridge{returnCount: 2}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)
	h.evalRuntime = func(ctx context.Context, expression string, out any, opts bridge.EvalOpts) error {
		return fmt.Errorf("top-level eval should not run for frame-scoped count")
	}
	b.SetFrameScope("tab1", bridge.FrameScope{FrameID: "frame-xyz"})

	req := httptest.NewRequest("GET", "/count?selector=xpath://a", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if b.lastFrameID != "frame-xyz" {
		t.Fatalf("expected frame-scoped evaluation, got frame %q", b.lastFrameID)
	}
	if !strings.Contains(b.lastExpression, "snapshotLength") {
		t.Fatalf("expected snapshotLength expression in frame, got %s", b.lastExpression)
	}
}

func TestHandleCount_TextUsesLeafMostCandidateCount(t *testing.T) {
	h, lastExpr := newCountHarness(t, 3)
	req := httptest.NewRequest("GET", "/count?selector="+url.QueryEscape("text:Sign In"), nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// The text count reuses the leaf-most candidate logic: it should normalize,
	// collect candidates, and return their length (not call querySelectorAll
	// with the raw "text:..." string).
	for _, want := range []string{"normalize", "getElementsByTagName", "minSize", `"Sign In"`} {
		if !strings.Contains(*lastExpr, want) {
			t.Fatalf("expected text count expression to contain %q, got %s", want, *lastExpr)
		}
	}
	if strings.Contains(*lastExpr, "querySelectorAll(\"text:") || strings.Contains(*lastExpr, "text:Sign In") {
		t.Fatalf("expected text: prefix stripped and no raw querySelectorAll, got %s", *lastExpr)
	}
	var resp countResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 3 {
		t.Fatalf("expected count 3, got %d", resp.Count)
	}
	if resp.Selector != "text:Sign In" {
		t.Fatalf("expected response selector to echo input, got %s", resp.Selector)
	}
}

func TestHandleCount_RefSingleNodeNotFoundIsZero(t *testing.T) {
	// mockBridge.GetRefCache returns nil, so a ref never resolves: count is 0,
	// not an error and not a 404.
	h, _ := newCountHarness(t, 99)
	req := httptest.NewRequest("GET", "/count?selector=e7", nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp countResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 0 {
		t.Fatalf("expected ref count 0 when unresolved, got %d", resp.Count)
	}
}

func TestHandleCount_SemanticWithoutMatcherReturnsNotConfigured(t *testing.T) {
	h, _ := newCountHarness(t, 0)
	h.Matcher = nil
	req := httptest.NewRequest("GET", "/count?selector="+url.QueryEscape("role:button Save"), nil)
	w := httptest.NewRecorder()
	h.HandleCount(w, req)
	if w.Code == http.StatusOK {
		t.Fatalf("expected error status, got 200: %s", w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "not configured") {
		t.Fatalf("expected 'not configured' error, got %s", w.Body.String())
	}
}

func TestCountElements_DispatchByKind(t *testing.T) {
	cases := []struct {
		name     string
		selector string
		wantSub  []string // substrings expected in the JS expression
	}{
		{"css explicit", "css:button.submit", []string{"document.querySelectorAll(", "button.submit"}},
		{"css auto", ".item", []string{"document.querySelectorAll(", ".item"}},
		{"xpath", "xpath://li", []string{"document.evaluate(", "snapshotLength"}},
		{"text", "text:Go", []string{"normalize", "minSize"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, lastExpr := newCountHarness(t, 1)
			if _, err := h.countElements(context.Background(), "tab1", tc.selector); err != nil {
				t.Fatalf("countElements(%q): %v", tc.selector, err)
			}
			for _, sub := range tc.wantSub {
				if !strings.Contains(*lastExpr, sub) {
					t.Fatalf("selector %q: expected expression to contain %q, got %s", tc.selector, sub, *lastExpr)
				}
			}
		})
	}
}

func TestHandleTabCount_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//count?selector=button", nil)
	w := httptest.NewRecorder()
	h.HandleTabCount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabCount_ForwardsTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/count?selector=button", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabCount(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCountRoutesRegistered(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	paths := []string{"/count?selector=button", "/tabs/tab1/count?selector=button"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code == http.StatusNotFound && strings.Contains(w.Body.String(), "404 page not found") {
				t.Fatalf("route %s not registered", path)
			}
		})
	}
}
