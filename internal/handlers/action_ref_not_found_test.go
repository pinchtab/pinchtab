package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// A nodeId keeps selector resolution out of CDP so the endpoint can be driven with the
// package mock; the ref rides along because it is what the refusal has to name.
func targetedActionBody(kind, extra string) string {
	return fmt.Sprintf(`{"kind":%q,"ref":"e99","nodeId":42,"tabId":"tab1"%s}`, kind, extra)
}

func postTargetedAction(t *testing.T, kind, extra string, executeErr error) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	mb := &mockBridge{
		availableActions: []string{kind},
		executeActionErr: executeErr,
	}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	w := httptest.NewRecorder()
	h.HandleAction(w, httptest.NewRequest("POST", "/action", strings.NewReader(targetedActionBody(kind, extra))))
	return w, decodeJSONMap(t, w.Body.Bytes())
}

// The reported defect: the same unresolvable ref answered 404 with the remedy on a declared
// submit and 500 retryable:true with the recovery matcher's score on every other verb. It is
// the same fact about the same request, so it is the same answer.
func TestAnUnresolvableRefIsANotFoundForEveryVerb(t *testing.T) {
	for _, kind := range []string{bridge.ActionClick, bridge.ActionHover, bridge.ActionType, bridge.ActionFocus, bridge.ActionCheck} {
		t.Run(kind, func(t *testing.T) {
			w, body := postTargetedAction(t, kind, "", refNotFound("e99"))

			if w.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d — the ref is absent, which is the caller's request being wrong, not the server failing: %s", w.Code, http.StatusNotFound, w.Body.String())
			}
			if body["code"] != "ref_not_found" {
				t.Errorf("code = %v, want ref_not_found", body["code"])
			}
			if retryable, present := body["retryable"]; present && retryable != false {
				t.Errorf("retryable = %v; retrying an absent ref can never succeed, so advertising a retry sends the caller into a loop that cannot end", retryable)
			}
			if got, want := body["error"], "ref e99 not found - take a /snapshot first"; got != want {
				t.Errorf("error = %q, want %q — the remedy is the whole value of this refusal", got, want)
			}
		})
	}
}

func postAbsentRef(t *testing.T, cache *bridge.RefCache) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	mb := &mockBridge{availableActions: []string{bridge.ActionHover}, refCache: cache, executeActionErr: bridge.ErrElementStale}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	w := httptest.NewRecorder()
	h.HandleAction(w, httptest.NewRequest("POST", "/action", strings.NewReader(targetedActionBody(bridge.ActionHover, ""))))
	return w, decodeJSONMap(t, w.Body.Bytes())
}

func TestARefNotFoundNamesTheTabItSearched(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cache     *bridge.RefCache
		wantError string
	}{
		{
			name:      "a tab never snapshotted",
			wantError: "ref e99 not found - tab tab1 has no snapshot - take a /snapshot first",
		},
		{
			name:      "a stale ref on a snapshotted tab",
			cache:     &bridge.RefCache{Refs: map[string]int64{"e1": 7}},
			wantError: "ref e99 not found - take a /snapshot first",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, body := postAbsentRef(t, tc.cache)

			if w.Code != http.StatusNotFound || body["code"] != "ref_not_found" {
				t.Fatalf("status = %d code = %v, want 404 ref_not_found: %s", w.Code, body["code"], w.Body.String())
			}
			if got, _ := body["error"].(string); !strings.HasPrefix(got, tc.wantError+": ") {
				t.Errorf("error = %q, want it to open with %q", got, tc.wantError)
			}
			details, _ := body["details"].(map[string]any)
			if got := details["tabId"]; got != "tab1" {
				t.Errorf("details.tabId = %v, want tab1 — the remedy does not say which tab to snapshot: %s", got, w.Body.String())
			}
		})
	}
}

// The card's third case: a click naming nothing to click is the caller having omitted a
// required argument. The bridge refuses it with the sentinel (its own census pins that), and
// the endpoint answers the status an omitted argument deserves rather than a server fault
// the caller is told to retry.
func TestAClickNamingNothingToClickIsACallerError(t *testing.T) {
	mb := &mockBridge{
		availableActions: []string{bridge.ActionClick},
		executeActionErr: bridge.NewInvalidActionRequestError("need selector, ref, nodeId, or x/y coordinates"),
	}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	w := httptest.NewRecorder()
	h.HandleAction(w, httptest.NewRequest("POST", "/action", strings.NewReader(`{"kind":"click","tabId":"tab1"}`)))
	body := decodeJSONMap(t, w.Body.Bytes())

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if body["code"] != "invalid_action_request" {
		t.Errorf("code = %v, want invalid_action_request", body["code"])
	}
	if body["retryable"] == true {
		t.Error("an omitted argument was advertised as retryable; the identical body is unsatisfiable on every attempt")
	}
	if got, _ := body["error"].(string); !strings.Contains(got, "need selector, ref, nodeId, or x/y coordinates") {
		t.Errorf("error = %q, which does not say what the caller left out", got)
	}
}

// The submit path and the ordinary one word the same fact identically because ONE site
// spells the remedy. Four sites used to format it, which is how they came to disagree about
// the status while agreeing about the words. A new copy reds here by file and line, and the
// count is checked against a floor so a renamed constant cannot pass the guard vacuously.
func TestOneSiteSpellsTheSnapshotRemedy(t *testing.T) {
	var sites []string
	for name, src := range srccensus.Load(t, ".", 80).Sources() {
		for i, line := range strings.Split(src, "\n") {
			if strings.Contains(line, `take a /snapshot first`) {
				sites = append(sites, fmt.Sprintf("%s:%d %s", name, i+1, strings.TrimSpace(line)))
			}
		}
	}
	slices.Sort(sites)

	if len(sites) != 1 {
		t.Fatalf("the snapshot remedy is spelled at %d sites; one owner or they drift:\n  %s", len(sites), strings.Join(sites, "\n  "))
	}
	if !strings.Contains(sites[0], "takeASnapshotFirst =") {
		t.Errorf("the one site is not the constant every refusal composes from: %s", sites[0])
	}
}

// Both sentences the endpoint answers carry the remedy, so the constant is load-bearing
// rather than merely present.
func TestBothRefusalsCarryTheSnapshotRemedy(t *testing.T) {
	for name, err := range map[string]error{
		"an absent ref":    refNotFound("e99"),
		"a stale submit":   ErrStaleSubmitTarget,
		"a gone drag drop": targetNotFound("drag destination e7"),
	} {
		if !strings.Contains(err.Error(), takeASnapshotFirst) {
			t.Errorf("%s refuses without the only advice that fixes it: %s", name, err.Error())
		}
	}
	if errors.Is(ErrStaleSubmitTarget, ErrTargetNotFound) {
		t.Error("the stale-submit refusal now matches the absent-target arm, which would swallow its dispatch state and its own code")
	}
}

// The matcher's threshold and score describe how recovery searched, not what the caller can
// do. They stay in the recovery record, which the refusal carries in details.
func TestTheRecoveryMatchersInternalsStayOutOfTheCallerFacingError(t *testing.T) {
	mb := &mockBridge{availableActions: []string{bridge.ActionHover}}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	req := bridge.ActionRequest{Kind: bridge.ActionHover, Ref: "e99", TabID: "tab1"}

	_, _, rr, err := h.executeActionResilient(context.Background(), &req, h.Config, "tab1", true)

	if !errors.Is(err, ErrTargetNotFound) {
		t.Fatalf("exhausted recovery returned %v, which no arm classifies as an absent target", err)
	}
	for _, leak := range []string{"threshold", "best:", "recovery failed", "cached intent"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("the caller-facing error carries the matcher's internals (%q): %s", leak, err.Error())
		}
	}
	if rr == nil {
		t.Fatal("no recovery record was published, so the diagnosis the message dropped is nowhere")
	}
	if !strings.Contains(rr.Error, "e99") {
		t.Errorf("the recovery record does not say why recovery failed: %+v", rr)
	}
}

// retryable now answers the question it names. It used to be !submitClick — whether the
// caller declared a submit, which says nothing about the failure.
func TestRetryableIsDerivedFromTheFailureNotFromTheVerb(t *testing.T) {
	for _, tc := range []struct {
		name              string
		err               error
		mayHaveLanded     bool
		wantRetryable     bool
		wantBecauseItWere string
	}{
		{
			name:              "an absent target",
			err:               refNotFound("e99"),
			wantBecauseItWere: "an absent ref stays absent however often it is asked for",
		},
		{
			name:              "an absent drag destination",
			err:               targetNotFound("drag destination e7"),
			wantBecauseItWere: "the destination is gone, and the identical drag has nowhere to land",
		},
		{
			name:              "a body the action cannot satisfy",
			err:               bridge.NewInvalidActionRequestError("need selector, ref, nodeId, or x/y coordinates"),
			wantBecauseItWere: "the same body is unsatisfiable on every attempt",
		},
		{
			name:              "a transport failure",
			err:               errors.New("dispatch mouse event: websocket closed"),
			wantRetryable:     true,
			wantBecauseItWere: "a dropped connection is exactly what the flag exists for",
		},
		{
			name:              "a transport failure after a dispatch that may have landed",
			err:               errors.New("dispatch mouse event: websocket closed"),
			mayHaveLanded:     true,
			wantBecauseItWere: "repeating a submit that may already have posted is worse than failing",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := actionFailureIsRetryable(tc.err, tc.mayHaveLanded); got != tc.wantRetryable {
				t.Errorf("retryable = %v, want %v — %s", got, tc.wantRetryable, tc.wantBecauseItWere)
			}
		})
	}
}

func decodeJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response: %v body=%s", err, raw)
	}
	return out
}

func readAbsentRef(t *testing.T, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	mb := &mockBridge{refCache: &bridge.RefCache{Refs: map[string]int64{"e1": 7}}}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)
	mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w, decodeJSONMap(t, w.Body.Bytes())
}

func TestAReadNamingAnAbsentRefAnswersAsAnActionDoes(t *testing.T) {
	click, clickBody := postAbsentRef(t, &bridge.RefCache{Refs: map[string]int64{"e1": 7}})
	clickError, _ := clickBody["error"].(string)
	for _, path := range []string{"/box?tabId=tab1&selector=e99", "/screenshot?tabId=tab1&format=png&selector=e99"} {
		t.Run(path, func(t *testing.T) {
			w, body := readAbsentRef(t, path)

			if w.Code != click.Code || body["code"] != clickBody["code"] {
				t.Fatalf("status = %d code = %v, want %d %v as on /action: %s", w.Code, body["code"], click.Code, clickBody["code"], w.Body.String())
			}
			got, _ := body["error"].(string)
			if want := "ref e99 not found - take a /snapshot first"; got != want || !strings.HasPrefix(clickError, want) {
				t.Errorf("error = %q, want %q, the sentence /action opens with (%q)", got, want, clickError)
			}
			details, _ := body["details"].(map[string]any)
			if details["tabId"] != "tab1" {
				t.Errorf("details.tabId = %v, want tab1: %s", details["tabId"], w.Body.String())
			}
		})
	}
}

func TestASelectorMatchingNothingKeepsElementNotFound(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	w := httptest.NewRecorder()

	h.respondSelectorFailure(w, "tab1", fmt.Errorf("%w: %q: %w", ErrElementNotFound, "#missing", bridge.ErrSelectorNoMatch))

	if body := decodeJSONMap(t, w.Body.Bytes()); w.Code != http.StatusNotFound || body["code"] != CodeElementNotFound {
		t.Errorf("status = %d code = %v, want 404 %s: %s", w.Code, body["code"], CodeElementNotFound, w.Body.String())
	}
}
