package browsersession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultSessionIdleTimeout     = 7 * 24 * time.Hour
	DefaultSessionMaxLifetime     = 7 * 24 * time.Hour
	DefaultSessionElevationWindow = 15 * time.Minute
)

// touchPersistInterval bounds how often a LastSeen-only update is flushed to disk.
const touchPersistInterval = 30 * time.Second

type Config struct {
	IdleTimeout                   time.Duration
	MaxLifetime                   time.Duration
	ElevationWindow               time.Duration
	Persist                       bool
	PersistPath                   string
	PersistElevationAcrossRestart bool
}

type Manager struct {
	mu                            sync.RWMutex
	sessions                      map[string]sessionState
	idleTimeout                   time.Duration
	maxLifetime                   time.Duration
	elevationWindow               time.Duration
	persist                       bool
	persistPath                   string
	persistElevationAcrossRestart bool
	now                           func() time.Time
	lastTouchSave                 time.Time // last LastSeen-only flush (debounce gate)

	// A snapshot is built under mu and stamped with a monotonic saveSeq, then
	// marshalled + written under saveMu so session traffic is never serialized
	// behind disk I/O. writtenSeq (guarded by saveMu) lets a writer skip a
	// snapshot older than one already on disk.
	saveMu     sync.Mutex
	saveSeq    uint64 // guarded by mu
	writtenSeq uint64 // guarded by saveMu

	beforeWrite func() // test seam: runs inside writeSnapshot, before the syscalls
}

// saveJob is a self-contained snapshot built under mu and written outside it. A
// zero path means there is nothing to write.
type saveJob struct {
	snapshot persistedSessions
	seq      uint64
	path     string
}

// authRequest is the lock-free prelude of a session lookup.
type authRequest struct {
	id       string
	now      time.Time
	expected [32]byte
}

type sessionState struct {
	CreatedAt     time.Time
	LastSeen      time.Time
	ElevatedUntil time.Time
	TokenHash     [32]byte
}

type persistedSessions struct {
	SavedAt  time.Time                `json:"savedAt"`
	Sessions []persistedSessionRecord `json:"sessions"`
}

type persistedSessionRecord struct {
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"createdAt"`
	LastSeen      time.Time `json:"lastSeen"`
	ElevatedUntil time.Time `json:"elevatedUntil,omitempty"`
	TokenHash     string    `json:"tokenHash"`
}

func NewManager(cfg Config) *Manager {
	m := &Manager{
		sessions: make(map[string]sessionState),
		now:      time.Now,
	}
	m.mu.Lock()
	m.applyConfigLocked(cfg)
	m.mu.Unlock()
	m.loadPersisted()
	return m
}

func (m *Manager) Create(token string) (string, error) {
	if m == nil {
		return "", nil
	}
	id, err := randomSessionID()
	if err != nil {
		return "", err
	}
	now := m.now()
	m.mu.Lock()
	m.sessions[id] = sessionState{
		CreatedAt: now,
		LastSeen:  now,
		TokenHash: hashToken(token),
	}
	job := m.snapshotLocked()
	m.mu.Unlock()
	m.writeSnapshot(job)
	return id, nil
}

func (m *Manager) newAuthRequest(sessionID, token string) (authRequest, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return authRequest{}, false
	}
	return authRequest{id: sessionID, now: m.now(), expected: hashToken(token)}, true
}

// withValidSession runs the auth prelude under the write lock and, on success,
// invokes apply to mutate state and return the snapshot to persist. An invalid
// session is deleted and persisted. Every write happens after m.mu is released.
func (m *Manager) withValidSession(sessionID, token string, apply func(req authRequest, state sessionState) (saveJob, bool)) bool {
	if m == nil {
		return false
	}
	req, ok := m.newAuthRequest(sessionID, token)
	if !ok {
		return false
	}

	m.mu.Lock()
	state, found := m.sessions[req.id]
	var job saveJob
	var result bool
	switch {
	case !found:
	case m.sessionValid(state, req.now, req.expected):
		job, result = apply(req, state)
	default:
		delete(m.sessions, req.id)
		job = m.snapshotLocked()
	}
	m.mu.Unlock()

	m.writeSnapshot(job)
	return result
}

// withValidSessionRead answers a read-only question under the read lock. An
// invalid session escalates to withValidSession, which owns the delete.
func (m *Manager) withValidSessionRead(sessionID, token string, read func(req authRequest, state sessionState) bool) bool {
	if m == nil {
		return false
	}
	req, ok := m.newAuthRequest(sessionID, token)
	if !ok {
		return false
	}

	m.mu.RLock()
	state, found := m.sessions[req.id]
	valid := found && m.sessionValid(state, req.now, req.expected)
	m.mu.RUnlock()

	if valid {
		return read(req, state)
	}
	if !found {
		return false
	}
	return m.withValidSession(sessionID, token, func(authRequest, sessionState) (saveJob, bool) {
		return saveJob{}, false
	})
}

func (m *Manager) Validate(sessionID, token string) bool {
	return m.withValidSession(sessionID, token, func(req authRequest, state sessionState) (saveJob, bool) {
		state.LastSeen = req.now
		m.sessions[req.id] = state
		return m.snapshotTouchLocked(req.now), true
	})
}

func (m *Manager) Elevate(sessionID, token string) bool {
	return m.withValidSession(sessionID, token, func(req authRequest, state sessionState) (saveJob, bool) {
		state.LastSeen = req.now
		state.ElevatedUntil = req.now.Add(m.elevationWindow)
		m.sessions[req.id] = state
		return m.snapshotLocked(), true
	})
}

func (m *Manager) IsElevated(sessionID, token string) bool {
	return m.withValidSessionRead(sessionID, token, func(req authRequest, state sessionState) bool {
		return !state.ElevatedUntil.IsZero() && !req.now.After(state.ElevatedUntil)
	})
}

func (m *Manager) Revoke(sessionID string) {
	if m == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	m.mu.Lock()
	delete(m.sessions, sessionID)
	job := m.snapshotLocked()
	m.mu.Unlock()
	m.writeSnapshot(job)
}

func (m *Manager) MaxLifetime() time.Duration {
	if m == nil {
		return DefaultSessionMaxLifetime
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxLifetime
}

func (m *Manager) IdleTimeout() time.Duration {
	if m == nil {
		return DefaultSessionIdleTimeout
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.idleTimeout
}

func (m *Manager) ElevationWindow() time.Duration {
	if m == nil {
		return DefaultSessionElevationWindow
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.elevationWindow
}

func (m *Manager) UpdateConfig(cfg Config) {
	if m == nil {
		return
	}

	m.mu.Lock()
	oldPath := m.persistPath
	oldPersist := m.persist
	m.applyConfigLocked(cfg)
	m.pruneExpiredLocked(m.now())
	job := m.snapshotLocked()
	persist, persistPath := m.persist, m.persistPath
	m.mu.Unlock()
	m.writeSnapshot(job)

	if oldPersist && oldPath != "" && (!persist || oldPath != persistPath) {
		_ = os.Remove(oldPath)
	}
}

func (m *Manager) applyConfigLocked(cfg Config) {
	idle := cfg.IdleTimeout
	if idle <= 0 {
		idle = DefaultSessionIdleTimeout
	}
	maxLifetime := cfg.MaxLifetime
	if maxLifetime <= 0 {
		maxLifetime = DefaultSessionMaxLifetime
	}
	elevationWindow := cfg.ElevationWindow
	if elevationWindow <= 0 {
		elevationWindow = DefaultSessionElevationWindow
	}
	persistPath := strings.TrimSpace(cfg.PersistPath)
	persist := cfg.Persist && persistPath != ""

	m.idleTimeout = idle
	m.maxLifetime = maxLifetime
	m.elevationWindow = elevationWindow
	m.persist = persist
	m.persistPath = persistPath
	m.persistElevationAcrossRestart = cfg.PersistElevationAcrossRestart
}

func (m *Manager) sessionValid(state sessionState, now time.Time, expected [32]byte) bool {
	return m.sessionTimeValid(state, now) &&
		subtle.ConstantTimeCompare(state.TokenHash[:], expected[:]) == 1
}

func (m *Manager) sessionTimeValid(state sessionState, now time.Time) bool {
	return now.Sub(state.LastSeen) <= m.idleTimeout &&
		now.Sub(state.CreatedAt) <= m.maxLifetime
}

// MaintenanceInterval is how often RunMaintenance sweeps expired sessions.
const MaintenanceInterval = 5 * time.Minute

// PruneExpired drops expired sessions and persists the result. Expiry is
// otherwise only noticed when a session's own cookie is presented again, so a
// dashboard login that is simply abandoned — a closed browser, a second device —
// is never detected, and its record plus its token hash survive in memory and in
// the persisted file for the life of the process.
func (m *Manager) PruneExpired() {
	if m == nil {
		return
	}
	m.mu.Lock()
	before := len(m.sessions)
	m.pruneExpiredLocked(m.now())
	var job saveJob
	if len(m.sessions) != before {
		job = m.snapshotLocked()
	}
	m.mu.Unlock()
	m.writeSnapshot(job)
}

// RunMaintenance sweeps expired sessions until ctx is done.
func (m *Manager) RunMaintenance(ctx context.Context) {
	if m == nil {
		return
	}
	t := time.NewTicker(MaintenanceInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.PruneExpired()
		}
	}
}

func (m *Manager) pruneExpiredLocked(now time.Time) {
	for id, state := range m.sessions {
		if !m.sessionTimeValid(state, now) {
			delete(m.sessions, id)
		}
	}
}

func (m *Manager) loadPersisted() {
	if m == nil {
		return
	}

	m.mu.RLock()
	persist, persistPath := m.persist, m.persistPath
	m.mu.RUnlock()
	if !persist || persistPath == "" {
		return
	}

	data, err := os.ReadFile(persistPath)
	if err != nil {
		return
	}

	var persisted persistedSessions
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}

	m.mu.Lock()
	now := m.now()
	loaded := make(map[string]sessionState, len(persisted.Sessions))
	for _, record := range persisted.Sessions {
		tokenHash, err := hex.DecodeString(strings.TrimSpace(record.TokenHash))
		if err != nil || len(tokenHash) != sha256.Size {
			continue
		}
		var hash [32]byte
		copy(hash[:], tokenHash)

		state := sessionState{
			CreatedAt:     record.CreatedAt,
			LastSeen:      record.LastSeen,
			ElevatedUntil: record.ElevatedUntil,
			TokenHash:     hash,
		}
		if !m.persistElevationAcrossRestart {
			state.ElevatedUntil = time.Time{}
		}
		if !m.sessionTimeValid(state, now) {
			continue
		}
		recordID := strings.TrimSpace(record.ID)
		if recordID == "" {
			continue
		}
		loaded[recordID] = state
	}
	m.sessions = loaded
	job := m.snapshotLocked()
	m.mu.Unlock()
	m.writeSnapshot(job)
}

// snapshotTouchLocked snapshots a LastSeen-only update at most once per
// touchPersistInterval. Caller holds m.mu. The next real mutation's snapshot
// opportunistically flushes any debounced LastSeen for all sessions.
func (m *Manager) snapshotTouchLocked(now time.Time) saveJob {
	if now.Sub(m.lastTouchSave) < touchPersistInterval {
		return saveJob{}
	}
	m.lastTouchSave = now
	return m.snapshotLocked()
}

// snapshotLocked captures the session map plus every config field the writer
// needs, stamped with a monotonic sequence. Caller holds m.mu.
func (m *Manager) snapshotLocked() saveJob {
	if !m.persist || m.persistPath == "" {
		return saveJob{}
	}

	snapshot := persistedSessions{
		SavedAt:  m.now().UTC(),
		Sessions: make([]persistedSessionRecord, 0, len(m.sessions)),
	}
	for id, state := range m.sessions {
		record := persistedSessionRecord{
			ID:            id,
			CreatedAt:     state.CreatedAt,
			LastSeen:      state.LastSeen,
			ElevatedUntil: state.ElevatedUntil,
			TokenHash:     hex.EncodeToString(state.TokenHash[:]),
		}
		if !m.persistElevationAcrossRestart {
			record.ElevatedUntil = time.Time{}
		}
		snapshot.Sessions = append(snapshot.Sessions, record)
	}

	m.saveSeq++
	return saveJob{snapshot: snapshot, seq: m.saveSeq, path: m.persistPath}
}

// writeSnapshot marshals and atomically writes a snapshot with m.mu released.
// Writers serialize on saveMu; a snapshot older than one already written is
// skipped so a stale snapshot can never clobber a fresher one.
func (m *Manager) writeSnapshot(job saveJob) {
	if job.path == "" {
		return
	}

	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	if job.seq <= m.writtenSeq {
		return
	}
	m.writtenSeq = job.seq

	if m.beforeWrite != nil {
		m.beforeWrite()
	}

	data, err := json.MarshalIndent(job.snapshot, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(job.path), 0755); err != nil {
		return
	}
	// Atomic write: a torn file here fails to unmarshal on the next start and
	// silently logs every dashboard user out.
	tmpPath := job.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return
	}
	if err := os.Rename(tmpPath, job.path); err != nil {
		_ = os.Remove(tmpPath)
	}
}

func randomSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(token string) [32]byte {
	return sha256.Sum256([]byte(strings.TrimSpace(token)))
}
