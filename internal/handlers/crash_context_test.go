package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// crashContextBridge serves requests on a browser context the test controls, so
// a crash can be staged against that exact context (and against a replacement).
type crashContextBridge struct {
	mockBridge
	browserCtx context.Context
}

func (m *crashContextBridge) BrowserContext() context.Context { return m.browserCtx }

func newCrashContextBridge(t *testing.T) (*crashContextBridge, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &crashContextBridge{browserCtx: ctx}, ctx
}

func crashHandlers(t *testing.T, b *crashContextBridge) *Handlers {
	t.Helper()
	bridge.ResetCrashMonitoringForTests()
	t.Cleanup(bridge.ResetCrashMonitoringForTests)
	return New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
}

func decodeErrorBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return body
}

func screenshotResponse(t *testing.T, h *Handlers) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.HandleScreenshot(w, httptest.NewRequest("GET", "/screenshot?selector=h1&format=png", nil))
	return w
}

func actionResponse(t *testing.T, h *Handlers) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.HandleAction(w, httptest.NewRequest("GET", "/action?kind=click&selector=h1", nil))
	return w
}

// The reported symptom: after the browser died every call blamed the caller's
// selector, and only /health knew what had happened.
func TestReadAndActionErrorsNameTheBrowserCrash(t *testing.T) {
	cases := []struct {
		name    string
		request func(*testing.T, *Handlers) *httptest.ResponseRecorder
	}{
		{name: "screenshot", request: screenshotResponse},
		{name: "action", request: actionResponse},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, ctx := newCrashContextBridge(t)
			h := crashHandlers(t, b)
			bridge.RecordCrashForContextTests(ctx, bridge.CrashEvent{Reason: "inspector.targetCrashed"})

			w := tc.request(t, h)
			if w.Code < 400 {
				t.Fatalf("status = %d, want a failure to annotate (body=%s)", w.Code, w.Body.String())
			}
			body := decodeErrorBody(t, w)

			message, _ := body["error"].(string)
			if !strings.Contains(message, "inspector.targetCrashed") {
				t.Errorf("error = %q, want the crash reason in the message", message)
			}

			details, ok := body["details"].(map[string]any)
			if !ok {
				t.Fatalf("response has no details: %s", w.Body.String())
			}
			// The CLI renders details.hint, so this is what reaches the user
			// without a separate /health call.
			hint, _ := details["hint"].(string)
			if !strings.Contains(hint, "browser crashed") || !strings.Contains(hint, "inspector.targetCrashed") {
				t.Errorf("hint = %q, want it to name the crash", hint)
			}
			if details["browserCrashed"] != true {
				t.Errorf("details = %v, want browserCrashed", details)
			}
		})
	}
}

// Without a crash the error must be exactly what it was before this change.
func TestErrorsAreUnchangedWithoutACrash(t *testing.T) {
	b, _ := newCrashContextBridge(t)
	h := crashHandlers(t, b)

	shot := decodeErrorBody(t, screenshotResponse(t, h))
	if shot["code"] != "error" {
		t.Errorf("screenshot code = %v, want the plain error code", shot["code"])
	}
	if _, ok := shot["details"]; ok {
		t.Errorf("screenshot response gained details without a crash: %v", shot)
	}
	if message, _ := shot["error"].(string); strings.Contains(message, "browser crashed") {
		t.Errorf("screenshot error mentions a crash that did not happen: %q", message)
	}

	act := decodeErrorBody(t, actionResponse(t, h))
	if message, _ := act["error"].(string); strings.Contains(message, "browser crashed") {
		t.Errorf("action error mentions a crash that did not happen: %q", message)
	}
	if details, ok := act["details"].(map[string]any); ok {
		if _, crashed := details["browserCrashed"]; crashed {
			t.Errorf("action response claims a crash: %v", details)
		}
	}
}

// A crash on the browser that has since been replaced must not annotate
// anything: the guard is generation-scoped, not a lifetime flag.
func TestCrashFromAPreviousGenerationDoesNotAnnotate(t *testing.T) {
	b, _ := newCrashContextBridge(t)
	h := crashHandlers(t, b)

	deadCtx, cancelDead := context.WithCancel(context.Background())
	defer cancelDead()
	bridge.RecordCrashForContextTests(deadCtx, bridge.CrashEvent{Reason: "killed"})

	body := decodeErrorBody(t, screenshotResponse(t, h))
	if message, _ := body["error"].(string); strings.Contains(message, "killed") {
		t.Errorf("error = %q, want no annotation from a replaced browser", message)
	}
	if _, ok := body["details"]; ok {
		t.Errorf("response annotated from a previous generation: %v", body)
	}
}

// After the browser is replaced the same handler must stop reporting the crash,
// which is what keeps one transient fault from failing every later request.
func TestRestartedBrowserStopsReportingTheOldCrash(t *testing.T) {
	b, ctx := newCrashContextBridge(t)
	h := crashHandlers(t, b)
	bridge.RecordCrashForContextTests(ctx, bridge.CrashEvent{Reason: "inspector.targetCrashed"})

	crashed := decodeErrorBody(t, screenshotResponse(t, h))
	if message, _ := crashed["error"].(string); !strings.Contains(message, "inspector.targetCrashed") {
		t.Fatalf("error = %q, want the crash reported while it is current", message)
	}

	restarted, cancelRestarted := context.WithCancel(context.Background())
	defer cancelRestarted()
	b.browserCtx = restarted

	recovered := decodeErrorBody(t, screenshotResponse(t, h))
	if message, _ := recovered["error"].(string); strings.Contains(message, "inspector.targetCrashed") {
		t.Errorf("error = %q, want the restarted browser to stop reporting the old crash", message)
	}
}

// context canceled from browser death must not read like a client disconnect or
// an action timeout, which produce the same bare string today.
func TestCanceledContextFromBrowserDeathIsDistinguishable(t *testing.T) {
	b, ctx := newCrashContextBridge(t)
	h := crashHandlers(t, b)
	bridge.RecordCrashForContextTests(ctx, bridge.CrashEvent{Reason: "unexpected context cancellation"})

	w := httptest.NewRecorder()
	h.errorWithCrashContext(w, http.StatusInternalServerError, context.Canceled)
	body := decodeErrorBody(t, w)

	message, _ := body["error"].(string)
	if !strings.Contains(message, "context canceled") {
		t.Errorf("error = %q, want the original cancellation text preserved", message)
	}
	if !strings.Contains(message, "unexpected context cancellation") {
		t.Errorf("error = %q, want the browser death named alongside it", message)
	}
	if body["code"] != "browser_crashed" {
		t.Errorf("code = %v, want browser_crashed", body["code"])
	}
}

// Annotating must not drop what the caller already relies on: the action path
// keeps its own code, retryable flag and details.
func TestCrashAnnotationPreservesExistingErrorCodeAndDetails(t *testing.T) {
	b, ctx := newCrashContextBridge(t)
	h := crashHandlers(t, b)
	bridge.RecordCrashForContextTests(ctx, bridge.CrashEvent{Reason: "killed"})

	w := httptest.NewRecorder()
	h.errorCodeWithCrashContext(w, 500, "action_failed", "action click: boom", true, map[string]any{"dispatch": "unconfirmed"})
	body := decodeErrorBody(t, w)

	if body["code"] != "action_failed" {
		t.Errorf("code = %v, want the original action_failed", body["code"])
	}
	if body["retryable"] != true {
		t.Errorf("retryable = %v, want it preserved", body["retryable"])
	}
	details, ok := body["details"].(map[string]any)
	if !ok || details["dispatch"] != "unconfirmed" {
		t.Fatalf("details = %v, want the original entries kept", body["details"])
	}
	if details["browserCrashReason"] != "killed" {
		t.Errorf("details = %v, want the crash reason added", details)
	}
}

// The action-execution failure path is a second, separately-reachable error
// site: a request that resolves fine and then fails in the browser must also
// name the crash, not just report "action click: ...".
func TestActionExecutionFailureNamesTheBrowserCrash(t *testing.T) {
	b, ctx := newCrashContextBridge(t)
	b.executeActionErr = errors.New("context canceled")
	h := crashHandlers(t, b)
	bridge.RecordCrashForContextTests(ctx, bridge.CrashEvent{Reason: "unexpected context cancellation"})

	w := httptest.NewRecorder()
	h.HandleAction(w, httptest.NewRequest("POST", "/action", strings.NewReader(`{"kind":"click"}`)))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body=%s)", w.Code, w.Body.String())
	}
	body := decodeErrorBody(t, w)
	if body["code"] != "action_failed" {
		t.Errorf("code = %v, want action_failed preserved", body["code"])
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, "unexpected context cancellation") {
		t.Errorf("error = %q, want the crash named on the execution failure path", message)
	}
	details, ok := body["details"].(map[string]any)
	if !ok || details["browserCrashReason"] != "unexpected context cancellation" {
		t.Fatalf("details = %v, want the crash reason", body["details"])
	}
}

type lostTabBridge struct {
	mockBridge
	err error
}

func (m *lostTabBridge) TabContext(string) (*bridge.TabHandle, string, error) { return nil, "", m.err }

func consoleLogsResponse(t *testing.T, b bridge.BridgeAPI) *httptest.ResponseRecorder {
	t.Helper()
	bridge.ResetCrashMonitoringForTests()
	t.Cleanup(bridge.ResetCrashMonitoringForTests)
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	w := httptest.NewRecorder()
	h.HandleGetConsoleLogs(w, httptest.NewRequest("GET", "/console?tabId=tab_dead", nil))
	return w
}

// The dominant post-crash error: a crash deletes every tab, so what a caller sees
// next is "tab not found", and that used to read as a bad id with no crash in it.
func TestATabDestroyedByACrashSaysSoInsteadOfBlamingTheID(t *testing.T) {
	crash := bridge.CrashEvent{Time: time.Now(), Reason: "unexpected context cancellation"}
	w := consoleLogsResponse(t, &lostTabBridge{err: &bridge.TabNotFoundError{TabID: "tab_dead", Crash: &crash}})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
	body := decodeErrorBody(t, w)
	if body["code"] != "browser_crashed" {
		t.Errorf("code = %v, want browser_crashed", body["code"])
	}
	message, _ := body["error"].(string)
	if !strings.Contains(message, "tab tab_dead not found") || !strings.Contains(message, "unexpected context cancellation") {
		t.Errorf("error = %q, want the miss and the crash reason", message)
	}
	details, _ := body["details"].(map[string]any)
	if details["browserCrashed"] != true || details["browserCrashReason"] != "unexpected context cancellation" {
		t.Errorf("details = %v, want the crash fields", details)
	}
	hint, _ := details["hint"].(string)
	if !strings.Contains(hint, "browser crashed") || !strings.Contains(hint, "open a new tab") {
		t.Errorf("hint = %q, want it to name the crash and the remedy", hint)
	}
}

func TestATabMissWithoutACrashIsTheBareNotFound(t *testing.T) {
	w := consoleLogsResponse(t, &lostTabBridge{err: &bridge.TabNotFoundError{TabID: "tab_dead"}})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	body := decodeErrorBody(t, w)
	if body["code"] != "error" || body["error"] != "tab tab_dead not found" {
		t.Errorf("body = %v, want the plain not-found", body)
	}
	if _, ok := body["details"]; ok {
		t.Errorf("details appeared without a crash: %v", body)
	}
}
