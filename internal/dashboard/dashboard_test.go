package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
)

func TestNewDashboard(t *testing.T) {
	d := NewDashboard(nil)
	if d == nil {
		t.Fatal("expected non-nil dashboard")
	}
}

func TestDashboardBroadcastSystemEvent(t *testing.T) {
	d := NewDashboard(nil)

	// Create a test handler and register it
	mux := http.NewServeMux()
	d.RegisterHandlers(mux)

	// In a real scenario, a client would be connected to /api/events
	// For this test, we just verify the broadcast method doesn't panic
	evt := SystemEvent{
		Type: "instance.started",
	}
	d.BroadcastSystemEvent(evt)
}

func TestDashboardSSEHandlerRegistration(t *testing.T) {
	d := NewDashboard(nil)
	mux := http.NewServeMux()
	d.RegisterHandlers(mux)

	// Verify the SSE handler is registered by checking the mux
	// (can't easily test the full SSE flow with httptest due to streaming nature)
	// Just verify handlers are registered without error
}

func TestDashboardShutdown(t *testing.T) {
	d := NewDashboard(nil)
	// Just verify it doesn't panic
	d.Shutdown()
}

func TestDashboardSetInstanceLister(t *testing.T) {
	d := NewDashboard(nil)
	d.SetInstanceLister(nil)
	// Just verify it doesn't panic
}

func TestDashboardCacheHeaders(t *testing.T) {
	d := NewDashboard(nil)

	// Test long cache (assets)
	handler := d.withLongCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/assets/app.js", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl != "public, max-age=31536000, immutable" {
		t.Errorf("expected long cache header, got %q", cacheControl)
	}

	// Test no cache (HTML)
	handler = d.withNoCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req = httptest.NewRequest("GET", "/dashboard", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	cacheControl = w.Header().Get("Cache-Control")
	if cacheControl != "no-store" {
		t.Errorf("expected no-store cache header, got %q", cacheControl)
	}
}

func TestDashboardShutdownTimeout(t *testing.T) {
	d := NewDashboard(&DashboardConfig{
		IdleTimeout:       10 * time.Millisecond,
		DisconnectTimeout: 20 * time.Millisecond,
		ReaperInterval:    5 * time.Millisecond,
		SSEBufferSize:     8,
	})

	d.Shutdown()
	time.Sleep(50 * time.Millisecond) // Verify shutdown completes
}

func TestDashboardRecordEventTracksAgentsAndReplay(t *testing.T) {
	d := NewDashboard(nil)
	now := time.Now().UTC()
	d.RecordEvent(apiTypes.ActivityEvent{
		ID:        "evt-1",
		AgentID:   "agent-1",
		Channel:   "tool_call",
		Type:      "navigate",
		Method:    http.MethodPost,
		Path:      "/navigate",
		Timestamp: now,
	})

	agents := d.Agents()
	if len(agents) != 1 {
		t.Fatalf("Agents() len = %d, want 1", len(agents))
	}
	if agents[0].ID != "agent-1" {
		t.Fatalf("Agents()[0].ID = %q, want agent-1", agents[0].ID)
	}
	if agents[0].RequestCount != 1 {
		t.Fatalf("Agents()[0].RequestCount = %d, want 1", agents[0].RequestCount)
	}
	if d.AgentCount() != 1 {
		t.Fatalf("AgentCount() = %d, want 1", d.AgentCount())
	}

	events := d.RecentEvents()
	if len(events) != 1 {
		t.Fatalf("RecentEvents() len = %d, want 1", len(events))
	}
	if events[0].ID != "evt-1" {
		t.Fatalf("RecentEvents()[0].ID = %q, want evt-1", events[0].ID)
	}
}

func TestDashboardHandleAgentsReturnsTrackedAgents(t *testing.T) {
	d := NewDashboard(nil)
	d.RecordEvent(apiTypes.ActivityEvent{
		ID:        "evt-1",
		AgentID:   "agent-1",
		Channel:   "progress",
		Type:      "progress",
		Message:   "Thinking",
		Timestamp: time.Now().UTC(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	w := httptest.NewRecorder()
	d.handleAgents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleAgents() status = %d, want %d", w.Code, http.StatusOK)
	}

	var agents []apiTypes.Agent
	if err := json.NewDecoder(w.Body).Decode(&agents); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(agents) != 1 || agents[0].ID != "agent-1" {
		t.Fatalf("agents = %#v, want tracked agent", agents)
	}
}

func TestDashboardHandleAgentReturnsDetail(t *testing.T) {
	d := NewDashboard(nil)
	d.RecordEvent(apiTypes.ActivityEvent{
		ID:        "evt-1",
		AgentID:   "agent-1",
		Channel:   "tool_call",
		Type:      "navigate",
		Method:    http.MethodPost,
		Path:      "/navigate",
		Timestamp: time.Now().UTC(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/agents/agent-1", nil)
	req.SetPathValue("id", "agent-1")
	w := httptest.NewRecorder()
	d.handleAgent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handleAgent() status = %d, want %d", w.Code, http.StatusOK)
	}

	var detail apiTypes.AgentDetail
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if detail.Agent.ID != "agent-1" {
		t.Fatalf("detail.Agent.ID = %q, want agent-1", detail.Agent.ID)
	}
	if len(detail.Events) != 1 || detail.Events[0].AgentID != "agent-1" {
		t.Fatalf("detail.Events = %#v, want agent-specific events", detail.Events)
	}
}

func TestDashboardHandleAgentEventsByIDUsesRouteAgent(t *testing.T) {
	d := NewDashboard(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent-1/events", bytes.NewBufferString(`{"message":"Thinking"}`))
	req.SetPathValue("id", "agent-1")
	w := httptest.NewRecorder()
	d.handleAgentEventsByID(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("handleAgentEventsByID() status = %d, want %d", w.Code, http.StatusCreated)
	}

	events := d.RecentEvents()
	if len(events) != 1 || events[0].AgentID != "agent-1" {
		t.Fatalf("events = %#v, want route agent id", events)
	}
}

func TestMatchesMode(t *testing.T) {
	tests := []struct {
		mode    string
		channel string
		want    bool
	}{
		{mode: "tool_calls", channel: "tool_call", want: true},
		{mode: "tool_calls", channel: "progress", want: false},
		{mode: "progress", channel: "tool_call", want: false},
		{mode: "progress", channel: "progress", want: true},
		{mode: "both", channel: "tool_call", want: true},
		{mode: "both", channel: "progress", want: true},
	}

	for _, tc := range tests {
		if got := matchesMode(tc.mode, tc.channel); got != tc.want {
			t.Fatalf("matchesMode(%q, %q) = %v, want %v", tc.mode, tc.channel, got, tc.want)
		}
	}
}
