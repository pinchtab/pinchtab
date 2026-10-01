package handlers

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const maxRecentFailures = 20

// FailureEvent records a recent HTTP failure for bridge-side diagnostics.
type FailureEvent struct {
	Time      time.Time `json:"time"`
	RequestID string    `json:"requestId,omitempty"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Type      string    `json:"type"`
	// Code and Message are the reason the request failed, taken from the producer that
	// wrote the response rather than parsed back out of it. Type stays the constant
	// "http_error": it names the KIND of record, and never said anything about this one.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

var (
	failureMu      sync.Mutex
	recentFailures []FailureEvent
)

func recordFailureEvent(ev FailureEvent) {
	failureMu.Lock()
	defer failureMu.Unlock()

	recentFailures = append(recentFailures, ev)
	if len(recentFailures) > maxRecentFailures {
		recentFailures = append([]FailureEvent(nil), recentFailures[len(recentFailures)-maxRecentFailures:]...)
	}
}

// Layer names the process a set of counters describes. Server mode runs TWO
// counter sets in two processes — the orchestrator front door, which is the only
// one that sees auth rejections and unrouted paths, and each browser-control
// instance — so every number a client reads has to say which one it came from.
// They are never summed: one requestsTotal meaning two layers is a number an
// operator cannot act on.
const (
	LayerFrontDoor = "frontDoor"
	LayerInstance  = "instance"
)

// FailureSnapshot returns recent failure diagnostics for /health and /metrics,
// labelled with the layer that recorded them. The label is repeated on each event
// as well as on the block, so an event stays self-describing when it is copied
// out of the response it arrived in.
func FailureSnapshot(layer string) map[string]any {
	failureMu.Lock()
	recent := append([]FailureEvent(nil), recentFailures...)
	failureMu.Unlock()

	events := make([]map[string]any, 0, len(recent))
	for _, ev := range recent {
		event := map[string]any{
			"time":      ev.Time,
			"requestId": ev.RequestID,
			"method":    ev.Method,
			"path":      ev.Path,
			"status":    ev.Status,
			"type":      ev.Type,
			"layer":     layer,
		}
		if ev.Message != "" {
			event["code"] = ev.Code
			event["message"] = ev.Message
		}
		events = append(events, event)
	}

	return map[string]any{
		"layer":          layer,
		"requestsFailed": atomic.LoadUint64(&metricRequestsFailed),
		"recent":         events,
	}
}

// DiagnosticsSnapshot is the whole observability payload for one process: its own
// counters plus the failure and crash logs it recorded. Both modes serve it, so
// the two cannot drift into exposing different keys — the divergence this closes
// was exactly that.
func DiagnosticsSnapshot(layer string) map[string]any {
	snapshot := map[string]any{
		"layer":   layer,
		"metrics": SnapshotMetrics(),
	}
	snapshot["failures"] = FailureSnapshot(layer)
	snapshot["crashes"] = bridge.CrashSnapshot()
	return snapshot
}

func hasFailureDiagnostics() bool {
	failureMu.Lock()
	recentCount := len(recentFailures)
	failureMu.Unlock()
	return atomic.LoadUint64(&metricRequestsFailed) > 0 || recentCount > 0
}

// ResetObservabilityForTests clears the process counters and failure log. It is
// exported because the front-door chain lives in internal/server, and the
// counters it must be proven to move are owned here.
func ResetObservabilityForTests() {
	atomic.StoreUint64(&metricRequestsTotal, 0)
	atomic.StoreUint64(&metricRequestsFailed, 0)
	atomic.StoreUint64(&metricRequestLatencyN, 0)
	atomic.StoreUint64(&metricRateLimited, 0)
	atomic.StoreUint64(&metricStaleRefRetries, 0)
	failureMu.Lock()
	recentFailures = nil
	failureMu.Unlock()
}

// writeUnavailable answers the {status,reason} shape that /health and /stealth/status
// clients parse by key, and records the reason for the failure sinks with it. The shape
// is a client contract, which is why these do not become ordinary error envelopes; the
// reason travels either way.
func writeUnavailable(w http.ResponseWriter, status int, code, reason string) {
	httpx.JSONError(w, status, code, reason, map[string]any{"status": "error", "reason": reason})
}
