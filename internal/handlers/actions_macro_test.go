package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestMacroRejectsBrowserConflictLikeActions(t *testing.T) {
	newHandlers := func() *Handlers {
		return New(&mockBridge{runningBrowser: config.BrowserChrome}, &config.RuntimeConfig{
			ActionTimeout:     time.Second,
			AllowMacro:        true,
			BrowsersAvailable: []string{config.BrowserChrome, config.BrowserCloak},
		}, nil, nil, nil)
	}
	step := `{"kind":"click","selector":"#go","browser":"cloak"}`

	actions := httptest.NewRecorder()
	newHandlers().HandleActions(actions, httptest.NewRequest(http.MethodPost, "/actions", strings.NewReader(`{"actions":[`+step+`]}`)))
	macro := httptest.NewRecorder()
	newHandlers().HandleMacro(macro, httptest.NewRequest(http.MethodPost, "/macro", strings.NewReader(`{"steps":[`+step+`]}`)))

	if actions.Code != http.StatusConflict {
		t.Fatalf("/actions status = %d, want 409: %s", actions.Code, actions.Body.String())
	}
	if macro.Code != actions.Code || macro.Body.String() != actions.Body.String() {
		t.Fatalf("/macro answered %d %s, want the /actions conflict %d %s", macro.Code, macro.Body.String(), actions.Code, actions.Body.String())
	}
}

type tabRecordingBridge struct {
	mockBridge
	actionTabs []string
}

func (m *tabRecordingBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	if tabID == "" {
		tabID = "tab1"
	}
	return bridge.NewTabHandle(context.Background()), tabID, nil
}

func (m *tabRecordingBridge) ExecuteAction(_ context.Context, _ string, req bridge.ActionRequest) (map[string]any, error) {
	m.actionTabs = append(m.actionTabs, req.TabID)
	return map[string]any{"ok": true}, nil
}

func TestMacroStepNamingAnotherTabSwitchesTheRunLikeABatchStep(t *testing.T) {
	steps := `[{"kind":"click","tabId":"tab2"},{"kind":"type","text":"after"}]`
	run := func(handle func(*Handlers) http.HandlerFunc, path, body string) []string {
		b := &tabRecordingBridge{}
		h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second, AllowMacro: true}, nil, nil, nil)
		w := httptest.NewRecorder()
		handle(h)(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d: %s", path, w.Code, w.Body.String())
		}
		return b.actionTabs
	}

	batch := run(func(h *Handlers) http.HandlerFunc { return h.HandleActions }, "/actions", `{"actions":`+steps+`}`)
	macro := run(func(h *Handlers) http.HandlerFunc { return h.HandleMacro }, "/macro", `{"steps":`+steps+`}`)

	if want := "tab2,tab2"; strings.Join(batch, ",") != want {
		t.Fatalf("/actions ran on %v, want %s", batch, want)
	}
	if strings.Join(macro, ",") != strings.Join(batch, ",") {
		t.Fatalf("/macro ran on %v, want the /actions tabs %v", macro, batch)
	}
}
