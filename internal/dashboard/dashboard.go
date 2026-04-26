package dashboard

import (
	"context"
	"embed"
	"os"
	"sort"
	"sync"
	"time"

	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

func envWithFallback(newKey, oldKey string) string {
	if v := os.Getenv(newKey); v != "" {
		return v
	}
	return os.Getenv(oldKey)
}

type DashboardConfig struct {
	IdleTimeout       time.Duration
	DisconnectTimeout time.Duration
	ReaperInterval    time.Duration
	SSEBufferSize     int
}

//go:embed dashboard/*
var dashboardFS embed.FS

// SystemEvent is sent for instance lifecycle changes.
type SystemEvent struct {
	Type     string      `json:"type"` // "instance.started", "instance.stopped", "instance.error"
	Instance interface{} `json:"instance,omitempty"`
}

// InstanceLister returns running instances (provided by Orchestrator).
type InstanceLister interface {
	List() []bridge.Instance
}

type Dashboard struct {
	cfg            DashboardConfig
	activityConns  map[chan apiTypes.ActivityEvent]struct{}
	sysConns       map[chan SystemEvent]struct{}
	cancel         context.CancelFunc
	instances      InstanceLister
	monitoring     MonitoringSource
	serverMetrics  ServerMetricsProvider
	childAuthToken string

	agents       map[string]*apiTypes.Agent
	recentEvents []apiTypes.ActivityEvent
	maxEvents    int
	seenEventIDs map[string]struct{}
	seenEventLog []string
	maxSeenIDs   int

	mu sync.RWMutex
}

func NewDashboard(cfg *DashboardConfig) *Dashboard {
	c := DashboardConfig{
		IdleTimeout:       30 * time.Second,
		DisconnectTimeout: 5 * time.Minute,
		ReaperInterval:    10 * time.Second,
		SSEBufferSize:     64,
	}
	if cfg != nil {
		if cfg.IdleTimeout > 0 {
			c.IdleTimeout = cfg.IdleTimeout
		}
		if cfg.DisconnectTimeout > 0 {
			c.DisconnectTimeout = cfg.DisconnectTimeout
		}
		if cfg.ReaperInterval > 0 {
			c.ReaperInterval = cfg.ReaperInterval
		}
		if cfg.SSEBufferSize > 0 {
			c.SSEBufferSize = cfg.SSEBufferSize
		}
	}

	_, cancel := context.WithCancel(context.Background())
	return &Dashboard{
		cfg:            c,
		activityConns:  make(map[chan apiTypes.ActivityEvent]struct{}),
		sysConns:       make(map[chan SystemEvent]struct{}),
		cancel:         cancel,
		childAuthToken: envWithFallback("PINCHTAB_TOKEN", "BRIDGE_TOKEN"),
		agents:         make(map[string]*apiTypes.Agent),
		recentEvents:   make([]apiTypes.ActivityEvent, 0, 200),
		maxEvents:      200,
		seenEventIDs:   make(map[string]struct{}),
		seenEventLog:   make([]string, 0, 2000),
		maxSeenIDs:     2000,
	}
}

func (d *Dashboard) Shutdown() { d.cancel() }

// BroadcastSystemEvent sends a system event to all SSE clients.
func (d *Dashboard) BroadcastSystemEvent(evt SystemEvent) {
	d.mu.RLock()
	chans := make([]chan SystemEvent, 0, len(d.sysConns))
	for ch := range d.sysConns {
		chans = append(chans, ch)
	}
	d.mu.RUnlock()

	for _, ch := range chans {
		select {
		case ch <- evt:
		default:
		}
	}
}

// SetInstanceLister sets the orchestrator for managing instances.
func (d *Dashboard) SetInstanceLister(il InstanceLister) {
	d.instances = il
}

// Agents returns the current agent summary list ordered by most recent activity.
func (d *Dashboard) Agents() []apiTypes.Agent {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]apiTypes.Agent, 0, len(d.agents))
	for _, agent := range d.agents {
		result = append(result, *agent)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastActivity.After(result[j].LastActivity)
	})
	return result
}

// Agent returns the current summary for a single observed agent.
func (d *Dashboard) Agent(agentID string) (apiTypes.Agent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	agent, ok := d.agents[agentIDOrAnonymous(agentID)]
	if !ok {
		return apiTypes.Agent{}, false
	}
	return *agent, true
}

// AgentCount returns the number of currently observed agents.
func (d *Dashboard) AgentCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.agents)
}

// RecentEvents returns a copy of the buffered live event history.
func (d *Dashboard) RecentEvents() []apiTypes.ActivityEvent {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]apiTypes.ActivityEvent, len(d.recentEvents))
	copy(out, d.recentEvents)
	return out
}

// EventsForAgent returns buffered events for a single agent filtered by mode.
func (d *Dashboard) EventsForAgent(agentID, mode string) []apiTypes.ActivityEvent {
	agentID = agentIDOrAnonymous(agentID)
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]apiTypes.ActivityEvent, 0, len(d.recentEvents))
	for _, evt := range d.recentEvents {
		if evt.AgentID != agentID || !matchesMode(mode, evt.Channel) {
			continue
		}
		out = append(out, evt)
	}
	return out
}
