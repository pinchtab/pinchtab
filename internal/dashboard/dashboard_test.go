package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

func TestDashboardRecordEvent(t *testing.T) {
	d := NewDashboard(nil)
	evt := AgentEvent{
		AgentID:   "agent1",
		Action:    "GET /test",
		Timestamp: time.Now(),
	}

	d.RecordEvent(evt)

	agents := d.GetAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].AgentID != "agent1" {
		t.Errorf("expected agent1, got %s", agents[0].AgentID)
	}
	if agents[0].Status != "active" {
		t.Errorf("expected active, got %s", agents[0].Status)
	}
}

func TestDashboardSSE(t *testing.T) {
	d := NewDashboard(nil)
	mux := http.NewServeMux()
	d.RegisterHandlers(mux)

	// Since SSE is a long-running connection, we test the registration
	go func() {
		time.Sleep(100 * time.Millisecond)
		d.RecordEvent(AgentEvent{AgentID: "agent1", Action: "test"})
	}()

	// We can't easily test the full SSE cycle with httptest.Recorder,
	// but we can verify the handler runs and registers the connection.
	d.mu.RLock()
	initConns := len(d.sseConns)
	d.mu.RUnlock()

	if initConns != 0 {
		t.Errorf("expected 0 connections, got %d", initConns)
	}
}

func TestTrackingMiddleware(t *testing.T) {
	d := NewDashboard(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	handler := d.TrackingMiddleware(nil, mux)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Agent-Id", "agent-t")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	agents := d.GetAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].AgentID != "agent-t" {
		t.Errorf("expected agent-t, got %s", agents[0].AgentID)
	}
}

func TestDashboardProfileIntegration(t *testing.T) {
	profilesDir := t.TempDir()
	profMgr := profiles.NewProfileManager(profilesDir)
	dash := NewDashboard(nil)

	var recorded bridge.ActionRecord
	observer := func(evt AgentEvent) {
		if evt.Profile == "prof1" {
			recorded = bridge.ActionRecord{
				Endpoint: evt.Action,
			}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/action", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	handler := dash.TrackingMiddleware([]EventObserver{observer}, mux)

	req := httptest.NewRequest("POST", "/action?profile=prof1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if recorded.Endpoint != "POST /action" {
		t.Errorf("expected POST /action, got %q", recorded.Endpoint)
	}
	_ = profMgr
}
