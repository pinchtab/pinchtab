package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// semanticInspectHandlers serves three same-named buttons through the accessibility
// snapshot, which is all a semantic selector needs — no browser, so these run in CI.
func semanticInspectHandlers(t *testing.T, withMatcher bool) *Handlers {
	t.Helper()

	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Save"},
			{Ref: "e1", Role: "button", Name: "Save"},
			{Ref: "e2", Role: "button", Name: "Save"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2, "e2": 3},
	}
	h := New(&findMockBridge{refCache: cache}, &config.RuntimeConfig{ActionTimeout: 10 * time.Second}, nil, nil, nil)
	if !withMatcher {
		// New always installs a matcher, so an unconfigured one has to be arranged
		// deliberately — see TestAnUnconfiguredMatcherIsNotAMissingElement.
		h.Matcher = nil
	}
	return h
}

// inspectEndpoints are the read surfaces that accept a semantic selector. The defect
// this covers is that the status mapping had one caller — the action path — so all six
// of these answered 500 for a selector that simply did not match, telling an agent to
// retry a page that is fine instead of to re-snapshot and pick another selector.
var inspectEndpoints = []struct {
	name string
	path string
	// countsMatches marks the endpoint whose answer to "nothing matched" is a NUMBER
	// rather than a refusal: /count reports zero, which is the correct answer to how
	// many elements match, not an error. Every other endpoint has to return an element.
	countsMatches bool
	serve         func(*Handlers, http.ResponseWriter, *http.Request)
}{
	{name: "attr", path: "/attr?name=id&selector=", serve: func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleGetAttr(w, r) }},
	{name: "value", path: "/value?selector=", serve: func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleGetValue(w, r) }},
	{name: "enabled", path: "/enabled?selector=", serve: func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleGetEnabled(w, r) }},
	{name: "visible", path: "/visible?selector=", serve: func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleGetVisible(w, r) }},
	{name: "checked", path: "/checked?selector=", serve: func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleGetChecked(w, r) }},
	{name: "count", path: "/count?selector=", countsMatches: true, serve: func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleCount(w, r) }},
}

func serveInspect(t *testing.T, h *Handlers, index int, selectorValue string) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := inspectEndpoints[index]
	req := httptest.NewRequest("GET", endpoint.path+strings.ReplaceAll(selectorValue, " ", "%20"), nil)
	w := httptest.NewRecorder()
	endpoint.serve(h, w, req)
	return w
}

// A selector that matched nothing is the caller's problem, so it must read as 404 on
// every read surface — asserted through the HANDLER, because the mapper being right is
// what the previous test proved while five callers never reached it.
//
// Both empty-match shapes are driven: a bare selector nothing matches, and a positional
// wrapper over one. They are separate branches of the refusal — the wrapper case counts
// the base's matches before it can tell an empty set from a wrong index — so dropping the
// sentinel from either is invisible to a test that drives only the other.
//
// /count answers differently, and correctly: asked how many match a bare selector it says
// zero, which is a number, not a refusal. Asked for a WRAPPED element it is resolving one
// element again, so the miss is a miss. (A wrapper over a css selector nothing matches
// still counts 0 rather than refusing — a pre-existing inconsistency in count's
// single-node path, not one this change introduces.)
func TestASemanticSelectorThatMatchesNothingIs404OnEveryInspectEndpoint(t *testing.T) {
	for index, endpoint := range inspectEndpoints {
		for _, tc := range []struct {
			selector   string
			countIsNum bool
		}{
			{selector: "role:slider Volume", countIsNum: true},
			{selector: "nth:0:role:slider Volume"},
		} {
			t.Run(endpoint.name+" "+tc.selector, func(t *testing.T) {
				w := serveInspect(t, semanticInspectHandlers(t, true), index, tc.selector)

				if endpoint.countsMatches && tc.countIsNum {
					if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"count":0`) {
						t.Fatalf("status = %d body %s, want 200 with a count of 0 — zero is the answer to how many matched", w.Code, w.Body.String())
					}
					return
				}
				if w.Code != http.StatusNotFound {
					t.Fatalf("status = %d, want 404; a 5xx tells an agent to retry a page that is fine (body %s)", w.Code, w.Body.String())
				}
				if !strings.Contains(w.Body.String(), "no matching element found") {
					t.Errorf("body %s does not say the selector matched nothing", w.Body.String())
				}
			})
		}
	}
}

// The out-of-range refusal keeps naming the caller's zero-based index and the bare
// selector's match count, at 404, on every read surface.
func TestAnOutOfRangeWrapperIndexIs404OnEveryInspectEndpoint(t *testing.T) {
	for index, endpoint := range inspectEndpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			w := serveInspect(t, semanticInspectHandlers(t, true), index, "nth:7:role:button Save")

			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body %s)", w.Code, w.Body.String())
			}
			body := w.Body.String()
			for _, needle := range []string{"index 7", "out of range", "3 element(s)"} {
				if !strings.Contains(body, needle) {
					t.Errorf("body %s does not carry %q", body, needle)
				}
			}
			if strings.Contains(body, "nth:8") {
				t.Errorf("body %s names the translated one-based index instead of what the caller sent", body)
			}
		})
	}
}

// Criterion five: do not blanket-404 the inspect path. An unconfigured matcher is a
// server-side gap, not a bad selector, and it keeps its own code on every surface.
func TestAnUnconfiguredMatcherIsNotAMissingElement(t *testing.T) {
	for index, endpoint := range inspectEndpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			w := serveInspect(t, semanticInspectHandlers(t, false), index, "role:button Save")

			if w.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, want 501 for an unconfigured matcher (body %s)", w.Code, w.Body.String())
			}
		})
	}
}

// The action path and the inspect path must reach the SAME mapping. A second copy of
// the switch is how they drifted apart, so this asserts agreement per outcome rather
// than trusting that both call the same name today.
func TestBothPathsAgreeOnTheStatusForTheSameFailure(t *testing.T) {
	h := semanticInspectHandlers(t, true)

	for _, tc := range []struct {
		name     string
		selector string
		want     int
	}{
		{"no match", "role:slider Volume", http.StatusNotFound},
		{"out of range", "nth:7:role:button Save", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The in-scope resolver, not resolveActionRequestSelector: the outer one
			// probes the topmost dialog through CDP, which no mock can answer.
			req := bridge.ActionRequest{Selector: tc.selector}
			resolution, err := h.resolveActionRequestSelectorInScope(context.Background(), "tab1", "", 0, false, &req)
			if err == nil {
				t.Fatal("the action path accepted a selector the inspect path refuses")
			}
			if resolution.status != tc.want {
				t.Errorf("action path status = %d, want %d", resolution.status, tc.want)
			}
			if w := serveInspect(t, h, 0, tc.selector); w.Code != resolution.status {
				t.Errorf("inspect /attr answered %d while the action path answered %d for the same failure", w.Code, resolution.status)
			}
		})
	}
}

// The status must come from the sentinel, not from the wording. The out-of-range
// message was being reworded while this mapping was unified, and a status keyed on text
// silently changes with it.
func TestTheStatusSurvivesRewordingTheMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"reworded miss keeps its 404", fmt.Errorf("%w: absolutely nothing like that on the page", ErrElementNotFound), http.StatusNotFound},
		{"reworded unavailable keeps its 501", fmt.Errorf("%w: turn the matcher on", ErrSemanticMatcherUnavailable), http.StatusNotImplemented},
		{"the old wording alone buys nothing", errors.New("no matching element found"), http.StatusInternalServerError},
		{"out-of-range wording alone buys nothing", errors.New(`index 7 is out of range, "role:button Save" matched 3 element(s)`), http.StatusInternalServerError},
		{"not-configured wording alone buys nothing", errors.New("semantic selectors require a matcher (not configured)"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectorFailureStatus(tc.err); got != tc.want {
				t.Errorf("selectorFailureStatus(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// Criterion three, structurally: every path must REACH the one mapper. The inspect
// path and the action path each mapped selector failures themselves, and the two
// switches drifted — semantic misses answered 404 on /action and 500 on all six read
// endpoints, while the capture endpoints hardcoded 400 and the inspect ones 500.
//
// This pins the call sites; TestBothPathsAgreeOnTheStatusForTheSameFailure pins the
// outcome. Both are needed: a census cannot see a second mapper under a new name, and an
// agreement test cannot see a caller that stops mapping at all.
//
// The census reads BOTH spellings, because a site either asks the mapper for a status
// it must carry itself (the screenshot clip path returns a statusError) or hands the
// whole refusal to respondSelectorFailure, which asks the same owner.
// A status literal sitting in the same call as the selector-failure wrapper:
// `httpx.Error(w, 400, frameScopedSelectorError(...))` and
// `&statusError{500, frameScopedSelectorError(...)}` were both live before this card.
var statusLiteralBesideSelectorFailure = regexp.MustCompile(`[,{]\s*[45]\d\d\s*,`)

func TestOneMapperServesEverySelectorResolvingPath(t *testing.T) {
	pkg := srccensus.Load(t, ".", 20)

	callers := map[string]bool{}
	for _, name := range []string{"selectorFailureStatus", "respondSelectorFailure"} {
		for _, site := range pkg.Calls(t, name) {
			callers[site.Func] = true
		}
	}
	exits := map[string]bool{}
	for _, site := range pkg.Calls(t, "respondSelectorFailure") {
		exits[site.Func] = true
	}
	for _, required := range []string{"HandleText", "HandleSnapshot", "handleInspect"} {
		if !exits[required] {
			t.Errorf("%s does not answer a selector failure through respondSelectorFailure; asking the owner for a status and writing it with httpx.Error answers the right status with the generic code", required)
		}
	}
	for _, required := range []string{
		"inspectElement", "resolveActionRequestSelectorInScope", "handleInspect",
		"resolveScreenshotClip", "HandleText", "HandleSnapshot",
	} {
		if !callers[required] {
			t.Errorf("%s no longer maps its selector failures through the one owner; a second copy of the status switch is how the read paths came to give five different answers", required)
		}
	}

	// The deleted twin must not come back: it read the raw no-match sentinel while
	// this one reads the wrapped one, and they agreed on 404 by coincidence.
	if _, found := pkg.Func("selectorResolutionHTTPStatus"); found {
		t.Error("selectorResolutionHTTPStatus is back; one question needs one mapper, and a second one reading a different sentinel is how these statuses drifted")
	}

	// The shape this card removed, banned so a verb added later cannot bring it
	// back: a selector failure written out beside a status LITERAL. Building the
	// wrapper and returning it is fine — that is what a producer does, and its
	// caller answers through the owner — so the ban is on the answering line, which
	// is where /screenshot said 400 and /html said 500 for the same condition.
	for _, file := range srccensus.Tree(t, ".", 20) {
		for i, line := range strings.Split(file.Text, "\n") {
			if !strings.Contains(line, "frameScopedSelectorError(") {
				continue
			}
			if statusLiteralBesideSelectorFailure.MatchString(line) {
				t.Errorf("%s:%d answers a selector failure with a hardcoded status: %s\nask selectorFailureStatus, or hand the whole refusal to respondSelectorFailure",
					file.Name, i+1, strings.TrimSpace(line))
			}
		}
	}

	// The string-matching mapper is gone on purpose: a status keyed on message text
	// changes whenever a sentence is reworded, and these messages are reworded often.
	if _, found := pkg.Func("semanticSelectorHTTPStatus"); found {
		t.Error("semanticSelectorHTTPStatus is back; the semantic statuses come from sentinels now, so a message-matching mapper would silently re-key them on wording")
	}
}
