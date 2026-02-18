package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

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
	w.Write([]byte(dashboardHTML))
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
// Dashboard HTML
// ---------------------------------------------------------------------------

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>🦀 Pinchtab Dashboard</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: 'SF Mono', 'Fira Code', monospace;
    background: #0a0a0a;
    color: #e0e0e0;
    min-height: 100vh;
  }
  header {
    background: #111;
    border-bottom: 2px solid #f5c542;
    padding: 16px 24px;
    display: flex;
    align-items: center;
    gap: 16px;
  }
  header h1 { font-size: 20px; color: #f5c542; }
  header .status { font-size: 13px; color: #888; }
  header .status .dot {
    display: inline-block;
    width: 8px; height: 8px;
    border-radius: 50%;
    background: #4ade80;
    margin-right: 4px;
    animation: pulse 2s infinite;
  }
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }

  .container { display: flex; height: calc(100vh - 60px); }

  /* Left panel — agents */
  .agents-panel {
    width: 340px;
    border-right: 1px solid #222;
    overflow-y: auto;
    padding: 16px;
  }
  .agents-panel h2 {
    font-size: 13px;
    text-transform: uppercase;
    letter-spacing: 1px;
    color: #888;
    margin-bottom: 12px;
  }
  .agent-card {
    background: #151515;
    border: 1px solid #222;
    border-radius: 8px;
    padding: 14px;
    margin-bottom: 10px;
    cursor: pointer;
    transition: border-color 0.2s;
  }
  .agent-card:hover, .agent-card.selected { border-color: #f5c542; }
  .agent-card .agent-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }
  .agent-card .agent-name { font-weight: bold; font-size: 15px; }
  .agent-card .agent-status {
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 10px;
    font-weight: 600;
  }
  .agent-status.active { background: #064e3b; color: #4ade80; }
  .agent-status.idle { background: #422006; color: #fbbf24; }
  .agent-status.disconnected { background: #1c1917; color: #666; }
  .agent-card .agent-url {
    font-size: 12px;
    color: #888;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    margin-bottom: 4px;
  }
  .agent-card .agent-meta {
    font-size: 11px;
    color: #555;
    display: flex;
    gap: 12px;
  }

  /* Right panel — activity feed */
  .feed-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .feed-header {
    padding: 16px 24px;
    border-bottom: 1px solid #222;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .feed-header h2 { font-size: 14px; color: #888; text-transform: uppercase; letter-spacing: 1px; }
  .feed-header .filter-btns { display: flex; gap: 6px; }
  .feed-header .filter-btn {
    background: #1a1a1a;
    border: 1px solid #333;
    color: #aaa;
    padding: 4px 10px;
    border-radius: 4px;
    font-size: 11px;
    cursor: pointer;
    font-family: inherit;
  }
  .feed-header .filter-btn.active { border-color: #f5c542; color: #f5c542; }

  .feed-list {
    flex: 1;
    overflow-y: auto;
    padding: 8px 24px;
  }
  .event-row {
    display: grid;
    grid-template-columns: 90px 100px 1fr 70px 60px;
    gap: 12px;
    padding: 8px 0;
    border-bottom: 1px solid #1a1a1a;
    font-size: 13px;
    align-items: center;
    animation: fadeIn 0.3s ease;
  }
  @keyframes fadeIn { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; } }
  .event-row .time { color: #555; font-size: 11px; }
  .event-row .agent {
    color: #f5c542;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .event-row .action-detail {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .event-row .action-detail .method {
    display: inline-block;
    padding: 1px 5px;
    border-radius: 3px;
    font-size: 10px;
    font-weight: bold;
    margin-right: 4px;
  }
  .method.GET { background: #1a3a2a; color: #4ade80; }
  .method.POST { background: #1a2a3a; color: #60a5fa; }
  .method.DELETE { background: #3a1a1a; color: #f87171; }
  .event-row .duration { color: #666; text-align: right; font-size: 12px; }
  .event-row .status-code { text-align: right; font-size: 12px; }
  .status-code.ok { color: #4ade80; }
  .status-code.err { color: #f87171; }

  /* Profiles panel (bottom) */
  .profiles-bar {
    border-top: 1px solid #222;
    padding: 12px 24px;
    display: flex;
    gap: 10px;
    overflow-x: auto;
    background: #0d0d0d;
  }
  .profile-chip {
    background: #1a1a1a;
    border: 1px solid #333;
    border-radius: 6px;
    padding: 8px 14px;
    font-size: 12px;
    white-space: nowrap;
    cursor: pointer;
    transition: border-color 0.2s;
  }
  .profile-chip:hover { border-color: #f5c542; }
  .profile-chip .pname { font-weight: bold; color: #e0e0e0; }
  .profile-chip .psize { color: #555; margin-left: 8px; }
  .profile-chip .psource { color: #f5c542; font-size: 10px; margin-left: 6px; }

  /* Settings view */
  .settings-view { height: calc(100vh - 60px); overflow-y: auto; }
  .settings-container { max-width: 600px; margin: 0 auto; padding: 24px; }
  .settings-title { color: #f5c542; font-size: 18px; margin-bottom: 24px; }
  .settings-section {
    background: #151515;
    border: 1px solid #222;
    border-radius: 10px;
    padding: 18px;
    margin-bottom: 16px;
  }
  .settings-section h3 { color: #e0e0e0; font-size: 14px; margin-bottom: 14px; }
  .setting-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 0;
    border-bottom: 1px solid #1a1a1a;
  }
  .setting-row:last-child { border-bottom: none; }
  .setting-row label { color: #888; font-size: 13px; }
  .setting-control { display: flex; align-items: center; gap: 10px; }
  .setting-value { color: #f5c542; font-size: 13px; min-width: 50px; text-align: right; }
  .setting-control input[type="range"] {
    width: 140px;
    accent-color: #f5c542;
  }
  .setting-control select {
    background: #0a0a0a;
    border: 1px solid #333;
    color: #e0e0e0;
    padding: 6px 10px;
    border-radius: 4px;
    font-family: inherit;
    font-size: 12px;
  }
  .setting-info {
    font-size: 11px;
    color: #666;
    margin-top: 8px;
    padding: 8px;
    background: #0d0d0d;
    border-radius: 4px;
  }
  .toggle { display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 12px; color: #888; }
  .toggle input { accent-color: #f5c542; }
  .toggle-label { min-width: 24px; }
  .server-info { font-size: 12px; color: #888; line-height: 1.8; }
  .server-info .info-row { display: flex; justify-content: space-between; }
  .server-info .info-label { color: #666; }
  .server-info .info-val { color: #e0e0e0; }
  .settings-actions { display: flex; gap: 8px; margin-top: 8px; }

  /* Instances view */
  .instances-view { height: calc(100vh - 60px); display: flex; flex-direction: column; }
  .inst-toolbar {
    padding: 12px 24px;
    border-bottom: 1px solid #222;
    display: flex;
    gap: 8px;
  }
  .launch-btn {
    background: #f5c542;
    color: #0a0a0a;
    border: none;
    padding: 8px 18px;
    border-radius: 6px;
    font-weight: bold;
    cursor: pointer;
    font-family: inherit;
    font-size: 13px;
  }
  .refresh-btn {
    background: #333;
    color: #e0e0e0;
    border: 1px solid #444;
    padding: 8px 14px;
    border-radius: 6px;
    cursor: pointer;
    font-family: inherit;
    font-size: 13px;
  }
  .instances-grid {
    flex: 1;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
    gap: 14px;
    padding: 16px 24px;
    overflow-y: auto;
    align-content: start;
  }
  .inst-card {
    background: #151515;
    border: 1px solid #222;
    border-radius: 10px;
    overflow: hidden;
  }
  .inst-card .inst-header {
    padding: 14px 16px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-bottom: 1px solid #1a1a1a;
  }
  .inst-card .inst-name { font-weight: bold; font-size: 16px; color: #f5c542; }
  .inst-card .inst-badge {
    font-size: 11px;
    padding: 3px 10px;
    border-radius: 10px;
    font-weight: 600;
  }
  .inst-badge.running { background: #064e3b; color: #4ade80; }
  .inst-badge.starting { background: #422006; color: #fbbf24; }
  .inst-badge.stopped { background: #1c1917; color: #666; }
  .inst-badge.error { background: #3a1a1a; color: #f87171; }
  .inst-card .inst-body { padding: 12px 16px; }
  .inst-card .inst-row {
    display: flex;
    justify-content: space-between;
    font-size: 13px;
    padding: 3px 0;
  }
  .inst-card .inst-row .label { color: #666; }
  .inst-card .inst-row .value { color: #ccc; }
  .inst-card .inst-actions {
    padding: 10px 16px;
    border-top: 1px solid #1a1a1a;
    display: flex;
    gap: 6px;
  }
  .inst-card .inst-actions button {
    flex: 1;
    background: #1a1a1a;
    border: 1px solid #333;
    color: #aaa;
    padding: 6px;
    border-radius: 4px;
    font-size: 11px;
    cursor: pointer;
    font-family: inherit;
  }
  .inst-card .inst-actions button:hover { border-color: #f5c542; color: #f5c542; }
  .inst-card .inst-actions button.danger:hover { border-color: #f87171; color: #f87171; }

  /* View toggle */
  .view-toggle { margin-left: auto; display: flex; gap: 4px; }
  .view-btn {
    background: #1a1a1a;
    border: 1px solid #333;
    color: #aaa;
    padding: 6px 14px;
    border-radius: 4px;
    font-size: 12px;
    cursor: pointer;
    font-family: inherit;
  }
  .view-btn.active { border-color: #f5c542; color: #f5c542; }

  /* Live view */
  .live-view { height: calc(100vh - 60px); display: flex; flex-direction: column; }
  .live-toolbar { padding: 12px 24px; border-bottom: 1px solid #222; }
  .screencast-grid {
    flex: 1;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(400px, 1fr));
    gap: 12px;
    padding: 16px;
    overflow-y: auto;
  }
  .screen-tile {
    background: #111;
    border: 1px solid #222;
    border-radius: 8px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  .screen-tile .tile-header {
    padding: 8px 12px;
    background: #151515;
    border-bottom: 1px solid #222;
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 12px;
  }
  .screen-tile .tile-header .tile-url {
    color: #888;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 280px;
  }
  .screen-tile .tile-header .tile-id { color: #f5c542; font-weight: 600; }
  .screen-tile .tile-header .tile-status {
    width: 8px; height: 8px; border-radius: 50%;
    display: inline-block; margin-left: 8px;
  }
  .screen-tile .tile-header .tile-status.streaming { background: #4ade80; }
  .screen-tile .tile-header .tile-status.connecting { background: #fbbf24; }
  .screen-tile .tile-header .tile-status.error { background: #f87171; }
  .screen-tile canvas {
    width: 100%;
    background: #000;
    cursor: pointer;
  }
  .screen-tile .tile-footer {
    padding: 4px 12px;
    background: #0d0d0d;
    font-size: 11px;
    color: #555;
    display: flex;
    gap: 12px;
  }

  .empty-state {
    text-align: center;
    color: #444;
    padding: 60px;
    font-size: 14px;
  }
  .empty-state .crab { font-size: 48px; margin-bottom: 12px; }

  /* Profile management modal */
  .modal-overlay {
    display: none;
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.7);
    z-index: 100;
    justify-content: center;
    align-items: center;
  }
  .modal-overlay.open { display: flex; }
  .modal {
    background: #151515;
    border: 1px solid #333;
    border-radius: 12px;
    padding: 24px;
    width: 420px;
    max-width: 90vw;
  }
  .modal h3 { color: #f5c542; margin-bottom: 16px; }
  .modal input, .modal select {
    width: 100%;
    background: #0a0a0a;
    border: 1px solid #333;
    color: #e0e0e0;
    padding: 10px;
    border-radius: 6px;
    font-family: inherit;
    font-size: 13px;
    margin-bottom: 12px;
  }
  .modal .btn-row { display: flex; gap: 8px; justify-content: flex-end; }
  .modal button {
    background: #f5c542;
    color: #0a0a0a;
    border: none;
    padding: 8px 18px;
    border-radius: 6px;
    font-weight: bold;
    cursor: pointer;
    font-family: inherit;
  }
  .modal button.secondary { background: #333; color: #e0e0e0; }
</style>
</head>
<body>
<header>
  <h1>🦀 Pinchtab</h1>
  <div class="status"><span class="dot"></span>Live</div>
  <div class="view-toggle">
    <button class="view-btn active" data-view="feed" onclick="switchView('feed')">🤖 Agents</button>
    <button class="view-btn" data-view="instances" onclick="switchView('instances')">🖥️ Instances</button>
    <button class="view-btn" data-view="live" onclick="switchView('live')">📺 Live</button>
    <button class="view-btn" data-view="settings" onclick="switchView('settings')">⚙️</button>
  </div>
</header>

<!-- Live screencast view (hidden by default) -->
<div id="live-view" class="live-view" style="display:none">
  <div class="live-toolbar">
    <button onclick="refreshTabs()" style="background:#333;color:#e0e0e0;border:1px solid #444;padding:6px 14px;border-radius:4px;cursor:pointer;font-family:inherit;font-size:12px">↻ Refresh Tabs</button>
    <span id="live-tab-count" style="color:#666;font-size:12px;margin-left:12px"></span>
  </div>
  <div id="screencast-grid" class="screencast-grid"></div>
</div>

<!-- Instances view -->
<div id="instances-view" class="instances-view" style="display:none">
  <div class="inst-toolbar">
    <button onclick="showLaunchModal()" class="launch-btn">+ Launch Instance</button>
    <button onclick="loadInstances()" class="refresh-btn">↻ Refresh</button>
    <div id="profiles-bar" style="display:flex;gap:8px;margin-left:16px;overflow-x:auto"></div>
  </div>
  <div id="instances-grid" class="instances-grid">
    <div class="empty-state"><div class="crab">🦀</div>No instances running.<br>Launch one to get started.</div>
  </div>
</div>

<!-- Launch modal -->
<div class="modal-overlay" id="launch-modal">
  <div class="modal">
    <h3>🦀 Launch Instance</h3>
    <label style="color:#888;font-size:12px;display:block;margin-bottom:4px">Profile Name</label>
    <input id="launch-name" placeholder="e.g. personal, work, scraping" />
    <label style="color:#888;font-size:12px;display:block;margin-bottom:4px">Port</label>
    <input id="launch-port" placeholder="e.g. 9868" />
    <label style="color:#888;font-size:12px;display:flex;align-items:center;gap:8px;margin-bottom:12px">
      <input type="checkbox" id="launch-headless" checked /> Headless
    </label>
    <div class="btn-row">
      <button class="secondary" onclick="closeLaunchModal()">Cancel</button>
      <button onclick="doLaunch()">Launch</button>
    </div>
  </div>
</div>

<!-- Settings view -->
<div id="settings-view" class="settings-view" style="display:none">
  <div class="settings-container">
    <h2 class="settings-title">⚙️ Settings</h2>

    <div class="settings-section">
      <h3>📺 Screencast</h3>
      <div class="setting-row">
        <label>Frame Rate</label>
        <div class="setting-control">
          <input type="range" id="set-fps" min="1" max="15" value="1" oninput="document.getElementById('fps-val').textContent=this.value+' fps'">
          <span id="fps-val" class="setting-value">1 fps</span>
        </div>
      </div>
      <div class="setting-row">
        <label>Quality</label>
        <div class="setting-control">
          <input type="range" id="set-quality" min="10" max="80" value="30" oninput="document.getElementById('quality-val').textContent=this.value+'%'">
          <span id="quality-val" class="setting-value">30%</span>
        </div>
      </div>
      <div class="setting-row">
        <label>Max Width</label>
        <div class="setting-control">
          <select id="set-maxwidth">
            <option value="400">400px</option>
            <option value="600">600px</option>
            <option value="800" selected>800px</option>
            <option value="1024">1024px</option>
            <option value="1280">1280px</option>
          </select>
        </div>
      </div>
    </div>

    <div class="settings-section">
      <h3>🛡️ Stealth</h3>
      <div class="setting-row">
        <label>Level</label>
        <div class="setting-control">
          <select id="set-stealth" onchange="applyStealth()">
            <option value="light">Light (default — works with X, Gmail)</option>
            <option value="full">Full (canvas noise, WebGL, fonts)</option>
          </select>
        </div>
      </div>
      <div id="stealth-info" class="setting-info"></div>
    </div>

    <div class="settings-section">
      <h3>🌐 Browser</h3>
      <div class="setting-row">
        <label>Block Images</label>
        <div class="setting-control">
          <label class="toggle"><input type="checkbox" id="set-block-images"><span class="toggle-label">Off</span></label>
        </div>
      </div>
      <div class="setting-row">
        <label>Block Media</label>
        <div class="setting-control">
          <label class="toggle"><input type="checkbox" id="set-block-media"><span class="toggle-label">Off</span></label>
        </div>
      </div>
      <div class="setting-row">
        <label>No Animations</label>
        <div class="setting-control">
          <label class="toggle"><input type="checkbox" id="set-no-animations"><span class="toggle-label">Off</span></label>
        </div>
      </div>
    </div>

    <div class="settings-section">
      <h3>📊 Server Info</h3>
      <div id="server-info" class="server-info">Loading...</div>
    </div>

    <div class="settings-actions">
      <button onclick="applySettings()" class="launch-btn">Apply & Reconnect Live</button>
      <button onclick="loadSettings()" class="refresh-btn">Reset</button>
    </div>
  </div>
</div>

<!-- Feed view (default) -->
<div id="feed-view" class="container">
  <div class="agents-panel">
    <h2>Agents</h2>
    <div id="agents-list">
      <div class="empty-state"><div class="crab">🦀</div>No agents connected yet.<br>Make an API call with <code>X-Agent-Id</code> header.</div>
    </div>
  </div>

  <div class="feed-panel">
    <div class="feed-header">
      <h2>Activity Feed</h2>
      <div class="filter-btns">
        <button class="filter-btn active" data-filter="all">All</button>
        <button class="filter-btn" data-filter="navigate">Navigate</button>
        <button class="filter-btn" data-filter="snapshot">Snapshot</button>
        <button class="filter-btn" data-filter="actions">Actions</button>
      </div>
    </div>
    <div id="feed-list" class="feed-list">
      <div class="empty-state"><div class="crab">🦀</div>Waiting for events...</div>
    </div>
  </div>
</div>

</div><!-- end feed-view -->

<!-- Profile management modal -->
<div class="modal-overlay" id="modal">
  <div class="modal">
    <h3 id="modal-title">Manage Profile</h3>
    <div id="modal-body"></div>
  </div>
</div>

<script>
const MAX_EVENTS = 500;
const events = [];
const agents = {};
let selectedAgent = null;
let currentFilter = 'all';

// SSE connection
function connect() {
  const es = new EventSource('/dashboard/events');

  es.addEventListener('init', (e) => {
    const list = JSON.parse(e.data);
    list.forEach(a => { agents[a.agentId] = a; });
    renderAgents();
  });

  es.addEventListener('action', (e) => {
    const evt = JSON.parse(e.data);
    events.unshift(evt);
    if (events.length > MAX_EVENTS) events.pop();

    // Update agent state
    if (!agents[evt.agentId]) {
      agents[evt.agentId] = { agentId: evt.agentId, actionCount: 0, status: 'active' };
    }
    const a = agents[evt.agentId];
    a.lastAction = evt.action;
    a.lastSeen = evt.timestamp;
    a.currentUrl = evt.url || a.currentUrl;
    a.currentTab = evt.tabId || a.currentTab;
    a.profile = evt.profile || a.profile;
    a.status = 'active';
    a.actionCount++;

    renderAgents();
    renderFeed();
  });

  es.onerror = () => {
    es.close();
    setTimeout(connect, 3000);
  };
}

function renderAgents() {
  const el = document.getElementById('agents-list');
  const ids = Object.keys(agents);
  if (ids.length === 0) return;

  el.innerHTML = ids.map(id => {
    const a = agents[id];
    const sel = selectedAgent === id ? 'selected' : '';
    const ago = a.lastSeen ? timeAgo(new Date(a.lastSeen)) : '—';
    return ` + "`" + `
      <div class="agent-card ${sel}" onclick="selectAgent('${id}')">
        <div class="agent-header">
          <span class="agent-name">${esc(id)}</span>
          <span class="agent-status ${a.status}">${a.status}</span>
        </div>
        <div class="agent-url">${esc(a.currentUrl || 'No URL yet')}</div>
        <div class="agent-meta">
          <span>${a.profile ? '📁 ' + esc(a.profile) : ''}</span>
          <span>📊 ${a.actionCount} actions</span>
          <span>${ago}</span>
        </div>
      </div>
    ` + "`" + `;
  }).join('');
}

function renderFeed() {
  const el = document.getElementById('feed-list');
  let filtered = events;

  if (selectedAgent) {
    filtered = filtered.filter(e => e.agentId === selectedAgent);
  }
  if (currentFilter !== 'all') {
    filtered = filtered.filter(e => e.action.toLowerCase().includes(currentFilter));
  }

  if (filtered.length === 0) {
    el.innerHTML = '<div class="empty-state"><div class="crab">🦀</div>No matching events.</div>';
    return;
  }

  el.innerHTML = filtered.slice(0, 200).map(evt => {
    const parts = evt.action.split(' ');
    const method = parts[0] || '';
    const path = parts.slice(1).join(' ');
    const statusClass = evt.status < 400 ? 'ok' : 'err';
    const detail = evt.detail || evt.url || '';
    const t = new Date(evt.timestamp);
    const time = t.toLocaleTimeString();

    return ` + "`" + `
      <div class="event-row">
        <span class="time">${time}</span>
        <span class="agent">${esc(evt.agentId)}</span>
        <span class="action-detail">
          <span class="method ${method}">${method}</span>
          ${esc(path)}${detail ? ' <span style="color:#666">— ' + esc(detail) + '</span>' : ''}
        </span>
        <span class="duration">${evt.durationMs}ms</span>
        <span class="status-code ${statusClass}">${evt.status}</span>
      </div>
    ` + "`" + `;
  }).join('');
}

function selectAgent(id) {
  selectedAgent = selectedAgent === id ? null : id;
  renderAgents();
  renderFeed();
}

// Filters
document.querySelectorAll('.filter-btn').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    currentFilter = btn.dataset.filter;
    renderFeed();
  });
});

// Load profiles
async function loadProfiles() {
  try {
    const res = await fetch('/profiles');
    const profiles = await res.json();
    const bar = document.getElementById('profiles-bar');
    if (!profiles || profiles.length === 0) {
      bar.innerHTML = '<div style="color:#555;font-size:12px;padding:4px">No profiles managed yet. Use POST /profiles/create or /profiles/import.</div>';
      return;
    }
    bar.innerHTML = profiles.map(p => ` + "`" + `
      <div class="profile-chip" onclick="showProfileModal('${esc(p.name)}')">
        <span class="pname">${esc(p.name)}</span>
        <span class="psize">${p.sizeMB.toFixed(1)}MB</span>
        <span class="psource">${p.source}</span>
      </div>
    ` + "`" + `).join('');
  } catch (e) {}
}

function showProfileModal(name) {
  const modal = document.getElementById('modal');
  const title = document.getElementById('modal-title');
  const body = document.getElementById('modal-body');
  title.textContent = 'Profile: ' + name;
  body.innerHTML = ` + "`" + `
    <p style="color:#888;margin-bottom:16px">Manage this Chrome profile.</p>
    <div class="btn-row">
      <button class="secondary" onclick="closeModal()">Close</button>
      <button class="secondary" onclick="resetProfile('${name}')">Reset</button>
      <button onclick="viewAnalytics('${name}')">Analytics</button>
    </div>
  ` + "`" + `;
  modal.classList.add('open');
}

function closeModal() {
  document.getElementById('modal').classList.remove('open');
}

async function resetProfile(name) {
  if (!confirm('Reset profile "' + name + '"? This clears session data, cookies, and cache.')) return;
  await fetch('/profiles/' + name + '/reset', { method: 'POST' });
  closeModal();
  loadProfiles();
}

async function viewAnalytics(name) {
  const res = await fetch('/profiles/' + name + '/analytics');
  const data = await res.json();
  const body = document.getElementById('modal-body');
  body.innerHTML = ` + "`" + `
    <p style="color:#888;margin-bottom:8px">${data.totalActions} total actions</p>
    ${data.suggestions.map(s => '<p style="color:#f5c542;font-size:13px;margin-bottom:4px">💡 ' + esc(s) + '</p>').join('')}
    ${data.topEndpoints ? '<h4 style="color:#888;margin-top:12px;font-size:12px">TOP ENDPOINTS</h4>' + data.topEndpoints.map(e => '<div style="font-size:12px;color:#aaa;padding:2px 0">' + esc(e.endpoint) + ' — ' + e.count + 'x, avg ' + e.avgMs + 'ms</div>').join('') : ''}
    <div class="btn-row" style="margin-top:16px">
      <button class="secondary" onclick="closeModal()">Close</button>
    </div>
  ` + "`" + `;
}

document.getElementById('modal').addEventListener('click', (e) => {
  if (e.target.classList.contains('modal-overlay')) closeModal();
});

function esc(s) { if (!s) return ''; const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
function timeAgo(d) {
  const s = Math.floor((Date.now() - d.getTime()) / 1000);
  if (s < 5) return 'just now';
  if (s < 60) return s + 's ago';
  if (s < 3600) return Math.floor(s/60) + 'm ago';
  return Math.floor(s/3600) + 'h ago';
}

connect();

// ---------------------------------------------------------------------------
// View switching
// ---------------------------------------------------------------------------
function switchView(view) {
  document.querySelectorAll('.view-btn').forEach(b => b.classList.remove('active'));
  document.querySelector('[data-view="'+view+'"]').classList.add('active');
  document.getElementById('feed-view').style.display = view === 'feed' ? 'flex' : 'none';
  document.getElementById('instances-view').style.display = view === 'instances' ? 'flex' : 'none';
  document.getElementById('live-view').style.display = view === 'live' ? 'flex' : 'none';
  document.getElementById('settings-view').style.display = view === 'settings' ? 'block' : 'none';
  if (view === 'live') refreshTabs();
  if (view === 'instances') loadInstances();
  if (view === 'settings') loadSettings();
}

// ---------------------------------------------------------------------------
// Instances
// ---------------------------------------------------------------------------
async function loadInstances() {
  // Load profiles into toolbar
  try {
    const pres = await fetch('/profiles');
    const profiles = await pres.json();
    const bar = document.getElementById('profiles-bar');
    if (profiles && profiles.length > 0) {
      bar.innerHTML = profiles.map(p => ` + "`" + `
        <div class="profile-chip" onclick="showProfileModal('${esc(p.name)}')">
          <span class="pname">${esc(p.name)}</span>
          <span class="psize">${p.sizeMB.toFixed(0)}MB</span>
          <span class="psource">${p.source}</span>
        </div>
      ` + "`" + `).join('');
    } else {
      bar.innerHTML = '';
    }
  } catch(e) {}

  try {
    const res = await fetch('/instances');
    const instances = await res.json();
    const grid = document.getElementById('instances-grid');

    if (!instances || instances.length === 0) {
      grid.innerHTML = '<div class="empty-state"><div class="crab">🦀</div>No instances running.<br>Click <b>+ Launch Instance</b> to start one.</div>';
      return;
    }

    grid.innerHTML = instances.map(inst => ` + "`" + `
      <div class="inst-card">
        <div class="inst-header">
          <span class="inst-name">${esc(inst.name)}</span>
          <span class="inst-badge ${inst.status}">${inst.status}</span>
        </div>
        <div class="inst-body">
          <div class="inst-row"><span class="label">Port</span><span class="value">${esc(inst.port)}</span></div>
          <div class="inst-row"><span class="label">Mode</span><span class="value">${inst.headless ? '🔲 Headless' : '🖥️ Headed'}</span></div>
          <div class="inst-row"><span class="label">Tabs</span><span class="value">${inst.tabCount}</span></div>
          <div class="inst-row"><span class="label">PID</span><span class="value">${inst.pid || '—'}</span></div>
          <div class="inst-row"><span class="label">Profile</span><span class="value" style="font-size:11px;max-width:180px;overflow:hidden;text-overflow:ellipsis">${esc(inst.profile)}</span></div>
          ${inst.error ? '<div class="inst-row"><span class="label">Error</span><span class="value" style="color:#f87171">' + esc(inst.error) + '</span></div>' : ''}
        </div>
        <div class="inst-actions">
          <button onclick="viewInstanceLive('${esc(inst.id)}', '${esc(inst.port)}')">📺 Live</button>
          <button onclick="viewInstanceLogs('${esc(inst.id)}')">📄 Logs</button>
          <button onclick="openInstanceDirect('${esc(inst.port)}')">🔗 Open</button>
          ${inst.status === 'running' ? '<button class="danger" onclick="stopInstance(\'' + esc(inst.id) + '\')">⏹ Stop</button>' : ''}
        </div>
      </div>
    ` + "`" + `).join('');
  } catch (e) {
    console.error('Failed to load instances', e);
  }
}

function showLaunchModal() {
  document.getElementById('launch-modal').classList.add('open');
  document.getElementById('launch-name').focus();
}
function closeLaunchModal() {
  document.getElementById('launch-modal').classList.remove('open');
}

async function doLaunch() {
  const name = document.getElementById('launch-name').value.trim();
  const port = document.getElementById('launch-port').value.trim();
  const headless = document.getElementById('launch-headless').checked;

  if (!name || !port) { alert('Name and port required'); return; }

  closeLaunchModal();

  try {
    const res = await fetch('/instances/launch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, port, headless })
    });
    const data = await res.json();
    if (!res.ok) {
      alert('Launch failed: ' + (data.error || 'unknown'));
      return;
    }
    // Poll until running
    pollInstanceStatus(data.id);
  } catch (e) {
    alert('Launch error: ' + e.message);
  }
}

function pollInstanceStatus(id) {
  let attempts = 0;
  const poll = setInterval(async () => {
    attempts++;
    await loadInstances();
    if (attempts > 30) clearInterval(poll);
    try {
      const res = await fetch('/instances');
      const instances = await res.json();
      const inst = instances.find(i => i.id === id);
      if (inst && (inst.status === 'running' || inst.status === 'error' || inst.status === 'stopped')) {
        clearInterval(poll);
        loadInstances();
      }
    } catch(e) { clearInterval(poll); }
  }, 1000);
}

async function stopInstance(id) {
  if (!confirm('Stop instance ' + id + '?')) return;
  await fetch('/instances/' + id + '/stop', { method: 'POST' });
  setTimeout(loadInstances, 1000);
}

async function viewInstanceLogs(id) {
  const res = await fetch('/instances/' + id + '/logs');
  const text = await res.text();
  const modal = document.getElementById('modal');
  const title = document.getElementById('modal-title');
  const body = document.getElementById('modal-body');
  title.textContent = 'Logs: ' + id;
  body.innerHTML = '<pre style="background:#0a0a0a;padding:12px;border-radius:6px;font-size:11px;max-height:400px;overflow:auto;color:#aaa;white-space:pre-wrap">' + esc(text) + '</pre><div class="btn-row" style="margin-top:12px"><button class="secondary" onclick="closeModal()">Close</button></div>';
  modal.classList.add('open');
}

async function viewInstanceLive(id, port) {
  // Switch to live view and load tabs from that instance
  switchView('live');
  try {
    const res = await fetch('/instances/tabs');
    const tabs = await res.json();
    const instTabs = tabs.filter(t => t.instancePort === port);
    const grid = document.getElementById('screencast-grid');
    document.getElementById('live-tab-count').textContent = instTabs.length + ' tab(s) on ' + id;

    if (instTabs.length === 0) {
      grid.innerHTML = '<div class="empty-state"><div class="crab">🦀</div>No tabs in this instance.</div>';
      return;
    }

    grid.innerHTML = instTabs.map(t => ` + "`" + `
      <div class="screen-tile" id="tile-${t.tabId}">
        <div class="tile-header">
          <span>
            <span class="tile-id">${esc(t.instanceName)}:${t.tabId.substring(0, 6)}</span>
            <span class="tile-status connecting" id="status-${t.tabId}"></span>
          </span>
          <span class="tile-url" id="url-${t.tabId}">${esc(t.url || 'about:blank')}</span>
        </div>
        <canvas id="canvas-${t.tabId}" width="800" height="600"></canvas>
        <div class="tile-footer">
          <span id="fps-${t.tabId}">—</span>
          <span id="size-${t.tabId}">—</span>
        </div>
      </div>
    ` + "`" + `).join('');

    // Connect screencast directly to child instance
    instTabs.forEach(t => {
      startScreencastDirect(t.tabId, t.instancePort);
    });
  } catch (e) {
    console.error('Failed to load instance tabs', e);
  }
}

function startScreencastDirect(tabId, port) {
  const wsUrl = 'ws://localhost:' + port + '/screencast?tabId=' + tabId + getScreencastParams();
  const socket = new WebSocket(wsUrl);
  socket.binaryType = 'arraybuffer';
  screencastSockets[tabId] = socket;

  const canvas = document.getElementById('canvas-' + tabId);
  if (!canvas) return;
  const ctx2d = canvas.getContext('2d');

  let frameCount = 0;
  let lastFpsTime = Date.now();
  const statusEl = document.getElementById('status-' + tabId);
  const fpsEl = document.getElementById('fps-' + tabId);
  const sizeEl = document.getElementById('size-' + tabId);

  socket.onopen = () => { if (statusEl) statusEl.className = 'tile-status streaming'; };
  socket.onmessage = (evt) => {
    const blob = new Blob([evt.data], { type: 'image/jpeg' });
    const url = URL.createObjectURL(blob);
    const img = new Image();
    img.onload = () => { canvas.width = img.width; canvas.height = img.height; ctx2d.drawImage(img, 0, 0); URL.revokeObjectURL(url); };
    img.src = url;
    frameCount++;
    const now = Date.now();
    if (now - lastFpsTime >= 1000) {
      if (fpsEl) fpsEl.textContent = frameCount + ' fps';
      if (sizeEl) sizeEl.textContent = (evt.data.byteLength / 1024).toFixed(0) + ' KB/frame';
      frameCount = 0; lastFpsTime = now;
    }
  };
  socket.onerror = () => { if (statusEl) statusEl.className = 'tile-status error'; };
  socket.onclose = () => { if (statusEl) statusEl.className = 'tile-status error'; };
}

function openInstanceDirect(port) {
  window.open('http://localhost:' + port + '/dashboard', '_blank');
}

// ---------------------------------------------------------------------------
// Screencast
// ---------------------------------------------------------------------------
const screencastSockets = {};

async function refreshTabs() {
  // Clean up existing connections
  Object.values(screencastSockets).forEach(s => s.close());
  Object.keys(screencastSockets).forEach(k => delete screencastSockets[k]);

  try {
    const res = await fetch('/screencast/tabs');
    const tabs = await res.json();
    const grid = document.getElementById('screencast-grid');
    document.getElementById('live-tab-count').textContent = tabs.length + ' tab(s)';

    if (tabs.length === 0) {
      grid.innerHTML = '<div class="empty-state"><div class="crab">🦀</div>No tabs open.</div>';
      return;
    }

    grid.innerHTML = tabs.map(t => ` + "`" + `
      <div class="screen-tile" id="tile-${t.id}">
        <div class="tile-header">
          <span>
            <span class="tile-id">${t.id.substring(0, 8)}</span>
            <span class="tile-status connecting" id="status-${t.id}"></span>
          </span>
          <span class="tile-url" id="url-${t.id}">${esc(t.url || 'about:blank')}</span>
        </div>
        <canvas id="canvas-${t.id}" width="800" height="600"></canvas>
        <div class="tile-footer">
          <span id="fps-${t.id}">—</span>
          <span id="size-${t.id}">—</span>
        </div>
      </div>
    ` + "`" + `).join('');

    // Start screencast for each tab
    tabs.forEach(t => startScreencast(t.id));
  } catch (e) {
    console.error('Failed to load tabs', e);
  }
}

function startScreencast(tabId) {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = proto + '//' + location.host + '/screencast?tabId=' + tabId + getScreencastParams();
  const socket = new WebSocket(wsUrl);
  socket.binaryType = 'arraybuffer';
  screencastSockets[tabId] = socket;

  const canvas = document.getElementById('canvas-' + tabId);
  if (!canvas) return;
  const ctx2d = canvas.getContext('2d');

  let frameCount = 0;
  let lastFpsTime = Date.now();

  const statusEl = document.getElementById('status-' + tabId);
  const fpsEl = document.getElementById('fps-' + tabId);
  const sizeEl = document.getElementById('size-' + tabId);

  socket.onopen = () => {
    if (statusEl) { statusEl.className = 'tile-status streaming'; }
  };

  socket.onmessage = (evt) => {
    const blob = new Blob([evt.data], { type: 'image/jpeg' });
    const url = URL.createObjectURL(blob);
    const img = new Image();
    img.onload = () => {
      canvas.width = img.width;
      canvas.height = img.height;
      ctx2d.drawImage(img, 0, 0);
      URL.revokeObjectURL(url);
    };
    img.src = url;

    frameCount++;
    const now = Date.now();
    if (now - lastFpsTime >= 1000) {
      if (fpsEl) fpsEl.textContent = frameCount + ' fps';
      if (sizeEl) sizeEl.textContent = (evt.data.byteLength / 1024).toFixed(0) + ' KB/frame';
      frameCount = 0;
      lastFpsTime = now;
    }
  };

  socket.onerror = () => {
    if (statusEl) { statusEl.className = 'tile-status error'; }
  };

  socket.onclose = () => {
    if (statusEl) { statusEl.className = 'tile-status error'; }
  };
}

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
  Object.values(screencastSockets).forEach(s => s.close());
});

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------
const screencastSettings = { fps: 1, quality: 30, maxWidth: 800 };

async function loadSettings() {
  // Load stealth status
  try {
    const res = await fetch('/stealth/status');
    const data = await res.json();
    document.getElementById('set-stealth').value = data.level || 'light';
    updateStealthInfo(data);
  } catch(e) {}

  // Load server health info
  try {
    const res = await fetch('/health');
    const data = await res.json();
    const tabs = await fetch('/tabs').then(r => r.json());
    document.getElementById('server-info').innerHTML = ` + "`" + `
      <div class="info-row"><span class="info-label">Status</span><span class="info-val">${data.status}</span></div>
      <div class="info-row"><span class="info-label">Tabs</span><span class="info-val">${tabs.tabs ? tabs.tabs.length : 0}</span></div>
      <div class="info-row"><span class="info-label">CDP</span><span class="info-val">${data.cdp || 'embedded'}</span></div>
      <div class="info-row"><span class="info-label">Port</span><span class="info-val">${location.port || '80'}</span></div>
    ` + "`" + `;
  } catch(e) {
    document.getElementById('server-info').textContent = 'Failed to load server info';
  }

  // Restore saved settings from localStorage
  const saved = JSON.parse(localStorage.getItem('pinchtab-settings') || '{}');
  if (saved.fps) { document.getElementById('set-fps').value = saved.fps; document.getElementById('fps-val').textContent = saved.fps + ' fps'; screencastSettings.fps = saved.fps; }
  if (saved.quality) { document.getElementById('set-quality').value = saved.quality; document.getElementById('quality-val').textContent = saved.quality + '%'; screencastSettings.quality = saved.quality; }
  if (saved.maxWidth) { document.getElementById('set-maxwidth').value = saved.maxWidth; screencastSettings.maxWidth = saved.maxWidth; }

  // Toggle labels
  document.querySelectorAll('.toggle input').forEach(cb => {
    cb.addEventListener('change', () => {
      cb.parentElement.querySelector('.toggle-label').textContent = cb.checked ? 'On' : 'Off';
    });
  });
}

function updateStealthInfo(data) {
  const el = document.getElementById('stealth-info');
  if (!data || !data.level) { el.textContent = ''; return; }
  const tips = {
    light: 'Patches webdriver, CDP markers, plugins, languages, permissions. Works with X.com and Gmail.',
    full: 'Adds canvas noise, WebGL vendor spoofing, font metrics randomization. May break some sites (e.g. X.com crypto).'
  };
  el.textContent = tips[data.level] || '';
}

function applySettings() {
  screencastSettings.fps = parseInt(document.getElementById('set-fps').value);
  screencastSettings.quality = parseInt(document.getElementById('set-quality').value);
  screencastSettings.maxWidth = parseInt(document.getElementById('set-maxwidth').value);

  // Save to localStorage
  localStorage.setItem('pinchtab-settings', JSON.stringify(screencastSettings));

  // Reconnect all screencasts with new settings
  Object.values(screencastSockets).forEach(s => s.close());
  Object.keys(screencastSockets).forEach(k => delete screencastSockets[k]);

  alert('Settings saved. Switch to Live view to see changes.');
}

function getScreencastParams() {
  return '&quality=' + screencastSettings.quality + '&maxWidth=' + screencastSettings.maxWidth + '&fps=' + screencastSettings.fps;
}

async function applyStealth() {
  // Stealth level change would need a restart — just inform the user
  const level = document.getElementById('set-stealth').value;
  updateStealthInfo({ level });
  alert('Stealth level change requires restarting Pinchtab with BRIDGE_STEALTH=' + level);
}
</script>
</body>
</html>`
