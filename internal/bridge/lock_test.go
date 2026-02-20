package bridge

import (
	"testing"
	"time"
)

func TestLockManager(t *testing.T) {
	m := NewLockManager()
	tabID := "tab1"
	owner1 := "agent1"
	owner2 := "agent2"
	ttl := 100 * time.Millisecond

	if err := m.TryLock(tabID, owner1, ttl); err != nil {
		t.Fatalf("lock failed: %v", err)
	}

	if err := m.TryLock(tabID, owner1, ttl); err != nil {
		t.Fatalf("re-lock same owner failed: %v", err)
	}

	if err := m.TryLock(tabID, owner2, ttl); err == nil {
		t.Error("expected conflict error")
	}

	if err := m.Unlock(tabID, owner1); err != nil {
		t.Fatalf("unlock failed: %v", err)
	}

	if err := m.TryLock(tabID, owner2, ttl); err != nil {
		t.Fatalf("lock after unlock failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	if err := m.TryLock(tabID, owner1, ttl); err != nil {
		t.Fatalf("lock after expiration failed: %v", err)
	}
}

func TestLockManagerGet(t *testing.T) {
	m := NewLockManager()
	tabID := "tab1"
	owner := "agent1"
	ttl := 1 * time.Hour

	if info := m.Get(tabID); info != nil {
		t.Error("expected nil info for unlocked tab")
	}

	_ = m.TryLock(tabID, owner, ttl)
	info := m.Get(tabID)
	if info == nil {
		t.Fatal("expected info")
	}
	if info.Owner != owner {
		t.Errorf("expected owner %s, got %s", owner, info.Owner)
	}
}
