package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/engine"
)

type failMockBridge struct {
	bridge.BridgeAPI
}

type liteActionBridge struct {
	mockBridge
	ensureChromeCalled bool
}

func (m *liteActionBridge) AvailableActions() []string {
	return []string{bridge.ActionClick, bridge.ActionType, bridge.ActionPress}
}

func (m *liteActionBridge) EnsureChrome(cfg *config.RuntimeConfig) error {
	m.ensureChromeCalled = true
	return fmt.Errorf("ensureChrome should not be called for lite-routed actions")
}

func (m *liteActionBridge) RestartBrowser(cfg *config.RuntimeConfig) error {
	return nil
}

type fakeLiteEngine struct {
	clickRefs []string
	typeCalls []struct {
		ref  string
		text string
	}
}

func (f *fakeLiteEngine) Name() string { return "lite-test" }
func (f *fakeLiteEngine) Navigate(ctx context.Context, url string) (*engine.NavigateResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeLiteEngine) Snapshot(ctx context.Context, tabID, filter string) (*engine.SnapshotResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeLiteEngine) Text(ctx context.Context, tabID string) (*engine.TextResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeLiteEngine) Click(ctx context.Context, tabID, ref string) error {
	f.clickRefs = append(f.clickRefs, ref)
	return nil
}
func (f *fakeLiteEngine) Type(ctx context.Context, tabID, ref, text string) error {
	f.typeCalls = append(f.typeCalls, struct {
		ref  string
		text string
	}{ref: ref, text: text})
	return nil
}
func (f *fakeLiteEngine) Capabilities() []engine.Capability {
	return []engine.Capability{engine.CapClick, engine.CapType}
}
func (f *fakeLiteEngine) Close() error { return nil }

func (m *failMockBridge) TabContext(tabID string) (context.Context, string, error) {
	return nil, "", fmt.Errorf("tab not found")
}

func (m *failMockBridge) ListTargets() ([]*target.Info, error) {
	return nil, fmt.Errorf("list targets failed")
}

func (m *failMockBridge) EnsureChrome(cfg *config.RuntimeConfig) error {
	return nil
}

func (m *failMockBridge) RestartBrowser(cfg *config.RuntimeConfig) error {
	return nil
}

func (m *failMockBridge) AvailableActions() []string {
	return []string{bridge.ActionClick, bridge.ActionType}
}

func (m *failMockBridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	return task(ctx)
}

func TestHandleActions_EmptyArray(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/actions", bytes.NewReader([]byte(`{"actions": []}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleActions(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["error"] != "actions array is empty" {
		t.Errorf("expected empty array error, got %v", resp["error"])
	}
}

func TestHandleTabAction_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//action", bytes.NewReader([]byte(`{"kind":"click"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTabAction(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabAction_TabIDMismatch(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/action", bytes.NewReader([]byte(`{"tabId":"tab_other","kind":"click"}`)))
	req.SetPathValue("id", "tab_abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTabAction(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabAction_NoTab(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/action", bytes.NewReader([]byte(`{"kind":"click"}`)))
	req.SetPathValue("id", "tab_abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTabAction(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleActions_NoTabError(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{
		"actions": [
			{"kind": "click", "selector": "button"}
		]
	}`

	req := httptest.NewRequest("POST", "/actions", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleActions(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestHandleTabActions_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//actions", bytes.NewReader([]byte(`{"actions":[{"kind":"click"}]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTabActions(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabActions_TabIDMismatch(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/actions", bytes.NewReader([]byte(`{"tabId":"tab_other","actions":[{"kind":"click"}]}`)))
	req.SetPathValue("id", "tab_abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTabActions(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabActions_NoTab(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/actions", bytes.NewReader([]byte(`{"actions":[{"kind":"click"}]}`)))
	req.SetPathValue("id", "tab_abc")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleTabActions(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetCookies_NoTab(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/cookies", nil)
	w := httptest.NewRecorder()

	h.HandleGetCookies(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestHandleSetCookies_EmptyURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"cookies": [{"name": "test", "value": "123"}]}`
	req := httptest.NewRequest("POST", "/cookies", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleSetCookies(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for missing url, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["error"] != "url is required" {
		t.Errorf("expected url required error, got %v", resp["error"])
	}
}

func TestHandleSetCookies_EmptyCookies(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"url": "https://pinchtab.com", "cookies": []}`
	req := httptest.NewRequest("POST", "/cookies", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleSetCookies(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for empty cookies, got %d", w.Code)
	}
}

func TestHandleFingerprintRotate_NoTab(t *testing.T) {
	h := New(&failMockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"os": "windows", "browser": "chrome"}`
	req := httptest.NewRequest("POST", "/fingerprint/rotate", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleFingerprintRotate(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestHandleAction_GetMissingKind(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/action?tabId=tab1", nil)
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for missing kind, got %d", w.Code)
	}
}

func TestHandleAction_LiteClickRoutesWithoutChrome(t *testing.T) {
	b := &liteActionBridge{}
	lite := &fakeLiteEngine{}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)
	h.Router = engine.NewRouter(engine.ModeLite, lite)

	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"click","ref":"e1"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Engine"); got != "lite" {
		t.Fatalf("expected X-Engine=lite, got %q", got)
	}
	if b.ensureChromeCalled {
		t.Fatal("expected lite action to skip chrome initialization")
	}
	if len(lite.clickRefs) != 1 || lite.clickRefs[0] != "e1" {
		t.Fatalf("expected click ref e1, got %+v", lite.clickRefs)
	}
}

func TestHandleAction_LiteUnsupportedReturns501(t *testing.T) {
	b := &liteActionBridge{}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)
	h.Router = engine.NewRouter(engine.ModeLite, &fakeLiteEngine{})

	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"press","ref":"e1","key":"Enter"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 501 {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
	if b.ensureChromeCalled {
		t.Fatal("expected unsupported lite action to avoid chrome initialization")
	}
}

func TestHandleActions_LiteBatchSupportsClickAndType(t *testing.T) {
	b := &liteActionBridge{}
	lite := &fakeLiteEngine{}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)
	h.Router = engine.NewRouter(engine.ModeLite, lite)

	body := `{
		"actions": [
			{"kind":"click","ref":"e1"},
			{"kind":"type","ref":"e2","text":"hello"}
		]
	}`
	req := httptest.NewRequest("POST", "/actions", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleActions(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if b.ensureChromeCalled {
		t.Fatal("expected lite batch actions to skip chrome initialization")
	}

	resp := struct {
		Successful int `json:"successful"`
		Failed     int `json:"failed"`
	}{}
	if err := json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&resp); err != nil && err != io.EOF {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Successful != 2 || resp.Failed != 0 {
		t.Fatalf("expected 2 successful actions, got %+v", resp)
	}
	if len(lite.clickRefs) != 1 || lite.clickRefs[0] != "e1" {
		t.Fatalf("unexpected click refs: %+v", lite.clickRefs)
	}
	if len(lite.typeCalls) != 1 || lite.typeCalls[0].ref != "e2" || lite.typeCalls[0].text != "hello" {
		t.Fatalf("unexpected type calls: %+v", lite.typeCalls)
	}
}

func TestHandleMacro_EmptySteps(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowMacro: true}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/macro", bytes.NewReader([]byte(`{"tabId":"tab1","steps":[]}`)))
	w := httptest.NewRecorder()
	h.HandleMacro(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400 for empty macro steps, got %d", w.Code)
	}
}

func TestCountSuccessful(t *testing.T) {
	results := []actionResult{
		{Success: true},
		{Success: false},
		{Success: true},
		{Success: true},
	}

	count := countSuccessful(results)
	if count != 3 {
		t.Errorf("expected 3 successful, got %d", count)
	}
}

func TestHandleAction_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.HandleAction(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleMacro_Disabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/macro", bytes.NewReader([]byte(`{"steps":[{"kind":"click","ref":"e0"}]}`)))
	w := httptest.NewRecorder()
	h.HandleMacro(w, req)
	if w.Code != 403 {
		t.Errorf("expected 403 when macro disabled, got %d", w.Code)
	}
}
