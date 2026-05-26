package session

import (
	"sync"
	"testing"
	"time"
)

// collectingHook returns a hook function plus a goroutine-safe getter for
// the events seen so far. The getter waits up to `wait` for the count
// to reach `want`.
func collectingHook(t *testing.T) (LifecycleHook, func(want int, wait time.Duration) []LifecycleEvent) {
	t.Helper()
	var (
		mu     sync.Mutex
		events []LifecycleEvent
		signal = make(chan struct{}, 1024)
	)
	hook := func(evt LifecycleEvent) {
		mu.Lock()
		events = append(events, evt)
		mu.Unlock()
		select {
		case signal <- struct{}{}:
		default:
		}
	}
	wait := func(want int, timeout time.Duration) []LifecycleEvent {
		deadline := time.Now().Add(timeout)
		for {
			mu.Lock()
			if len(events) >= want {
				out := make([]LifecycleEvent, len(events))
				copy(out, events)
				mu.Unlock()
				return out
			}
			mu.Unlock()
			if remaining := time.Until(deadline); remaining > 0 {
				select {
				case <-signal:
				case <-time.After(remaining):
				}
			} else {
				mu.Lock()
				out := make([]LifecycleEvent, len(events))
				copy(out, events)
				mu.Unlock()
				return out
			}
		}
	}
	return hook, wait
}

func TestLifecycle_RevokeFiresHookAfterUnlock(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred"})
	_, token, err := s.Create("agent-1", "test", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, ok := s.Authenticate(token)
	if !ok {
		t.Fatal("Authenticate failed")
	}

	hook, wait := collectingHook(t)
	// Hook tries to grab the store mutex — must succeed (i.e. mutex
	// already released by the time we are called).
	wrapped := func(evt LifecycleEvent) {
		if !s.mu.TryLock() {
			t.Errorf("hook ran while store mutex was held")
			return
		}
		s.mu.Unlock()
		hook(evt)
	}
	s.OnLifecycle(wrapped)

	if !s.Revoke(sess.ID) {
		t.Fatal("Revoke returned false")
	}
	got := wait(1, time.Second)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %#v", len(got), got)
	}
	if got[0].SessionID != sess.ID || got[0].Reason != LifecycleReasonRevoked {
		t.Fatalf("event = %#v, want sessionID=%s reason=revoked", got[0], sess.ID)
	}
}

func TestLifecycle_PrunePropagatesEvents(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	clock := t0
	s := NewStore(Config{Enabled: true, Mode: "preferred", IdleTimeout: time.Hour})
	s.now = func() time.Time { return clock }

	_, token, err := s.Create("agent-stale", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok := s.Authenticate(token); !ok {
		t.Fatal("Authenticate failed")
	}

	hook, wait := collectingHook(t)
	s.OnLifecycle(hook)

	// Advance past idle timeout and trigger a prune via UpdateConfig.
	clock = t0.Add(2 * time.Hour)
	s.UpdateConfig(Config{Enabled: true, Mode: "preferred", IdleTimeout: time.Hour})

	got := wait(1, time.Second)
	if len(got) != 1 || got[0].Reason != LifecycleReasonPruned {
		t.Fatalf("expected one pruned event, got %#v", got)
	}
}

func TestLifecycle_AuthenticateExpiryFiresEvent(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	clock := t0
	s := NewStore(Config{Enabled: true, Mode: "preferred", IdleTimeout: time.Hour})
	s.now = func() time.Time { return clock }

	_, token, err := s.Create("agent-expire", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	hook, wait := collectingHook(t)
	s.OnLifecycle(hook)

	clock = t0.Add(2 * time.Hour)
	if _, ok := s.Authenticate(token); ok {
		t.Fatal("expired token should fail auth")
	}

	got := wait(1, time.Second)
	if len(got) != 1 || got[0].Reason != LifecycleReasonExpired {
		t.Fatalf("expected one expired event, got %#v", got)
	}
}

func TestLifecycle_NoSubscribersIsSafe(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred"})
	_, token, _ := s.Create("agent-quiet", "", "")
	sess, _ := s.Authenticate(token)
	if !s.Revoke(sess.ID) {
		t.Fatal("Revoke")
	}
	// No assertion — just must not panic / deadlock.
}
