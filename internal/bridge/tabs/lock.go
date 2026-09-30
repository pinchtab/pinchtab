package tabs

import (
	"fmt"
	"sync"
	"time"
)

const DefaultLockTimeout = 10 * time.Minute

type LockInfo struct {
	Owner     string
	ExpiresAt time.Time
}

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

// Release drops a tab's lease without asking who held it, for a tab that no
// longer exists.
//
// Unlock is the owner's operation and refuses anyone else, which is right while
// there is still a tab to protect. A closed tab has no work left to serialize,
// so there is nobody the lease could still be protecting it from, and requiring
// the owner would mean an agent that closed a tab without unlocking it left an
// entry no call could ever remove.
//
// Nothing did remove them: purgeTrackedTabState clears the four per-tab maps on
// the TabManager and notifies the dialog, executor, log and route managers, but
// the lock manager hangs off the Bridge and was not among them. Every tab locked
// and then closed left an entry behind for the life of the process.
func (m *LockManager) Release(tabID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.locks, tabID)
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
