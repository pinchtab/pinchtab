package providerhooks

import "testing"

// ShutdownAll must invoke every registered provider's shutdown hook so no
// provider's browsers survive a signal-time sweep (M4).
func TestShutdownAllInvokesEveryRegisteredHook(t *testing.T) {
	mu.Lock()
	saved := registry
	registry = map[string]Hooks{}
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		registry = saved
		mu.Unlock()
	})

	called := map[string]int{}
	Register("alpha", Hooks{Shutdown: func() { called["alpha"]++ }})
	Register("beta", Hooks{Shutdown: func() { called["beta"]++ }})
	Register("no-shutdown-hook", Hooks{})

	ShutdownAll()
	ShutdownAll() // hooks are documented idempotent; calling twice must be safe

	if called["alpha"] != 2 || called["beta"] != 2 {
		t.Fatalf("expected both hooks called twice, got %v", called)
	}
}
