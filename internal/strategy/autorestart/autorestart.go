// Package autorestart implements the "simple-autorestart" allocation strategy.
//
// It behaves like the "simple" strategy (single instance, shorthand proxy)
// but adds automatic crash recovery: if the managed Chrome instance exits
// unexpectedly, the strategy re-launches it with exponential backoff.
//
// Configuration is done via AutorestartConfig passed to WithConfig, or
// via defaults (3 max restarts, 2s initial backoff, 5 min stable period).
package autorestart

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
	"github.com/pinchtab/pinchtab/internal/strategy"
)

const (
	defaultMaxRestarts  = 3
	defaultInitBackoff  = 2 * time.Second
	defaultMaxBackoff   = 60 * time.Second
	defaultStableAfter  = 5 * time.Minute
	defaultProfileName  = "default"
	defaultStrategyName = "simple-autorestart"
	defaultStatusPath   = "/autorestart/status"
	healthPollInterval  = 500 * time.Millisecond
	healthPollTimeout   = 30 * time.Second
)

func init() {
	// simple-autorestart uses autorestart defaults (MaxRestarts=3).
	strategy.MustRegister("simple-autorestart", func() strategy.Strategy {
		return New(AutorestartConfig{})
	})
}

// AutorestartConfig configures the autorestart behavior.
type AutorestartConfig struct {
	MaxRestarts  int           // Max consecutive restarts before giving up (0 = use default 3, <0 = unlimited)
	InitBackoff  time.Duration // Initial backoff between restarts (0 = use default 2s)
	MaxBackoff   time.Duration // Maximum backoff cap (0 = use default 60s)
	StableAfter  time.Duration // Reset counter after running this long (0 = use default 5m)
	ProfileName  string        // Profile to launch (empty = "default")
	Headless     bool          // Chrome headless mode
	HeadlessSet  bool          // Whether Headless was explicitly set (false = use default true)
	StrategyName string        // Exposed strategy identifier (empty = "simple-autorestart")
	StatusPath   string        // Status endpoint path (empty = "/autorestart/status")
}

// RestartState tracks the restart state of the managed instance.
type RestartState struct {
	InstanceID   string    `json:"instanceId"`
	RestartCount int       `json:"restartCount"`
	MaxRestarts  int       `json:"maxRestarts"`
	LastCrash    time.Time `json:"lastCrash,omitempty"`
	LastStart    time.Time `json:"lastStart"`
	Status       string    `json:"status"` // "running", "restarting", "crashed", "stopped"
}

// Strategy monitors a single Chrome instance and auto-restarts on crash.
type Strategy struct {
	orch   *orchestrator.Orchestrator
	config AutorestartConfig

	mu           sync.Mutex
	instanceID   string    // Currently managed instance ID
	restartCount int       // Consecutive restart count
	lastCrash    time.Time // Last crash timestamp
	lastStart    time.Time // Last successful start timestamp
	deliberate   bool      // True if stop was deliberate (not a crash)
	restarting   bool      // True while a restart is in progress (prevents re-entrancy)
	ctx          context.Context
	cancel       context.CancelFunc
}

// New creates a new autorestart strategy with the given config.
func New(cfg AutorestartConfig) *Strategy {
	if cfg.MaxRestarts == 0 {
		cfg.MaxRestarts = defaultMaxRestarts
	}
	if cfg.InitBackoff <= 0 {
		cfg.InitBackoff = defaultInitBackoff
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = defaultMaxBackoff
	}
	if cfg.StableAfter <= 0 {
		cfg.StableAfter = defaultStableAfter
	}
	if cfg.ProfileName == "" {
		cfg.ProfileName = defaultProfileName
	}
	if !cfg.HeadlessSet {
		cfg.Headless = true
	}
	if cfg.StrategyName == "" {
		cfg.StrategyName = defaultStrategyName
	}
	if cfg.StatusPath == "" {
		cfg.StatusPath = defaultStatusPath
	}

	return &Strategy{
		config: cfg,
	}
}

func (s *Strategy) Name() string { return s.config.StrategyName }

func (s *Strategy) SetRuntimeConfig(cfg *config.RuntimeConfig) {
	if cfg == nil {
		return
	}
	if cfg.RestartMaxRestarts != 0 {
		s.config.MaxRestarts = cfg.RestartMaxRestarts
	}
	if cfg.RestartInitBackoff > 0 {
		s.config.InitBackoff = cfg.RestartInitBackoff
	}
	if cfg.RestartMaxBackoff > 0 {
		s.config.MaxBackoff = cfg.RestartMaxBackoff
	}
	if cfg.RestartStableAfter > 0 {
		s.config.StableAfter = cfg.RestartStableAfter
	}
}

// SetOrchestrator injects the orchestrator after construction.
func (s *Strategy) SetOrchestrator(o *orchestrator.Orchestrator) {
	s.orch = o
}

// Start begins the autorestart lifecycle: launches an initial instance
// and subscribes to orchestrator events for crash detection.
func (s *Strategy) Start(ctx context.Context) error {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	// Subscribe to orchestrator events for crash detection.
	s.orch.OnEvent(func(evt orchestrator.InstanceEvent) {
		s.handleEvent(evt)
	})

	// Launch the initial instance.
	go s.launchInitial()

	// Start the stability checker.
	go s.stabilityLoop()

	return nil
}

// Stop gracefully shuts down the strategy.
func (s *Strategy) Stop() error {
	s.mu.Lock()
	s.deliberate = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	return nil
}

// RegisterRoutes adds shorthand endpoints that proxy to the managed instance.
func (s *Strategy) RegisterRoutes(mux *http.ServeMux) {
	s.orch.RegisterHandlers(mux)

	shorthandRoutes := []string{
		"GET /snapshot", "GET /screenshot", "GET /text", "GET /pdf", "POST /pdf",
		"GET /console", "POST /console/clear",
		"GET /errors", "POST /errors/clear",
		"GET /clipboard/read", "POST /clipboard/write", "POST /clipboard/copy", "GET /clipboard/paste",
		"POST /navigate", "POST /back", "POST /forward", "POST /reload",
		"POST /action", "POST /actions",
		"POST /tab", "POST /tab/lock", "POST /tab/unlock",
		"GET /cookies", "POST /cookies",
		"GET /stealth/status", "POST /fingerprint/rotate",
		"POST /find",
	}
	for _, route := range shorthandRoutes {
		mux.HandleFunc(route, s.proxyToManaged)
	}
	strategy.RegisterCapabilityRoute(mux, "POST /evaluate", s.orch.AllowsEvaluate(), "evaluate", "security.allowEvaluate", "evaluate_disabled", s.proxyToManaged)
	strategy.RegisterCapabilityRoute(mux, "GET /download", s.orch.AllowsDownload(), "download", "security.allowDownload", "download_disabled", s.proxyToManaged)
	strategy.RegisterCapabilityRoute(mux, "POST /upload", s.orch.AllowsUpload(), "upload", "security.allowUpload", "upload_disabled", s.proxyToManaged)
	strategy.RegisterCapabilityRoute(mux, "GET /screencast", s.orch.AllowsScreencast(), "screencast", "security.allowScreencast", "screencast_disabled", s.proxyToManaged)
	strategy.RegisterCapabilityRoute(mux, "GET /screencast/tabs", s.orch.AllowsScreencast(), "screencast", "security.allowScreencast", "screencast_disabled", s.proxyToManaged)
	strategy.RegisterCapabilityRoute(mux, "POST /macro", s.orch.AllowsMacro(), "macro", "security.allowMacro", "macro_disabled", s.proxyToManaged)

	mux.HandleFunc("GET /tabs", s.handleTabs)
	mux.HandleFunc("GET "+s.config.StatusPath, s.handleStatus)
}

// State returns the current restart state for observability.
func (s *Strategy) State() RestartState {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := "running"
	if s.hasRestartLimit() && s.restartCount >= s.config.MaxRestarts {
		status = "crashed"
	} else if s.restarting {
		status = "restarting"
	} else if s.instanceID == "" {
		status = "starting"
	}

	return RestartState{
		InstanceID:   s.instanceID,
		RestartCount: s.restartCount,
		MaxRestarts:  s.config.MaxRestarts,
		LastCrash:    s.lastCrash,
		LastStart:    s.lastStart,
		Status:       status,
	}
}

// --- Internal ---

// launchInitial launches the first instance (or after strategy start).
func (s *Strategy) launchInitial() {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()

	if ctx == nil || ctx.Err() != nil {
		return
	}

	inst, err := s.orch.Launch(s.config.ProfileName, "", s.config.Headless, nil)
	if err != nil {
		slog.Error(s.logPrefix("initial launch failed"), "profile", s.config.ProfileName, "err", err)
		return
	}

	s.mu.Lock()
	s.instanceID = inst.ID
	s.lastStart = time.Now()
	s.mu.Unlock()

	slog.Info(s.logPrefix("instance launched"), "id", inst.ID, "profile", s.config.ProfileName)
}

// handleEvent processes orchestrator lifecycle events.
func (s *Strategy) handleEvent(evt orchestrator.InstanceEvent) {
	s.mu.Lock()
	managedID := s.instanceID
	deliberate := s.deliberate
	restarting := s.restarting
	ctx := s.ctx
	s.mu.Unlock()

	// Only handle events for the managed instance.
	if evt.Instance == nil || evt.Instance.ID != managedID {
		return
	}

	// Skip if a restart is already in progress (prevents duplicate handling
	// when both instance.error and instance.stopped fire for the same crash).
	if restarting {
		return
	}

	switch evt.Type {
	case "instance.stopped":
		if deliberate {
			slog.Info(s.logPrefix("instance stopped deliberately"), "id", managedID)
			return
		}
		// Instance exited unexpectedly — check if we should restart.
		s.handleCrash(ctx, managedID)

	case "instance.error":
		if deliberate {
			return
		}
		s.handleCrash(ctx, managedID)
	}
}

// handleCrash decides whether to restart the crashed instance.
func (s *Strategy) handleCrash(ctx context.Context, crashedID string) {
	if ctx == nil || ctx.Err() != nil {
		return
	}

	s.mu.Lock()
	s.restarting = true
	s.restartCount++
	s.lastCrash = time.Now()
	count := s.restartCount
	maxRestarts := s.config.MaxRestarts
	backoff := s.config.InitBackoff * time.Duration(1<<uint(count-1))
	if backoff > s.config.MaxBackoff {
		backoff = s.config.MaxBackoff
	}
	s.mu.Unlock()

	if s.hasRestartLimit() && count > maxRestarts {
		slog.Error(s.logPrefix("max restarts exceeded, giving up"),
			"id", crashedID,
			"restartCount", count-1,
			"maxRestarts", maxRestarts,
		)
		s.mu.Lock()
		s.restarting = false
		s.mu.Unlock()
		if s.orch != nil {
			s.orch.EmitEvent("instance.crashed", &bridge.Instance{
				ID:     crashedID,
				Status: "crashed",
			})
		}
		return
	}

	args := []any{
		"id", crashedID,
		"attempt", count,
		"backoff", backoff,
	}
	if s.hasRestartLimit() {
		args = append(args, "maxRestarts", maxRestarts)
	}
	slog.Warn(s.logPrefix("instance crashed, scheduling restart"), args...)

	// Emit restarting event.
	if s.orch != nil {
		s.orch.EmitEvent("instance.restarting", &bridge.Instance{
			ID:     crashedID,
			Status: "restarting",
		})
	}

	// Wait for backoff period (respecting cancellation).
	select {
	case <-time.After(backoff):
	case <-ctx.Done():
		return
	}

	s.restartInstance()
}

// restartInstance launches a new instance to replace the crashed one.
func (s *Strategy) restartInstance() {
	s.mu.Lock()
	ctx := s.ctx
	oldID := s.instanceID
	s.mu.Unlock()

	if ctx == nil || ctx.Err() != nil {
		s.mu.Lock()
		s.restarting = false
		s.mu.Unlock()
		return
	}

	// Clean up the old crashed instance so the orchestrator releases the
	// profile slot and allocated port before we attempt a new launch.
	if oldID != "" {
		if err := s.orch.Stop(oldID); err != nil {
			slog.Debug(s.logPrefix("stop old instance (may already be gone)"), "id", oldID, "err", err)
		}
	}

	inst, err := s.orch.Launch(s.config.ProfileName, "", s.config.Headless, nil)
	if err != nil {
		slog.Error(s.logPrefix("restart failed"),
			"oldId", oldID,
			"err", err,
		)
		s.mu.Lock()
		s.restarting = false
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	s.instanceID = inst.ID
	s.lastStart = time.Now()
	count := s.restartCount
	s.restarting = false
	s.mu.Unlock()

	slog.Info(s.logPrefix("instance restarted"),
		"oldId", oldID,
		"newId", inst.ID,
		"attempt", count,
	)

	// Emit restarted event for dashboard/SSE consumers.
	s.orch.EmitEvent("instance.restarted", inst)
}

// stabilityLoop resets the restart counter after the instance runs stably.
func (s *Strategy) stabilityLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		ctx := s.ctx
		s.mu.Unlock()

		if ctx == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.restartCount > 0 && !s.lastStart.IsZero() && time.Since(s.lastStart) > s.config.StableAfter {
				slog.Info(s.logPrefix("instance stable, resetting restart counter"),
					"id", s.instanceID,
					"stableFor", time.Since(s.lastStart).Round(time.Second),
				)
				s.restartCount = 0
			}
			s.mu.Unlock()
		}
	}
}

// proxyToManaged ensures the managed instance is running, then proxies.
func (s *Strategy) proxyToManaged(w http.ResponseWriter, r *http.Request) {
	target, err := s.ensureRunning()
	if err != nil {
		httpx.Error(w, 503, err)
		return
	}
	strategy.EnrichForTarget(r, s.orch, target)
	proxy.HTTP(w, r, target+r.URL.Path)
}

// ensureRunning returns the URL of the managed instance if running.
func (s *Strategy) ensureRunning() (string, error) {
	if s.orch == nil {
		return "", fmt.Errorf("no orchestrator configured")
	}
	if target := s.orch.FirstRunningURL(); target != "" {
		return target, nil
	}
	return "", fmt.Errorf("instance not ready (may be restarting)")
}

func (s *Strategy) handleTabs(w http.ResponseWriter, r *http.Request) {
	target := s.orch.FirstRunningURL()
	if target == "" {
		httpx.JSON(w, 200, map[string]any{"tabs": []any{}})
		return
	}
	proxy.HTTP(w, r, target+"/tabs")
}

func (s *Strategy) handleStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, s.State())
}

func (s *Strategy) hasRestartLimit() bool {
	return s.config.MaxRestarts > 0
}

func (s *Strategy) logPrefix(message string) string {
	return s.Name() + ": " + message
}
