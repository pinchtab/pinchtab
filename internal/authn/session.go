package authn

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const (
	DefaultSessionIdleTimeout     = 12 * time.Hour
	DefaultSessionMaxLifetime     = 7 * 24 * time.Hour
	DefaultSessionElevationWindow = 15 * time.Minute
)

type SessionConfig struct {
	IdleTimeout     time.Duration
	MaxLifetime     time.Duration
	ElevationWindow time.Duration
}

type SessionManager struct {
	mu              sync.Mutex
	sessions        map[string]sessionState
	idleTimeout     time.Duration
	maxLifetime     time.Duration
	elevationWindow time.Duration
	now             func() time.Time
}

type sessionState struct {
	CreatedAt     time.Time
	LastSeen      time.Time
	ElevatedUntil time.Time
	TokenHash     [32]byte
}

func NewSessionManager(cfg SessionConfig) *SessionManager {
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
	return &SessionManager{
		sessions:        make(map[string]sessionState),
		idleTimeout:     idle,
		maxLifetime:     maxLifetime,
		elevationWindow: elevationWindow,
		now:             time.Now,
	}
}

func (m *SessionManager) Create(token string) (string, error) {
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
	m.mu.Unlock()
	return id, nil
}

func (m *SessionManager) Validate(sessionID, token string) bool {
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
	state.LastSeen = now
	m.sessions[sessionID] = state
	return true
}

func (m *SessionManager) Elevate(sessionID, token string) bool {
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
	state.LastSeen = now
	state.ElevatedUntil = now.Add(m.elevationWindow)
	m.sessions[sessionID] = state
	return true
}

func (m *SessionManager) IsElevated(sessionID, token string) bool {
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

func (m *SessionManager) Revoke(sessionID string) {
	if m == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
}

func (m *SessionManager) MaxLifetime() time.Duration {
	if m == nil {
		return DefaultSessionMaxLifetime
	}
	return m.maxLifetime
}

func (m *SessionManager) ElevationWindow() time.Duration {
	if m == nil {
		return DefaultSessionElevationWindow
	}
	return m.elevationWindow
}

func (m *SessionManager) sessionValid(state sessionState, now time.Time, expected [32]byte) bool {
	return now.Sub(state.LastSeen) <= m.idleTimeout &&
		now.Sub(state.CreatedAt) <= m.maxLifetime &&
		state.TokenHash == expected
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
