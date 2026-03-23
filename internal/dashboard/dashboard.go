package dashboard

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func envWithFallback(newKey, oldKey string) string {
	if v := os.Getenv(newKey); v != "" {
		return v
	}
	return os.Getenv(oldKey)
}

type DashboardConfig struct {
	IdleTimeout       time.Duration
	DisconnectTimeout time.Duration
	ReaperInterval    time.Duration
	SSEBufferSize     int
}

//go:embed dashboard/*
var dashboardFS embed.FS

// SystemEvent is sent for instance lifecycle changes.
type SystemEvent struct {
	Type     string      `json:"type"` // "instance.started", "instance.stopped", "instance.error"
	Instance interface{} `json:"instance,omitempty"`
}

// InstanceLister returns running instances (provided by Orchestrator).
type InstanceLister interface {
	List() []bridge.Instance
}

type Dashboard struct {
	cfg            DashboardConfig
	activityConns  map[chan apiTypes.ActivityEvent]struct{}
	sysConns       map[chan SystemEvent]struct{}
	cancel         context.CancelFunc
	instances      InstanceLister
	monitoring     MonitoringSource
	serverMetrics  ServerMetricsProvider
	childAuthToken string

	agents       map[string]*apiTypes.Agent
	recentEvents []apiTypes.ActivityEvent
	maxEvents    int

	mu sync.RWMutex
}

// BroadcastSystemEvent sends a system event to all SSE clients.
func (d *Dashboard) BroadcastSystemEvent(evt SystemEvent) {
	d.mu.RLock()
	chans := make([]chan SystemEvent, 0, len(d.sysConns))
	for ch := range d.sysConns {
		chans = append(chans, ch)
	}
	d.mu.RUnlock()

	for _, ch := range chans {
		select {
		case ch <- evt:
		default:
		}
	}
}

// SetInstanceLister sets the orchestrator for managing instances.
func (d *Dashboard) SetInstanceLister(il InstanceLister) {
	d.instances = il
}

// RecordActivityEvent converts a backend activity record into a live tool-call event.
func (d *Dashboard) RecordActivityEvent(evt activity.Event) {
	details := map[string]any{
		"status":     evt.Status,
		"durationMs": evt.DurationMs,
	}
	if evt.Source != "" {
		details["source"] = evt.Source
	}
	if evt.RequestID != "" {
		details["requestId"] = evt.RequestID
	}
	if evt.SessionID != "" {
		details["sessionId"] = evt.SessionID
	}
	if evt.ActorID != "" {
		details["actorId"] = evt.ActorID
	}
	if evt.InstanceID != "" {
		details["instanceId"] = evt.InstanceID
	}
	if evt.ProfileID != "" {
		details["profileId"] = evt.ProfileID
	}
	if evt.ProfileName != "" {
		details["profileName"] = evt.ProfileName
	}
	if evt.TabID != "" {
		details["tabId"] = evt.TabID
	}
	if evt.URL != "" {
		details["url"] = evt.URL
	}
	if evt.Ref != "" {
		details["ref"] = evt.Ref
	}
	if evt.Engine != "" {
		details["engine"] = evt.Engine
	}

	d.RecordEvent(apiTypes.ActivityEvent{
		ID:        evt.RequestID,
		AgentID:   agentIDOrAnonymous(evt.AgentID),
		Channel:   "tool_call",
		Type:      classifyActivityType(evt),
		Method:    evt.Method,
		Path:      evt.Path,
		Timestamp: evt.Timestamp,
		Details:   details,
	})
}

// RecordEvent records an activity event, updates the live agent summary, and broadcasts to SSE subscribers.
func (d *Dashboard) RecordEvent(evt apiTypes.ActivityEvent) {
	if evt.ID == "" {
		evt.ID = generateID()
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	if evt.Channel == "" {
		evt.Channel = "tool_call"
	}
	if evt.AgentID == "" {
		evt.AgentID = "anonymous"
	}

	d.mu.Lock()
	d.upsertAgentLocked(evt)
	if len(d.recentEvents) >= d.maxEvents {
		copy(d.recentEvents, d.recentEvents[1:])
		d.recentEvents[len(d.recentEvents)-1] = evt
	} else {
		d.recentEvents = append(d.recentEvents, evt)
	}

	chans := make([]chan apiTypes.ActivityEvent, 0, len(d.activityConns))
	for ch := range d.activityConns {
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

func (d *Dashboard) upsertAgentLocked(evt apiTypes.ActivityEvent) {
	agentID := agentIDOrAnonymous(evt.AgentID)
	agent, ok := d.agents[agentID]
	if !ok {
		agent = &apiTypes.Agent{
			ID:          agentID,
			Name:        agentID,
			ConnectedAt: evt.Timestamp,
		}
		d.agents[agentID] = agent
	}
	agent.LastActivity = evt.Timestamp
	agent.RequestCount++
}

// Agents returns the current agent summary list ordered by most recent activity.
func (d *Dashboard) Agents() []apiTypes.Agent {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]apiTypes.Agent, 0, len(d.agents))
	for _, agent := range d.agents {
		result = append(result, *agent)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastActivity.After(result[j].LastActivity)
	})
	return result
}

// Agent returns the current summary for a single observed agent.
func (d *Dashboard) Agent(agentID string) (apiTypes.Agent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	agent, ok := d.agents[agentIDOrAnonymous(agentID)]
	if !ok {
		return apiTypes.Agent{}, false
	}
	return *agent, true
}

// AgentCount returns the number of currently observed agents.
func (d *Dashboard) AgentCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.agents)
}

// RecentEvents returns a copy of the buffered live event history.
func (d *Dashboard) RecentEvents() []apiTypes.ActivityEvent {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]apiTypes.ActivityEvent, len(d.recentEvents))
	copy(out, d.recentEvents)
	return out
}

// EventsForAgent returns buffered events for a single agent filtered by mode.
func (d *Dashboard) EventsForAgent(agentID, mode string) []apiTypes.ActivityEvent {
	agentID = agentIDOrAnonymous(agentID)
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]apiTypes.ActivityEvent, 0, len(d.recentEvents))
	for _, evt := range d.recentEvents {
		if evt.AgentID != agentID || !matchesMode(mode, evt.Channel) {
			continue
		}
		out = append(out, evt)
	}
	return out
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

	_, cancel := context.WithCancel(context.Background())
	return &Dashboard{
		cfg:            c,
		activityConns:  make(map[chan apiTypes.ActivityEvent]struct{}),
		sysConns:       make(map[chan SystemEvent]struct{}),
		cancel:         cancel,
		childAuthToken: envWithFallback("PINCHTAB_TOKEN", "BRIDGE_TOKEN"),
		agents:         make(map[string]*apiTypes.Agent),
		recentEvents:   make([]apiTypes.ActivityEvent, 0, 200),
		maxEvents:      200,
	}
}

func (d *Dashboard) Shutdown() { d.cancel() }

func (d *Dashboard) RegisterHandlers(mux *http.ServeMux) {
	// API endpoints
	mux.HandleFunc("GET /api/events", d.handleSSE)
	mux.HandleFunc("GET /api/agents", d.handleAgents)
	mux.HandleFunc("GET /api/agents/{id}", d.handleAgent)
	mux.HandleFunc("GET /api/agents/{id}/events", d.handleAgentSSE)
	mux.HandleFunc("POST /api/agents/{id}/events", d.handleAgentEventsByID)

	// Static files served at /dashboard/
	sub, _ := fs.Sub(dashboardFS, "dashboard")
	fileServer := http.FileServer(http.FS(sub))

	// Serve static assets under /dashboard/ with long cache (hashed filenames)
	mux.Handle("GET /dashboard/assets/", http.StripPrefix("/dashboard", d.withLongCache(fileServer)))
	mux.Handle("GET /dashboard/favicon.png", http.StripPrefix("/dashboard", d.withLongCache(fileServer)))

	// SPA: serve dashboard.html for /, /login, and /dashboard/*
	mux.Handle("GET /{$}", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
	mux.Handle("GET /login", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
	mux.Handle("GET /dashboard", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
	mux.Handle("GET /dashboard/{path...}", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
}

func (d *Dashboard) handleAgents(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, d.Agents())
}

func (d *Dashboard) handleAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	agent, ok := d.Agent(agentID)
	if !ok {
		httpx.ErrorCode(w, http.StatusNotFound, "agent_not_found", "agent not found", false, nil)
		return
	}

	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "both"
	}
	if mode != "tool_calls" && mode != "progress" && mode != "both" {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_mode", "mode must be tool_calls, progress, or both", false, nil)
		return
	}

	httpx.JSON(w, http.StatusOK, apiTypes.AgentDetail{
		Agent:  agent,
		Events: d.EventsForAgent(agentID, mode),
	})
}

func (d *Dashboard) handleAgentEventsByID(w http.ResponseWriter, r *http.Request) {
	pathAgentID := agentIDOrAnonymous(r.PathValue("id"))
	evt, ok := d.decodeProgressEvent(w, r, pathAgentID)
	if !ok {
		return
	}
	if evt.AgentID != "" && evt.AgentID != pathAgentID {
		httpx.ErrorCode(w, http.StatusBadRequest, "agent_mismatch", "agentId must match route parameter", false, nil)
		return
	}
	evt.AgentID = pathAgentID
	d.RecordEvent(evt)
	httpx.JSON(w, http.StatusCreated, map[string]string{"status": "ok", "id": evt.ID})
}

func (d *Dashboard) decodeProgressEvent(w http.ResponseWriter, r *http.Request, fallbackAgentID string) (apiTypes.ActivityEvent, bool) {
	var evt apiTypes.ActivityEvent
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&evt); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_agent_event", "invalid activity event payload", false, nil)
		return apiTypes.ActivityEvent{}, false
	}

	evt.AgentID = strings.TrimSpace(evt.AgentID)
	if evt.AgentID == "" {
		evt.AgentID = agentIDOrAnonymous(fallbackAgentID)
	}
	evt.Message = strings.TrimSpace(evt.Message)
	if evt.AgentID == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "missing_agent_id", "agentId is required", false, nil)
		return apiTypes.ActivityEvent{}, false
	}
	if evt.Message == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "missing_message", "message is required", false, nil)
		return apiTypes.ActivityEvent{}, false
	}
	if evt.Channel != "" && evt.Channel != "progress" {
		httpx.ErrorCode(w, http.StatusBadRequest, "invalid_channel", "channel must be progress", false, nil)
		return apiTypes.ActivityEvent{}, false
	}

	evt.Channel = "progress"
	evt.Type = "progress"
	return evt, true
}

func (d *Dashboard) handleAgentSSE(w http.ResponseWriter, r *http.Request) {
	agentID := agentIDOrAnonymous(r.PathValue("id"))
	if _, ok := d.Agent(agentID); !ok {
		httpx.ErrorCode(w, http.StatusNotFound, "agent_not_found", "agent not found", false, nil)
		return
	}

	r2 := r.Clone(r.Context())
	q := r2.URL.Query()
	q.Set("agentId", agentID)
	r2.URL.RawQuery = q.Encode()
	d.handleSSE(w, r2)
}

func (d *Dashboard) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// SSE connections are intentionally long-lived. Clear the server-level write
	// deadline for this response so the stream is not terminated after
	// http.Server.WriteTimeout elapses.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		http.Error(w, "streaming deadline unsupported", http.StatusInternalServerError)
		return
	}

	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "tool_calls"
	}
	if mode != "tool_calls" && mode != "progress" && mode != "both" {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_mode", "mode must be tool_calls, progress, or both", false, nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	activityCh := make(chan apiTypes.ActivityEvent, d.cfg.SSEBufferSize)
	sysCh := make(chan SystemEvent, d.cfg.SSEBufferSize)
	d.mu.Lock()
	d.activityConns[activityCh] = struct{}{}
	d.sysConns[sysCh] = struct{}{}
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.activityConns, activityCh)
		delete(d.sysConns, sysCh)
		d.mu.Unlock()
	}()

	includeMemory := r.URL.Query().Get("memory") == "1"
	agentFilter := agentIDOrAnonymous(strings.TrimSpace(r.URL.Query().Get("agentId")))
	if strings.TrimSpace(r.URL.Query().Get("agentId")) == "" {
		agentFilter = ""
	}
	agents := d.Agents()
	data, _ := json.Marshal(agents)
	_, _ = fmt.Fprintf(w, "event: init\ndata: %s\n\n", data)
	flusher.Flush()

	for _, evt := range d.RecentEvents() {
		if !matchesMode(mode, evt.Channel) {
			continue
		}
		if agentFilter != "" && evt.AgentID != agentFilter {
			continue
		}
		d.emitActivityEvent(w, flusher, evt)
	}

	if d.monitoring != nil || d.instances != nil {
		data, _ = json.Marshal(d.monitoringSnapshot(includeMemory))
		_, _ = fmt.Fprintf(w, "event: monitoring\ndata: %s\n\n", data)
		flusher.Flush()
	}

	keepalive := time.NewTicker(30 * time.Second)
	monitoring := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	defer monitoring.Stop()

	for {
		select {
		case evt := <-activityCh:
			if matchesMode(mode, evt.Channel) && (agentFilter == "" || evt.AgentID == agentFilter) {
				d.emitActivityEvent(w, flusher, evt)
			}
		case evt := <-sysCh:
			data, _ := json.Marshal(evt)
			_, _ = fmt.Fprintf(w, "event: system\ndata: %s\n\n", data)
			flusher.Flush()
			if d.monitoring != nil || d.instances != nil {
				data, _ = json.Marshal(d.monitoringSnapshot(includeMemory))
				_, _ = fmt.Fprintf(w, "event: monitoring\ndata: %s\n\n", data)
				flusher.Flush()
			}
		case <-monitoring.C:
			if d.monitoring != nil || d.instances != nil {
				data, _ := json.Marshal(d.monitoringSnapshot(includeMemory))
				_, _ = fmt.Fprintf(w, "event: monitoring\ndata: %s\n\n", data)
				flusher.Flush()
			}
		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (d *Dashboard) emitActivityEvent(w http.ResponseWriter, flusher http.Flusher, evt apiTypes.ActivityEvent) {
	name := "action"
	if evt.Channel == "progress" {
		name = "progress"
	}
	data, _ := json.Marshal(evt)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
	flusher.Flush()
}

func matchesMode(mode, channel string) bool {
	switch mode {
	case "both":
		return channel == "tool_call" || channel == "progress"
	case "progress":
		return channel == "progress"
	default:
		return channel == "tool_call"
	}
}

func classifyActivityType(evt activity.Event) string {
	if evt.Action != "" {
		switch evt.Action {
		case "navigate", "snapshot", "screenshot", "text":
			return evt.Action
		default:
			return "action"
		}
	}
	path := evt.Path
	switch {
	case strings.Contains(path, "/navigate"):
		return "navigate"
	case strings.Contains(path, "/snapshot"):
		return "snapshot"
	case strings.Contains(path, "/screenshot"):
		return "screenshot"
	case strings.Contains(path, "/text"):
		return "text"
	case strings.Contains(path, "/action"):
		return "action"
	default:
		return "other"
	}
}

func agentIDOrAnonymous(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "anonymous"
	}
	return agentID
}

const fallbackHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"/><meta name="viewport" content="width=device-width,initial-scale=1.0"/>
<title>PinchTab Dashboard</title>
<style>body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#0a0a0a;color:#e0e0e0}.c{text-align:center;max-width:480px;padding:2rem}h1{font-size:1.5rem;margin-bottom:.5rem}p{color:#888;line-height:1.6}code{background:#1a1a2e;padding:2px 8px;border-radius:4px;font-size:.9em}</style>
</head><body><div class="c"><h1>🦀 Dashboard not built</h1>
<p>The React dashboard needs to be compiled before use.<br/>
Run <code>./dev build</code> or <code>./scripts/build-dashboard.sh</code> then rebuild the Go binary.</p>
</div></body></html>`

func (d *Dashboard) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	data, err := dashboardFS.ReadFile("dashboard/dashboard.html")
	if err != nil {
		_, _ = w.Write([]byte(fallbackHTML))
		return
	}
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

func (d *Dashboard) withLongCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assets have hashes in filenames - cache for 1 year
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
