package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// failingRequest drives the production middleware chain — activity outside, logging
// inside, exactly as internal/server stacks them — around a handler that fails the way a
// real one does, and returns what each of the three sinks recorded for that one request:
// the server log, the activity JSONL on disk, and failures.recent.
func failingRequest(t *testing.T, handler http.HandlerFunc) (logOutput string, activityLine map[string]any, failures []map[string]any) {
	t.Helper()

	logs := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	logDir := t.TempDir()
	store, err := activity.NewStore(logDir, 1)
	if err != nil {
		t.Fatalf("activity store: %v", err)
	}
	ResetObservabilityForTests()

	chain := activity.Middleware(store, "client", LoggingMiddleware(handler))
	req := httptest.NewRequest(http.MethodPost, "/tabs/tab1/action", strings.NewReader(`{"kind":"click","ref":"e99"}`))
	req.Header.Set("X-PinchTab-Source", "client")
	chain.ServeHTTP(httptest.NewRecorder(), req)

	return logs.String(), lastActivityEvent(t, logDir), recentFailureEvents(t)
}

func lastActivityEvent(t *testing.T, logDir string) map[string]any {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(logDir, "events-*.jsonl"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no activity log written in %s (glob err %v); the sink under test recorded nothing", logDir, err)
	}
	body, err := os.ReadFile(paths[len(paths)-1])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	var event map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &event); err != nil {
		t.Fatalf("decode activity line %q: %v", lines[len(lines)-1], err)
	}
	return event
}

func recentFailureEvents(t *testing.T) []map[string]any {
	t.Helper()

	snapshot := FailureSnapshot(LayerInstance)
	events, _ := snapshot["recent"].([]map[string]any)
	if len(events) == 0 {
		t.Fatal("failures.recent is empty; the request was not recorded as a failure at all")
	}
	return events
}

const staleRefReason = "action click: ref e99 not found - take a /snapshot first"

func failWithReason(w http.ResponseWriter, _ *http.Request) {
	httpx.ErrorCode(w, http.StatusInternalServerError, "action_failed", staleRefReason, true, nil)
}

// The defect: all three sinks recorded THAT a request failed and none recorded WHY. They
// are asserted together from one request, because the point is that they were
// independently reasonless — fixing one and calling it done is the failure mode.
func TestAllThreeSinksCarryTheReasonForOneFailedRequest(t *testing.T) {
	logs, event, failures := failingRequest(t, failWithReason)

	if !strings.Contains(logs, staleRefReason) {
		t.Errorf("the server log does not carry the reason the caller was given:\n%s", logs)
	}
	if !strings.Contains(logs, "action_failed") {
		t.Errorf("the server log does not carry the error code:\n%s", logs)
	}
	if got, _ := event["error"].(string); got != staleRefReason {
		t.Errorf("activity error = %q, want %q — the record carries tabId, action and ref, and was one field short of a complete one", got, staleRefReason)
	}
	if got, _ := event["code"].(string); got != "action_failed" {
		t.Errorf("activity code = %q, want action_failed", got)
	}
	last := failures[len(failures)-1]
	if got, _ := last["message"].(string); got != staleRefReason {
		t.Errorf("failures.recent message = %q, want %q", got, staleRefReason)
	}
	if got, _ := last["code"].(string); got != "action_failed" {
		t.Errorf("failures.recent code = %q, want action_failed", got)
	}
	if got, _ := last["type"].(string); got != "http_error" {
		t.Errorf("type = %q; it names the kind of record and must not be repurposed as the reason", got)
	}
}

// `grep level=ERROR` is what an operator actually runs, so that is what this asserts.
func TestRequestsLogAtASeverityAnOperatorCanRouteOn(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		wantLevel  string
		wantReason bool
	}{
		{name: "a server fault", status: http.StatusInternalServerError, wantLevel: "level=ERROR", wantReason: true},
		{name: "a caller error", status: http.StatusNotFound, wantLevel: "level=WARN", wantReason: true},
		{name: "a success", status: http.StatusOK, wantLevel: "level=INFO"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := &bytes.Buffer{}
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })

			handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status >= 400 {
					httpx.ErrorCode(w, tc.status, "action_failed", staleRefReason, false, nil)
					return
				}
				httpx.JSON(w, tc.status, map[string]any{"success": true})
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/tabs/tab1/action", nil))

			line := logs.String()
			if !strings.Contains(line, tc.wantLevel) {
				t.Errorf("a %d logged as %q, want %s — a level-based alert cannot see it otherwise", tc.status, strings.TrimSpace(line), tc.wantLevel)
			}
			if tc.wantReason && !strings.Contains(line, staleRefReason) {
				t.Errorf("a %d logged without its reason: %s", tc.status, line)
			}
			if !tc.wantReason && (strings.Contains(line, "error=") || strings.Contains(line, "code=")) {
				t.Errorf("a successful request logged an error-shaped line, so every line now reads like a failure: %s", line)
			}
		})
	}
}

// The sinks get the SANITIZED message — byte-identical to the response body — because the
// producer hands over what it serialized. A page-controlled error string reaching a
// terminal raw is the hazard ErrorCode already closed for the response; recording it
// would have re-opened it one frame away.
func TestTheRecordedReasonIsTheSanitizedForm(t *testing.T) {
	const raw = "action click: \x1b[31mred\x1b[0m \x07alert"

	recorder := httptest.NewRecorder()
	logs := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.ErrorCode(w, http.StatusInternalServerError, "action_failed", raw, false, nil)
	}))
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/action", nil))

	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error == raw {
		t.Fatal("the response body is the raw message, so this fixture no longer drives anything sanitizable")
	}
	if !strings.Contains(logs.String(), body.Error) {
		t.Errorf("the log does not carry the sanitized message the caller received (%q):\n%s", body.Error, logs.String())
	}
	if strings.Contains(logs.String(), "\x1b[31m") || strings.Contains(logs.String(), "\x07") {
		t.Errorf("the log carries the RAW page-controlled string; a sink reading it in a terminal gets what the response body was cleaned of:\n%q", logs.String())
	}
}

// Problem-Details responses must not be the one family without a reason.
func TestProblemResponsesRecordTheirReason(t *testing.T) {
	sw := &httpx.StatusWriter{ResponseWriter: httptest.NewRecorder(), Code: 200}
	httpx.Problem(sw, http.StatusConflict, "tab_locked", "tab tab1 is locked by agent-7", false, nil)

	if sw.FailureCode != "tab_locked" || sw.FailureMessage != "tab tab1 is locked by agent-7" {
		t.Errorf("Problem recorded code=%q message=%q; the Problem serialiser holds both and must hand them over like ErrorCode does", sw.FailureCode, sw.FailureMessage)
	}
}

// The reason travels OUTWARD through every wrapper: the handler holds the innermost
// writer, and both the logging and the activity middleware wrap their own StatusWriter
// around it. A recorder that only stamped the writer it was handed would leave whichever
// sink sits further out reasonless.
func TestTheReasonReachesEveryWrappedWriter(t *testing.T) {
	outer := &httpx.StatusWriter{ResponseWriter: httptest.NewRecorder(), Code: 200}
	inner := &httpx.StatusWriter{ResponseWriter: outer, Code: 200}

	httpx.ErrorCode(inner, http.StatusInternalServerError, "action_failed", staleRefReason, false, nil)

	if outer.FailureMessage != staleRefReason {
		t.Errorf("the outer writer recorded %q; a sink one wrapper further out stays reasonless", outer.FailureMessage)
	}
	if inner.FailureMessage != staleRefReason {
		t.Errorf("the writer the producer was handed recorded %q", inner.FailureMessage)
	}
}
