package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
