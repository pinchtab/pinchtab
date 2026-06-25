// Package solver defines the interface for pluggable challenge solvers.
//
// A Solver handles a specific class of browser challenge (CAPTCHA, interstitial,
// bot gate, etc.). Solvers are registered by name and can be invoked explicitly
// or auto-detected via CanHandle.
//
// Example registration (in init):
//
//	solver.MustRegister("cloudflare", &CloudflareSolver{})
package solver

import (
	"context"
	"fmt"
	"sync"
)

// Solver handles a specific type of browser challenge.
type Solver interface {
	// Name returns the solver identifier (e.g., "cloudflare").
	Name() string

	// CanHandle reports whether this solver recognises a challenge on the
	// current page. Implementations should be lightweight (title check, DOM
	// marker) and avoid side-effects.
	CanHandle(ctx context.Context) (bool, error)

	// Solve attempts to resolve the challenge on the current page.
	Solve(ctx context.Context, opts Options) (*Result, error)
}

// Options configures a solve attempt.
type Options struct {
	MaxAttempts int `json:"maxAttempts,omitempty"`
}

// Result is the outcome of a solve attempt.
type Result struct {
	Solver        string `json:"solver,omitempty"`
	Solved        bool   `json:"solved"`
	ChallengeType string `json:"challengeType,omitempty"`
	Attempts      int    `json:"attempts"`
	Title         string `json:"title"`
	Token         string `json:"token,omitempty"`
}

var (
	registry = make(map[string]Solver)
	order    []string // insertion order for deterministic auto-detect
	mu       sync.RWMutex
)

// Register adds a solver to the global registry.
func Register(name string, s Solver) error {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[name]; exists {
		return fmt.Errorf("solver %q already registered", name)
	}
	registry[name] = s
	order = append(order, name)
	return nil
}

// MustRegister is like Register but panics on duplicate name.
func MustRegister(name string, s Solver) {
	if err := Register(name, s); err != nil {
		panic(err)
	}
}

// Get returns a solver by name.
func Get(name string) (Solver, bool) {
	mu.RLock()
	defer mu.RUnlock()
	s, ok := registry[name]
	return s, ok
}

// Names returns all registered solver names in registration order.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, len(order))
	copy(out, order)
	return out
}

// Solve invokes a named solver, or if name is empty, auto-detects the first
// solver whose CanHandle returns true. When no challenge is detected, it
// returns a Result with Solved=true and Attempts=0.
func Solve(ctx context.Context, name string, opts Options) (*Result, error) {
	if name != "" {
		s, ok := Get(name)
		if !ok {
			return nil, fmt.Errorf("unknown solver: %s (available: %v)", name, Names())
		}
		return s.Solve(ctx, opts)
	}

	mu.RLock()
	names := make([]string, len(order))
	copy(names, order)
	mu.RUnlock()

	for _, n := range names {
		s, _ := Get(n)
		if s == nil {
			continue
		}
		can, err := s.CanHandle(ctx)
		if err != nil || !can {
			continue
		}
		return s.Solve(ctx, opts)
	}

	var title string
	return &Result{Solved: true, Title: title}, nil
}

// Unregister removes a solver from the registry. Intended for tests only.
func Unregister(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, name)
	for i, n := range order {
		if n == name {
			order = append(order[:i], order[i+1:]...)
			break
		}
	}
}
