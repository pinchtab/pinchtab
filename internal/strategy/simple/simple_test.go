package simple_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/allocation"
	bridgepkg "github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/instance"
	"github.com/pinchtab/pinchtab/internal/strategy/simple"
)

// --- Test doubles ---

type mockLauncher struct {
	nextID int
}

func (m *mockLauncher) Launch(name, port string, headless bool) (*bridgepkg.Instance, error) {
	m.nextID++
	return &bridgepkg.Instance{
		ID:          fmt.Sprintf("inst_%d", m.nextID),
		ProfileName: name,
		Port:        port,
		Status:      "running",
	}, nil
}

func (m *mockLauncher) Stop(id string) error { return nil }

// fakeBridge simulates a real bridge's HTTP endpoints.
func fakeBridge() *httptest.Server {
	mux := http.NewServeMux()

	// POST /tab — create/close tabs.
	mux.HandleFunc("POST /tab", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string `json:"action"`
			URL    string `json:"url"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		switch req.Action {
		case "new":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"tabId": "tab_fake_123",
				"url":   req.URL,
			})
		case "close":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "closed"})
		}
	})

	// GET /tabs — list tabs.
	mux.HandleFunc("GET /tabs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"id": "tab_fake_123", "url": "https://example.com", "title": "Example"},
		})
	})

	// Tab operations — return canned responses.
	mux.HandleFunc("GET /tabs/{id}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"text": "snapshot of " + r.PathValue("id"),
		})
	})
	mux.HandleFunc("GET /tabs/{id}/screenshot", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fake-png"))
	})
	mux.HandleFunc("GET /tabs/{id}/text", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "page text"})
	})
	mux.HandleFunc("POST /tabs/{id}/action", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /tabs/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{{"status": "ok"}})
	})
	mux.HandleFunc("POST /tabs/{id}/evaluate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"result": 42})
	})
	mux.HandleFunc("POST /tabs/{id}/navigate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /tabs/{id}/cookies", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{})
	})
	mux.HandleFunc("POST /tabs/{id}/cookies", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /tabs/{id}/pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("fake-pdf"))
	})

	return httptest.NewServer(mux)
}

func setupStrategy(t *testing.T) (*simple.Strategy, *httptest.Server) {
	t.Helper()
	bridge := fakeBridge()
	t.Cleanup(bridge.Close)

	// Extract port.
	port := bridge.Listener.Addr().String()
	// Get just the port part (after last colon).
	parts := strings.Split(port, ":")
	portStr := parts[len(parts)-1]

	launcher := &mockLauncher{}
	fetcher := instance.NewBridgeClient()
	mgr := instance.NewManager(launcher, fetcher, &allocation.FCFS{})

	// Register a running instance with the test bridge's port.
	mgr.Repo.Add(&bridgepkg.Instance{
		ID:     "inst_test",
		Port:   portStr,
		Status: "running",
	})

	s := simple.New(mgr)
	return s, bridge
}

func serveMux(s *simple.Strategy) *http.ServeMux {
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	return mux
}

// --- Tests ---

func TestSimple_Name(t *testing.T) {
	launcher := &mockLauncher{}
	mgr := instance.NewManager(launcher, instance.NewBridgeClient(), nil)
	s := simple.New(mgr)
	if s.Name() != "simple" {
		t.Errorf("expected simple, got %s", s.Name())
	}
}

func TestSimple_Navigate_CreatesTab(t *testing.T) {
	s, _ := setupStrategy(t)
	mux := serveMux(s)

	body := `{"url":"https://example.com"}`
	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["tabId"] == "" {
		t.Error("expected tabId in response")
	}
	if resp["url"] != "https://example.com" {
		t.Errorf("expected url https://example.com, got %s", resp["url"])
	}
}

func TestSimple_Navigate_RequiresURL(t *testing.T) {
	s, _ := setupStrategy(t)
	mux := serveMux(s)

	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestSimple_Snapshot_AfterNavigate(t *testing.T) {
	s, _ := setupStrategy(t)
	mux := serveMux(s)

	// Navigate first to create a tab.
	navReq := httptest.NewRequest("POST", "/navigate", strings.NewReader(`{"url":"https://example.com"}`))
	navReq.Header.Set("Content-Type", "application/json")
	navRec := httptest.NewRecorder()
	mux.ServeHTTP(navRec, navReq)
	if navRec.Code != 200 {
		t.Fatalf("navigate failed: %d", navRec.Code)
	}

	// Snapshot using the current tab.
	snapReq := httptest.NewRequest("GET", "/snapshot", nil)
	snapRec := httptest.NewRecorder()
	mux.ServeHTTP(snapRec, snapReq)

	if snapRec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", snapRec.Code, snapRec.Body.String())
	}
}

func TestSimple_Snapshot_NoTab_ReturnsError(t *testing.T) {
	launcher := &mockLauncher{}
	mgr := instance.NewManager(launcher, instance.NewBridgeClient(), nil)
	s := simple.New(mgr)
	mux := serveMux(s)

	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Should fail gracefully (no running instances/tabs).
	if rec.Code == 200 {
		t.Error("expected error when no tabs available")
	}
}

func TestSimple_TabSpecific_Snapshot(t *testing.T) {
	s, _ := setupStrategy(t)
	mux := serveMux(s)

	// Navigate to create a tab.
	navReq := httptest.NewRequest("POST", "/navigate", strings.NewReader(`{"url":"https://example.com"}`))
	navReq.Header.Set("Content-Type", "application/json")
	navRec := httptest.NewRecorder()
	mux.ServeHTTP(navRec, navReq)

	var navResp map[string]string
	_ = json.NewDecoder(navRec.Body).Decode(&navResp)
	tabID := navResp["tabId"]

	// Snapshot with explicit tab ID.
	snapReq := httptest.NewRequest("GET", "/tabs/"+tabID+"/snapshot", nil)
	snapRec := httptest.NewRecorder()
	mux.ServeHTTP(snapRec, snapReq)

	if snapRec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", snapRec.Code, snapRec.Body.String())
	}
}

func TestSimple_ListInstances(t *testing.T) {
	s, _ := setupStrategy(t)
	mux := serveMux(s)

	req := httptest.NewRequest("GET", "/instances", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestSimple_ListTabs(t *testing.T) {
	s, _ := setupStrategy(t)
	mux := serveMux(s)

	req := httptest.NewRequest("GET", "/tabs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestSimple_TabClose(t *testing.T) {
	s, _ := setupStrategy(t)
	mux := serveMux(s)

	// Navigate first.
	navReq := httptest.NewRequest("POST", "/navigate", strings.NewReader(`{"url":"https://example.com"}`))
	navReq.Header.Set("Content-Type", "application/json")
	navRec := httptest.NewRecorder()
	mux.ServeHTTP(navRec, navReq)

	var navResp map[string]string
	_ = json.NewDecoder(navRec.Body).Decode(&navResp)
	tabID := navResp["tabId"]

	// Close the tab.
	closeReq := httptest.NewRequest("POST", "/tabs/"+tabID+"/close", nil)
	closeRec := httptest.NewRecorder()
	mux.ServeHTTP(closeRec, closeReq)

	if closeRec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", closeRec.Code, closeRec.Body.String())
	}
}

func TestSimple_NavigateGet(t *testing.T) {
	s, _ := setupStrategy(t)
	mux := serveMux(s)

	req := httptest.NewRequest("GET", "/navigate?url=https://example.com", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
