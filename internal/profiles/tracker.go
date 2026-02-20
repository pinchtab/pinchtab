package profiles

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

type ActionTracker struct {
	logs map[string][]bridge.ActionRecord
	mu   sync.RWMutex
}

func NewActionTracker() *ActionTracker {
	t := &ActionTracker{
		logs: make(map[string][]bridge.ActionRecord),
	}
	_ = t.load()
	return t
}

func (t *ActionTracker) Record(profile string, record bridge.ActionRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.logs[profile] = append(t.logs[profile], record)
	if len(t.logs[profile]) > 1000 {
		t.logs[profile] = t.logs[profile][len(t.logs[profile])-1000:]
	}
	_ = t.save()
}

func (t *ActionTracker) GetLogs(profile string, limit int) []bridge.ActionRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()
	logs := t.logs[profile]
	if limit > 0 && len(logs) > limit {
		return logs[len(logs)-limit:]
	}
	return logs
}

func (t *ActionTracker) Analyze(profile string) bridge.AnalyticsReport {
	t.mu.RLock()
	defer t.mu.RUnlock()
	logs := t.logs[profile]

	report := bridge.AnalyticsReport{
		TotalActions: len(logs),
		CommonHosts:  make(map[string]int),
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
	}
	return report
}

func (t *ActionTracker) save() error {
	h, _ := os.UserHomeDir()
	path := filepath.Join(h, ".pinchtab", "action_logs.json")
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	data, err := json.Marshal(t.logs)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (t *ActionTracker) load() error {
	h, _ := os.UserHomeDir()
	path := filepath.Join(h, ".pinchtab", "action_logs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &t.logs)
}
