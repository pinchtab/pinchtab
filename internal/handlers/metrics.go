package handlers

import "sync/atomic"

func recordStaleRefRetry() {
	atomic.AddUint64(&metricStaleRefRetries, 1)
}

func snapshotMetrics() map[string]any {
	total := atomic.LoadUint64(&metricRequestsTotal)
	failed := atomic.LoadUint64(&metricRequestsFailed)
	latencySum := atomic.LoadUint64(&metricRequestLatencyN)
	avgMs := 0.0
	if total > 0 {
		avgMs = float64(latencySum) / float64(total)
	}
	return map[string]any{
		"requestsTotal":   total,
		"requestsFailed":  failed,
		"avgLatencyMs":    avgMs,
		"rateLimited":     atomic.LoadUint64(&metricRateLimited),
		"staleRefRetries": atomic.LoadUint64(&metricStaleRefRetries),
	}
}
