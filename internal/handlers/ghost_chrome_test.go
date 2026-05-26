package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome/staticfetch"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/netguard"
)

func newGhostChromeTestPage() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html><html><body><input placeholder="Name"><button>Save</button></body></html>`))
	}))
}

func ghostChromeHandlersWithPage(t *testing.T) (*Handlers, string, string) {
	t.Helper()
	ts := newGhostChromeTestPage()
	t.Cleanup(ts.Close)

	lite := staticfetch.NewBrowser()
	t.Cleanup(func() { _ = lite.Close() })

	h := New(&mockBridge{}, &config.RuntimeConfig{DefaultBrowser: config.BrowserGhostChrome}, nil, nil, nil)
	h.StaticBrowser = lite

	res, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	return h, ts.URL, res.TabID
}

func TestHandleAction_GhostChromeTypeAndClick(t *testing.T) {
	h, _, tabID := ghostChromeHandlersWithPage(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/snapshot?tabId="+tabID+"&filter=interactive", nil)
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d body=%s", w.Code, w.Body.String())
	}

	var snap struct {
		Nodes []struct {
			Ref  string `json:"ref"`
			Role string `json:"role"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	var inputRef, buttonRef string
	for _, n := range snap.Nodes {
		switch n.Role {
		case "textbox":
			inputRef = n.Ref
		case "button":
			buttonRef = n.Ref
		}
	}
	if inputRef == "" || buttonRef == "" {
		t.Fatalf("missing refs: textbox=%q button=%q", inputRef, buttonRef)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"tabId":"`+tabID+`","kind":"type","ref":"`+inputRef+`","text":"hello"}`)))
	req.Header.Set("Content-Type", "application/json")
	h.HandleAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("type status = %d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"tabId":"`+tabID+`","kind":"click","ref":"`+buttonRef+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	h.HandleAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("click status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAction_GhostChromeUnsupportedAction(t *testing.T) {
	h, _, tabID := ghostChromeHandlersWithPage(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/action", strings.NewReader(`{"tabId":"`+tabID+`","kind":"press","key":"Enter"}`))
	req.Header.Set("Content-Type", "application/json")
	h.HandleAction(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleText_GhostChromeRespectsTabID(t *testing.T) {
	page1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>first page</body></html>`))
	}))
	defer page1.Close()
	page2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>second page</body></html>`))
	}))
	defer page2.Close()

	lite := staticfetch.NewBrowser()
	defer func() { _ = lite.Close() }()
	h := New(&mockBridge{}, &config.RuntimeConfig{DefaultBrowser: config.BrowserGhostChrome}, nil, nil, nil)
	h.StaticBrowser = lite

	res1, err := lite.Navigate(context.Background(), page1.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = lite.Navigate(context.Background(), page2.URL)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/text?tabId="+res1.TabID+"&format=text", nil)
	h.HandleText(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "first page") {
		t.Fatalf("expected first page text, got %q", w.Body.String())
	}
}

func TestGhostChromeRouteMetadataInResponse(t *testing.T) {
	h, _, tabID := ghostChromeHandlersWithPage(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/snapshot?tabId="+tabID, nil)
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d body=%s", w.Code, w.Body.String())
	}

	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	routeRaw, ok := m["route"]
	if !ok {
		t.Fatal("expected \"route\" key in snapshot response")
	}
	route, ok := routeRaw.(map[string]any)
	if !ok {
		t.Fatalf("route is %T, want map[string]any", routeRaw)
	}

	if _, ok := route["requestedProvider"]; !ok {
		t.Fatal("route missing \"requestedProvider\" key")
	}
	if _, ok := route["usedProvider"]; !ok {
		t.Fatal("route missing \"usedProvider\" key")
	}
}

// assertRoutePresent checks that a JSON response body contains "route" with
// decision metadata and does NOT contain "browserops".
func assertRoutePresent(t *testing.T, body []byte, label string) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("%s: unmarshal response: %v", label, err)
	}
	if _, ok := m["browserops"]; ok {
		t.Fatalf("%s: response must not contain \"browserops\" key", label)
	}
	routeRaw, ok := m["route"]
	if !ok {
		t.Fatalf("%s: response must contain \"route\" key", label)
	}
	route, ok := routeRaw.(map[string]any)
	if !ok {
		t.Fatalf("%s: route is %T, want map[string]any", label, routeRaw)
	}
	if _, ok := route["requestedProvider"]; !ok {
		t.Fatalf("%s: route missing \"requestedProvider\"", label)
	}
	if _, ok := route["usedProvider"]; !ok {
		t.Fatalf("%s: route missing \"usedProvider\"", label)
	}
}

func TestGhostChromeNavigateResponseHasRouteNoEngine(t *testing.T) {
	h, _, _ := ghostChromeHandlersWithPage(t)

	ts := newGhostChromeTestPage()
	defer ts.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(`{"url":"`+ts.URL+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.HandleNavigate(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("navigate status = %d body=%s", w.Code, w.Body.String())
	}
	assertRoutePresent(t, w.Body.Bytes(), "navigate")
}

func TestGhostChromeSnapshotResponseHasRouteNoEngine(t *testing.T) {
	h, _, tabID := ghostChromeHandlersWithPage(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/snapshot?tabId="+tabID, nil)
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d body=%s", w.Code, w.Body.String())
	}
	assertRoutePresent(t, w.Body.Bytes(), "snapshot")
}

func TestGhostChromeTextResponseHasRouteHeader(t *testing.T) {
	h, _, tabID := ghostChromeHandlersWithPage(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/text?tabId="+tabID+"&format=text", nil)
	h.HandleText(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("text status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestGhostChromeActionResponseHasRouteNoEngine(t *testing.T) {
	h, _, tabID := ghostChromeHandlersWithPage(t)

	// Take a snapshot first so refs are available
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/snapshot?tabId="+tabID+"&filter=interactive", nil)
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d body=%s", w.Code, w.Body.String())
	}
	var snap struct {
		Nodes []struct {
			Ref  string `json:"ref"`
			Role string `json:"role"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	var buttonRef string
	for _, n := range snap.Nodes {
		if n.Role == "button" {
			buttonRef = n.Ref
			break
		}
	}
	if buttonRef == "" {
		t.Fatal("no button ref found in snapshot")
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/action", strings.NewReader(`{"tabId":"`+tabID+`","kind":"click","ref":"`+buttonRef+`"}`))
	req.Header.Set("Content-Type", "application/json")
	h.HandleAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("action status = %d body=%s", w.Code, w.Body.String())
	}
	assertRoutePresent(t, w.Body.Bytes(), "action")
}

func TestHandleNavigate_GhostChromeBlocksDNSRebinding(t *testing.T) {
	lite := staticfetch.NewBrowser()
	defer func() { _ = lite.Close() }()

	h := New(&mockBridge{}, &config.RuntimeConfig{DefaultBrowser: config.BrowserGhostChrome}, nil, nil, nil)
	h.StaticBrowser = lite

	resolveCount := 0
	oldResolve := netguard.ResolveHostIPs
	netguard.ResolveHostIPs = func(context.Context, string, string) ([]net.IP, error) {
		resolveCount++
		if resolveCount == 1 {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return []net.IP{net.ParseIP("10.0.0.7")}, nil
	}
	t.Cleanup(func() { netguard.ResolveHostIPs = oldResolve })

	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(`{"url":"https://safe.example/index.html"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for rebinding attempt, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "blocked remote IP") {
		t.Fatalf("expected blocked remote IP error, got %s", w.Body.String())
	}
}
