package handlers

import (
	"runtime"
)

func recordStaleRefRetry() {
	metricStaleRefRetries.Add(1)
}

func RateBucketHostCount() int {
	return requestLimiter.TrackedKeys()
}

func SnapshotMetrics() map[string]any {
	total := metricRequestsTotal.Load()
	failed := metricRequestsFailed.Load()
	latencySum := metricRequestLatencyN.Load()
	avgMs := 0.0
	if total > 0 {
		avgMs = float64(latencySum) / float64(total)
	}
	bucketHosts := RateBucketHostCount()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]any{
		"requestsTotal":   total,
		"requestsFailed":  failed,
		"avgLatencyMs":    avgMs,
		"rateLimited":     metricRateLimited.Load(),
		"staleRefRetries": metricStaleRefRetries.Load(),
		"rateBucketHosts": bucketHosts,
		"goHeapAllocMB":   float64(memStats.HeapAlloc) / (1024 * 1024),
		"goHeapSysMB":     float64(memStats.HeapSys) / (1024 * 1024),
		"goNumGoroutine":  runtime.NumGoroutine(),
		"goHeapObjects":   memStats.HeapObjects,
		"goGCPauseNs":     memStats.PauseNs[(memStats.NumGC+255)%256], // Last GC pause
		"goNumGC":         memStats.NumGC,
	}
}
