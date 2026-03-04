package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

type DashboardConfig struct {
	IdleTimeout       time.Duration
	DisconnectTimeout time.Duration
	ReaperInterval    time.Duration
	SSEBufferSize     int
}

//go:embed dashboard/*
var dashboardFS embed.FS

// AgentEvent is sent via SSE when an agent performs an action.
type AgentEvent struct {
	AgentID    string    `json:"agentId"`
	Profile    string    `json:"profile,omitempty"`
	Action     string    `json:"action"`
	URL        string    `json:"url,omitempty"`
	TabID      string    `json:"tabId,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	Status     int       `json:"status"`
	DurationMs int64     `json:"durationMs"`
	Timestamp  time.Time `json:"timestamp"`
}

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
	sseConns       map[chan AgentEvent]struct{}
	sysConns       map[chan SystemEvent]struct{}
	cancel         context.CancelFunc
	instances      InstanceLister
	childAuthToken string
	mu             sync.RWMutex
}

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
	d := &Dashboard{
		cfg:            c,
		sseConns:       make(map[chan AgentEvent]struct{}),
		sysConns:       make(map[chan SystemEvent]struct{}),
		cancel:         cancel,
		childAuthToken: os.Getenv("BRIDGE_TOKEN"),
	}
	return d
}

func (d *Dashboard) Shutdown() { d.cancel() }

func (d *Dashboard) RegisterHandlers(mux *http.ServeMux) {
	// API endpoints
	mux.HandleFunc("GET /api/events", d.handleSSE)

	// Static files served at /dashboard/
	sub, _ := fs.Sub(dashboardFS, "dashboard")
	fileServer := http.FileServer(http.FS(sub))

	// Serve static assets under /dashboard/ with long cache (hashed filenames)
	mux.Handle("GET /dashboard/assets/", http.StripPrefix("/dashboard", d.withLongCache(fileServer)))
	mux.Handle("GET /dashboard/favicon.png", http.StripPrefix("/dashboard", d.withLongCache(fileServer)))

	// SPA: serve dashboard.html for /dashboard
	mux.Handle("GET /dashboard", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
	mux.Handle("GET /dashboard/", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
}

func (d *Dashboard) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	agentCh := make(chan AgentEvent, d.cfg.SSEBufferSize)
	sysCh := make(chan SystemEvent, d.cfg.SSEBufferSize)
	d.mu.Lock()
	d.sseConns[agentCh] = struct{}{}
	d.sysConns[sysCh] = struct{}{}
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.sseConns, agentCh)
		delete(d.sysConns, sysCh)
		d.mu.Unlock()
	}()

	// Send initial empty agent list
	data, _ := json.Marshal([]interface{}{})
	_, _ = fmt.Fprintf(w, "event: init\ndata: %s\n\n", data)
	flusher.Flush()

	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case evt := <-agentCh:
			data, _ := json.Marshal(evt)
			_, _ = fmt.Fprintf(w, "event: action\ndata: %s\n\n", data)
			flusher.Flush()
		case evt := <-sysCh:
			data, _ := json.Marshal(evt)
			_, _ = fmt.Fprintf(w, "event: system\ndata: %s\n\n", data)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (d *Dashboard) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	data, _ := dashboardFS.ReadFile("dashboard/dashboard.html")
	_, _ = w.Write(data)
}

func (d *Dashboard) withNoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func (d *Dashboard) withLongCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assets have hashes in filenames - cache for 1 year
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}
