package main

import (
	"fmt"
	"sync"
	"time"
)

type TabLock struct {
	Owner     string    `json:"owner"`
	LockedAt  time.Time `json:"lockedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type lockManager struct {
	locks map[string]*TabLock
	mu    sync.Mutex
}

const (
	defaultLockTimeout = 30 * time.Second
	maxLockTimeout     = 5 * time.Minute
)

func newLockManager() *lockManager {
	return &lockManager{locks: make(map[string]*TabLock)}
}

func (lm *lockManager) Lock(tabID, owner string, timeout time.Duration) error {
	if owner == "" {
		return fmt.Errorf("owner required")
	}
	if timeout <= 0 {
		timeout = defaultLockTimeout
	}
	if timeout > maxLockTimeout {
		timeout = maxLockTimeout
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	if existing, ok := lm.locks[tabID]; ok {
		if time.Now().Before(existing.ExpiresAt) && existing.Owner != owner {
			return fmt.Errorf("tab locked by %q until %s", existing.Owner, existing.ExpiresAt.Format(time.RFC3339))
		}
	}

	lm.locks[tabID] = &TabLock{
		Owner:     owner,
		LockedAt:  time.Now(),
		ExpiresAt: time.Now().Add(timeout),
	}
	return nil
}

func (lm *lockManager) Unlock(tabID, owner string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	existing, ok := lm.locks[tabID]
	if !ok {
		return nil
	}

	if existing.Owner != owner && time.Now().Before(existing.ExpiresAt) {
		return fmt.Errorf("tab locked by %q, cannot unlock", existing.Owner)
	}

	delete(lm.locks, tabID)
	return nil
}

func (lm *lockManager) Get(tabID string) *TabLock {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lock, ok := lm.locks[tabID]
	if !ok {
		return nil
	}
	if time.Now().After(lock.ExpiresAt) {
		delete(lm.locks, tabID)
		return nil
	}
	return lock
}

func (lm *lockManager) CheckAccess(tabID, owner string) error {
	lock := lm.Get(tabID)
	if lock == nil {
		return nil
	}
	if owner == "" || lock.Owner != owner {
		return fmt.Errorf("tab locked by %q until %s", lock.Owner, lock.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}
