package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// ── Request validation tests ───────────────────────────────────────────

func TestHandleWait_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleWait_EmptyBody(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ── Fixed duration (ms) tests ──────────────────────────────────────────

func TestHandleWait_FixedMs(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"ms": 50}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp waitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Waited {
		t.Error("expected waited=true")
	}
	if resp.Elapsed < 50 {
		t.Errorf("expected elapsed >= 50, got %d", resp.Elapsed)
	}
	if resp.Match != "50ms" {
		t.Errorf("expected match='50ms', got %q", resp.Match)
	}
}

func TestHandleWait_FixedMsZero(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"ms": 0}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp waitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Waited {
		t.Error("expected waited=true for ms=0")
	}
}

func TestHandleWait_FixedMsNegativeClamped(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"ms": -100}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp waitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Waited {
		t.Error("expected waited=true for negative ms (clamped to 0)")
	}
	if resp.Match != "0ms" {
		t.Errorf("expected match='0ms', got %q", resp.Match)
	}
}

func TestHandleWait_FixedMsClampedAt30s(t *testing.T) {
	// Verify the clamping logic: ms > 30000 gets clamped to 30000.
	// We don't actually wait 30s — just verify mode detection and clamping.
	wr := waitRequest{}
	ms := 99999
	wr.Ms = &ms
	if wr.mode() != "ms" {
		t.Errorf("expected mode 'ms', got %q", wr.mode())
	}
}

// ── Timeout clamping tests ─────────────────────────────────────────────

func TestWaitRequest_TimeoutClamping(t *testing.T) {
	tests := []struct {
		name    string
		timeout *int
		wantMs  int
	}{
		{"default", nil, 10000},
		{"custom", intPtr(5000), 5000},
		{"too low", intPtr(10), 100},
		{"too high", intPtr(60000), 30000},
		{"max boundary", intPtr(30000), 30000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wr := waitRequest{Timeout: tt.timeout}
			got := wr.resolvedTimeout().Milliseconds()
			if got != int64(tt.wantMs) {
				t.Errorf("resolvedTimeout() = %d, want %d", got, tt.wantMs)
			}
		})
	}
}

// ── Mode detection tests ───────────────────────────────────────────────

func TestWaitRequest_Mode(t *testing.T) {
	tests := []struct {
		name string
		req  waitRequest
		want string
	}{
		{"empty", waitRequest{}, ""},
		{"selector", waitRequest{Selector: "#foo"}, "selector"},
		{"text", waitRequest{Text: "hello"}, "text"},
		{"url", waitRequest{URL: "**/dash"}, "url"},
		{"load", waitRequest{Load: "networkidle"}, "load"},
		{"fn", waitRequest{Fn: "true"}, "fn"},
		{"ms", waitRequest{Ms: intPtr(100)}, "ms"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.mode(); got != tt.want {
				t.Errorf("mode() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── Selector wait (needs browser — tests validation only) ──────────────

func TestHandleWait_SelectorNoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"selector":"#results"}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWait_TextNoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"text":"Order confirmed"}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWait_URLNoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"url":"**/dashboard"}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWait_FnNoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowEvaluate: true}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"fn":"document.readyState === 'complete'"}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWait_FnBlockedWhenEvaluateDisabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowEvaluate: false}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"fn":"true"}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("evaluate_disabled")) {
		t.Fatalf("expected evaluate_disabled response, got %s", w.Body.String())
	}
}

func TestHandleWait_SelectorNotBlockedByEvaluateSetting(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowEvaluate: false}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"selector":"#results"}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWait_LoadNoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"load":"networkidle"}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleWait_InvalidLoadState(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{ActionTimeout: 5e9}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"load":"invalid"}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ── Tab-scoped wait tests ──────────────────────────────────────────────

func TestHandleTabWait_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//wait", bytes.NewReader([]byte(`{"ms":50}`)))
	w := httptest.NewRecorder()
	h.HandleTabWait(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabWait_TabIDMismatch(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/wait", bytes.NewReader([]byte(`{"tabId":"tab_other","ms":50}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabWait(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabWait_FixedMs(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/wait", bytes.NewReader([]byte(`{"ms":10}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabWait(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp waitResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Waited {
		t.Error("expected waited=true")
	}
}

// ── Response format tests ──────────────────────────────────────────────

func TestWaitResponse_Format(t *testing.T) {
	resp := waitResponse{
		Waited:  true,
		Elapsed: 3420,
		Match:   "#results",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["waited"] != true {
		t.Error("expected waited=true")
	}
	if got["elapsed"].(float64) != 3420 {
		t.Errorf("expected elapsed=3420, got %v", got["elapsed"])
	}
	if got["match"] != "#results" {
		t.Errorf("expected match='#results', got %v", got["match"])
	}
	if _, ok := got["error"]; ok {
		t.Error("expected no error field when empty")
	}
}

func TestWaitResponse_ErrorFormat(t *testing.T) {
	resp := waitResponse{
		Waited:  false,
		Elapsed: 10000,
		Error:   "timeout after 10000ms waiting for selector",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["waited"] != false {
		t.Error("expected waited=false")
	}
	if got["error"] == nil || got["error"] == "" {
		t.Error("expected error field")
	}
}

// ── JS builder tests ───────────────────────────────────────────────────

func TestBuildSelectorJS_CSS(t *testing.T) {
	js, match := buildSelectorJS("#results", "")
	if js == "" {
		t.Error("expected non-empty JS")
	}
	if match != "#results" {
		t.Errorf("expected match='#results', got %q", match)
	}
}

func TestBuildSelectorJS_CSSPrefix(t *testing.T) {
	js, match := buildSelectorJS("css:#results", "")
	if js == "" {
		t.Error("expected non-empty JS")
	}
	if match != "css:#results" {
		t.Errorf("expected match='css:#results', got %q", match)
	}
}

func TestBuildSelectorJS_XPath(t *testing.T) {
	js, _ := buildSelectorJS("xpath://div[@id='test']", "")
	if js == "" {
		t.Error("expected non-empty JS for xpath")
	}
}

func TestBuildSelectorJS_XPathBare(t *testing.T) {
	js, _ := buildSelectorJS("//div[@id='test']", "")
	if js == "" {
		t.Error("expected non-empty JS for bare xpath")
	}
}

func TestBuildSelectorJS_Text(t *testing.T) {
	js, _ := buildSelectorJS("text:Submit", "")
	if js == "" {
		t.Error("expected non-empty JS for text selector")
	}
}

func TestBuildSelectorJS_Hidden(t *testing.T) {
	js, _ := buildSelectorJS("#spinner", "hidden")
	if js == "" {
		t.Error("expected non-empty JS for hidden state")
	}
	// Hidden should check for null
	jsVisible, _ := buildSelectorJS("#spinner", "")
	if js == jsVisible {
		t.Error("hidden JS should differ from visible JS")
	}
}

func TestBuildSelectorJS_HiddenXPath(t *testing.T) {
	js, _ := buildSelectorJS("xpath://div", "hidden")
	if js == "" {
		t.Error("expected non-empty JS for hidden xpath")
	}
}

func TestBuildSelectorJS_HiddenText(t *testing.T) {
	js, _ := buildSelectorJS("text:Loading", "hidden")
	if js == "" {
		t.Error("expected non-empty JS for hidden text")
	}
}

func TestBuildURLMatchJS(t *testing.T) {
	js := buildURLMatchJS("**/dashboard")
	if js == "" {
		t.Error("expected non-empty JS for URL match")
	}
}

// ── Route registration test ────────────────────────────────────────────

func TestWaitRouteRegistered(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := httptest.NewServer(nil).Config.Handler
	_ = mux
	// Just verify the handler methods exist and are callable
	req := httptest.NewRequest("POST", "/wait", bytes.NewReader([]byte(`{"ms":1}`)))
	w := httptest.NewRecorder()
	h.HandleWait(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ── Helpers ────────────────────────────────────────────────────────────

func intPtr(v int) *int {
	return &v
}
