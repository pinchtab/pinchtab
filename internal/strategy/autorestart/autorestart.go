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
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
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
	headless     bool      // Headless mode of the managed instance
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
		config:   cfg,
		headless: cfg.Headless,
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
	if cfg.HeadlessSet {
		s.config.Headless = cfg.Headless
		s.config.HeadlessSet = true
		s.headless = cfg.Headless
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

	s.orch.OnEvent(func(evt orchestrator.InstanceEvent) {
		s.handleEvent(evt)
	})

	go s.launchInitial()
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

func (s *Strategy) hasRestartLimit() bool {
	return s.config.MaxRestarts > 0
}

func (s *Strategy) logPrefix(message string) string {
	return s.Name() + ": " + message
}
