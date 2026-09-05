package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// tabScopedFamily is every /tabs/{id}/... handler that decodes a typed body and
// reconciles it against the path id through decodeJSONBody + requirePathTabIDMatch.
// The prologue lives in one place now, so the point of walking all six is that a
// handler which stops routing through it fails here rather than drifting quietly.
var tabScopedFamily = []struct {
	name      string
	handler   func(*Handlers) http.HandlerFunc
	validBody string
}{
	{"emulation/viewport", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetViewport }, `{"width":800,"height":600}`},
	{"emulation/geolocation", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetGeolocation }, `{"latitude":1,"longitude":2}`},
	{"emulation/media", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetMedia }, `{"feature":"prefers-color-scheme","value":"dark"}`},
	{"emulation/offline", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetOffline }, `{"offline":true}`},
	{"emulation/credentials", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetCredentials }, `{"username":"u","password":"p"}`},
	{"emulation/headers", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetHeaders }, `{"headers":{"X-Probe":"1"}}`},
}

func driveTabScoped(t *testing.T, handler http.HandlerFunc, pathID, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/tabs/tab_abc/probe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if pathID != "" {
		req.SetPathValue("id", pathID)
	}
	w := httptest.NewRecorder()
	handler(w, req)

	var resp struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp.Error
}

func TestTabScopedPrologueRejections(t *testing.T) {
	oversized := `{"tabId":"` + strings.Repeat("x", maxBodySize+1) + `"}`

	cases := []struct {
		name      string
		pathID    string
		body      func(valid string) string
		wantError string
	}{
		{
			name:      "missing tab id",
			pathID:    "",
			body:      func(valid string) string { return valid },
			wantError: "tab id required",
		},
		{
			name:      "malformed body",
			pathID:    "tab_abc",
			body:      func(string) string { return `{"tabId":` },
			wantError: "decode: unexpected EOF",
		},
		{
			name:      "oversized body",
			pathID:    "tab_abc",
			body:      func(string) string { return oversized },
			wantError: "decode: http: request body too large",
		},
		{
			name:      "body tabId disagrees with the path",
			pathID:    "tab_abc",
			body:      func(string) string { return `{"tabId":"tab_other"}` },
			wantError: "tabId in body does not match path id",
		},
	}

	for _, ep := range tabScopedFamily {
		for _, tc := range cases {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
				status, gotError := driveTabScoped(t, ep.handler(h), tc.pathID, tc.body(ep.validBody))

				if status != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400 (error=%q)", status, gotError)
				}
				if gotError != tc.wantError {
					t.Fatalf("error = %q, want %q", gotError, tc.wantError)
				}
			})
		}
	}

	if len(tabScopedFamily) == 0 || len(cases) == 0 {
		t.Fatal("the family or the case table is empty, so this test proves nothing")
	}
}

// TestTabScopedPrologueDecodesBeforeCheckingThePathID pins the one ordering the
// fold changed: the body is decoded first, so a request that is both malformed
// and missing its path id reports the decode error. No client can reach it —
// ServeMux redirects /tabs//... rather than matching {id} to an empty segment,
// which TestEmptyPathSegmentNeverReachesATabHandler holds — but the direct-call
// precedence is now uniform across the family instead of split two ways.
func TestTabScopedPrologueDecodesBeforeCheckingThePathID(t *testing.T) {
	for _, ep := range tabScopedFamily {
		t.Run(ep.name, func(t *testing.T) {
			h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
			status, gotError := driveTabScoped(t, ep.handler(h), "", `{"tabId":`)

			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (error=%q)", status, gotError)
			}
			if gotError != "decode: unexpected EOF" {
				t.Fatalf("error = %q, want the decode error to win over the missing path id", gotError)
			}
		})
	}
}

// TestEmptyPathSegmentNeverReachesATabHandler is what makes the reordering above
// safe to reason about: the "missing tab id" branch is unreachable over HTTP, so
// its position in the prologue cannot change any response a client can observe.
func TestEmptyPathSegmentNeverReachesATabHandler(t *testing.T) {
	reached := false
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tabs/{id}/emulation/viewport", func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tabs//emulation/viewport", strings.NewReader("{}")))

	if reached {
		t.Fatal("an empty {id} segment reached the handler, so the missing-tab-id branch is client-reachable after all and its ordering is observable")
	}
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want 307; ServeMux is expected to redirect the empty segment rather than match it", w.Code)
	}
}

type echoingTabBridge struct {
	*mockBridge
}

func (b *echoingTabBridge) TabContext(tabID string) (*bridge.TabHandle, string, error) {
	if tabID == "" {
		tabID = "tabA"
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return bridge.NewTabHandle(ctx), tabID, nil
}

func newTwoTabHandlers(t *testing.T) *Handlers {
	t.Helper()
	stubNavigateHostResolution(t, func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	const target = "http://localhost/page.html"
	return New(&echoingTabBridge{mockBridge: &mockBridge{currentURL: target, navigateResult: &bridge.NavigateResult{URL: target}}}, &config.RuntimeConfig{}, nil, nil, nil)
}

var postBodyVerbs = []struct {
	endpoint string
	handler  func(*Handlers) http.HandlerFunc
	body     string
}{
	{"/navigate", func(h *Handlers) http.HandlerFunc { return h.HandleNavigate }, `{"url":"http://localhost/page.html","tabId":"tabB"}`},
	{"/action", func(h *Handlers) http.HandlerFunc { return h.HandleAction }, `{"kind":"click","selector":"#btn","tabId":"tabB"}`},
	{"/actions", func(h *Handlers) http.HandlerFunc { return h.HandleActions }, `{"actions":[{"kind":"click","selector":"#btn","tabId":"tabB"}]}`},
	{"/frame", func(h *Handlers) http.HandlerFunc { return h.HandleFrame }, `{"target":"main","tabId":"tabB"}`},
}

func drive(h *Handlers, handler http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func actedOnTab(w *httptest.ResponseRecorder) string {
	if header := w.Header().Get(activity.HeaderPTTabID); header != "" {
		return header
	}
	var resp struct {
		TabID string `json:"tabId"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.TabID
}

func TestPostQueryParametersAreRefusedNotDropped(t *testing.T) {
	for _, verb := range postBodyVerbs {
		t.Run(verb.endpoint, func(t *testing.T) {
			h := newTwoTabHandlers(t)
			w := drive(h, verb.handler(h), http.MethodPost, verb.endpoint+"?tabId=tabB", verb.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("POST %s?tabId= answered %d, acting on %q; a query the POST branch never reads must be refused: %s", verb.endpoint, w.Code, actedOnTab(w), w.Body.String())
			}
			for _, want := range []string{"tabId (send it in the JSON body)", "POST " + verb.endpoint} {
				if !strings.Contains(w.Body.String(), want) {
					t.Fatalf("refusal %s does not say %q", w.Body.String(), want)
				}
			}
			w = drive(h, verb.handler(h), http.MethodPost, verb.endpoint+"?stray=1", verb.body)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "stray:") || strings.Contains(w.Body.String(), "JSON body)") {
				t.Fatalf("a stray query key must be refused by name without the body hint: %d %s", w.Code, w.Body.String())
			}
			w = drive(h, verb.handler(h), http.MethodPost, verb.endpoint+"?browser=chrome", verb.body)
			if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "would silently drop this query parameter") {
				t.Fatalf("?browser= is the router's parameter and the MCP client's POST shape; refusing it breaks a shipped caller: %s", w.Body.String())
			}
			w = drive(h, verb.handler(h), http.MethodPost, verb.endpoint+"?tabId=", verb.body)
			if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "would silently drop this query parameter") {
				t.Fatalf("an empty query value is absent, not a dropped parameter: %s", w.Body.String())
			}
		})
	}
}

func TestTargetingActsOnTheTabNamedNotTheCurrentOne(t *testing.T) {
	h := newTwoTabHandlers(t)
	cases := []struct {
		name    string
		method  string
		target  string
		body    string
		handler http.HandlerFunc
	}{
		{"GET /navigate?tabId", http.MethodGet, "/navigate?url=http://localhost/page.html&tabId=tabB", "", h.HandleNavigate},
		{"POST /navigate body tabId", http.MethodPost, "/navigate", `{"url":"http://localhost/page.html","tabId":"tabB"}`, h.HandleNavigate},
		{"GET /action?tabId", http.MethodGet, "/action?kind=click&selector=%23btn&tabId=tabB", "", h.HandleAction},
		{"POST /action body tabId", http.MethodPost, "/action", `{"kind":"click","selector":"#btn","tabId":"tabB"}`, h.HandleAction},
		{"GET /frame?tabId", http.MethodGet, "/frame?tabId=tabB", "", h.HandleFrame},
		{"POST /frame body tabId", http.MethodPost, "/frame", `{"target":"main","tabId":"tabB"}`, h.HandleFrame},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := drive(h, tc.handler, tc.method, tc.target, tc.body)
			if got := actedOnTab(w); got != "tabB" {
				t.Fatalf("acted on %q, want the named tabB (status %d: %s)", got, w.Code, w.Body.String())
			}
		})
	}
	t.Run("the fixture's current tab is a different tab", func(t *testing.T) {
		w := drive(h, h.HandleAction, http.MethodPost, "/action", `{"kind":"click","selector":"#btn"}`)
		if got := actedOnTab(w); got != "tabA" {
			t.Fatalf("an unnamed target resolved to %q, want the current tabA; without this the named-tab rows prove nothing", got)
		}
	})
}

func TestMistypedTargetingIsRefusedOnReadVerbs(t *testing.T) {
	h := newTwoTabHandlers(t)
	for _, spelling := range mistypedTabTargets {
		for _, verb := range []struct {
			path    string
			handler http.HandlerFunc
		}{{"/snapshot", h.HandleSnapshot}, {"/frame", h.HandleFrame}} {
			t.Run(verb.path+"?"+spelling, func(t *testing.T) {
				w := drive(h, verb.handler, http.MethodGet, verb.path+"?"+spelling+"=tabB", "")
				if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), spelling+": not a targeting parameter") {
					t.Fatalf("GET %s?%s= answered %d about %q instead of refusing: %s", verb.path, spelling, w.Code, actedOnTab(w), w.Body.String())
				}
			})
		}
	}
	controls, err := ParseSnapshotCostControls(url.Values{"bogus": []string{"1"}})
	if err != nil || len(controls.Ignored) != 1 || controls.Ignored[0] != "bogus" {
		t.Fatalf("a non-targeting unknown parameter must stay diagnostic in ignoredParams, got %v %v", controls.Ignored, err)
	}
}

func TestMistypedTargetingIsRefusedAcrossRegisteredGetEndpoints(t *testing.T) {
	h := newTwoTabHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)
	endpoints := []string{"/snapshot", "/text", "/title", "/console", "/screencast", "/download?url=http://localhost/file"}
	for _, spelling := range mistypedTabTargets {
		for _, endpoint := range endpoints {
			t.Run(endpoint+"?"+spelling, func(t *testing.T) {
				separator := "?"
				if strings.Contains(endpoint, "?") {
					separator = "&"
				}
				r := httptest.NewRequest(http.MethodGet, endpoint+separator+spelling+"=tabB", nil)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), spelling+": not a targeting parameter") {
					t.Fatalf("GET %s with %s answered %d instead of naming the mistyped target: %s", endpoint, spelling, w.Code, w.Body.String())
				}
			})
		}
	}
}
