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
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/config"
)

type failMockBridge struct {
	bridge.BridgeAPI
}

type recordingActionBridge struct {
	mockBridge
	lastKind string
	lastReq  bridge.ActionRequest
}

type handoffRecordingBridge struct {
	mockBridge
	state bridge.TabHandoffState
	has   bool
}

type autoSwitchActionBridge struct {
	mockBridge
	actionTabs []string
}

func (m *autoSwitchActionBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	if tabID == "" {
		tabID = "tab1"
	}
	return bridge.NewTabHandle(context.Background()), tabID, nil
}

func (m *autoSwitchActionBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	m.actionTabs = append(m.actionTabs, req.TabID)
	if len(m.actionTabs) == 1 {
		return map[string]any{"clicked": true, "switchedToTab": "tab2"}, nil
	}
	return map[string]any{"ok": true, "tabId": req.TabID}, nil
}

func (m *recordingActionBridge) AvailableActions() []string {
	return []string{
		bridge.ActionMouseMove,
		bridge.ActionMouseDown,
		bridge.ActionMouseUp,
		bridge.ActionMouseWheel,
	}
}

func (m *recordingActionBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	m.lastKind = kind
	m.lastReq = req
	return map[string]any{"ok": true}, nil
}

func (m *handoffRecordingBridge) SetTabHandoff(tabID, reason string, timeout time.Duration) error {
	now := time.Now().UTC()
	m.state = bridge.TabHandoffState{
		Status:        "paused_handoff",
		Reason:        reason,
		PausedAt:      now,
		LastUpdatedAt: now,
	}
	if timeout > 0 {
		m.state.ExpiresAt = now.Add(timeout)
	}
	m.has = true
	return nil
}

func (m *handoffRecordingBridge) ResumeTabHandoff(tabID string) error {
	m.has = false
	return nil
}

func (m *handoffRecordingBridge) TabHandoffState(tabID string) (bridge.TabHandoffState, bool) {
	return m.state, m.has
}

func (m *failMockBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	return nil, "", fmt.Errorf("tab not found")
}

func (m *failMockBridge) ListTargets() ([]bridge.TabTarget, error) {
	return nil, fmt.Errorf("list targets failed")
}

func (m *failMockBridge) EnsureBrowser(cfg *config.RuntimeConfig) error {
	return nil
}

func (m *failMockBridge) RestartBrowser(cfg *config.RuntimeConfig) error {
	return nil
}

func (m *failMockBridge) AvailableActions() []string {
	return []string{bridge.ActionClick, bridge.ActionType}
}

func (m *failMockBridge) Evaluate(ctx context.Context, expression string, result any, opts bridge.EvalOpts) error {
	return nil
}

func (m *failMockBridge) Execute(ctx context.Context, tabID string, task func(ctx context.Context) error) error {
	return task(ctx)
}

func (m *failMockBridge) CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return fmt.Errorf("not implemented")
}

func (m *failMockBridge) EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts bridge.EvalOpts) error {
	return fmt.Errorf("not implemented")
}

func (m *failMockBridge) DescribeNode(ctx context.Context, backendNodeID int64) (*bridge.NodeInfo, error) {
	return nil, fmt.Errorf("not implemented")
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

func TestHandleActions_FollowsAutoSwitchedTab(t *testing.T) {
	b := &autoSwitchActionBridge{}
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)

	body := `{"actions":[{"kind":"click"},{"kind":"type","text":"after"}]}`
	req := httptest.NewRequest("POST", "/actions", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleActions(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got, want := strings.Join(b.actionTabs, ","), "tab1,tab2"; got != want {
		t.Fatalf("action tabs = %s, want %s", got, want)
	}
}

func TestHandleMacro_FollowsAutoSwitchedTab(t *testing.T) {
	b := &autoSwitchActionBridge{}
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second, AllowMacro: true}, nil, nil, nil)

	body := `{"steps":[{"kind":"click"},{"kind":"type","text":"after"}]}`
	req := httptest.NewRequest("POST", "/macro", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleMacro(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got, want := strings.Join(b.actionTabs, ","), "tab1,tab2"; got != want {
		t.Fatalf("action tabs = %s, want %s", got, want)
	}
}

func TestHandleActions_ResponseIncludesRoute(t *testing.T) {
	b := &autoSwitchActionBridge{}
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)

	body := `{"actions":[{"kind":"click"},{"kind":"type","text":"hello"}]}`
	req := httptest.NewRequest("POST", "/actions", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleActions(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Route *browserops.RouteMetadata `json:"route"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Route == nil {
		t.Fatal("expected route in batch /actions response, got nil")
	}
	if resp.Route.UsedBrowser == "" {
		t.Fatal("expected route.usedProvider to be set")
	}
	if len(resp.Route.Attempts) == 0 {
		t.Fatal("expected route.attempts to be non-empty")
	}
}

func TestHandleMacro_ResponseIncludesRoute(t *testing.T) {
	b := &autoSwitchActionBridge{}
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second, AllowMacro: true}, nil, nil, nil)

	body := `{"steps":[{"kind":"click"},{"kind":"type","text":"hello"}]}`
	req := httptest.NewRequest("POST", "/macro", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleMacro(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Route *browserops.RouteMetadata `json:"route"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Route == nil {
		t.Fatal("expected route in /macro response, got nil")
	}
	if resp.Route.UsedBrowser == "" {
		t.Fatal("expected route.usedProvider to be set")
	}
	if len(resp.Route.Attempts) == 0 {
		t.Fatal("expected route.attempts to be non-empty")
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
	h := New(&failMockBridge{}, &config.RuntimeConfig{AllowCookies: true}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/cookies", nil)
	w := httptest.NewRecorder()

	h.HandleGetCookies(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for no tab, got %d", w.Code)
	}
}

func TestHandleSetCookies_EmptyURL(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowCookies: true}, nil, nil, nil)

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
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowCookies: true}, nil, nil, nil)

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

func TestHandleAction_AutoCloseArmedAfterActionError(t *testing.T) {
	mb := &mockBridge{executeActionErr: fmt.Errorf("boom")}
	h := New(mb, &config.RuntimeConfig{
		ActionTimeout:      time.Second,
		TabLifecyclePolicy: "close_idle",
	}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"click"}`)))
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if got := mb.autoCloseArmed; len(got) != 1 || got[0] != "tab1" {
		t.Fatalf("autoCloseArmed = %#v, want [tab1]", got)
	}
}

func TestHandleAction_PostRejectsInvalidDialogAction(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"click","selector":"#btn","dialogAction":"maybe"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "dialogAction must be 'accept' or 'dismiss'") {
		t.Fatalf("expected dialogAction validation error, got %s", w.Body.String())
	}
}

func TestHandleAction_GetAcceptsValidDialogAction(t *testing.T) {
	b := &recordingActionBridge{}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/action?kind=mouse-move&x=0&y=0&dialogAction=accept&dialogText=ok", nil)
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if b.lastReq.DialogAction != "accept" {
		t.Fatalf("dialogAction = %q, want accept", b.lastReq.DialogAction)
	}
	if b.lastReq.DialogText != "ok" {
		t.Fatalf("dialogText = %q, want ok", b.lastReq.DialogText)
	}
}

func TestHandleAction_PostCanonicalMouseFieldsAreAccepted(t *testing.T) {
	b := &recordingActionBridge{}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"mouse-wheel","x":0,"y":0,"deltaY":240}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if b.lastKind != bridge.ActionMouseWheel {
		t.Fatalf("kind = %q, want %q", b.lastKind, bridge.ActionMouseWheel)
	}
	if !b.lastReq.HasXY || b.lastReq.X != 0 || b.lastReq.Y != 0 {
		t.Fatalf("expected zero coordinates with HasXY=true, got %+v", b.lastReq)
	}
	if b.lastReq.DeltaY != 240 {
		t.Fatalf("deltaY = %d, want 240", b.lastReq.DeltaY)
	}
}

func TestHandleAction_GetCanonicalMouseQueryIsAccepted(t *testing.T) {
	b := &recordingActionBridge{}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/action?kind=mouse-move&x=0&y=0", nil)
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if b.lastKind != bridge.ActionMouseMove {
		t.Fatalf("kind = %q, want %q", b.lastKind, bridge.ActionMouseMove)
	}
	if !b.lastReq.HasXY || b.lastReq.X != 0 || b.lastReq.Y != 0 {
		t.Fatalf("expected zero coordinates with HasXY=true, got %+v", b.lastReq)
	}
}

func TestHandleAction_LegacyMouseKindIsRejected(t *testing.T) {
	b := &recordingActionBridge{}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"mousewheel","x":0,"y":0,"deltaY":240}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAction_BlockedDuringHumanHandoff(t *testing.T) {
	b := &handoffRecordingBridge{
		state: bridge.TabHandoffState{
			Status:        "paused_handoff",
			Reason:        "captcha_manual",
			PausedAt:      time.Now().UTC(),
			LastUpdatedAt: time.Now().UTC(),
		},
		has: true,
	}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"click","selector":"button"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
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

// L7(f): differing browser values across batch actions would be silently
// ignored (only actions[0] is consulted) — they must 400 instead.
func TestHandleBatchActions_MixedBrowsersRejected(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		BrowsersAvailable: []string{config.BrowserChrome, config.BrowserCloak},
	}, nil, nil, nil)

	body := []byte(`{"actions":[{"kind":"click","ref":"e1","browser":"chrome"},{"kind":"click","ref":"e2","browser":"cloak"}]}`)
	req := httptest.NewRequest("POST", "/actions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleActions(w, req)

	if w.Code != 400 {
		t.Fatalf("mixed browsers should 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "mixed browser") {
		t.Fatalf("error should name the mixed-browser problem: %s", w.Body.String())
	}
}

// L7(d): the pre-rename lazy-init path must keep serving for old orchestrators.
func TestEnsureChromeAliasServes(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, func() {})

	req := httptest.NewRequest("POST", "/ensure-chrome", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatal("/ensure-chrome back-compat alias must not 404")
	}
}
