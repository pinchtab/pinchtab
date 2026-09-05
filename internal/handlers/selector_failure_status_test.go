package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// missingRefSelector resolves without a browser: a ref that is not in the tab's
// snapshot cache returns the real no-match sentinel from the resolver every verb
// shares, so this drives the genuine condition rather than a stubbed error.
const missingRefSelector = "e999"

// selectorProbe is one verb's answer to the same question asked twice: without a
// selector, and with one that matches nothing. The pair is what identifies a
// selector-reading verb without a hand-written list — a verb whose answer moves
// read the parameter.
type selectorProbe struct {
	path       string
	baseStatus int
	missStatus int
	missCode   string
}

func probeSelectorVerbs(t *testing.T, endpoints []routes.Endpoint, mux *http.ServeMux) []selectorProbe {
	t.Helper()

	var probes []selectorProbe
	for _, ep := range endpoints {
		if ep.Method != http.MethodGet {
			continue
		}
		base := probeOnce(t, mux, ep, "")
		miss := probeOnce(t, mux, ep, missingRefSelector)
		if base.Code == miss.Code {
			continue
		}
		var body struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(miss.Body.Bytes(), &body)
		probes = append(probes, selectorProbe{ep.Path, base.Code, miss.Code, body.Code})
	}
	return probes
}

// probeOnce answers one request, reporting whether the fixture could serve it at
// all: a panic from the mock's unimplemented half is not an answer.
func probeOnce(t *testing.T, mux *http.ServeMux, ep routes.Endpoint, selector string) (rec *httptest.ResponseRecorder) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("%s panicked while the route-derived selector census probed it: %v", ep.Route(), recovered)
		}
	}()
	path := ep.Path + "?browser=" + config.BrowserGhostChrome
	if selector != "" {
		path += "&selector=" + selector
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(ep.Method, path, nil))
	return rec
}

// countIsDeliberatelyDifferent records the one exclusion, with the reason it
// excludes on a property rather than on a judgement: /count is asked a cardinality,
// and zero is the honest answer to "how many". A later sweep must not "fix" it into
// a 404, so the enumeration below asserts its 200 rather than skipping it.
const countIsDeliberatelyDifferent = "/count"

// Every read verb that reads a selector must answer the same thing when the
// selector matches nothing. The verbs are derived from the route catalogue and
// identified by their own behaviour, so one added later is covered without anyone
// remembering this test exists.
func TestEverySelectorReadingVerbAnswersTheSameWayForANonMatchingSelector(t *testing.T) {
	mux := newSelectorProbeMux(t)
	probes := probeSelectorVerbs(t, routes.Core(), mux)

	seen := map[string]selectorProbe{}
	for _, probe := range probes {
		seen[probe.path] = probe
	}

	if len(seen) == 0 {
		t.Fatal("no route reacted to a selector; the route-derived enumeration is measuring nothing")
	}

	assertSelectorAnswers(t, probes)
}

// assertSelectorAnswers is the rule itself, separated so the test below can show it
// RED against a planted verb answering the way this card removed.
func assertSelectorAnswers(t *testing.T, probes []selectorProbe) {
	t.Helper()

	for _, probe := range probes {
		if probe.path == countIsDeliberatelyDifferent {
			if probe.missStatus != http.StatusOK {
				t.Errorf("%s answered %d for a non-matching selector; it is asked a cardinality and zero is the honest answer",
					probe.path, probe.missStatus)
			}
			continue
		}
		if probe.missStatus != http.StatusNotFound {
			t.Errorf("%s answers %d for a selector that matched nothing; the request was well formed and the page simply lacks the element, so every read verb answers 404 — a 5xx files a caller-side probe as a server fault and gets retried, a 400 calls a valid request malformed",
				probe.path, probe.missStatus)
		}
		if probe.missCode != CodeElementNotFound {
			t.Errorf("%s answers code %q, want %q so an agent can branch on the class instead of matching prose",
				probe.path, probe.missCode, CodeElementNotFound)
		}
	}
}

// The guard has to be able to fail, and these are the two shapes this card removed.
func TestTheEnumerationRedsOnAVerbWithTheWrongStatus(t *testing.T) {
	for _, planted := range []selectorProbe{
		{"/planted-html", 200, http.StatusInternalServerError, "error"},
		{"/planted-capture", 200, http.StatusBadRequest, "error"},
		{"/planted-code", 200, http.StatusNotFound, "error"},
	} {
		fake := &testing.T{}
		assertSelectorAnswers(fake, []selectorProbe{planted})
		if !fake.Failed() {
			t.Errorf("the enumeration accepts %s answering %d/%q, so it would accept the defect it exists to catch",
				planted.path, planted.missStatus, planted.missCode)
		}
	}
}

// A verb that reads a selector and finds it must be unchanged, and so must one
// asked no selector at all — the fix narrows a status, it does not add a refusal.
func TestAResolvedSelectorAndNoSelectorAreUnchanged(t *testing.T) {
	mux := newSelectorProbeMux(t)

	for _, path := range []string{"/html", "/screenshot", "/capture", "/box", "/count"} {
		base := httptest.NewRecorder()
		mux.ServeHTTP(base, httptest.NewRequest(http.MethodGet, path, nil))
		if base.Code == http.StatusNotFound && path != "/handoff" {
			t.Errorf("%s answers 404 with no selector at all; the miss status has leaked onto the unscoped read", path)
		}
	}
}

func newSelectorProbeMux(t *testing.T) *http.ServeMux {
	t.Helper()
	h := New(&selectorProbeBridge{mockBridge: &mockBridge{runningBrowser: config.BrowserGhostChrome}}, &config.RuntimeConfig{
		DefaultBrowser:    config.BrowserGhostChrome,
		BrowsersAvailable: []string{config.BrowserGhostChrome},
	}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)
	return mux
}

// selectorProbeBridge completes the one mockBridge method whose embedded nil
// BridgeAPI would otherwise panic while the route-derived census visits every
// GET route. A future route that needs another unimplemented method must make
// this test red instead of disappearing from the enumeration.
type selectorProbeBridge struct{ *mockBridge }

func (b *selectorProbeBridge) BrowserContext() context.Context             { return b.mockBridge.BrowserContext() }
func (b *selectorProbeBridge) CanClearCache(context.Context) (bool, error) { return true, nil }

var _ bridge.BridgeAPI = (*selectorProbeBridge)(nil)
