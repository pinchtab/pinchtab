package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed dashboard
var dashboardFS embed.FS

// ---------------------------------------------------------------------------
// Agent Activity — tracks what each agent is doing in real time
// ---------------------------------------------------------------------------

type AgentActivity struct {
	AgentID    string    `json:"agentId"`
	Profile    string    `json:"profile,omitempty"`
	CurrentURL string    `json:"currentUrl,omitempty"`
	CurrentTab string    `json:"currentTab,omitempty"`
	LastAction string    `json:"lastAction,omitempty"`
	LastSeen   time.Time `json:"lastSeen"`
	Status     string    `json:"status"` // "active", "idle", "disconnected"
	ActionCount int      `json:"actionCount"`
}

type AgentEvent struct {
	AgentID   string `json:"agentId"`
	Profile   string `json:"profile,omitempty"`
	Action    string `json:"action"`
	URL       string `json:"url,omitempty"`
	TabID     string `json:"tabId,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Status    int    `json:"status"`
	DurationMs int64 `json:"durationMs"`
	Timestamp time.Time `json:"timestamp"`
}

type Dashboard struct {
	agents   map[string]*AgentActivity
	sseConns map[chan AgentEvent]struct{}
	mu       sync.RWMutex
}

func NewDashboard() *Dashboard {
	d := &Dashboard{
		agents:   make(map[string]*AgentActivity),
		sseConns: make(map[chan AgentEvent]struct{}),
	}
	// Background goroutine to mark idle/disconnected agents
	go d.reaper()
	return d
}

func (d *Dashboard) reaper() {
	for {
		time.Sleep(10 * time.Second)
		d.mu.Lock()
		now := time.Now()
		for id, a := range d.agents {
			if a.Status == "disconnected" {
				continue
			}
			if now.Sub(a.LastSeen) > 5*time.Minute {
				d.agents[id].Status = "disconnected"
			} else if now.Sub(a.LastSeen) > 30*time.Second {
				d.agents[id].Status = "idle"
			}
		}
		d.mu.Unlock()
	}
}

// RecordEvent processes an agent action and broadcasts to SSE subscribers.
func (d *Dashboard) RecordEvent(evt AgentEvent) {
	d.mu.Lock()

	a, ok := d.agents[evt.AgentID]
	if !ok {
		a = &AgentActivity{AgentID: evt.AgentID}
		d.agents[evt.AgentID] = a
	}
	a.LastSeen = evt.Timestamp
	a.LastAction = evt.Action
	a.Status = "active"
	a.ActionCount++
	a.Profile = evt.Profile
	if evt.URL != "" {
		a.CurrentURL = evt.URL
	}
	if evt.TabID != "" {
		a.CurrentTab = evt.TabID
	}

	// Copy SSE channels
	chans := make([]chan AgentEvent, 0, len(d.sseConns))
	for ch := range d.sseConns {
		chans = append(chans, ch)
	}
	d.mu.Unlock()

	// Non-blocking broadcast
	for _, ch := range chans {
		select {
		case ch <- evt:
		default: // drop if slow
		}
	}
}

// GetAgents returns current state of all agents.
func (d *Dashboard) GetAgents() []AgentActivity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	agents := make([]AgentActivity, 0, len(d.agents))
	for _, a := range d.agents {
		agents = append(agents, *a)
	}
	return agents
}

// ---------------------------------------------------------------------------
// HTTP Handlers
// ---------------------------------------------------------------------------

func (d *Dashboard) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard", d.handleDashboardUI)
	mux.HandleFunc("GET /dashboard/agents", d.handleAgents)
	mux.HandleFunc("GET /dashboard/events", d.handleSSE)

	// Serve static assets (CSS, JS) from embedded filesystem
	sub, _ := fs.Sub(dashboardFS, "dashboard")
	mux.Handle("GET /dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.FS(sub))))
}

func (d *Dashboard) handleAgents(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, 200, d.GetAgents())
}

func (d *Dashboard) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan AgentEvent, 64)
	d.mu.Lock()
	d.sseConns[ch] = struct{}{}
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.sseConns, ch)
		d.mu.Unlock()
	}()

	// Send current agent state as initial event
	agents := d.GetAgents()
	data, _ := json.Marshal(agents)
	fmt.Fprintf(w, "event: init\ndata: %s\n\n", data)
	flusher.Flush()

	for {
		select {
		case evt := <-ch:
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "event: action\ndata: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (d *Dashboard) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	data, _ := dashboardFS.ReadFile("dashboard/dashboard.html")
	w.Write(data)
}

// ---------------------------------------------------------------------------
// Tracking Middleware — extracts agent ID from header or query
// ---------------------------------------------------------------------------

func (d *Dashboard) TrackingMiddleware(pm *ProfileManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Skip dashboard, profile, instance, and screencast management routes
		p := r.URL.Path
		if strings.HasPrefix(p, "/dashboard") || strings.HasPrefix(p, "/profiles") ||
			strings.HasPrefix(p, "/instances") || strings.HasPrefix(p, "/screencast/tabs") ||
			p == "/welcome" || p == "/favicon.ico" || p == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)

		// Extract agent identity
		agentID := r.Header.Get("X-Agent-Id")
		if agentID == "" {
			agentID = r.URL.Query().Get("agentId")
		}
		if agentID == "" {
			agentID = "anonymous"
		}

		profile := r.Header.Get("X-Profile")
		if profile == "" {
			profile = r.URL.Query().Get("profile")
		}

		// Build detail string for interesting actions
		detail := ""
		switch r.URL.Path {
		case "/navigate":
			detail = r.URL.Query().Get("url")
		case "/actions":
			detail = "batch action"
		case "/snapshot":
			sel := r.URL.Query().Get("selector")
			if sel != "" {
				detail = "selector=" + sel
			}
		}

		evt := AgentEvent{
			AgentID:    agentID,
			Profile:    profile,
			Action:     r.Method + " " + r.URL.Path,
			URL:        r.URL.Query().Get("url"),
			TabID:      r.URL.Query().Get("tabId"),
			Detail:     detail,
			Status:     sw.code,
			DurationMs: time.Since(start).Milliseconds(),
			Timestamp:  start,
		}

		d.RecordEvent(evt)

		// Also record in profile tracker if profile specified
		if profile != "" {
			pm.tracker.Record(profile, ActionRecord{
				Timestamp:  start,
				Method:     r.Method,
				Endpoint:   r.URL.Path,
				URL:        r.URL.Query().Get("url"),
				TabID:      r.URL.Query().Get("tabId"),
				DurationMs: time.Since(start).Milliseconds(),
				Status:     sw.code,
			})
		}
	})
}

// ---------------------------------------------------------------------------
