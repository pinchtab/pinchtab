package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (d *Dashboard) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/events", d.handleSSE)
	mux.HandleFunc("GET /api/agents", d.handleAgents)
	mux.HandleFunc("GET /api/agents/{id}", d.handleAgent)
	mux.HandleFunc("GET /api/agents/{id}/events", d.handleAgentSSE)
	mux.HandleFunc("POST /api/agents/{id}/events", d.handleAgentEventsByID)

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
		httpx.Problem(w, http.StatusInternalServerError, "streaming_not_supported", "streaming not supported", false, nil)
		return
	}

	// SSE connections are intentionally long-lived. Clear the server-level write
	// deadline for this response so the stream is not terminated after
	// http.Server.WriteTimeout elapses.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		httpx.Problem(w, http.StatusInternalServerError, "streaming_deadline_unsupported", "streaming deadline unsupported", false, nil)
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
