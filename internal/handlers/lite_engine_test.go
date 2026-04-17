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

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/engine"
	"github.com/pinchtab/pinchtab/internal/netguard"
)

func newLiteTestPage() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html><html><body><input placeholder="Name"><button>Save</button></body></html>`))
	}))
}

func liteHandlersWithPage(t *testing.T) (*Handlers, string, string) {
	t.Helper()
	ts := newLiteTestPage()
	t.Cleanup(ts.Close)

	lite := engine.NewLiteEngine()
	t.Cleanup(func() { _ = lite.Close() })

	h := New(&mockBridge{}, &config.RuntimeConfig{Engine: "lite"}, nil, nil, nil)
	h.Router = engine.NewRouter(engine.ModeLite, lite)

	res, err := lite.Navigate(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	return h, ts.URL, res.TabID
}

func TestHandleAction_LiteTypeAndClick(t *testing.T) {
	h, _, tabID := liteHandlersWithPage(t)

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
	if got := w.Header().Get("X-Engine"); got != "lite" {
		t.Fatalf("X-Engine = %q, want lite", got)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/action", bytes.NewReader([]byte(`{"tabId":"`+tabID+`","kind":"click","ref":"`+buttonRef+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	h.HandleAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("click status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAction_LiteUnsupportedAction(t *testing.T) {
	h, _, tabID := liteHandlersWithPage(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/action", strings.NewReader(`{"tabId":"`+tabID+`","kind":"press","key":"Enter"}`))
	req.Header.Set("Content-Type", "application/json")
	h.HandleAction(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleText_LiteRespectsTabID(t *testing.T) {
	page1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>first page</body></html>`))
	}))
	defer page1.Close()
	page2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>second page</body></html>`))
	}))
	defer page2.Close()

	lite := engine.NewLiteEngine()
	defer func() { _ = lite.Close() }()
	h := New(&mockBridge{}, &config.RuntimeConfig{Engine: "lite"}, nil, nil, nil)
	h.Router = engine.NewRouter(engine.ModeLite, lite)

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

func TestHandleNavigate_LiteBlocksDNSRebinding(t *testing.T) {
	lite := engine.NewLiteEngine()
	defer func() { _ = lite.Close() }()

	h := New(&mockBridge{}, &config.RuntimeConfig{Engine: "lite"}, nil, nil, nil)
	h.Router = engine.NewRouter(engine.ModeLite, lite)

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
