package main

import (
	"testing"
	"time"
)

func TestLockBasic(t *testing.T) {
	lm := newLockManager()

	if err := lm.Lock("tab1", "agent-a", 5*time.Second); err != nil {
		t.Fatal(err)
	}

	if err := lm.Lock("tab1", "agent-a", 5*time.Second); err != nil {
		t.Fatal(err)
	}

	if err := lm.Lock("tab1", "agent-b", 5*time.Second); err == nil {
		t.Fatal("expected lock conflict")
	}

	if err := lm.Unlock("tab1", "agent-a"); err != nil {
		t.Fatal(err)
	}

	if err := lm.Lock("tab1", "agent-b", 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestLockExpiry(t *testing.T) {
	lm := newLockManager()

	if err := lm.Lock("tab1", "agent-a", 1*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	if err := lm.Lock("tab1", "agent-b", 5*time.Second); err != nil {
		t.Fatalf("expected expired lock to allow new owner: %v", err)
	}
}

func TestLockCheckAccess(t *testing.T) {
	lm := newLockManager()

	if err := lm.CheckAccess("tab1", "anyone"); err != nil {
		t.Fatal(err)
	}

	_ = lm.Lock("tab1", "agent-a", 5*time.Second)

	if err := lm.CheckAccess("tab1", "agent-a"); err != nil {
		t.Fatal(err)
	}

	if err := lm.CheckAccess("tab1", "agent-b"); err == nil {
		t.Fatal("expected access denied")
	}

	if err := lm.CheckAccess("tab1", ""); err == nil {
		t.Fatal("expected access denied for empty owner")
	}
}

func TestUnlockIdempotent(t *testing.T) {
	lm := newLockManager()

	if err := lm.Unlock("tab1", "anyone"); err != nil {
		t.Fatal(err)
	}
}

func TestUnlockWrongOwner(t *testing.T) {
	lm := newLockManager()
	_ = lm.Lock("tab1", "agent-a", 5*time.Second)

	if err := lm.Unlock("tab1", "agent-b"); err == nil {
		t.Fatal("expected unlock denied for wrong owner")
	}
}

func TestMaxTimeout(t *testing.T) {
	lm := newLockManager()
	_ = lm.Lock("tab1", "agent-a", 10*time.Minute)

	lock := lm.Get("tab1")
	if lock == nil {
		t.Fatal("expected lock")
	}

	maxExpiry := time.Now().Add(maxLockTimeout + time.Second)
	if lock.ExpiresAt.After(maxExpiry) {
		t.Fatalf("lock timeout not capped: expires %v", lock.ExpiresAt)
	}
}

func TestLockRequiresOwner(t *testing.T) {
	lm := newLockManager()
	if err := lm.Lock("tab1", "", 5*time.Second); err == nil {
		t.Fatal("expected error for empty owner")
	}
}
