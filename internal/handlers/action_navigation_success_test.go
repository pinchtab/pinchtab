package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func navigatedActionResult() map[string]any {
	return map[string]any{
		"clicked":                true,
		bridge.ResultNavigated:   true,
		bridge.ResultLandedURL:   "https://www.iana.org/help/example-domains",
		bridge.ResultPreviousURL: "https://example.com/",
		bridge.ResultRefsStale:   true,
	}
}

// A click that succeeded in the browser must not appear in the failure telemetry.
// The assertion walks the WHOLE diagnostics envelope rather than naming today's two
// fields, so a failure counter added later inherits the rule instead of quietly
// counting successful clicks again.
func TestASuccessfulNavigatingClickIsInNoFailureField(t *testing.T) {
	ResetObservabilityForTests()

	mb := &mockBridge{availableActions: []string{bridge.ActionClick}, actionResult: navigatedActionResult()}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	handler := LoggingMiddleware(http.HandlerFunc(h.HandleAction))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"click","ref":"e1","nodeId":42,"tabId":"tab1"}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("click %d returned %d: %s", i, w.Code, w.Body.String())
		}
	}

	for _, site := range nonZeroFailureFields(t, DiagnosticsSnapshot("bridge")) {
		t.Errorf("%s — two successful clicks were counted as failures", site)
	}
}

// The counter is real, so the sweep above is not passing on an envelope it cannot
// read: a genuine 4xx moves exactly the fields the successful clicks must not.
func TestTheFailureSweepStillSeesAGenuineFailure(t *testing.T) {
	ResetObservabilityForTests()

	mb := &mockBridge{availableActions: []string{bridge.ActionClick}, executeActionErr: refNotFound("e99")}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	handler := LoggingMiddleware(http.HandlerFunc(h.HandleAction))

	req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"kind":"click","ref":"e99","nodeId":42,"tabId":"tab1"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code < 400 {
		t.Fatalf("the fixture no longer produces a failure: %d %s", w.Code, w.Body.String())
	}

	if len(nonZeroFailureFields(t, DiagnosticsSnapshot("bridge"))) == 0 {
		t.Fatal("a genuine 4xx moved no failure field, so the sweep above proves nothing")
	}
}

// nonZeroFailureFields walks the envelope and names every failure-shaped field
// carrying anything. It is keyed on the field NAME rather than on a list, which is
// what makes a future failure counter inherit the rule.
func nonZeroFailureFields(t *testing.T, envelope map[string]any) []string {
	t.Helper()

	var found []string
	var walk func(path string, value any)
	walk = func(path string, value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				// A container named "failures" holds the fields; it is not one.
				if isContainer(child) {
					walk(childPath, child)
					continue
				}
				if strings.Contains(strings.ToLower(key), "fail") && !emptyMetric(child) {
					found = append(found, fmt.Sprintf("%s = %v", childPath, child))
				}
			}
		case []map[string]any:
			for i, child := range typed {
				walk(fmt.Sprintf("%s[%d]", path, i), child)
			}
		}
	}
	walk("", envelope)

	// The recent-failure log is the other half of the same claim and is not named
	// "fail", so it is checked by its own path rather than by the sweep.
	if failures, ok := envelope["failures"].(map[string]any); ok {
		if recent, ok := failures["recent"].([]map[string]any); ok && len(recent) > 0 {
			found = append(found, fmt.Sprintf("failures.recent has %d entries", len(recent)))
		}
	}
	return found
}

func isContainer(value any) bool {
	switch value.(type) {
	case map[string]any, []map[string]any, []any:
		return true
	}
	return false
}

func emptyMetric(value any) bool {
	switch typed := value.(type) {
	case uint64:
		return typed == 0
	case int:
		return typed == 0
	case float64:
		return typed == 0
	case nil:
		return true
	}
	return false
}
