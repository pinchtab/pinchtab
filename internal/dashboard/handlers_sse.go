package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

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
	emitSSE(w, flusher, "init", d.Agents())

	for _, evt := range d.RecentEvents() {
		if !matchesMode(mode, evt.Channel) {
			continue
		}
		if agentFilter != "" && evt.AgentID != agentFilter {
			continue
		}
		d.emitActivityEvent(w, flusher, evt)
	}

	d.emitMonitoring(w, flusher, includeMemory)

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
			emitSSE(w, flusher, "system", evt)
			d.emitMonitoring(w, flusher, includeMemory)
		case <-monitoring.C:
			d.emitMonitoring(w, flusher, includeMemory)
		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// emitSSE marshals payload and writes one SSE frame, then flushes.
func emitSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload any) {
	data, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	flusher.Flush()
}

// emitMonitoring sends a monitoring snapshot when a monitoring/instances source
// exists, reusing a shared TTL-cached marshaled payload so concurrent connections
// do not each recompute + marshal the full snapshot.
func (d *Dashboard) emitMonitoring(w http.ResponseWriter, flusher http.Flusher, includeMemory bool) {
	if d.monitoring != nil || d.instances != nil {
		data := d.monitoringPayloadBytes(includeMemory)
		_, _ = fmt.Fprintf(w, "event: monitoring\ndata: %s\n\n", data)
		flusher.Flush()
	}
}

func (d *Dashboard) emitActivityEvent(w http.ResponseWriter, flusher http.Flusher, evt apiTypes.ActivityEvent) {
	name := "action"
	if evt.Channel == "progress" {
		name = "progress"
	}
	emitSSE(w, flusher, name, evt)
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
