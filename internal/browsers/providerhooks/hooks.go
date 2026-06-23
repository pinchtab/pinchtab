package providerhooks

import (
	"sync"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// Hooks lets browser providers expose optional bridge decoration and
// lifecycle cleanup behavior without leaking concrete implementations into
// generic server/orchestrator code.
type Hooks struct {
	DecorateBridge func(bridge.BridgeAPI, *config.RuntimeConfig) bridge.BridgeAPI
	CleanupProfile func(string)
	Shutdown       func()
}

var (
	mu       sync.RWMutex
	registry = map[string]Hooks{}
)

func Register(browserID string, hooks Hooks) {
	mu.Lock()
	defer mu.Unlock()
	registry[browserID] = hooks
}

func DecorateBridge(browserID string, api bridge.BridgeAPI, cfg *config.RuntimeConfig) bridge.BridgeAPI {
	mu.RLock()
	hooks, ok := registry[browserID]
	mu.RUnlock()
	if !ok || hooks.DecorateBridge == nil {
		return api
	}
	return hooks.DecorateBridge(api, cfg)
}

func CleanupProfile(browserID, profileDir string) {
	mu.RLock()
	hooks, ok := registry[browserID]
	mu.RUnlock()
	if !ok || hooks.CleanupProfile == nil {
		return
	}
	hooks.CleanupProfile(profileDir)
}

func Shutdown(browserID string) {
	mu.RLock()
	hooks, ok := registry[browserID]
	mu.RUnlock()
	if !ok || hooks.Shutdown == nil {
		return
	}
	hooks.Shutdown()
}

// ShutdownAll runs every registered provider's shutdown cleanup. Used at
// signal time so no provider's browser processes can be orphaned regardless
// of which browser is the configured default. Hooks are copied out under the
// lock and invoked outside it; they must be idempotent (the built-in ones are
// process sweeps).
func ShutdownAll() {
	mu.RLock()
	fns := make([]func(), 0, len(registry))
	for _, hooks := range registry {
		if hooks.Shutdown != nil {
			fns = append(fns, hooks.Shutdown)
		}
	}
	mu.RUnlock()
	for _, fn := range fns {
		fn()
	}
}
