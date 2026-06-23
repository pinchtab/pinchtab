package dashboard

import (
	"encoding/json"
	"time"

	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

// monitoringCacheTTL bounds how long a marshaled monitoring snapshot is reused
// across SSE connections. Well under the 5s emit interval, so staleness stays
// within the polled monitoring view's tolerance while concurrent emits (connect
// storms, a system event fanning out to every connection) share one compute.
const monitoringCacheTTL = 1 * time.Second

type monitoringPayload struct {
	data []byte
	at   time.Time
}

type MonitoringSource interface {
	List() []bridge.Instance
	AllTabs() []bridge.InstanceTab
	AllMetrics() []apiTypes.InstanceMetrics
}

type MonitoringServerMetrics struct {
	GoHeapAllocMB   float64 `json:"goHeapAllocMB"`
	GoNumGoroutine  int     `json:"goNumGoroutine"`
	RateBucketHosts int     `json:"rateBucketHosts"`
}

type MonitoringSnapshot struct {
	Timestamp     int64                      `json:"timestamp"`
	Instances     []bridge.Instance          `json:"instances"`
	Tabs          []bridge.InstanceTab       `json:"tabs"`
	Metrics       []apiTypes.InstanceMetrics `json:"metrics"`
	ServerMetrics MonitoringServerMetrics    `json:"serverMetrics"`
}

type ServerMetricsProvider func() MonitoringServerMetrics

func (d *Dashboard) SetMonitoringSource(src MonitoringSource) {
	d.monitoring = src
	if src != nil {
		d.instances = src
	}
}

func (d *Dashboard) SetServerMetricsProvider(provider ServerMetricsProvider) {
	d.serverMetrics = provider
}

// monitoringPayloadBytes returns the marshaled monitoring snapshot for the given
// includeMemory, computing and caching it at most once per monitoringCacheTTL so
// concurrent SSE emits share one List/AllTabs/AllMetrics + marshal instead of
// each recomputing. Holding monCacheMu across the compute is intentional: a
// second emitter blocks briefly, then reuses the just-computed payload.
func (d *Dashboard) monitoringPayloadBytes(includeMemory bool) []byte {
	d.monCacheMu.Lock()
	defer d.monCacheMu.Unlock()
	now := d.now()
	if e, ok := d.monCache[includeMemory]; ok && now.Sub(e.at) < monitoringCacheTTL {
		return e.data
	}
	data, _ := json.Marshal(d.monitoringSnapshot(includeMemory))
	d.monCache[includeMemory] = monitoringPayload{data: data, at: now}
	return data
}

func (d *Dashboard) monitoringSnapshot(includeMemory bool) MonitoringSnapshot {
	snapshot := MonitoringSnapshot{
		Timestamp: d.now().UnixMilli(),
		Instances: []bridge.Instance{},
		Tabs:      []bridge.InstanceTab{},
		Metrics:   []apiTypes.InstanceMetrics{},
	}

	if d.monitoring != nil {
		snapshot.Instances = d.monitoring.List()
		snapshot.Tabs = d.monitoring.AllTabs()
		if includeMemory {
			snapshot.Metrics = d.monitoring.AllMetrics()
		}
	} else if d.instances != nil {
		snapshot.Instances = d.instances.List()
	}

	if d.serverMetrics != nil {
		snapshot.ServerMetrics = d.serverMetrics()
	}

	return snapshot
}
