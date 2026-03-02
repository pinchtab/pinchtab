package dashboard

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/web"
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

// InstanceLister returns running instances (provided by Orchestrator).
type InstanceLister interface {
	List() []bridge.Instance
}

type Dashboard struct {
	cfg            DashboardConfig
	agents         map[string]*AgentActivity
	sseConns       map[chan AgentEvent]struct{}
	cancel         context.CancelFunc
	instances      InstanceLister
	childAuthToken string
	mu             sync.RWMutex
}

// SetInstanceLister sets the orchestrator for aggregating agents from child instances.
// Also starts a background relay that subscribes to child instance SSE events.
func (d *Dashboard) SetInstanceLister(il InstanceLister) {
	d.instances = il
	go d.relayChildEvents()
}

// relayChildEvents periodically checks for running child instances and subscribes
// to their SSE streams, re-broadcasting events through the dashboard.
func (d *Dashboard) relayChildEvents() {
	tracked := make(map[string]context.CancelFunc) // port -> cancel

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if d.instances == nil {
			continue
		}

		activePorts := make(map[string]bool)
		for _, inst := range d.instances.List() {
			if inst.Status != "running" || inst.Port == "" {
				continue
			}
			activePorts[inst.Port] = true
			if _, ok := tracked[inst.Port]; !ok {
				ctx, cancel := context.WithCancel(context.Background())
				tracked[inst.Port] = cancel
				go d.subscribeChildSSE(ctx, inst.Port, inst.ProfileName)
			}
		}

		// Stop subscriptions for instances that are no longer running.
		for port, cancel := range tracked {
			if !activePorts[port] {
				cancel()
				delete(tracked, port)
			}
		}
	}
}

func (d *Dashboard) subscribeChildSSE(ctx context.Context, port, profileName string) {
	url := "http://127.0.0.1:" + port + "/dashboard/events"
	for {
		if ctx.Err() != nil {
			return
		}
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return
		}
		if d.childAuthToken != "" {
			req.Header.Set("Authorization", "Bearer "+d.childAuthToken)
		}
		client := &http.Client{Timeout: 0} // no timeout for SSE
		resp, err := client.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
				continue
			}
		}
		d.readSSEStream(ctx, resp, profileName)
		_ = resp.Body.Close()

		// Reconnect after disconnect.
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (d *Dashboard) readSSEStream(ctx context.Context, resp *http.Response, profileName string) {
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		var evt AgentEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}
		if evt.Profile == "" {
			evt.Profile = profileName
		}
		d.RecordEvent(evt)
	}
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
		cfg:            c,
		agents:         make(map[string]*AgentActivity),
		sseConns:       make(map[chan AgentEvent]struct{}),
		cancel:         cancel,
		childAuthToken: os.Getenv("BRIDGE_TOKEN"),
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
	// API endpoints
	mux.HandleFunc("GET /api/agents", d.handleAgents)
	mux.HandleFunc("GET /api/events", d.handleSSE)

	// Static files served at /
	sub, _ := fs.Sub(dashboardFS, "dashboard")
	fileServer := http.FileServer(http.FS(sub))

	// Serve static assets
	mux.Handle("GET /assets/", d.withNoCache(fileServer))
	mux.Handle("GET /pinchtab-headed-192.png", d.withNoCache(fileServer))

	// SPA: serve dashboard.html for all other routes
	mux.Handle("GET /", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
}

func (d *Dashboard) handleAgents(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, 200, d.GetAgents())
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

		sw := &web.StatusWriter{ResponseWriter: w, Code: 200}
		next.ServeHTTP(sw, r)

		evt := AgentEvent{
			AgentID:    extractAgentID(r),
			Profile:    extractProfile(r),
			Action:     r.Method + " " + r.URL.Path,
			URL:        r.URL.Query().Get("url"),
			TabID:      r.URL.Query().Get("tabId"),
			Detail:     actionDetail(r),
			Status:     sw.Code,
			DurationMs: time.Since(start).Milliseconds(),
			Timestamp:  start,
		}

		d.RecordEvent(evt)

		for _, obs := range observers {
			obs(evt)
		}
	})
}
