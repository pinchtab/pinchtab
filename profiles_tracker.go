package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type ActionTracker struct {
	mu      sync.Mutex
	records map[string][]ActionRecord
}

type ActionRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method"`
	Endpoint   string    `json:"endpoint"`
	URL        string    `json:"url,omitempty"`
	TabID      string    `json:"tabId,omitempty"`
	DurationMs int64     `json:"durationMs"`
	Status     int       `json:"status"`
}

type AnalyticsReport struct {
	TotalActions   int             `json:"totalActions"`
	Since          time.Time       `json:"since"`
	TopEndpoints   []EndpointCount `json:"topEndpoints"`
	RepeatPatterns []RepeatPattern `json:"repeatPatterns"`
	Suggestions    []string        `json:"suggestions"`
}

type EndpointCount struct {
	Endpoint string `json:"endpoint"`
	Count    int    `json:"count"`
	AvgMs    int64  `json:"avgMs"`
}

type RepeatPattern struct {
	Pattern   string  `json:"pattern"`
	Count     int     `json:"count"`
	AvgGapSec float64 `json:"avgGapSec"`
}

func NewActionTracker() *ActionTracker {
	return &ActionTracker{records: make(map[string][]ActionRecord)}
}

func (at *ActionTracker) Record(profile string, rec ActionRecord) {
	at.mu.Lock()
	defer at.mu.Unlock()

	recs := at.records[profile]
	recs = append(recs, rec)
	if len(recs) > 10000 {
		recs = recs[len(recs)-10000:]
	}
	at.records[profile] = recs
}

func (at *ActionTracker) GetLogs(profile string, limit int) []ActionRecord {
	at.mu.Lock()
	defer at.mu.Unlock()

	recs := at.records[profile]
	if limit <= 0 || limit > len(recs) {
		limit = len(recs)
	}

	start := len(recs) - limit
	result := make([]ActionRecord, limit)
	copy(result, recs[start:])
	return result
}

func (at *ActionTracker) Analyze(profile string) AnalyticsReport {
	at.mu.Lock()
	defer at.mu.Unlock()

	recs := at.records[profile]
	if len(recs) == 0 {
		return AnalyticsReport{Suggestions: []string{"No actions recorded yet."}}
	}

	report := AnalyticsReport{
		TotalActions: len(recs),
		Since:        recs[0].Timestamp,
	}

	epCounts := map[string]struct {
		count   int
		totalMs int64
	}{}
	for _, rec := range recs {
		entry := epCounts[rec.Endpoint]
		entry.count++
		entry.totalMs += rec.DurationMs
		epCounts[rec.Endpoint] = entry
	}
	for endpoint, entry := range epCounts {
		report.TopEndpoints = append(report.TopEndpoints, EndpointCount{
			Endpoint: endpoint,
			Count:    entry.count,
			AvgMs:    entry.totalMs / int64(entry.count),
		})
	}
	sort.Slice(report.TopEndpoints, func(i, j int) bool {
		return report.TopEndpoints[i].Count > report.TopEndpoints[j].Count
	})
	if len(report.TopEndpoints) > 10 {
		report.TopEndpoints = report.TopEndpoints[:10]
	}

	urlSnapshots := map[string][]time.Time{}
	for _, rec := range recs {
		if rec.Endpoint == "/snapshot" && rec.URL != "" {
			urlSnapshots[rec.URL] = append(urlSnapshots[rec.URL], rec.Timestamp)
		}
	}
	for url, times := range urlSnapshots {
		if len(times) < 3 {
			continue
		}
		var totalGap float64
		for i := 1; i < len(times); i++ {
			totalGap += times[i].Sub(times[i-1]).Seconds()
		}
		avgGap := totalGap / float64(len(times)-1)
		report.RepeatPatterns = append(report.RepeatPatterns, RepeatPattern{
			Pattern:   fmt.Sprintf("snapshot %s", truncURL(url)),
			Count:     len(times),
			AvgGapSec: avgGap,
		})
	}

	navPattern := map[string]int{}
	for i := 1; i < len(recs); i++ {
		if recs[i-1].Endpoint == "/navigate" && recs[i].Endpoint == "/snapshot" {
			key := truncURL(recs[i-1].URL)
			navPattern[key]++
		}
	}
	for url, count := range navPattern {
		if count >= 3 {
			report.RepeatPatterns = append(report.RepeatPatterns, RepeatPattern{
				Pattern: fmt.Sprintf("navigate→snapshot %s", url),
				Count:   count,
			})
		}
	}

	for _, pattern := range report.RepeatPatterns {
		if strings.HasPrefix(pattern.Pattern, "snapshot") && pattern.AvgGapSec > 0 && pattern.AvgGapSec < 10 {
			report.Suggestions = append(report.Suggestions,
				fmt.Sprintf("High-frequency polling detected: %s every %.0fs — consider increasing interval or using smart diff", pattern.Pattern, pattern.AvgGapSec))
		}
		if strings.HasPrefix(pattern.Pattern, "navigate→snapshot") && pattern.Count > 5 {
			report.Suggestions = append(report.Suggestions,
				fmt.Sprintf("Repeated navigate→snapshot for same URL %s (%dx) — consider caching or using /text for lighter reads", pattern.Pattern, pattern.Count))
		}
	}

	snapCount := 0
	for _, rec := range recs {
		if rec.Endpoint == "/snapshot" {
			snapCount++
		}
	}
	if snapCount > 20 {
		report.Suggestions = append(report.Suggestions,
			fmt.Sprintf("Heavy snapshot usage (%d calls) — use ?selector= or ?maxTokens= to reduce token cost", snapCount))
	}

	if len(report.Suggestions) == 0 {
		report.Suggestions = []string{"No optimization suggestions — usage looks efficient."}
	}

	return report
}

func truncURL(u string) string {
	if len(u) > 60 {
		return u[:57] + "..."
	}
	return u
}
