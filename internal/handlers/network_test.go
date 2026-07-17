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
)

type noFlusherResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *noFlusherResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *noFlusherResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

func (w *noFlusherResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

// networkMockBridge extends mockBridge with a real NetworkMonitor.
type networkMockBridge struct {
	mockBridge
	nm *bridge.NetworkMonitor
}

func (m *networkMockBridge) NetworkMonitor() *bridge.NetworkMonitor {
	return m.nm
}

func newNetworkTestHandler(nm *bridge.NetworkMonitor) *Handlers {
	b := &networkMockBridge{nm: nm}
	return New(b, &config.RuntimeConfig{AllowNetworkIntercept: true}, nil, nil, nil)
}

func TestNetworkDetailAndClearRequireNetworkIntercept(t *testing.T) {
	h := New(&networkMockBridge{nm: bridge.NewNetworkMonitor(100)}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/network/request-1"},
		{http.MethodGet, "/tabs/tab1/network/request-1"},
		{http.MethodPost, "/network/clear"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "network_intercept_disabled") ||
				!strings.Contains(w.Body.String(), "security.allowNetworkIntercept") {
				t.Fatalf("disabled response missing catalog metadata: %s", w.Body.String())
			}
		})
	}
}

func seedBuffer(nm *bridge.NetworkMonitor, tabID string) {
	buf := nm.GetOrCreateBufferForTest(tabID)
	buf.Add(bridge.NetworkEntry{RequestID: "r1", URL: "https://api.example.com/users", Method: "GET", Status: 200, ResourceType: "XHR", Finished: true})
	buf.Add(bridge.NetworkEntry{RequestID: "r2", URL: "https://api.example.com/posts", Method: "POST", Status: 404, ResourceType: "XHR", Finished: true})
	buf.Add(bridge.NetworkEntry{RequestID: "r3", URL: "https://cdn.example.com/style.css", Method: "GET", Status: 200, ResourceType: "Stylesheet", Finished: true})
}

func TestHandleNetwork_ReturnsEntries(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network", nil)
	w := httptest.NewRecorder()
	h.HandleNetwork(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Entries []bridge.NetworkEntry `json:"entries"`
		Count   int                   `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 3 {
		t.Errorf("expected 3 entries, got %d", resp.Count)
	}
}

func TestHandleNetwork_FilterByMethod(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network?method=POST", nil)
	w := httptest.NewRecorder()
	h.HandleNetwork(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Entries []bridge.NetworkEntry `json:"entries"`
		Count   int                   `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 1 {
		t.Errorf("expected 1 POST entry, got %d", resp.Count)
	}
	if resp.Count > 0 && resp.Entries[0].RequestID != "r2" {
		t.Errorf("expected r2, got %s", resp.Entries[0].RequestID)
	}
}

func TestHandleNetwork_FilterByURLPattern(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network?filter=cdn.example", nil)
	w := httptest.NewRecorder()
	h.HandleNetwork(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 1 {
		t.Errorf("expected 1 entry matching cdn.example, got %d", resp.Count)
	}
}

func TestHandleNetwork_FilterByStatus(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network?status=4xx", nil)
	w := httptest.NewRecorder()
	h.HandleNetwork(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 1 {
		t.Errorf("expected 1 4xx entry, got %d", resp.Count)
	}
}

func TestHandleNetwork_FilterByType(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network?type=xhr", nil)
	w := httptest.NewRecorder()
	h.HandleNetwork(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 2 {
		t.Errorf("expected 2 XHR entries, got %d", resp.Count)
	}
}

func TestHandleNetwork_Limit(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network?limit=1", nil)
	w := httptest.NewRecorder()
	h.HandleNetwork(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 1 {
		t.Errorf("expected 1 entry with limit=1, got %d", resp.Count)
	}
}

func TestHandleNetwork_NilMonitor(t *testing.T) {
	h := newNetworkTestHandler(nil)

	req := httptest.NewRequest("GET", "/network", nil)
	w := httptest.NewRecorder()
	h.HandleNetwork(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Entries []any `json:"entries"`
		Count   int   `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 0 {
		t.Errorf("expected 0 entries when monitor is nil, got %d", resp.Count)
	}
}

func TestHandleNetworkByIDUsesRetainedBody(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	buf := nm.GetOrCreateBufferForTest("tab1")
	const retainedBody = "{\"ok\":true}"
	buf.Add(bridge.NetworkEntry{
		RequestID:     "retained-1",
		URL:           "https://api.example.com/data",
		Method:        "GET",
		ResourceType:  "XHR",
		Finished:      true,
		ResponseBody:  retainedBody,
		Base64Encoded: false,
		BodyRetained:  true,
	})
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network/retained-1?body=true", nil)
	req.SetPathValue("requestId", "retained-1")
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["responseBody"] != retainedBody {
		t.Fatalf("expected retained response body, got %v", got["responseBody"])
	}
	if got["bodyRetained"] != true {
		t.Fatalf("expected bodyRetained=true, got %v", got["bodyRetained"])
	}
}

func TestHandleNetworkByIDRetainedPreferredReturnsBodyAfterPendingResolves(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	buf := nm.GetOrCreateBufferForTest("tab1")
	buf.Add(bridge.NetworkEntry{
		RequestID:    "pending-1",
		URL:          "https://api.example.com/data",
		Method:       "GET",
		ResourceType: "XHR",
		Finished:     true,
		BodyPending:  true,
	})
	h := newNetworkTestHandler(nm)

	go func() {
		time.Sleep(40 * time.Millisecond)
		buf.Update("pending-1", func(entry *bridge.NetworkEntry) {
			entry.ResponseBody = "{\"ok\":true}"
			entry.BodyRetained = true
			entry.BodyPending = false
		})
		// Mirror maybeRetainBody: signal so the waiter wakes immediately.
		buf.SignalBodyChange()
	}()

	req := httptest.NewRequest("GET", "/network/pending-1?body=true&bodyMode=retained-preferred&timeoutMs=250", nil)
	req.SetPathValue("requestId", "pending-1")
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["responseBody"] != "{\"ok\":true}" {
		t.Fatalf("expected retained response body, got %v", got["responseBody"])
	}
	if got["bodyRetained"] != true {
		t.Fatalf("expected bodyRetained=true, got %v", got["bodyRetained"])
	}
	if got["bodySource"] != "retained" {
		t.Fatalf("expected bodySource=retained, got %v", got["bodySource"])
	}
	if _, ok := got["bodyPending"]; ok {
		t.Fatalf("expected bodyPending to clear after wait, got %v", got["bodyPending"])
	}
	entryMap, ok := got["entry"].(map[string]any)
	if !ok {
		t.Fatalf("expected entry map, got %T", got["entry"])
	}
	if entryMap["bodyRetained"] != true {
		t.Fatalf("expected nested entry bodyRetained=true, got %v", entryMap["bodyRetained"])
	}
}

func TestHandleNetworkByIDRetainedPreferredTimeoutReturnsPendingState(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	buf := nm.GetOrCreateBufferForTest("tab1")
	buf.Add(bridge.NetworkEntry{
		RequestID:    "pending-timeout",
		URL:          "https://api.example.com/data",
		Method:       "GET",
		ResourceType: "XHR",
		Finished:     true,
		BodyPending:  true,
	})
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network/pending-timeout?body=true&bodyMode=retained-preferred&timeoutMs=20", nil)
	req.SetPathValue("requestId", "pending-timeout")
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["bodyPending"] != true {
		t.Fatalf("expected bodyPending=true on timeout, got %v", got["bodyPending"])
	}
	if _, ok := got["responseBody"]; ok {
		t.Fatalf("expected no responseBody on timeout, got %v", got["responseBody"])
	}
	if _, ok := got["bodyError"]; ok {
		t.Fatalf("expected no bodyError on timeout, got %v", got["bodyError"])
	}
	entryMap, ok := got["entry"].(map[string]any)
	if !ok {
		t.Fatalf("expected entry map, got %T", got["entry"])
	}
	if entryMap["bodyPending"] != true {
		t.Fatalf("expected nested entry bodyPending=true, got %v", entryMap["bodyPending"])
	}
}

func TestWaitForRetainedBodyWakesOnSignal(t *testing.T) {
	buf := bridge.NewNetworkBuffer(10)
	buf.Add(bridge.NetworkEntry{RequestID: "r1", Finished: true, BodyPending: true})

	go func() {
		time.Sleep(20 * time.Millisecond)
		buf.Update("r1", func(e *bridge.NetworkEntry) {
			e.BodyRetained = true
			e.BodyPending = false
		})
		buf.SignalBodyChange()
	}()

	start := time.Now()
	// Generous 2s budget: if the signal weren't wired, this would block the full 2s.
	entry, ok := waitForRetainedBody(buf, "r1", 2*time.Second)
	elapsed := time.Since(start)

	if !ok || !entry.BodyRetained {
		t.Fatalf("expected retained body, got ok=%v entry=%+v", ok, entry)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("waitForRetainedBody did not wake promptly on signal (took %v)", elapsed)
	}
}

func TestWaitForRetainedBodyTimeoutReturnsCurrentEntry(t *testing.T) {
	buf := bridge.NewNetworkBuffer(10)
	buf.Add(bridge.NetworkEntry{RequestID: "r1", Finished: true, BodyPending: true})

	entry, ok := waitForRetainedBody(buf, "r1", 30*time.Millisecond)
	if !ok {
		t.Fatal("expected ok=true on timeout with a present entry")
	}
	if entry.BodyRetained || !entry.BodyPending {
		t.Fatalf("expected the unresolved pending entry at timeout, got %+v", entry)
	}

	// Missing request: no entry to return.
	if _, ok := waitForRetainedBody(buf, "missing", 10*time.Millisecond); ok {
		t.Fatal("expected ok=false for a missing request")
	}
}

func TestHandleNetworkByIDRetainedOnlyReturnsSkippedStateWithoutBodyError(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	buf := nm.GetOrCreateBufferForTest("tab1")
	buf.Add(bridge.NetworkEntry{
		RequestID:      "skipped-1",
		URL:            "https://api.example.com/data",
		Method:         "GET",
		ResourceType:   "XHR",
		Finished:       true,
		BodySkipped:    true,
		BodySkipReason: "retention budget exceeded",
	})
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network/skipped-1?body=true&bodyMode=retained-only", nil)
	req.SetPathValue("requestId", "skipped-1")
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["bodySkipped"] != true {
		t.Fatalf("expected bodySkipped=true, got %v", got["bodySkipped"])
	}
	if got["bodySkipReason"] != "retention budget exceeded" {
		t.Fatalf("expected bodySkipReason to be preserved, got %v", got["bodySkipReason"])
	}
	if _, ok := got["bodyError"]; ok {
		t.Fatalf("expected no bodyError for skipped retention, got %v", got["bodyError"])
	}
	if _, ok := got["responseBody"]; ok {
		t.Fatalf("expected no responseBody in retained-only skip state, got %v", got["responseBody"])
	}
}

func TestHandleNetworkByIDRetainedPreferredDoesNotPretendSkippedBodyWasRetained(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	buf := nm.GetOrCreateBufferForTest("tab1")
	buf.Update("r1", func(entry *bridge.NetworkEntry) {
		entry.BodySkipped = true
		entry.BodySkipReason = "retention disabled"
	})
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network/r1?body=true&bodyMode=retained-preferred", nil)
	req.SetPathValue("requestId", "r1")
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := got["bodyRetained"]; ok {
		t.Fatalf("expected no bodyRetained marker for skipped retention, got %v", got["bodyRetained"])
	}
	if got["bodySkipped"] != true {
		t.Fatalf("expected skip state to remain visible, got %v", got["bodySkipped"])
	}
	if got["bodySkipReason"] != "retention disabled" {
		t.Fatalf("expected skip reason to remain visible, got %v", got["bodySkipReason"])
	}
	entryMap, ok := got["entry"].(map[string]any)
	if !ok {
		t.Fatalf("expected entry map, got %T", got["entry"])
	}
	if entryMap["bodySkipped"] != true {
		t.Fatalf("expected nested entry bodySkipped=true, got %v", entryMap["bodySkipped"])
	}
}

func TestHandleNetworkByID_Found(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network/r1", nil)
	req.SetPathValue("requestId", "r1")
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Entry bridge.NetworkEntry `json:"entry"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Entry.RequestID != "r1" {
		t.Errorf("expected r1, got %s", resp.Entry.RequestID)
	}
	if resp.Entry.URL != "https://api.example.com/users" {
		t.Errorf("expected users URL, got %s", resp.Entry.URL)
	}
}

func TestHandleNetworkByID_NotFound(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network/nonexistent", nil)
	req.SetPathValue("requestId", "nonexistent")
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleNetworkByID_MissingRequestID(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network/", nil)
	// No path value set
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleNetworkByID_NilMonitor(t *testing.T) {
	h := newNetworkTestHandler(nil)

	req := httptest.NewRequest("GET", "/network/r1", nil)
	req.SetPathValue("requestId", "r1")
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleNetworkClear_All(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	seedBuffer(nm, "tab2")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("POST", "/network/clear", nil)
	w := httptest.NewRecorder()
	h.HandleNetworkClear(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Cleared bool `json:"cleared"`
		All     bool `json:"all"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Cleared || !resp.All {
		t.Errorf("expected cleared=true, all=true, got cleared=%v, all=%v", resp.Cleared, resp.All)
	}

	buf1 := nm.GetBuffer("tab1")
	buf2 := nm.GetBuffer("tab2")
	if buf1 != nil && buf1.Len() != 0 {
		t.Errorf("expected tab1 buffer cleared, got %d entries", buf1.Len())
	}
	if buf2 != nil && buf2.Len() != 0 {
		t.Errorf("expected tab2 buffer cleared, got %d entries", buf2.Len())
	}
}

func TestHandleNetworkClear_ByTab(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("POST", "/network/clear?tabId=tab1", nil)
	w := httptest.NewRecorder()
	h.HandleNetworkClear(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Cleared bool   `json:"cleared"`
		TabID   string `json:"tabId"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Cleared {
		t.Error("expected cleared=true")
	}
	if resp.TabID != "tab1" {
		t.Errorf("expected tabId=tab1, got %s", resp.TabID)
	}
}

func TestHandleNetworkClear_NilMonitor(t *testing.T) {
	h := newNetworkTestHandler(nil)

	req := httptest.NewRequest("POST", "/network/clear", nil)
	w := httptest.NewRecorder()
	h.HandleNetworkClear(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleTabNetwork(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	req := httptest.NewRequest("GET", "/tabs/tab1/network", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 3 {
		t.Errorf("expected 3 entries, got %d", resp.Count)
	}
}

func TestHandleTabNetwork_MissingTabID(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/tabs//network", nil)
	w := httptest.NewRecorder()
	h.HandleTabNetwork(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleNetwork_CombinedFilters(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network?method=GET&type=xhr", nil)
	w := httptest.NewRecorder()
	h.HandleNetwork(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 1 {
		t.Errorf("expected 1 GET+XHR entry, got %d", resp.Count)
	}
}

// networkFailTabBridge is a mock that fails TabContext calls.
type networkFailTabBridge struct {
	mockBridge
	nm *bridge.NetworkMonitor
}

func (m *networkFailTabBridge) NetworkMonitor() *bridge.NetworkMonitor {
	return m.nm
}

func (m *networkFailTabBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	return nil, "", fmt.Errorf("tab not found")
}

func (m *networkFailTabBridge) EnsureBrowser(cfg *config.RuntimeConfig) error {
	return nil
}

func (m *networkFailTabBridge) RestartBrowser(cfg *config.RuntimeConfig) error {
	return nil
}

func TestHandleNetwork_TabNotFound(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	b := &networkFailTabBridge{nm: nm}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/network?tabId=nonexistent", nil)
	w := httptest.NewRecorder()
	h.HandleNetwork(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleNetworkByID_NoBufferForTab(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	// Don't seed any buffer for tab1
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/network/r1", nil)
	req.SetPathValue("requestId", "r1")
	w := httptest.NewRecorder()
	h.HandleNetworkByID(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestParseBufferSize(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{"", 0},
		{"bufferSize=200", 200},
		{"bufferSize=0", 0},
		{"bufferSize=-1", 0},
		{"bufferSize=abc", 0},
		{"bufferSize=500", 500},
	}
	for _, tt := range tests {
		url := "/network"
		if tt.query != "" {
			url += "?" + tt.query
		}
		req := httptest.NewRequest("GET", url, nil)
		got := parseBufferSize(req)
		if got != tt.want {
			t.Errorf("parseBufferSize(%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

func TestHandleNetworkStream_SSEHeaders(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	seedBuffer(nm, "tab1")
	h := newNetworkTestHandler(nm)

	// Use a context that we cancel quickly to end the stream
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/network/stream", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.HandleNetworkStream(w, req)
		close(done)
	}()

	// Give it a moment to set headers, then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", cc)
	}
	if xa := w.Header().Get("X-Accel-Buffering"); xa != "no" {
		t.Errorf("expected X-Accel-Buffering no, got %s", xa)
	}
}

func TestHandleNetworkStream_ReceivesEntries(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	buf := nm.GetOrCreateBufferForTest("tab1")
	h := newNetworkTestHandler(nm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("GET", "/network/stream", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.HandleNetworkStream(w, req)
		close(done)
	}()

	// Wait for subscription to be set up
	time.Sleep(50 * time.Millisecond)

	// Add an entry — subscriber should receive it
	buf.Add(bridge.NetworkEntry{RequestID: "stream1", URL: "https://example.com/api", Method: "GET"})

	// Give time for the SSE write
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "event: network") {
		t.Errorf("expected SSE event, got: %s", body)
	}
	if !strings.Contains(body, "stream1") {
		t.Errorf("expected stream1 in SSE data, got: %s", body)
	}
}

func TestHandleNetworkStream_FilterApplied(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	buf := nm.GetOrCreateBufferForTest("tab1")
	h := newNetworkTestHandler(nm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("GET", "/network/stream?method=POST", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.HandleNetworkStream(w, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	// Add a GET entry (should be filtered out) and a POST entry (should pass)
	buf.Add(bridge.NetworkEntry{RequestID: "get1", Method: "GET"})
	buf.Add(bridge.NetworkEntry{RequestID: "post1", Method: "POST"})

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if strings.Contains(body, "get1") {
		t.Errorf("GET entry should have been filtered out, got: %s", body)
	}
	if !strings.Contains(body, "post1") {
		t.Errorf("expected POST entry in stream, got: %s", body)
	}
}

func TestHandleNetworkStream_NilMonitor(t *testing.T) {
	h := newNetworkTestHandler(nil)

	req := httptest.NewRequest("GET", "/network/stream", nil)
	w := httptest.NewRecorder()
	h.HandleNetworkStream(w, req)

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleNetworkStream_StreamingNotSupportedReturnsProblem(t *testing.T) {
	h := newNetworkTestHandler(bridge.NewNetworkMonitor(100))

	req := httptest.NewRequest("GET", "/network/stream", nil)
	w := &noFlusherResponseWriter{}
	h.HandleNetworkStream(w, req)

	if w.status != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.status)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("expected application/problem+json, got %q", ct)
	}

	var payload map[string]any
	if err := json.Unmarshal(w.body.Bytes(), &payload); err != nil {
		t.Fatalf("decode problem payload: %v", err)
	}
	if payload["code"] != "streaming_not_supported" {
		t.Fatalf("code = %v, want streaming_not_supported", payload["code"])
	}
}

func TestHandleTabNetworkStream_MissingTabID(t *testing.T) {
	nm := bridge.NewNetworkMonitor(100)
	h := newNetworkTestHandler(nm)

	req := httptest.NewRequest("GET", "/tabs//network/stream", nil)
	w := httptest.NewRecorder()
	h.HandleTabNetworkStream(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
