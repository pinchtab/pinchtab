package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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

	for i := range 2 {
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

// What this proves and what it does NOT. The bridge is mocked here, so it returns the
// navigated result whatever the request said — this pins that the outcome survives the
// endpoint and reaches the wire for both request shapes, and it would stay GREEN if the
// waitNav exclusion came back. The property that every form is MEASURED is owned by
// bridge.TestEveryFormOfANavigatingActionReportsWhereItLanded, which is the test that
// reds when either clause is restored.
func TestEveryFormOfANavigatingClickCarriesTheOutcomeOverTheWire(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"plain click", `{"kind":"click","ref":"e1","nodeId":42,"tabId":"tab1"}`},
		{"click that declares the navigation", `{"kind":"click","ref":"e1","nodeId":42,"tabId":"tab1","waitNav":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mb := &mockBridge{availableActions: []string{bridge.ActionClick}, actionResult: navigatedActionResult()}
			h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")

			h.HandleAction(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			var resp struct {
				Result map[string]any `json:"result"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got, _ := resp.Result[bridge.ResultLandedURL].(string); got == "" {
				t.Errorf("result carries no landed URL: %v", resp.Result)
			}
			if resp.Result[bridge.ResultRefsStale] != true {
				t.Errorf("result does not say the caller's refs are dead: %v — this is the form whose next action depends on the new page", resp.Result)
			}
		})
	}
}

type waitNavResult struct {
	Navigated *bool            `json:"navigated"`
	URL       string           `json:"url"`
	PostState *submitPostState `json:"postState"`
}

func postWaitNavClick(t *testing.T, b *submitActionBridge) waitNavResult {
	t.Helper()
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	w := postSubmitRequest(t, h, `{"kind":"click","tabId":"tab1","nodeId":42,"waitNav":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Result waitNavResult `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Result
}

func TestWaitNavClickOnABlockedSubmitReportsValidationBlocked(t *testing.T) {
	b := newSubmitActionBridge(t.Context())
	b.invalidFields = []submitInvalidField{{Name: "name", Label: "Name", Message: "Please fill out this field."}}

	got := postWaitNavClick(t, b)

	if got.Navigated == nil || *got.Navigated {
		t.Fatalf("navigated = %v, want false", got.Navigated)
	}
	if got.PostState == nil || got.PostState.Status != "blocked" || got.PostState.Signal != "validation_blocked" {
		t.Fatalf("postState = %+v, want blocked validation_blocked", got.PostState)
	}
	if len(got.PostState.InvalidFields) != 1 || got.PostState.InvalidFields[0] != b.invalidFields[0] {
		t.Fatalf("invalid fields = %+v, want %+v", got.PostState.InvalidFields, b.invalidFields)
	}
	if !slices.Equal(b.armedNodeIDs, []int64{42}) || b.actionCallsAtArm != 0 {
		t.Fatalf("armed node IDs = %v after %d dispatches, want node 42 armed before the dispatch", b.armedNodeIDs, b.actionCallsAtArm)
	}
	if !slices.Equal(b.disarmedNodeIDs, []int64{42}) {
		t.Fatalf("disarmed node IDs = %v, want the armed node 42", b.disarmedNodeIDs)
	}
}

func TestWaitNavClickOnANavigatingSubmitReportsTheLandingWithoutProbing(t *testing.T) {
	b := newSubmitActionBridge(t.Context())
	b.invalidFields = []submitInvalidField{{Name: "name", Message: "Please fill out this field."}}
	b.executeActionFn = func(context.Context, string, bridge.ActionRequest) (map[string]any, error) {
		return navigatedActionResult(), nil
	}

	got := postWaitNavClick(t, b)

	if got.Navigated == nil || !*got.Navigated || got.URL == "" {
		t.Fatalf("result = %+v, want navigated with a landed URL", got)
	}
	if got.PostState != nil {
		t.Fatalf("postState = %+v, want none on a navigating click", got.PostState)
	}
	if len(b.probedNodeIDs) != 0 {
		t.Fatalf("probed node IDs = %v, want no validation probe after a navigation", b.probedNodeIDs)
	}
}

func TestWaitNavClickThatStaysWithoutInvalidFieldsSaysItDidNotNavigate(t *testing.T) {
	got := postWaitNavClick(t, newSubmitActionBridge(t.Context()))

	if got.Navigated == nil || *got.Navigated {
		t.Fatalf("navigated = %v, want false", got.Navigated)
	}
	if got.PostState != nil {
		t.Fatalf("postState = %+v, want none when nothing blocked the click", got.PostState)
	}
}

func postPlainClick(t *testing.T, h *Handlers, body string) map[string]any {
	t.Helper()
	w := postSubmitRequest(t, h, body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Result
}

func plainClickPostState(t *testing.T, result map[string]any) *submitPostState {
	t.Helper()
	raw, ok := result["postState"]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("encode postState: %v", err)
	}
	var post submitPostState
	if err := json.Unmarshal(encoded, &post); err != nil {
		t.Fatalf("decode postState: %v", err)
	}
	return &post
}

const plainSubmitterClick = `{"kind":"click","tabId":"tab1","nodeId":42}`

func TestPlainClickOnABlockedSubmitReportsValidationBlocked(t *testing.T) {
	b := newSubmitActionBridge(t.Context())
	b.invalidFields = []submitInvalidField{{Name: "email", Label: "Email", Message: "Please include an '@' in the email address."}}
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)

	post := plainClickPostState(t, postPlainClick(t, h, plainSubmitterClick))

	if post == nil || post.Status != "blocked" || post.Signal != "validation_blocked" {
		t.Fatalf("postState = %+v, want blocked validation_blocked", post)
	}
	if len(post.InvalidFields) != 1 || post.InvalidFields[0] != b.invalidFields[0] {
		t.Fatalf("invalid fields = %+v, want %+v", post.InvalidFields, b.invalidFields)
	}
	if !slices.Equal(b.armedNodeIDs, []int64{42}) || b.actionCallsAtArm != 0 {
		t.Fatalf("armed node IDs = %v after %d dispatches, want node 42 armed before the dispatch", b.armedNodeIDs, b.actionCallsAtArm)
	}
	if !slices.Equal(b.disarmedNodeIDs, []int64{42}) {
		t.Fatalf("disarmed node IDs = %v, want the armed node 42", b.disarmedNodeIDs)
	}
}

func TestPlainClickOnAValidSubmitStaysPlain(t *testing.T) {
	for _, tc := range []struct {
		name     string
		navigate bool
	}{
		{"form that stays", false},
		{"form that navigates", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newSubmitActionBridge(t.Context())
			if tc.navigate {
				b.invalidFields = []submitInvalidField{{Name: "email", Message: "stale"}}
				b.executeActionFn = func(context.Context, string, bridge.ActionRequest) (map[string]any, error) {
					return navigatedActionResult(), nil
				}
			}
			h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)

			result := postPlainClick(t, h, plainSubmitterClick)

			if post := plainClickPostState(t, result); post != nil {
				t.Fatalf("postState = %+v, want none on a valid submit", post)
			}
			if result["clicked"] != true {
				t.Fatalf("result = %v, want clicked", result)
			}
			if tc.navigate && len(b.probedNodeIDs) != 0 {
				t.Fatalf("probed node IDs = %v, want no validation probe after a navigation", b.probedNodeIDs)
			}
		})
	}
}

func TestPlainClickOnANonSubmitterSkipsTheValidationRead(t *testing.T) {
	b := newSubmitActionBridge(t.Context())
	b.notSubmitter = true
	b.invalidFields = []submitInvalidField{{Name: "email", Message: "unreachable"}}
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)

	result := postPlainClick(t, h, plainSubmitterClick)

	if post := plainClickPostState(t, result); post != nil {
		t.Fatalf("postState = %+v, want none for a non-submitter", post)
	}
	if len(b.probedNodeIDs) != 0 || len(b.disarmedNodeIDs) != 0 {
		t.Fatalf("probed %v, disarmed %v, want neither call for a non-submitter", b.probedNodeIDs, b.disarmedNodeIDs)
	}
}

func TestBrowserPlainClickOnAnInvalidEmailSubmitIsValidationBlocked(t *testing.T) {
	h, tabID := submitFormHandlers(t, `
		<form action="/done"><label>Email <input type="email" name="email" value="not-an-email"></label><button id="b">Send</button></form>`)

	result := postPlainClick(t, h, `{"kind":"click","tabId":"`+tabID+`","selector":"#b"}`)

	post := plainClickPostState(t, result)
	if post == nil || post.Signal != "validation_blocked" || len(post.InvalidFields) != 1 || post.InvalidFields[0].Name != "email" {
		t.Fatalf("postState = %+v, want validation_blocked on the email field", post)
	}
}

func TestWaitNavClickOnANonSubmitterControlStillWatchesItsForm(t *testing.T) {
	b := newSubmitActionBridge(t.Context())
	b.notSubmitter = true
	b.invalidFields = []submitInvalidField{{Name: "email", Message: "Please include an '@' in the email address."}}

	got := postWaitNavClick(t, b)

	if got.PostState == nil || got.PostState.Signal != "validation_blocked" {
		t.Fatalf("postState = %+v, want validation_blocked for a requestSubmit button", got.PostState)
	}
}

const requestSubmitButtonForm = `
	<form action="/done"><input name="name" required value="Ada"><input type="email" name="email" value="not-an-email">
	<button type="button" id="b" onclick="this.form.requestSubmit()">Send</button></form>`

func TestBrowserWaitNavClickOnARequestSubmitButtonIsValidationBlocked(t *testing.T) {
	h, tabID := submitFormHandlers(t, requestSubmitButtonForm)

	result := postPlainClick(t, h, `{"kind":"click","tabId":"`+tabID+`","selector":"#b","waitNav":true}`)

	post := plainClickPostState(t, result)
	if post == nil || post.Signal != "validation_blocked" || len(post.InvalidFields) != 1 || post.InvalidFields[0].Name != "email" {
		t.Fatalf("postState = %+v, want validation_blocked on the email field", post)
	}
}

func TestBrowserSubmitClickOnARequestSubmitButtonIsValidationBlocked(t *testing.T) {
	h, tabID := submitFormHandlers(t, requestSubmitButtonForm)

	post := clickSubmitButton(t, h, tabID)

	if post.Signal != "validation_blocked" || len(post.InvalidFields) != 1 || post.InvalidFields[0].Name != "email" {
		t.Fatalf("postState = %+v, want validation_blocked on the email field", post)
	}
}
