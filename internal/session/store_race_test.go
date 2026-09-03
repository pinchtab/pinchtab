package session

import (
	"sync"
	"testing"
)

// Authenticate must hand back a defensive copy like Get/List do. The middleware
// keeps the returned session for the whole request (grant checks, request
// context), so a store-owned pointer races with concurrent SetGrants/Touch.
func TestAuthenticateReturnsDefensiveCopy(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "required"})
	id, token, err := s.Create("agent-1", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	sess, ok := s.Authenticate(token)
	if !ok || sess == nil {
		t.Fatal("authenticate should succeed")
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.SetGrants(id, []string{"GET /snapshot", "POST /action"})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = len(sess.Grants)
			_ = sess.LastSeenAt
		}
	}()
	wg.Wait()
}

// Enabled is read on every authenticated request and reads two config fields;
// UpdateConfig replaces the whole config struct, so unsynchronized reads race.
func TestConfigAccessorsAreSynchronized(t *testing.T) {
	s := NewStore(Config{Enabled: true, Mode: "preferred"})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.UpdateConfig(Config{Enabled: i%2 == 0, Mode: ModeOff})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = s.Enabled()
		}
	}()
	wg.Wait()
}
