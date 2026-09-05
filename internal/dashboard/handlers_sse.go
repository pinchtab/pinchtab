package dashboard

import (
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
	flusher, ok := httpx.BeginStream(w)
	if !ok {
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

	stream := httpx.NewEventStream(w, flusher)

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
	_ = stream.Event("init", d.Agents())

	for _, evt := range d.RecentEvents() {
		if !matchesMode(mode, evt.Channel) {
			continue
		}
		if agentFilter != "" && evt.AgentID != agentFilter {
			continue
		}
		d.emitActivityEvent(stream, evt)
	}

	d.emitMonitoring(stream, includeMemory)

	keepalive := time.NewTicker(30 * time.Second)
	monitoring := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	defer monitoring.Stop()

	for {
		select {
		case evt := <-activityCh:
			if matchesMode(mode, evt.Channel) && (agentFilter == "" || evt.AgentID == agentFilter) {
				d.emitActivityEvent(stream, evt)
			}
		case evt := <-sysCh:
			_ = stream.Event("system", evt)
			d.emitMonitoring(stream, includeMemory)
		case <-monitoring.C:
			d.emitMonitoring(stream, includeMemory)
		case <-keepalive.C:
			_ = stream.Keepalive()
		case <-r.Context().Done():
			return
		}
	}
}

// emitMonitoring sends a monitoring snapshot when a monitoring/instances source
// exists, reusing a shared TTL-cached marshaled payload so concurrent connections
// do not each recompute + marshal the full snapshot.
func (d *Dashboard) emitMonitoring(stream *httpx.EventStream, includeMemory bool) {
	if d.monitoring != nil || d.instances != nil {
		_ = stream.Raw("monitoring", d.monitoringPayloadBytes(includeMemory))
	}
}

func (d *Dashboard) emitActivityEvent(stream *httpx.EventStream, evt apiTypes.ActivityEvent) {
	name := "action"
	if evt.Channel == "progress" {
		name = "progress"
	}
	_ = stream.Event(name, evt)
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
