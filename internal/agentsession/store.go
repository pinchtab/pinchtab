// Package agentsession provides durable, revocable session-based
// authentication for automated agents. Each session maps a high-entropy
// token to an agentId, allowing agents to authenticate with a single
// environment variable (PINCHTAB_SESSION) instead of the server bearer token.
package agentsession

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Session represents a durable, revocable agent session.
type Session struct {
	ID          string        `json:"id"`
	AgentID     string        `json:"agentId"`
	Label       string        `json:"label,omitempty"`
	TokenHash   [32]byte      `json:"-"`
	CreatedAt   time.Time     `json:"createdAt"`
	LastSeenAt  time.Time     `json:"lastSeenAt"`
	ExpiresAt   time.Time     `json:"expiresAt,omitempty"`
	IdleTimeout time.Duration `json:"-"`
	Status      string        `json:"status"`
	Grants      []string      `json:"grants,omitempty"`
}

// Config controls store behavior.
type Config struct {
	Enabled     bool
	Mode        string // "off", "preferred", "required"
	IdleTimeout time.Duration
	MaxLifetime time.Duration
	PersistPath string
}

// Store manages agent sessions with persistence.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*Session // keyed by session ID
	cfg      Config
	now      func() time.Time
}

const (
	DefaultIdleTimeout = 7 * 24 * time.Hour
	DefaultMaxLifetime = 30 * 24 * time.Hour

	StatusActive  = "active"
	StatusRevoked = "revoked"
	StatusExpired = "expired"
)

// NewStore creates a new agent session store.
func NewStore(cfg Config) *Store {
	s := &Store{
		sessions: make(map[string]*Session),
		now:      time.Now,
	}
	s.applyConfig(cfg)
	s.loadPersisted()
	return s
}

func (s *Store) applyConfig(cfg Config) {
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.MaxLifetime <= 0 {
		cfg.MaxLifetime = DefaultMaxLifetime
	}
	if cfg.Mode == "" {
		cfg.Mode = "preferred"
	}
	s.cfg = cfg
}

// Create generates a new agent session and returns the session ID and
// plaintext token. The token is returned exactly once and is never stored.
func (s *Store) Create(agentID, label string) (sessionID, sessionToken string, err error) {
	if s == nil {
		return "", "", fmt.Errorf("store is nil")
	}

	id, err := generateSessionID()
	if err != nil {
		return "", "", err
	}
	token, err := generateToken()
	if err != nil {
		return "", "", err
	}

	now := s.now()
	session := &Session{
		ID:          id,
		AgentID:     strings.TrimSpace(agentID),
		Label:       strings.TrimSpace(label),
		TokenHash:   hashToken(token),
		CreatedAt:   now,
		LastSeenAt:  now,
		ExpiresAt:   now.Add(s.cfg.MaxLifetime),
		IdleTimeout: s.cfg.IdleTimeout,
		Status:      StatusActive,
	}

	s.mu.Lock()
	s.sessions[id] = session
	s.saveLocked()
	s.mu.Unlock()

	return id, token, nil
}

// Authenticate validates a token and returns the associated session.
// It updates LastSeenAt on success.
func (s *Store) Authenticate(token string) (*Session, bool) {
	if s == nil {
		return nil, false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}

	hash := hashToken(token)
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sess := range s.sessions {
		if sess.Status != StatusActive {
			continue
		}
		if subtle.ConstantTimeCompare(hash[:], sess.TokenHash[:]) != 1 {
			continue
		}
		if s.isExpired(sess, now) {
			sess.Status = StatusExpired
			s.saveLocked()
			return nil, false
		}
		sess.LastSeenAt = now
		s.saveLocked()
		return sess, true
	}
	return nil, false
}

// Get returns a session by its public ID.
func (s *Store) Get(sessionID string) (*Session, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return nil, false
	}
	return sess, true
}

// List returns all sessions.
func (s *Store) List() []Session {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, *sess)
	}
	return out
}

// Revoke marks a session as revoked.
func (s *Store) Revoke(sessionID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return false
	}
	sess.Status = StatusRevoked
	s.saveLocked()
	return true
}

// Rotate invalidates the current token and returns a new one.
func (s *Store) Rotate(sessionID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return "", fmt.Errorf("session not found")
	}
	if sess.Status != StatusActive {
		return "", fmt.Errorf("session is %s", sess.Status)
	}

	token, err := generateToken()
	if err != nil {
		return "", err
	}
	sess.TokenHash = hashToken(token)
	sess.LastSeenAt = s.now()
	s.saveLocked()
	return token, nil
}

// UpdateConfig applies new configuration.
func (s *Store) UpdateConfig(cfg Config) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.applyConfig(cfg)
	s.pruneExpiredLocked()
	s.saveLocked()
	s.mu.Unlock()
}

// Enabled reports whether agent sessions are enabled.
func (s *Store) Enabled() bool {
	if s == nil {
		return false
	}
	return s.cfg.Enabled
}

// Mode returns the current auth mode.
func (s *Store) Mode() string {
	if s == nil {
		return "off"
	}
	return s.cfg.Mode
}

func (s *Store) isExpired(sess *Session, now time.Time) bool {
	if !sess.ExpiresAt.IsZero() && now.After(sess.ExpiresAt) {
		return true
	}
	if s.cfg.IdleTimeout > 0 && now.Sub(sess.LastSeenAt) > s.cfg.IdleTimeout {
		return true
	}
	return false
}

func (s *Store) pruneExpiredLocked() {
	now := s.now()
	for id, sess := range s.sessions {
		if sess.Status == StatusRevoked {
			delete(s.sessions, id)
			continue
		}
		if s.isExpired(sess, now) {
			delete(s.sessions, id)
		}
	}
}

// persistence types

type persistedStore struct {
	SavedAt  time.Time               `json:"savedAt"`
	Sessions []persistedAgentSession `json:"sessions"`
}

type persistedAgentSession struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agentId"`
	Label      string    `json:"label,omitempty"`
	TokenHash  string    `json:"tokenHash"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time `json:"expiresAt,omitempty"`
	Status     string    `json:"status"`
	Grants     []string  `json:"grants,omitempty"`
}

func (s *Store) loadPersisted() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.PersistPath == "" {
		return
	}

	data, err := os.ReadFile(s.cfg.PersistPath)
	if err != nil {
		return
	}
	var persisted persistedStore
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}

	now := s.now()
	for _, rec := range persisted.Sessions {
		tokenHash, err := hex.DecodeString(strings.TrimSpace(rec.TokenHash))
		if err != nil || len(tokenHash) != sha256.Size {
			continue
		}
		var hash [32]byte
		copy(hash[:], tokenHash)

		sess := &Session{
			ID:          rec.ID,
			AgentID:     rec.AgentID,
			Label:       rec.Label,
			TokenHash:   hash,
			CreatedAt:   rec.CreatedAt,
			LastSeenAt:  rec.LastSeenAt,
			ExpiresAt:   rec.ExpiresAt,
			IdleTimeout: s.cfg.IdleTimeout,
			Status:      rec.Status,
			Grants:      rec.Grants,
		}
		if sess.Status != StatusActive {
			continue
		}
		if s.isExpired(sess, now) {
			continue
		}
		s.sessions[sess.ID] = sess
	}
}

func (s *Store) saveLocked() {
	if s.cfg.PersistPath == "" {
		return
	}

	snapshot := persistedStore{
		SavedAt:  s.now().UTC(),
		Sessions: make([]persistedAgentSession, 0, len(s.sessions)),
	}
	for _, sess := range s.sessions {
		snapshot.Sessions = append(snapshot.Sessions, persistedAgentSession{
			ID:         sess.ID,
			AgentID:    sess.AgentID,
			Label:      sess.Label,
			TokenHash:  hex.EncodeToString(sess.TokenHash[:]),
			CreatedAt:  sess.CreatedAt,
			LastSeenAt: sess.LastSeenAt,
			ExpiresAt:  sess.ExpiresAt,
			Status:     sess.Status,
			Grants:     sess.Grants,
		})
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.cfg.PersistPath), 0755); err != nil {
		return
	}
	// Atomic write: temp file + rename
	tmpPath := s.cfg.PersistPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmpPath, s.cfg.PersistPath)
}

func generateSessionID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "ses_" + hex.EncodeToString(buf), nil
}

func generateToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "ses_" + hex.EncodeToString(buf), nil
}

func hashToken(token string) [32]byte {
	return sha256.Sum256([]byte(strings.TrimSpace(token)))
}
