package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
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
	// The mock tab reports no current URL, so there is nothing to default to and the
	// refusal says which of the two it is.
	if !strings.HasPrefix(resp["error"], "url is required") {
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
	// The alias must keep the legacy status string for version-skewed orchestrators.
	if w.Code == http.StatusOK && !strings.Contains(w.Body.String(), "chrome_ready") {
		t.Errorf("/ensure-chrome should return legacy chrome_ready status, got %s", w.Body.String())
	}
}

// The endpoint's whole contract for a click that navigates: a success carrying where
// the caller landed and the fact that its refs are dead. It used to be a 409 whose
// hint and remedy told the caller to re-issue the click with --wait-nav — advice that
// re-clicked on the page the first click had already reached.
func TestHandleAction_ANavigatingClickIsASuccessCarryingTheLandedURL(t *testing.T) {
	mb := &mockBridge{actionResult: map[string]any{
		"clicked":                true,
		bridge.ResultNavigated:   true,
		bridge.ResultLandedURL:   "https://pinchtab.com/docs/",
		bridge.ResultPreviousURL: "https://pinchtab.com/",
		bridge.ResultRefsStale:   true,
	}}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"click"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAction(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Success bool           `json:"success"`
		Result  map[string]any `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success = false: %s", w.Body.String())
	}
	// The action's own result is what the API used to discard when it answered with
	// the navigation as an error instead.
	if resp.Result["clicked"] != true {
		t.Errorf("the action's own result was dropped: %v", resp.Result)
	}
	if resp.Result[bridge.ResultNavigated] != true {
		t.Errorf("result does not report the navigation: %v", resp.Result)
	}
	if got, _ := resp.Result[bridge.ResultLandedURL].(string); got != "https://pinchtab.com/docs/" {
		t.Errorf("result[%s] = %q, want the landed URL", bridge.ResultLandedURL, got)
	}
	if resp.Result[bridge.ResultRefsStale] != true {
		t.Errorf("result does not say the caller's refs are dead: %v", resp.Result)
	}
}

// The GET convenience form of /action is served by the bridge front, and it decoded no
// scroll delta and carried no presence flag: every scroll moved one 120px notch and said
// success, whether the caller asked for 400px or for an explicit zero. The delta rule has a
// single owner in the bridge (resolveScrollDelta), and it reads ScrollX/ScrollY plus the
// presence flags — so the whole defect is what this decoder hands it.
func TestActionQueryDecodesScrollDeltasAndTheirPresence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		query      string
		wantScroll [2]int
		wantDelta  [2]int
		wantHasS   bool
		wantHasD   bool
	}{
		{
			name:       "the requested scroll delta reaches the request",
			query:      "kind=scroll&scrollY=400",
			wantScroll: [2]int{0, 400},
			wantHasS:   true,
		},
		{
			name:       "a horizontal scroll delta too",
			query:      "kind=scroll&scrollX=-250",
			wantScroll: [2]int{-250, 0},
			wantHasS:   true,
		},
		{
			name:       "an explicit zero scroll is explicit, not absent",
			query:      "kind=scroll&scrollY=0",
			wantScroll: [2]int{0, 0},
			wantHasS:   true,
		},
		{
			name:      "an explicit zero wheel delta is explicit too",
			query:     "kind=mouse-wheel&deltaY=0",
			wantDelta: [2]int{0, 0},
			wantHasD:  true,
		},
		{
			name:      "a wheel delta reaches the request with its presence flag",
			query:     "kind=mouse-wheel&deltaY=400",
			wantDelta: [2]int{0, 400},
			wantHasD:  true,
		},
		{
			name:  "no delta key at all leaves both flags clear, so the notch still applies",
			query: "kind=scroll",
		},
		{
			name:  "an empty value is not a delta",
			query: "kind=scroll&scrollY=",
		},
		{
			name:     "the explicit presence parameter is honoured on its own",
			query:    "kind=scroll&hasScroll=true",
			wantHasS: true,
		},
		{
			name:     "and its wheel twin",
			query:    "kind=mouse-wheel&hasDelta=true",
			wantHasD: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, "/action?"+tc.query, nil))

			if !ok {
				t.Fatalf("decode refused %q: %s", tc.query, rec.Body.String())
			}
			if got := [2]int{req.ScrollX, req.ScrollY}; got != tc.wantScroll {
				t.Errorf("scrollX/scrollY = %v, want %v — the delta the caller asked for was discarded", got, tc.wantScroll)
			}
			if got := [2]int{req.DeltaX, req.DeltaY}; got != tc.wantDelta {
				t.Errorf("deltaX/deltaY = %v, want %v", got, tc.wantDelta)
			}
			if req.HasScroll != tc.wantHasS {
				t.Errorf("HasScroll = %v, want %v — the resolver reads this to tell an explicit zero from an absent delta", req.HasScroll, tc.wantHasS)
			}
			if req.HasDelta != tc.wantHasD {
				t.Errorf("HasDelta = %v, want %v", req.HasDelta, tc.wantHasD)
			}
		})
	}
}

// PARITY, derived from the request type rather than from a list of fields someone remembered:
// for every JSON key bridge.ActionRequest declares, the GET query form and the POST body form
// must decode to the same request. That is the guard this defect needed — scrollX/scrollY were
// simply missing from a hand-written decoder, and nothing could see the omission. A field wired
// into one branch only now reds by name.
//
// The excusal list is production's queryUndecodedActionFields, not a copy: the same record
// that excuses a field here is what makes the GET form refuse it, so the two cannot drift.
// Delete an entry and this guard demands the field decode identically on both forms.
func TestActionQueryAndBodyDecodeToTheSameRequest(t *testing.T) {
	fields := reflect.VisibleFields(reflect.TypeOf(bridge.ActionRequest{}))
	if len(fields) == 0 {
		t.Fatal("ActionRequest declares no fields; this parity guard would pass vacuously")
	}

	compared := 0
	for _, field := range fields {
		key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if key == "" || key == "-" {
			continue
		}
		if reason, excused := queryUndecodedActionFields[key]; excused {
			if reason == "" {
				t.Errorf("%q is excused with no reason", key)
			}
			continue
		}

		value, ok := parityValueFor(field.Type)
		if !ok {
			t.Errorf("%q has type %s, which this guard cannot drive — extend it rather than excusing the field", key, field.Type)
			continue
		}

		fromQuery, okQuery := decodeActionRequest(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/action?"+key+"="+value, nil))
		body := fmt.Sprintf(`{"%s":%s}`, key, jsonLiteral(field.Type, value))
		fromBody, okBody := decodeActionRequest(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader([]byte(body))))

		if !okQuery || !okBody {
			t.Errorf("%q: decode refused (query ok=%v, body ok=%v)", key, okQuery, okBody)
			continue
		}
		if !reflect.DeepEqual(fromQuery, fromBody) {
			t.Errorf("%q decodes differently on the two forms — the GET convenience form silently drops or mis-carries it:\n  query %+v\n  body  %+v", key, fromQuery, fromBody)
		}
		compared++
	}

	if compared == 0 {
		t.Fatal("no field was compared; the guard is not exercising either decoder")
	}
	for key := range queryUndecodedActionFields {
		if !declaresActionJSONKey(key) {
			t.Errorf("%q is recorded as undecodable but ActionRequest no longer declares it — the record is stale", key)
		}
	}
}

// The measured instance: a shift-click over the query form answered 200 and clicked plain,
// because the GET branch reads no modifiers key and dropped it without a word.
func TestActionQueryRefusesAShiftClickItCannotExpress(t *testing.T) {
	rec := httptest.NewRecorder()
	_, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&x=120&y=50&hasXY=true&modifiers=8", nil))

	if ok {
		t.Fatal("the query form accepted modifiers=8; it decodes no modifiers, so this is a plain click reported as the shift click the caller asked for")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "modifiers") {
		t.Errorf("the refusal does not name the field the caller must fix: %s", body)
	}
	if !strings.Contains(body, "POST /action") {
		t.Errorf("the refusal does not say where the field does work: %s", body)
	}
}

// Every recorded field, not just the measured one — the table IS the production record, so a
// field added there refuses with no second edit here.
func TestActionQueryRefusesEveryFieldItCannotDecode(t *testing.T) {
	if len(queryUndecodedActionFields) == 0 {
		t.Fatal("no field is recorded as undecodable; this guard would pass vacuously")
	}
	for key := range queryUndecodedActionFields {
		t.Run(key, func(t *testing.T) {
			rec := httptest.NewRecorder()
			_, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&"+key+"=1", nil))

			if ok {
				t.Fatalf("the query form accepted %q, which it does not decode — the caller's parameter is dropped and the wrong action is reported as success", key)
			}
			if !strings.Contains(rec.Body.String(), key) {
				t.Errorf("the refusal does not name %q: %s", key, rec.Body.String())
			}
		})
	}
}

// An empty value is absent, the same rule every decoded field follows, so it is not a
// supplied parameter and must not refuse.
func TestActionQueryAcceptsAnEmptyUndecodedField(t *testing.T) {
	rec := httptest.NewRecorder()
	if _, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&modifiers=", nil)); !ok {
		t.Fatalf("an empty modifiers refused, but no value was supplied to drop: %s", rec.Body.String())
	}
}

// The refusal names EVERY offender in a fixed order. A message built by ranging the record
// would name whichever key came first that run, so the same request would be refused two
// different ways and neither reproduces.
func TestActionQueryRefusalNamesEveryOffenderInAStableOrder(t *testing.T) {
	const query = "/action?kind=click&waitNav=true&modifiers=8&humanize=true"

	first := ""
	for range 20 {
		rec := httptest.NewRecorder()
		if _, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, query, nil)); ok {
			t.Fatal("three undecoded fields were accepted")
		}
		body := rec.Body.String()
		for _, want := range []string{"humanize", "modifiers", "waitNav"} {
			if !strings.Contains(body, want) {
				t.Fatalf("the refusal names only some offenders, so a caller fixes one and is refused again: %s", body)
			}
		}
		if first == "" {
			first = body
		}
		if body != first {
			t.Fatalf("the refusal differs between runs; map iteration order is deciding the message:\n  %s\n  %s", first, body)
		}
	}
	if !strings.Contains(first, "humanize, modifiers, waitNav") {
		t.Errorf("the offenders are not sorted: %s", first)
	}
}

// MEASURED: the undecoded-field refusal is a deny-list, so it closed the class for a
// correctly spelled parameter and left it open one typo over — ?modifers=8 and ?Modifiers=8
// both answered 200 and dispatched a plain click. Same harm, same 200.
func TestActionQueryRefusesAMisspelledOrMiscasedParameter(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      string
		wantHint string
	}{
		{name: "a dropped letter", key: "modifers", wantHint: "modifiers"},
		{name: "the wrong case", key: "Modifiers", wantHint: "modifiers"},
		{name: "the wrong case on a decoded field", key: "TabId", wantHint: "tabId"},
		{name: "a parameter of no endpoint", key: "cachebuster"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			_, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&"+tc.key+"=8", nil))

			if ok {
				t.Fatalf("the query form accepted %q, which it reads nowhere — the caller's parameter is dropped and the wrong action is reported as success", tc.key)
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.key) {
				t.Errorf("the refusal does not name the parameter the caller must fix: %s", body)
			}
			if tc.wantHint == "" {
				if strings.Contains(body, "did you mean") {
					t.Errorf("%q resembles no parameter, so a hint is a guess: %s", tc.key, body)
				}
				return
			}
			if !strings.Contains(body, "did you mean "+tc.wantHint+"?") {
				t.Errorf("the refusal does not point at %q, so the caller has to find the spelling itself: %s", tc.wantHint, body)
			}
		})
	}
}

// The allow-list is DERIVED from the request type, not a second list beside the decoder: every
// key ActionRequest declares is meaningful, so a new field is accepted with no edit to the
// refusal. Recorded-undecodable keys keep their own refusal, which says where they do work.
func TestActionQueryAcceptsEveryFieldTheRequestTypeDeclares(t *testing.T) {
	checked := 0
	for _, field := range reflect.VisibleFields(reflect.TypeOf(bridge.ActionRequest{})) {
		key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if key == "" || key == "-" {
			continue
		}
		if unknown := unknownQueryFields(url.Values{key: []string{"1"}}); len(unknown) > 0 {
			t.Errorf("%q is declared by ActionRequest and the GET form calls it unknown; the allow-list is not derived from the type", key)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no field was checked; the guard is not reading the request type")
	}
	if unknown := unknownQueryFields(url.Values{"modifers": []string{"8"}}); len(unknown) != 1 {
		t.Fatalf("a key the type does not declare was not called unknown: %v — the allow-list accepts everything", unknown)
	}
}

// A correctly spelled undecodable field keeps the refusal that tells the caller to POST it.
// The unknown-key refusal must not swallow that message, or the fix a caller is told to make
// changes from "send this as POST" to "check the spelling".
func TestARecordedFieldStillRefusesWithThePostRemedy(t *testing.T) {
	rec := httptest.NewRecorder()
	if _, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&modifiers=8", nil)); ok {
		t.Fatal("modifiers=8 was accepted")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "cannot be sent as query parameters") {
		t.Errorf("a recorded field got the unknown-key refusal instead of its own: %s", body)
	}
	if strings.Contains(body, "did you mean") {
		t.Errorf("the field is spelled correctly, so a spelling hint misdirects the caller: %s", body)
	}
}

// THE DECISION, pinned at the endpoint so the next reader cannot restore it by accident:
// ?timeout= stays a supported GET parameter. It exists precisely because the GET form has no
// body to carry a per-request timeout, and the block reading it is guarded by
// r.Method == http.MethodGet — so refusing it would have made that code unreachable on the only
// method that reaches it, behind a 400 saying it is not a parameter of /action.
//
// Asserted through the DECODER as a caller reaches it, not through the predicate alone: a
// predicate test passes on a build where the refusal runs before the allow-list is consulted.
func TestTheGetOnlyTimeoutParameterIsAccepted(t *testing.T) {
	rec := httptest.NewRecorder()
	if _, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&timeout=5", nil)); !ok {
		t.Fatalf("?timeout=5 was refused, but HandleAction reads it: %s", rec.Body.String())
	}
	// Its neighbours on the same request, since a caller sends them together.
	if _, ok := decodeActionRequest(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/action?kind=click&timeout=5&browser=chrome&tabId=T&owner=agent", nil)); !ok {
		t.Error("a routing-and-ownership request with a timeout was refused")
	}
}

// An empty value is absent, the rule every other presence check here follows, so a bare
// ?_= does not refuse a request that supplied nothing.
func TestActionQueryAcceptsAnEmptyUnknownParameter(t *testing.T) {
	rec := httptest.NewRecorder()
	if _, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&_=", nil)); !ok {
		t.Fatalf("an empty unknown parameter refused, but no value was supplied to drop: %s", rec.Body.String())
	}
}

func TestUnknownActionQueryRefusalNamesEveryOffenderInAStableOrder(t *testing.T) {
	const query = "/action?kind=click&zebra=1&modifers=8&alpha=2"

	first := ""
	for range 20 {
		rec := httptest.NewRecorder()
		if _, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, query, nil)); ok {
			t.Fatal("three unknown parameters were accepted")
		}
		body := rec.Body.String()
		if first == "" {
			first = body
		}
		if body != first {
			t.Fatalf("the refusal differs between runs; map iteration order is deciding the message:\n  %s\n  %s", first, body)
		}
	}
	if !strings.Contains(first, "alpha, modifers (did you mean modifiers?), zebra") {
		t.Errorf("the offenders are not named in a sorted order, so a caller fixes one and is refused again: %s", first)
	}
}

// The refusal is DERIVED from the record rather than parallel to it: a field added to the
// record refuses with no edit to the GET branch. Driven by adding one, since a reader cannot
// tell a derived rule from a coincidental one by looking at either.
func TestAFieldAddedToTheRecordRefusesWithNoSecondEdit(t *testing.T) {
	const key = "browser"
	if _, recorded := queryUndecodedActionFields[key]; recorded {
		t.Fatalf("%q is already recorded, so this mutation proves nothing — pick a decoded field", key)
	}
	if _, ok := decodeActionRequest(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/action?kind=click&"+key+"=chrome", nil)); !ok {
		t.Fatalf("%q is refused before the record names it, so the refusal is not the record's doing", key)
	}

	queryUndecodedActionFields[key] = "recorded by a test"
	defer delete(queryUndecodedActionFields, key)

	rec := httptest.NewRecorder()
	if _, ok := decodeActionRequest(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&"+key+"=chrome", nil)); ok {
		t.Fatalf("%q was recorded as undecodable and the GET form still accepted it; the refusal keeps its own list", key)
	}
	if !strings.Contains(rec.Body.String(), key) {
		t.Errorf("the refusal does not name %q: %s", key, rec.Body.String())
	}
}

func declaresActionJSONKey(key string) bool {
	for _, field := range reflect.VisibleFields(reflect.TypeOf(bridge.ActionRequest{})) {
		if name, _, _ := strings.Cut(field.Tag.Get("json"), ","); name == key {
			return true
		}
	}
	return false
}

func parityValueFor(t reflect.Type) (string, bool) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "z", true
	case reflect.Bool:
		return "true", true
	case reflect.Int, reflect.Int64:
		return "7", true
	case reflect.Float64:
		return "7.5", true
	}
	return "", false
}

func jsonLiteral(t reflect.Type, value string) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.String {
		return `"` + value + `"`
	}
	return value
}

// The defect this closes was not a wrong entry in a list — it was a whole OWNER missing from
// the derivation. HandleAction reads `timeout` straight from the query, so an allow-list built
// only from bridge.ActionRequest refused a parameter the same function implements, leaving the
// code that reads it unreachable behind a 400 saying it is not a parameter of /action.
//
// This walks every r.URL.Query().Get("…") in actions.go and requires the key to be allowed, so
// the NEXT such parameter fails here the day it lands rather than being refused in production.
// Scope is the file, not the package: a query read in another handler belongs to that handler.
func TestEveryQueryParameterActionReadsIsAllowed(t *testing.T) {
	keys := queryGetLiteralsIn(t, "actions.go")
	if len(keys) == 0 {
		t.Fatal("no r.URL.Query().Get literal found in actions.go; the walk stopped matching and this guard would pass over nothing — re-point it at wherever the query reads moved rather than deleting it")
	}

	for _, key := range keys {
		if _, ok := actionQueryKeys[key.name]; ok {
			continue
		}
		t.Errorf("actions.go:%d reads the query parameter %q, but the allow-list refuses it — a caller who sends it gets a 400 saying it is not a parameter of /action while this line reads it. Declare it in getOnlyActionQueryKeys with the reason it is GET-only, or stop reading it",
			key.line, key.name)
	}
}

// Every GET-only entry must be one the handler actually reads, so the record cannot outlive
// its parameter: a stale entry silently widens the allow-list and re-opens the typo class for
// that spelling.
func TestNoGetOnlyKeyOutlivesItsRead(t *testing.T) {
	read := map[string]bool{}
	for _, key := range queryGetLiteralsIn(t, "actions.go") {
		read[key.name] = true
	}
	for key, reason := range getOnlyActionQueryKeys {
		if reason == "" {
			t.Errorf("%q is recorded as GET-only with no reason; every entry widens the allow-list and has to say why", key)
		}
		if !read[key] {
			t.Errorf("%q is recorded as GET-only but actions.go reads no such query parameter; the entry outlived its code and now just permits a typo — drop it", key)
		}
	}
}

type queryGetLiteral struct {
	name string
	line int
}

// queryGetLiteralsIn finds the string literal of every `….Query().Get("x")` in one file of this
// package. It matches the CHAIN rather than the method name, so an unrelated Get on some other
// receiver is not mistaken for a query read.
func queryGetLiteralsIn(t *testing.T, file string) []queryGetLiteral {
	t.Helper()

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("cannot parse %s, so this guard would check nothing: %v", file, err)
	}

	var found []queryGetLiteral
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Get" {
			return true
		}
		receiver, ok := selector.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		if inner, ok := receiver.Fun.(*ast.SelectorExpr); !ok || inner.Sel.Name != "Query" {
			return true
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			t.Errorf("%s:%d reads the query with a non-literal key, which this guard cannot check — pass a literal, or exclude it here with a reason",
				file, fset.Position(call.Pos()).Line)
			return true
		}
		name, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatalf("cannot read the query key at %s:%d: %v", file, fset.Position(call.Pos()).Line, err)
		}
		found = append(found, queryGetLiteral{name: name, line: fset.Position(call.Pos()).Line})
		return true
	})
	return found
}
