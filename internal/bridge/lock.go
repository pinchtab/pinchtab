package bridge

import (
	"fmt"
	"sync"
	"time"
)

const DefaultLockTimeout = 10 * time.Minute

type lockEntry struct {
	owner   string
	expires time.Time
}

type LockManager struct {
	locks map[string]lockEntry
	mu    sync.Mutex
}

func NewLockManager() *LockManager {
	return &LockManager{
		locks: make(map[string]lockEntry),
	}
}

func (m *LockManager) TryLock(tabID, owner string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	l, ok := m.locks[tabID]
	if ok && time.Now().Before(l.expires) && l.owner != owner {
		return fmt.Errorf("tab %s is locked by %s for another %v", tabID, l.owner, time.Until(l.expires).Round(time.Second))
	}

	m.locks[tabID] = lockEntry{
		owner:   owner,
		expires: time.Now().Add(ttl),
	}
	return nil
}

func (m *LockManager) Unlock(tabID, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	l, ok := m.locks[tabID]
	if !ok || time.Now().After(l.expires) {
		delete(m.locks, tabID)
		return nil
	}

	if l.owner != owner {
		return fmt.Errorf("cannot unlock: tab %s is locked by %s", tabID, l.owner)
	}

	delete(m.locks, tabID)
	return nil
}

func (m *LockManager) Get(tabID string) *LockInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	l, ok := m.locks[tabID]
	if !ok || time.Now().After(l.expires) {
		return nil
	}

	return &LockInfo{
		Owner:     l.owner,
		ExpiresAt: l.expires,
	}
}
