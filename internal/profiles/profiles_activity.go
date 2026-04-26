package profiles

import (
	"net/url"
	"sort"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

func (pm *ProfileManager) RecordAction(profile string, record bridge.ActionRecord) {
	_ = profile
	_ = record
}

func (pm *ProfileManager) Logs(name string, limit int) []bridge.ActionRecord {
	return pm.logsFromActivity(name, limit)
}

func (pm *ProfileManager) Analytics(name string) bridge.AnalyticsReport {
	return analyticsFromActionRecords(pm.logsFromActivity(name, 1000))
}

func (pm *ProfileManager) logsFromActivity(name string, limit int) []bridge.ActionRecord {
	pm.mu.RLock()
	rec := pm.activity
	pm.mu.RUnlock()
	if rec == nil || !rec.Enabled() {
		return []bridge.ActionRecord{}
	}

	events, err := rec.Query(activity.Filter{
		ProfileName: name,
		Limit:       limit,
	})
	if err != nil {
		return []bridge.ActionRecord{}
	}

	logs := make([]bridge.ActionRecord, 0, len(events))
	for _, evt := range events {
		logs = append(logs, bridge.ActionRecord{
			Timestamp:  evt.Timestamp,
			Method:     evt.Method,
			Endpoint:   evt.Path,
			URL:        evt.URL,
			TabID:      evt.TabID,
			DurationMs: evt.DurationMs,
			Status:     evt.Status,
		})
	}
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp.Before(logs[j].Timestamp)
	})
	if limit > 0 && len(logs) > limit {
		return logs[len(logs)-limit:]
	}
	return logs
}

func analyticsFromActionRecords(logs []bridge.ActionRecord) bridge.AnalyticsReport {
	report := bridge.AnalyticsReport{
		TotalActions: len(logs),
		CommonHosts:  make(map[string]int),
		TopEndpoints: make(map[string]int),
	}

	last24h := time.Now().Add(-24 * time.Hour)
	for _, l := range logs {
		if l.Timestamp.After(last24h) {
			report.Last24h++
		}
		if l.URL != "" {
			u, err := url.Parse(l.URL)
			if err == nil && u.Host != "" {
				report.CommonHosts[u.Host]++
			}
		}
		if l.Endpoint != "" {
			report.TopEndpoints[l.Endpoint]++
		}
	}
	if len(report.TopEndpoints) == 0 {
		report.TopEndpoints = nil
	}
	if len(report.CommonHosts) == 0 {
		report.CommonHosts = map[string]int{}
	}
	return report
}
