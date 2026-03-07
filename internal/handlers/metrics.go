package handlers

import (
	"runtime"
	"sync/atomic"
)

func recordStaleRefRetry() {
	atomic.AddUint64(&metricStaleRefRetries, 1)
}

// RateBucketHostCount returns the number of unique hosts in rate limit tracking
func RateBucketHostCount() int {
	rateMu.Lock()
	defer rateMu.Unlock()
	return len(rateBuckets)
}

func SnapshotMetrics() map[string]any {
	total := atomic.LoadUint64(&metricRequestsTotal)
	failed := atomic.LoadUint64(&metricRequestsFailed)
	latencySum := atomic.LoadUint64(&metricRequestLatencyN)
	avgMs := 0.0
	if total > 0 {
		avgMs = float64(latencySum) / float64(total)
	}
	rateMu.Lock()
	bucketHosts := len(rateBuckets)
	rateMu.Unlock()

	// Go runtime metrics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]any{
		"requestsTotal":   total,
		"requestsFailed":  failed,
		"panicsRecovered": atomic.LoadUint64(&metricPanicsRecovered),
		"avgLatencyMs":    avgMs,
		"rateLimited":     atomic.LoadUint64(&metricRateLimited),
		"staleRefRetries": atomic.LoadUint64(&metricStaleRefRetries),
		"rateBucketHosts": bucketHosts,
		"goHeapAllocMB":   float64(memStats.HeapAlloc) / (1024 * 1024),
		"goHeapSysMB":     float64(memStats.HeapSys) / (1024 * 1024),
		"goNumGoroutine":  runtime.NumGoroutine(),
		"goHeapObjects":   memStats.HeapObjects,
		"goGCPauseNs":     memStats.PauseNs[(memStats.NumGC+255)%256], // Last GC pause
		"goNumGC":         memStats.NumGC,
	}
}
