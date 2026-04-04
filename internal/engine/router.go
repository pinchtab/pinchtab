package engine

import (
	"log/slog"
	"sync"
)

// Router selects the engine for each request.
//
// It evaluates an ordered list of RouteRules, picks the first non-Undecided
// verdict, and delegates to the matching engine.  Rules can be added or
// removed at runtime (under a write lock) so that the routing strategy can
// evolve without modifying handlers or bridge code.
type Router struct {
	mode  Mode
	lite  Engine // may be nil when Mode == ModeChrome
	rules []RouteRule
	mu    sync.RWMutex
}

// NewRouter creates a router for the given mode.
// Pass nil for lite when running in chrome-only mode.
func NewRouter(mode Mode, lite Engine) *Router {
	r := &Router{
		mode: mode,
		lite: lite,
	}

	switch mode {
	case ModeLite:
		r.rules = []RouteRule{
			CapabilityRule{},  // screenshot/pdf → chrome always
			DefaultLiteRule{}, // everything else → lite
		}
	case ModeLightpanda:
		r.rules = []RouteRule{
			LightpandaCapabilityRule{}, // screenshot/pdf/eval/cookies → chrome
			DefaultLightpandaRule{},    // everything else → lightpanda
		}
	case ModeAuto:
		r.rules = []RouteRule{
			CapabilityRule{},    // chrome-only caps first
			ContentHintRule{},   // static pages → lite
			DefaultChromeRule{}, // fallback
		}
	default: // ModeChrome
		r.rules = []RouteRule{
			DefaultChromeRule{},
		}
	}

	return r
}

// AddRule appends a rule to the chain.  It is evaluated after all
// currently registered rules but before the default fallback.
func (r *Router) AddRule(rule RouteRule) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Insert before the last rule (which is the fallback).
	if len(r.rules) > 0 {
		last := r.rules[len(r.rules)-1]
		r.rules[len(r.rules)-1] = rule
		r.rules = append(r.rules, last)
	} else {
		r.rules = append(r.rules, rule)
	}
	slog.Info("engine: rule added", "rule", rule.Name(), "total", len(r.rules))
}

// RemoveRule removes the first rule with the given name.
func (r *Router) RemoveRule(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, rule := range r.rules {
		if rule.Name() == name {
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			return true
		}
	}
	return false
}

// Route returns the engine to use for a given operation and URL.
func (r *Router) Route(op Capability, url string) Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rule := range r.rules {
		switch rule.Decide(op, url) {
		case UseLite:
			if r.lite != nil {
				return r.lite
			}
			// lite unavailable — fall through
		case UseChrome:
			return nil // nil signals "use chrome bridge"
		}
	}
	return nil // default: chrome
}

// UseLite returns true when the router would send this operation to the
// lite engine. Convenience helper for handler-level checks.
func (r *Router) UseLite(op Capability, url string) bool {
	return r.Route(op, url) != nil
}

// Lite returns the lite engine (may be nil).
func (r *Router) Lite() Engine {
	return r.lite
}

// Mode returns the configured engine mode.
func (r *Router) Mode() Mode {
	return r.mode
}

// Rules returns a snapshot of the current rule names (for diagnostics).
func (r *Router) Rules() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, len(r.rules))
	for i, rule := range r.rules {
		names[i] = rule.Name()
	}
	return names
}
