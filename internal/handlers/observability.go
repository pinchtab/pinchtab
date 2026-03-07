package handlers

import (
	"sync"
	"sync/atomic"
	"time"
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

// RecordRecoveredPanic increments recovered panic monitoring for bridge HTTP handlers.
func RecordRecoveredPanic() {
	atomic.AddUint64(&metricPanicsRecovered, 1)
}

// FailureSnapshot returns recent failure diagnostics for /health and /metrics.
func FailureSnapshot() map[string]any {
	failureMu.Lock()
	recent := append([]FailureEvent(nil), recentFailures...)
	failureMu.Unlock()

	return map[string]any{
		"requestsFailed":  atomic.LoadUint64(&metricRequestsFailed),
		"panicsRecovered": atomic.LoadUint64(&metricPanicsRecovered),
		"recent":          recent,
	}
}

func hasFailureDiagnostics() bool {
	failureMu.Lock()
	recentCount := len(recentFailures)
	failureMu.Unlock()
	return atomic.LoadUint64(&metricRequestsFailed) > 0 ||
		atomic.LoadUint64(&metricPanicsRecovered) > 0 ||
		recentCount > 0
}

func resetObservabilityForTests() {
	atomic.StoreUint64(&metricRequestsTotal, 0)
	atomic.StoreUint64(&metricRequestsFailed, 0)
	atomic.StoreUint64(&metricRequestLatencyN, 0)
	atomic.StoreUint64(&metricRateLimited, 0)
	atomic.StoreUint64(&metricStaleRefRetries, 0)
	atomic.StoreUint64(&metricPanicsRecovered, 0)
	failureMu.Lock()
	recentFailures = nil
	failureMu.Unlock()
}
