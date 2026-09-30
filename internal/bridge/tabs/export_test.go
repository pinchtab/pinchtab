package tabs

func (m *LockManager) trackedForTest() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.locks)
}
