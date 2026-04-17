package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
)

type stubActivityRecorder struct {
	events []activity.Event
}

func (s stubActivityRecorder) Enabled() bool { return true }

func (s stubActivityRecorder) Record(activity.Event) error { return nil }

func (s stubActivityRecorder) Query(activity.Filter) ([]activity.Event, error) {
	return s.events, nil
}

func TestNewDashboard(t *testing.T) {
	d := NewDashboard(nil)
	if d == nil {
		t.Fatal("expected non-nil dashboard")
	}
}

type noFlusherDashboardResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *noFlusherDashboardResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *noFlusherDashboardResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

func (w *noFlusherDashboardResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func TestDashboardHandleSSE_StreamingNotSupportedReturnsProblem(t *testing.T) {
	d := NewDashboard(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	w := &noFlusherDashboardResponseWriter{}

	d.handleSSE(w, req)

	if w.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.status, http.StatusInternalServerError)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type = %q, want application/problem+json", ct)
	}

	var payload map[string]any
	if err := json.Unmarshal(w.body.Bytes(), &payload); err != nil {
		t.Fatalf("decode problem payload: %v", err)
	}
	if payload["code"] != "streaming_not_supported" {
		t.Fatalf("code = %v, want streaming_not_supported", payload["code"])
	}
}

func TestDashboardHandleSSE_StreamingDeadlineUnsupportedReturnsProblem(t *testing.T) {
	d := NewDashboard(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	w := httptest.NewRecorder()

	d.handleSSE(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type = %q, want application/problem+json", ct)
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode problem payload: %v", err)
	}
	if payload["code"] != "streaming_deadline_unsupported" {
		t.Fatalf("code = %v, want streaming_deadline_unsupported", payload["code"])
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

func TestDashboardLoadPersistedAgentActivityRestoresAgentsAndEvents(t *testing.T) {
	d := NewDashboard(nil)
	now := time.Now().UTC()

	err := d.LoadPersistedAgentActivity(stubActivityRecorder{
		events: []activity.Event{
			{
				Timestamp:  now.Add(-2 * time.Minute),
				Source:     "client",
				RequestID:  "req-1",
				AgentID:    "agent-1",
				Method:     http.MethodPost,
				Path:       "/tabs/tab_1/action",
				Status:     http.StatusOK,
				DurationMs: 11,
				TabID:      "tab_1",
				Action:     "click",
			},
			{
				Timestamp:  now.Add(-1 * time.Minute),
				Source:     "client",
				RequestID:  "req-2",
				Method:     http.MethodGet,
				Path:       "/health",
				Status:     http.StatusOK,
				DurationMs: 4,
			},
			{
				Timestamp:  now,
				Source:     "client",
				RequestID:  "req-3",
				AgentID:    "agent-2",
				Method:     http.MethodGet,
				Path:       "/tabs/tab_2/text",
				Status:     http.StatusOK,
				DurationMs: 8,
				TabID:      "tab_2",
			},
		},
	})
	if err != nil {
		t.Fatalf("LoadPersistedAgentActivity() error = %v", err)
	}

	agents := d.Agents()
	if len(agents) != 3 {
		t.Fatalf("Agents() len = %d, want 3", len(agents))
	}

	events := d.RecentEvents()
	if len(events) != 3 {
		t.Fatalf("RecentEvents() len = %d, want 3", len(events))
	}
	if events[0].ID != "req-1" || events[1].ID != "req-2" || events[2].ID != "req-3" {
		t.Fatalf("RecentEvents() IDs = [%s, %s, %s], want [req-1, req-2, req-3]", events[0].ID, events[1].ID, events[2].ID)
	}
}

func TestDashboardIngestPersistedAgentActivityAddsNewEventsWithoutDuplicatingLiveOnes(t *testing.T) {
	d := NewDashboard(nil)
	now := time.Now().UTC()

	d.RecordActivityEvent(activity.Event{
		Timestamp:  now.Add(-2 * time.Second),
		Source:     "client",
		RequestID:  "req-live",
		AgentID:    "agent-1",
		Method:     http.MethodPost,
		Path:       "/tabs/tab_1/action",
		Status:     http.StatusOK,
		DurationMs: 12,
		TabID:      "tab_1",
		Action:     "click",
	})

	latest, err := d.IngestPersistedAgentActivity(stubActivityRecorder{
		events: []activity.Event{
			{
				Timestamp:  now.Add(-2 * time.Second),
				Source:     "client",
				RequestID:  "req-live",
				AgentID:    "agent-1",
				Method:     http.MethodPost,
				Path:       "/tabs/tab_1/action",
				Status:     http.StatusOK,
				DurationMs: 12,
				TabID:      "tab_1",
				Action:     "click",
			},
			{
				Timestamp:  now,
				Source:     "client",
				RequestID:  "req-new",
				AgentID:    "agent-2",
				Method:     http.MethodGet,
				Path:       "/tabs/tab_2/text",
				Status:     http.StatusOK,
				DurationMs: 7,
				TabID:      "tab_2",
			},
		},
	}, now.Add(-5*time.Second))
	if err != nil {
		t.Fatalf("IngestPersistedAgentActivity() error = %v", err)
	}
	if !latest.Equal(now) {
		t.Fatalf("latest = %v, want %v", latest, now)
	}

	events := d.RecentEvents()
	if len(events) != 2 {
		t.Fatalf("RecentEvents() len = %d, want 2", len(events))
	}
	if events[0].ID != "req-live" || events[1].ID != "req-new" {
		t.Fatalf("RecentEvents() IDs = [%s, %s], want [req-live, req-new]", events[0].ID, events[1].ID)
	}

	agents := d.Agents()
	if len(agents) != 2 {
		t.Fatalf("Agents() len = %d, want 2", len(agents))
	}
}

func TestDashboardLoadPersistedAgentActivityNormalizesBlankAgentIDToAnonymous(t *testing.T) {
	d := NewDashboard(nil)
	now := time.Now().UTC()

	err := d.LoadPersistedAgentActivity(stubActivityRecorder{
		events: []activity.Event{
			{
				Timestamp:  now,
				Source:     "client",
				RequestID:  "req-anon",
				Method:     http.MethodGet,
				Path:       "/text",
				Status:     http.StatusOK,
				DurationMs: 6,
				SessionID:  "ses_publicid123",
			},
		},
	})
	if err != nil {
		t.Fatalf("LoadPersistedAgentActivity() error = %v", err)
	}

	agents := d.Agents()
	if len(agents) != 1 {
		t.Fatalf("Agents() len = %d, want 1", len(agents))
	}
	if agents[0].ID != "anonymous" {
		t.Fatalf("Agents()[0].ID = %q, want anonymous", agents[0].ID)
	}

	events := d.RecentEvents()
	if len(events) != 1 {
		t.Fatalf("RecentEvents() len = %d, want 1", len(events))
	}
	if events[0].AgentID != "anonymous" {
		t.Fatalf("RecentEvents()[0].AgentID = %q, want anonymous", events[0].AgentID)
	}
	if got := events[0].Details["sessionId"]; got != "ses_publicid123" {
		t.Fatalf("RecentEvents()[0].Details[sessionId] = %#v, want ses_publicid123", got)
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
