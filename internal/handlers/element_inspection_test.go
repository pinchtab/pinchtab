package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func newTestHandlers(b bridge.BridgeAPI) *Handlers {
	return New(b, &config.RuntimeConfig{}, nil, nil, nil)
}

type elementInspectionEndpoint struct {
	name       string
	extraQuery string
	handle     func(*Handlers) http.HandlerFunc
	handleTab  func(*Handlers) http.HandlerFunc
}

var elementInspectionEndpoints = []elementInspectionEndpoint{
	{name: "enabled", handle: func(h *Handlers) http.HandlerFunc { return h.HandleGetEnabled }, handleTab: func(h *Handlers) http.HandlerFunc { return h.HandleTabGetEnabled }},
	{name: "checked", handle: func(h *Handlers) http.HandlerFunc { return h.HandleGetChecked }, handleTab: func(h *Handlers) http.HandlerFunc { return h.HandleTabGetChecked }},
	{name: "value", handle: func(h *Handlers) http.HandlerFunc { return h.HandleGetValue }, handleTab: func(h *Handlers) http.HandlerFunc { return h.HandleTabGetValue }},
	{name: "box", handle: func(h *Handlers) http.HandlerFunc { return h.HandleGetBox }, handleTab: func(h *Handlers) http.HandlerFunc { return h.HandleTabGetBox }},
	{name: "attr", extraQuery: "&name=href", handle: func(h *Handlers) http.HandlerFunc { return h.HandleGetAttr }, handleTab: func(h *Handlers) http.HandlerFunc { return h.HandleTabGetAttr }},
	{name: "visible", handle: func(h *Handlers) http.HandlerFunc { return h.HandleGetVisible }, handleTab: func(h *Handlers) http.HandlerFunc { return h.HandleTabGetVisible }},
}

func serveInspection(handler http.HandlerFunc, target, tabID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if tabID != "" {
		req.SetPathValue("id", tabID)
	}
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func expectInspection(t *testing.T, w *httptest.ResponseRecorder, status int, bodyContains string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d, want %d: %s", w.Code, status, w.Body.String())
	}
	if bodyContains != "" && !strings.Contains(w.Body.String(), bodyContains) {
		t.Fatalf("body does not mention %q: %s", bodyContains, w.Body.String())
	}
}

func TestElementInspectionEndpoints(t *testing.T) {
	snapshotWithE1 := &bridge.RefCache{
		Refs:    map[string]int64{"e1": 100},
		Targets: map[string]bridge.RefTarget{"e1": {BackendNodeID: 100}},
	}
	for _, ep := range elementInspectionEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			path := "/" + ep.name
			query := func(ref string) string { return "?ref=" + ref + ep.extraQuery }

			t.Run("MissingRef", func(t *testing.T) {
				h := newTestHandlers(&mockBridge{})
				target := path
				if ep.extraQuery != "" {
					target += "?" + strings.TrimPrefix(ep.extraQuery, "&")
				}
				expectInspection(t, serveInspection(ep.handle(h), target, ""), http.StatusBadRequest, "ref")
			})
			t.Run("NoTab", func(t *testing.T) {
				h := newTestHandlers(&mockBridge{failTab: true})
				expectInspection(t, serveInspection(ep.handle(h), path+query("e5"), ""), http.StatusNotFound, "")
			})
			t.Run("NoSnapshotCache", func(t *testing.T) {
				h := newTestHandlers(&mockBridge{})
				expectInspection(t, serveInspection(ep.handle(h), path+query("e5"), ""), http.StatusNotFound, "not found")
			})
			t.Run("RefNotFound", func(t *testing.T) {
				h := newTestHandlers(&mockBridge{refCache: snapshotWithE1})
				expectInspection(t, serveInspection(ep.handle(h), path+query("e99"), ""), http.StatusNotFound, "e99")
			})
			t.Run("MissingTabID", func(t *testing.T) {
				h := newTestHandlers(&mockBridge{})
				expectInspection(t, serveInspection(ep.handleTab(h), "/tabs/"+path+query("e5"), ""), http.StatusBadRequest, "")
			})
			t.Run("ForwardsTabID", func(t *testing.T) {
				h := newTestHandlers(&mockBridge{failTab: true})
				expectInspection(t, serveInspection(ep.handleTab(h), "/tabs/tab_abc"+path+query("e5"), "tab_abc"), http.StatusNotFound, "")
			})
			t.Run("RoutesRegistered", func(t *testing.T) {
				mux := http.NewServeMux()
				newTestHandlers(&mockBridge{failTab: true}).RegisterRoutes(mux, nil)
				for _, target := range []string{path + query("e1"), "/tabs/tab1" + path + query("e1")} {
					t.Run(target, func(t *testing.T) {
						w := httptest.NewRecorder()
						mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
						if w.Code == http.StatusNotFound && strings.Contains(w.Body.String(), "404 page not found") {
							t.Fatalf("route %s not registered", target)
						}
					})
				}
			})
		})
	}
}
