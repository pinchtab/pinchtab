package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func timeoutTestHandlers() *Handlers {
	return New(&mockBridge{runningBrowser: config.BrowserChrome}, &config.RuntimeConfig{
		ActionTimeout: time.Second,
		AllowMacro:    true,
	}, nil, nil, nil)
}

func assertInvalidTimeout(t *testing.T, rec *httptest.ResponseRecorder, field string) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if body["code"] != "invalid_timeout" {
		t.Fatalf("code = %v, want invalid_timeout: %s", body["code"], rec.Body.String())
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, field) || !strings.Contains(message, "between 0.1 and 60 seconds") {
		t.Fatalf("error %q does not name %s and its limits", message, field)
	}
}

func TestActionTimeoutQueryOutsideTheLimitIsRefused(t *testing.T) {
	for _, v := range []string{"120", "0", "-1", "0.01", "soon"} {
		t.Run(v, func(t *testing.T) {
			rec := httptest.NewRecorder()
			timeoutTestHandlers().HandleAction(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&selector=%23go&timeout="+v, nil))
			if v == "soon" {
				if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "timeout must be a number of seconds") {
					t.Fatalf("status = %d, want 400 naming the timeout: %s", rec.Code, rec.Body.String())
				}
				return
			}
			assertInvalidTimeout(t, rec, "timeout")
		})
	}
}

func TestActionTimeoutQueryInsideTheLimitIsAccepted(t *testing.T) {
	rec := httptest.NewRecorder()
	timeoutTestHandlers().HandleAction(rec, httptest.NewRequest(http.MethodGet, "/action?kind=click&selector=%23go&timeout=60", nil))
	if strings.Contains(rec.Body.String(), "invalid_timeout") {
		t.Fatalf("timeout=60 was refused: %s", rec.Body.String())
	}
}

func TestMacroStepTimeoutOutsideTheLimitIsRefused(t *testing.T) {
	for _, v := range []string{"0.0001", "999", "-5"} {
		t.Run(v, func(t *testing.T) {
			rec := httptest.NewRecorder()
			body := `{"stepTimeout":` + v + `,"steps":[{"kind":"click","selector":"#go"}]}`
			timeoutTestHandlers().HandleMacro(rec, httptest.NewRequest(http.MethodPost, "/macro", strings.NewReader(body)))
			assertInvalidTimeout(t, rec, "stepTimeout")
		})
	}
}

func TestMacroStepTimeoutInsideTheLimitIsAccepted(t *testing.T) {
	rec := httptest.NewRecorder()
	body := `{"stepTimeout":30,"steps":[{"kind":"click","selector":"#go"}]}`
	timeoutTestHandlers().HandleMacro(rec, httptest.NewRequest(http.MethodPost, "/macro", strings.NewReader(body)))
	if strings.Contains(rec.Body.String(), "invalid_timeout") {
		t.Fatalf("stepTimeout 30 was refused: %s", rec.Body.String())
	}
}
