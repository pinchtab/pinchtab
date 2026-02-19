package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"
)

type DashboardConfig struct {
	IdleTimeout       time.Duration
	DisconnectTimeout time.Duration
	ReaperInterval    time.Duration
	SSEBufferSize     int
}

//go:embed dashboard/*
var dashboardFS embed.FS

type AgentActivity struct {
	AgentID     string    `json:"agentId"`
	Profile     string    `json:"profile,omitempty"`
	CurrentURL  string    `json:"currentUrl,omitempty"`
	CurrentTab  string    `json:"currentTab,omitempty"`
	LastAction  string    `json:"lastAction,omitempty"`
	LastSeen    time.Time `json:"lastSeen"`
	Status      string    `json:"status"`
	ActionCount int       `json:"actionCount"`
}

type AgentEvent struct {
	AgentID    string    `json:"agentId"`
	Profile    string    `json:"profile,omitempty"`
	Action     string    `json:"action"`
	URL        string    `json:"url,omitempty"`
	TabID      string    `json:"tabId,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	Status     int       `json:"status"`
	DurationMs int64     `json:"durationMs"`
	Timestamp  time.Time `json:"timestamp"`
}

type Dashboard struct {
	cfg      DashboardConfig
	agents   map[string]*AgentActivity
	sseConns map[chan AgentEvent]struct{}
	cancel   context.CancelFunc
	mu       sync.RWMutex
}

func NewDashboard(cfg *DashboardConfig) *Dashboard {
	c := DashboardConfig{
		IdleTimeout:       30 * time.Second,
		DisconnectTimeout: 5 * time.Minute,
		ReaperInterval:    10 * time.Second,
		SSEBufferSize:     64,
	}
	if cfg != nil {
		if cfg.IdleTimeout > 0 {
			c.IdleTimeout = cfg.IdleTimeout
		}
		if cfg.DisconnectTimeout > 0 {
			c.DisconnectTimeout = cfg.DisconnectTimeout
		}
		if cfg.ReaperInterval > 0 {
			c.ReaperInterval = cfg.ReaperInterval
		}
		if cfg.SSEBufferSize > 0 {
			c.SSEBufferSize = cfg.SSEBufferSize
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	d := &Dashboard{
		cfg:      c,
		agents:   make(map[string]*AgentActivity),
		sseConns: make(map[chan AgentEvent]struct{}),
		cancel:   cancel,
	}
	go d.reaper(ctx)
	return d
}

func (d *Dashboard) Shutdown() { d.cancel() }

func (d *Dashboard) reaper(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.ReaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.mu.Lock()
			now := time.Now()
			for id, a := range d.agents {
				if a.Status == "disconnected" {
					continue
				}
				if now.Sub(a.LastSeen) > d.cfg.DisconnectTimeout {
					d.agents[id].Status = "disconnected"
				} else if now.Sub(a.LastSeen) > d.cfg.IdleTimeout {
					d.agents[id].Status = "idle"
				}
			}
			d.mu.Unlock()
		}
	}
}

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

	chans := make([]chan AgentEvent, 0, len(d.sseConns))
	for ch := range d.sseConns {
		chans = append(chans, ch)
	}
	d.mu.Unlock()

	for _, ch := range chans {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (d *Dashboard) GetAgents() []AgentActivity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	agents := make([]AgentActivity, 0, len(d.agents))
	for _, a := range d.agents {
		agents = append(agents, *a)
	}
	return agents
}

func (d *Dashboard) RegisterHandlers(mux *http.ServeMux) {
	mux.Handle("GET /dashboard", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
	mux.HandleFunc("GET /dashboard/agents", d.handleAgents)
	mux.HandleFunc("GET /dashboard/events", d.handleSSE)

	sub, _ := fs.Sub(dashboardFS, "dashboard")
	static := http.StripPrefix("/dashboard/", http.FileServer(http.FS(sub)))
	mux.Handle("GET /dashboard/", d.withNoCache(static))
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

	ch := make(chan AgentEvent, d.cfg.SSEBufferSize)
	d.mu.Lock()
	d.sseConns[ch] = struct{}{}
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.sseConns, ch)
		d.mu.Unlock()
	}()

	agents := d.GetAgents()
	data, _ := json.Marshal(agents)
	_, _ = fmt.Fprintf(w, "event: init\ndata: %s\n\n", data)
	flusher.Flush()

	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case evt := <-ch:
			data, _ := json.Marshal(evt)
			_, _ = fmt.Fprintf(w, "event: action\ndata: %s\n\n", data)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (d *Dashboard) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	data, _ := dashboardFS.ReadFile("dashboard/dashboard.html")
	_, _ = w.Write(data)
}

func (d *Dashboard) withNoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

type EventObserver func(evt AgentEvent)

func extractAgentID(r *http.Request) string {
	if id := r.Header.Get("X-Agent-Id"); id != "" {
		return id
	}
	if id := r.URL.Query().Get("agentId"); id != "" {
		return id
	}
	return "anonymous"
}

func extractProfile(r *http.Request) string {
	if p := r.Header.Get("X-Profile"); p != "" {
		return p
	}
	return r.URL.Query().Get("profile")
}

func isManagementRoute(path string) bool {
	return strings.HasPrefix(path, "/dashboard") ||
		strings.HasPrefix(path, "/profiles") ||
		strings.HasPrefix(path, "/instances") ||
		strings.HasPrefix(path, "/screencast/tabs") ||
		path == "/welcome" || path == "/favicon.ico" || path == "/health"
}

func actionDetail(r *http.Request) string {
	switch r.URL.Path {
	case "/navigate":
		return r.URL.Query().Get("url")
	case "/actions":
		return "batch action"
	case "/snapshot":
		if sel := r.URL.Query().Get("selector"); sel != "" {
			return "selector=" + sel
		}
	}
	return ""
}

func (d *Dashboard) TrackingMiddleware(observers []EventObserver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if isManagementRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)

		evt := AgentEvent{
			AgentID:    extractAgentID(r),
			Profile:    extractProfile(r),
			Action:     r.Method + " " + r.URL.Path,
			URL:        r.URL.Query().Get("url"),
			TabID:      r.URL.Query().Get("tabId"),
			Detail:     actionDetail(r),
			Status:     sw.code,
			DurationMs: time.Since(start).Milliseconds(),
			Timestamp:  start,
		}

		d.RecordEvent(evt)

		for _, obs := range observers {
			obs(evt)
		}
	})
}
