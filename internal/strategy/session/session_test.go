package session_test

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
	"github.com/pinchtab/pinchtab/internal/strategy/session"
)

type mockLauncher struct{ nextID int }

func (m *mockLauncher) Launch(name, port string, headless bool) (*bridgepkg.Instance, error) {
	m.nextID++
	return &bridgepkg.Instance{
		ID: fmt.Sprintf("inst_%d", m.nextID), ProfileName: name,
		Port: port, Status: "running",
	}, nil
}
func (m *mockLauncher) Stop(string) error { return nil }

func fakeBridge() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tab", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string `json:"action"`
			URL    string `json:"url"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Action {
		case "new":
			_ = json.NewEncoder(w).Encode(map[string]string{"tabId": "tab_sess_1", "url": req.URL})
		case "close":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "closed"})
		}
	})
	mux.HandleFunc("GET /tabs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"id": "tab_sess_1", "url": "https://example.com", "title": "Example"},
		})
	})
	mux.HandleFunc("GET /tabs/{id}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "snapshot"})
	})
	mux.HandleFunc("POST /tabs/{id}/navigate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /tabs/{id}/action", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /tabs/{id}/text", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "page text"})
	})
	mux.HandleFunc("GET /tabs/{id}/screenshot", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake-png"))
	})
	return httptest.NewServer(mux)
}

func setup(t *testing.T) (*session.Strategy, *http.ServeMux) {
	t.Helper()
	bridge := fakeBridge()
	t.Cleanup(bridge.Close)

	parts := strings.Split(bridge.Listener.Addr().String(), ":")
	port := parts[len(parts)-1]

	mgr := instance.NewManager(&mockLauncher{}, instance.NewBridgeClient(), &allocation.FCFS{})
	mgr.Repo.Add(&bridgepkg.Instance{ID: "inst_test", Port: port, Status: "running"})

	s := session.New(mgr)
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	return s, mux
}

func createSession(t *testing.T, mux *http.ServeMux) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/session", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("create session: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return resp["sessionId"]
}

// --- Tests ---

func TestSession_Name(t *testing.T) {
	mgr := instance.NewManager(&mockLauncher{}, instance.NewBridgeClient(), nil)
	s := session.New(mgr)
	if s.Name() != "session" {
		t.Errorf("expected session, got %s", s.Name())
	}
}

func TestSession_CreateSession(t *testing.T) {
	_, mux := setup(t)
	sessID := createSession(t, mux)
	if !strings.HasPrefix(sessID, "sess_") {
		t.Errorf("expected sess_ prefix, got %s", sessID)
	}
}

func TestSession_GetSession(t *testing.T) {
	_, mux := setup(t)
	sessID := createSession(t, mux)

	req := httptest.NewRequest("GET", "/session/"+sessID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["id"] != sessID {
		t.Errorf("expected session ID %s, got %v", sessID, resp["id"])
	}
}

func TestSession_GetSession_NotFound(t *testing.T) {
	_, mux := setup(t)

	req := httptest.NewRequest("GET", "/session/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestSession_DeleteSession(t *testing.T) {
	_, mux := setup(t)
	sessID := createSession(t, mux)

	req := httptest.NewRequest("DELETE", "/session/"+sessID, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify it's gone.
	getReq := httptest.NewRequest("GET", "/session/"+sessID, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != 404 {
		t.Errorf("expected 404 after delete, got %d", getRec.Code)
	}
}

func TestSession_ListSessions(t *testing.T) {
	_, mux := setup(t)
	createSession(t, mux)
	createSession(t, mux)

	req := httptest.NewRequest("GET", "/sessions", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var sessions []map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&sessions)
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSession_Navigate_CreatesSession(t *testing.T) {
	_, mux := setup(t)

	// Navigate without a session — should auto-create one.
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
	if resp["sessionId"] == "" {
		t.Error("expected sessionId in response")
	}
	if resp["tabId"] == "" {
		t.Error("expected tabId in response")
	}
}

func TestSession_Navigate_StickyRouting(t *testing.T) {
	_, mux := setup(t)

	// Navigate — creates session.
	body := `{"url":"https://example.com"}`
	req := httptest.NewRequest("POST", "/navigate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var resp1 map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp1)
	sessID := resp1["sessionId"]

	// Navigate again with session ID — should reuse.
	body2 := `{"url":"https://google.com"}`
	req2 := httptest.NewRequest("POST", "/navigate", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Session-ID", sessID)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var resp2 map[string]string
	_ = json.NewDecoder(rec2.Body).Decode(&resp2)
	if resp2["sessionId"] != sessID {
		t.Errorf("expected same session %s, got %s", sessID, resp2["sessionId"])
	}
}

func TestSession_Snapshot_RequiresHeader(t *testing.T) {
	_, mux := setup(t)

	// Snapshot without X-Session-ID should fail.
	req := httptest.NewRequest("GET", "/snapshot", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == 200 {
		t.Error("expected error without X-Session-ID")
	}
}

func TestSession_Snapshot_WithSession(t *testing.T) {
	_, mux := setup(t)
	sessID := createSession(t, mux)

	req := httptest.NewRequest("GET", "/snapshot", nil)
	req.Header.Set("X-Session-ID", sessID)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSession_Browse_AutoCreatesSession(t *testing.T) {
	_, mux := setup(t)

	body := `{"url":"https://example.com"}`
	req := httptest.NewRequest("POST", "/browse", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Browse proxies to navigate, so 200 is expected.
	if rec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// GET /instances is now handled by the orchestrator, not the strategy.

func TestSession_ListTabs(t *testing.T) {
	_, mux := setup(t)

	req := httptest.NewRequest("GET", "/tabs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
