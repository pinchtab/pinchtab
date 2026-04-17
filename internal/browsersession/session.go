package browsersession

import (
	"crypto/rand"
	"crypto/sha256"
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

type Config struct {
	IdleTimeout                   time.Duration
	MaxLifetime                   time.Duration
	ElevationWindow               time.Duration
	Persist                       bool
	PersistPath                   string
	PersistElevationAcrossRestart bool
}

type Manager struct {
	mu                            sync.Mutex
	sessions                      map[string]sessionState
	idleTimeout                   time.Duration
	maxLifetime                   time.Duration
	elevationWindow               time.Duration
	persist                       bool
	persistPath                   string
	persistElevationAcrossRestart bool
	now                           func() time.Time
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
	m.saveLocked()
	m.mu.Unlock()
	return id, nil
}

func (m *Manager) Validate(sessionID, token string) bool {
	if m == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}

	now := m.now()
	expected := hashToken(token)

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.sessions[sessionID]
	if !ok {
		return false
	}
	if !m.sessionValid(state, now, expected) {
		delete(m.sessions, sessionID)
		m.saveLocked()
		return false
	}
	state.LastSeen = now
	m.sessions[sessionID] = state
	m.saveLocked()
	return true
}

func (m *Manager) Elevate(sessionID, token string) bool {
	if m == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}

	now := m.now()
	expected := hashToken(token)

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.sessions[sessionID]
	if !ok {
		return false
	}
	if !m.sessionValid(state, now, expected) {
		delete(m.sessions, sessionID)
		m.saveLocked()
		return false
	}
	state.LastSeen = now
	state.ElevatedUntil = now.Add(m.elevationWindow)
	m.sessions[sessionID] = state
	m.saveLocked()
	return true
}

func (m *Manager) IsElevated(sessionID, token string) bool {
	if m == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}

	now := m.now()
	expected := hashToken(token)

	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.sessions[sessionID]
	if !ok {
		return false
	}
	if !m.sessionValid(state, now, expected) {
		delete(m.sessions, sessionID)
		return false
	}
	return !state.ElevatedUntil.IsZero() && !now.After(state.ElevatedUntil)
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
	m.saveLocked()
	m.mu.Unlock()
}

func (m *Manager) MaxLifetime() time.Duration {
	if m == nil {
		return DefaultSessionMaxLifetime
	}
	return m.maxLifetime
}

func (m *Manager) IdleTimeout() time.Duration {
	if m == nil {
		return DefaultSessionIdleTimeout
	}
	return m.idleTimeout
}

func (m *Manager) ElevationWindow() time.Duration {
	if m == nil {
		return DefaultSessionElevationWindow
	}
	return m.elevationWindow
}

func (m *Manager) UpdateConfig(cfg Config) {
	if m == nil {
		return
	}

	persistPath := strings.TrimSpace(cfg.PersistPath)
	persist := cfg.Persist && persistPath != ""

	m.mu.Lock()
	oldPath := m.persistPath
	oldPersist := m.persist
	m.applyConfigLocked(cfg)
	m.pruneExpiredLocked(m.now())
	m.saveLocked()
	m.mu.Unlock()

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
	return m.sessionTimeValid(state, now) && state.TokenHash == expected
}

func (m *Manager) sessionTimeValid(state sessionState, now time.Time) bool {
	return now.Sub(state.LastSeen) <= m.idleTimeout &&
		now.Sub(state.CreatedAt) <= m.maxLifetime
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

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.persist || m.persistPath == "" {
		return
	}

	data, err := os.ReadFile(m.persistPath)
	if err != nil {
		return
	}
	var persisted persistedSessions
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}

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
	m.saveLocked()
}

func (m *Manager) saveLocked() {
	if !m.persist || m.persistPath == "" {
		return
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

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.persistPath), 0755); err != nil {
		return
	}
	if err := os.WriteFile(m.persistPath, data, 0600); err != nil {
		return
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
