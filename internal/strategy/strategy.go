// Package strategy defines the interface for pluggable allocation strategies.
// Strategies control how tabs and instances are allocated to requests.
package strategy

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/pinchtab/pinchtab/internal/primitive"
)

// Strategy defines a browser allocation approach.
// Each strategy can expose different HTTP endpoints and implement
// different allocation logic while composing the same primitives.
type Strategy interface {
	// Name returns the strategy identifier (for config).
	Name() string

	// Init receives primitives to compose.
	// Called once during orchestrator setup.
	Init(p *primitive.Primitives) error

	// RegisterRoutes adds strategy-specific HTTP endpoints to the mux.
	// This is called after Init, before Start.
	RegisterRoutes(mux *http.ServeMux)

	// Start begins any background tasks (pool warming, cleanup goroutines).
	// Called after RegisterRoutes, when server is about to start.
	Start(ctx context.Context) error

	// Stop gracefully shuts down background tasks.
	// Called during server shutdown.
	Stop() error
}

// Factory creates a new Strategy instance.
type Factory func() Strategy

var (
	registry = make(map[string]Factory)
	mu       sync.RWMutex
)

// Register adds a strategy factory to the registry.
// Typically called from init() in each strategy package.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = factory
}

// New creates a strategy by name from the registry.
func New(name string) (Strategy, error) {
	mu.RLock()
	factory, ok := registry[name]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown strategy: %s (available: %v)", name, Names())
	}
	return factory(), nil
}

// Names returns all registered strategy names.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// Config holds strategy-specific configuration.
// Strategies can define their own typed config and parse from this.
type Config struct {
	// Common options
	DefaultProfile string `json:"defaultProfile" yaml:"default_profile"`
	AutoLaunch     bool   `json:"autoLaunch" yaml:"auto_launch"`

	// Session strategy options
	SessionTTL        string `json:"sessionTTL" yaml:"session_ttl"`
	MaxTabsPerSession int    `json:"maxTabsPerSession" yaml:"max_tabs_per_session"`

	// Pool strategy options
	PoolSize     int    `json:"poolSize" yaml:"pool_size"`
	MaxLeaseTime string `json:"maxLeaseTime" yaml:"max_lease_time"`
	WarmupURL    string `json:"warmupURL" yaml:"warmup_url"`

	// Raw options for custom strategies
	Options map[string]any `json:"options" yaml:"options"`
}
