// Package session provides durable, revocable session-based
// authentication for automated clients. Each session maps a high-entropy
// token to an agentId, allowing agents to authenticate with a single
// environment variable (PINCHTAB_SESSION) instead of the server bearer token.
package session

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

// Session represents a durable, revocable authenticated session.
type Session struct {
	ID          string        `json:"id"`
	AgentID     string        `json:"agentId"`
	Label       string        `json:"label,omitempty"`
	Browser     string        `json:"browser,omitempty"`
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

// LifecycleEvent describes a session-state transition that downstream
// components (orchestrator binding eviction, instance current-tab cleanup)
// may want to react to.
type LifecycleEvent struct {
	SessionID string
	AgentID   string
	Reason    string // "revoked" | "expired" | "pruned"
}

// LifecycleHook receives events after a store mutation has committed.
// Hooks are invoked outside the store lock; they may do I/O but must not
// re-enter the store synchronously in a way that risks recursion.
type LifecycleHook func(LifecycleEvent)

// Store manages authenticated sessions with persistence.
type Store struct {
	mu            sync.Mutex
	sessions      map[string]*Session   // keyed by session ID
	byTokenHash   map[[32]byte]*Session // secondary index: token hash → session (mirrors `sessions`)
	cfg           Config
	now           func() time.Time
	lastTouchSave time.Time // last time a LastSeen-only update was flushed (debounce gate)

	// Persistence is split off the data lock: a snapshot is built under mu (with
	// a monotonic saveSeq), then marshalled + written under saveMu so routine
	// session traffic isn't serialized behind disk I/O. writtenSeq (guarded by
	// saveMu) lets a writer skip a snapshot older than one already on disk, so a
	// stale snapshot can never clobber a fresher one.
	saveMu     sync.Mutex
	saveSeq    uint64 // guarded by mu
	writtenSeq uint64 // guarded by saveMu

	// hooksMu protects lifecycleHooks. Held only for very short reads /
	// writes so it never blocks anything else. Separate from `mu` so a
	// caller adding a hook never serializes against in-flight mutations.
	hooksMu        sync.RWMutex
	lifecycleHooks []LifecycleHook
}

// touchPersistInterval bounds how often a LastSeen-only update is flushed to
// disk; in-memory LastSeenAt is always current, so this only delays durability
// of the idle-timeout clock by < interval (negligible vs the multi-day idle timeout).
const touchPersistInterval = 30 * time.Second

const (
	DefaultIdleTimeout = 7 * 24 * time.Hour
	DefaultMaxLifetime = 30 * 24 * time.Hour

	StatusActive  = "active"
	StatusRevoked = "revoked"
	StatusExpired = "expired"

	LifecycleReasonRevoked = "revoked"
	LifecycleReasonExpired = "expired"
	LifecycleReasonPruned  = "pruned"
)

// OnLifecycle registers a hook for session lifecycle events. Hooks run
// outside the store lock after a mutation has committed, so they may
// perform I/O without blocking other store operations. Multiple hooks
// run concurrently in separate goroutines.
func (s *Store) OnLifecycle(fn LifecycleHook) {
	if s == nil || fn == nil {
		return
	}
	s.hooksMu.Lock()
	s.lifecycleHooks = append(s.lifecycleHooks, fn)
	s.hooksMu.Unlock()
}

// dispatchLifecycle fires the given events to every registered hook, each hook
// in its own goroutine. Must be called after the store lock has been released —
// never under s.mu.
func (s *Store) dispatchLifecycle(events []LifecycleEvent) {
	if s == nil || len(events) == 0 {
		return
	}
	s.hooksMu.RLock()
	hooks := make([]LifecycleHook, len(s.lifecycleHooks))
	copy(hooks, s.lifecycleHooks)
	s.hooksMu.RUnlock()
	if len(hooks) == 0 {
		return
	}
	// One goroutine per hook (not per event × hook): bounds a revoke/prune burst
	// to the small, fixed number of hooks and delivers events to each hook in
	// order. Hooks still run concurrently with one another. events is built fresh
	// per call and not mutated after dispatch, so the goroutines share it read-only.
	for _, fn := range hooks {
		fn := fn
		go func() {
			for _, evt := range events {
				fn(evt)
			}
		}()
	}
}

// NewStore creates a new session store.
func NewStore(cfg Config) *Store {
	s := &Store{
		sessions:    make(map[string]*Session),
		byTokenHash: make(map[[32]byte]*Session),
		now:         time.Now,
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

// Create generates a new session and returns the session ID and
// plaintext token. The token is returned exactly once and is never stored.
func (s *Store) Create(agentID, label, browser string) (sessionID, sessionToken string, err error) {
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
		Browser:     strings.TrimSpace(browser),
		TokenHash:   hashToken(token),
		CreatedAt:   now,
		LastSeenAt:  now,
		ExpiresAt:   now.Add(s.cfg.MaxLifetime),
		IdleTimeout: s.cfg.IdleTimeout,
		Status:      StatusActive,
	}

	s.mu.Lock()
	s.sessions[id] = session
	s.byTokenHash[session.TokenHash] = session
	job, persist := s.snapshotLocked()
	s.mu.Unlock()
	if persist {
		s.writeSnapshot(job)
	}

	return id, token, nil
}

// Authenticate validates a token and returns the associated session.
// It updates LastSeenAt on success.
func (s *Store) Authenticate(token string) (*Session, bool) {
	return s.authenticate(token, true)
}

// AuthenticateWithoutTouch validates a token without updating LastSeenAt.
func (s *Store) AuthenticateWithoutTouch(token string) (*Session, bool) {
	return s.authenticate(token, false)
}

func (s *Store) authenticate(token string, touch bool) (*Session, bool) {
	if s == nil {
		return nil, false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}

	hash := hashToken(token)
	now := s.now()

	var (
		match      *Session
		ok         bool
		expiredEvt *LifecycleEvent
		job        snapshotJob
		persist    bool
	)

	func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		sess, found := s.byTokenHash[hash]
		if !found || sess.Status != StatusActive {
			return
		}
		// Defense-in-depth: the map key already matched, but keep a constant-time
		// compare on the single candidate so the final match path is timing-safe.
		if subtle.ConstantTimeCompare(hash[:], sess.TokenHash[:]) != 1 {
			return
		}
		if s.isExpired(sess, now) {
			sess.Status = StatusExpired
			job, persist = s.snapshotLocked()
			expiredEvt = &LifecycleEvent{SessionID: sess.ID, AgentID: sess.AgentID, Reason: LifecycleReasonExpired}
			return
		}
		if touch {
			sess.LastSeenAt = now
			job, persist = s.maybeSnapshotTouchLocked(now)
		}
		match = sess
		ok = true
	}()

	if persist {
		s.writeSnapshot(job)
	}
	if expiredEvt != nil {
		s.dispatchLifecycle([]LifecycleEvent{*expiredEvt})
	}
	return match, ok
}

// Touch updates LastSeenAt for an active, unexpired session.
func (s *Store) Touch(sessionID string) bool {
	if s == nil {
		return false
	}

	now := s.now()

	s.mu.Lock()

	sess, ok := s.sessions[strings.TrimSpace(sessionID)]
	if !ok || sess.Status != StatusActive {
		s.mu.Unlock()
		return false
	}
	if s.isExpired(sess, now) {
		sess.Status = StatusExpired
		job, persist := s.snapshotLocked()
		s.mu.Unlock()
		if persist {
			s.writeSnapshot(job)
		}
		return false
	}
	sess.LastSeenAt = now
	job, persist := s.maybeSnapshotTouchLocked(now)
	s.mu.Unlock()
	if persist {
		s.writeSnapshot(job)
	}
	return true
}

// Get returns a defensive copy of a session by its public ID. Callers must not
// be able to mutate store-owned state outside the store lock, so the Grants
// slice is cloned rather than aliased.
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
	cp := *sess
	cp.Grants = append([]string(nil), sess.Grants...)
	return &cp, true
}

// List returns defensive copies of all sessions. Each element's Grants slice is
// cloned so callers cannot mutate store-owned state through the returned values.
func (s *Store) List() []Session {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		cp := *sess
		cp.Grants = append([]string(nil), sess.Grants...)
		out = append(out, cp)
	}
	return out
}

// SetGrants replaces a session's capability grants and persists the change. The
// input is cloned so the store owns the slice. Returns false if no such session.
func (s *Store) SetGrants(sessionID string, grants []string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	sess, ok := s.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		s.mu.Unlock()
		return false
	}
	sess.Grants = append([]string(nil), grants...)
	job, persist := s.snapshotLocked()
	s.mu.Unlock()
	if persist {
		s.writeSnapshot(job)
	}
	return true
}

// Revoke marks a session as revoked.
func (s *Store) Revoke(sessionID string) bool {
	if s == nil {
		return false
	}

	var (
		event   LifecycleEvent
		job     snapshotJob
		persist bool
	)
	revoked := func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()

		sess, ok := s.sessions[strings.TrimSpace(sessionID)]
		if !ok {
			return false
		}
		sess.Status = StatusRevoked
		job, persist = s.snapshotLocked()
		event = LifecycleEvent{SessionID: sess.ID, AgentID: sess.AgentID, Reason: LifecycleReasonRevoked}
		return true
	}()
	if !revoked {
		return false
	}
	if persist {
		s.writeSnapshot(job)
	}
	s.dispatchLifecycle([]LifecycleEvent{event})
	return true
}

// UpdateConfig applies new configuration.
func (s *Store) UpdateConfig(cfg Config) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.applyConfig(cfg)
	events := s.pruneExpiredLocked()
	job, persist := s.snapshotLocked()
	s.mu.Unlock()
	if persist {
		s.writeSnapshot(job)
	}
	s.dispatchLifecycle(events)
}

// Enabled reports whether session auth is enabled.
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

// pruneExpiredLocked removes revoked/expired sessions. Caller must hold
// s.mu. Returns lifecycle events the caller is responsible for dispatching
// AFTER releasing the lock.
func (s *Store) pruneExpiredLocked() []LifecycleEvent {
	now := s.now()
	var events []LifecycleEvent
	for id, sess := range s.sessions {
		if sess.Status == StatusRevoked {
			delete(s.sessions, id)
			delete(s.byTokenHash, sess.TokenHash)
			events = append(events, LifecycleEvent{SessionID: sess.ID, AgentID: sess.AgentID, Reason: LifecycleReasonPruned})
			continue
		}
		if s.isExpired(sess, now) {
			delete(s.sessions, id)
			delete(s.byTokenHash, sess.TokenHash)
			events = append(events, LifecycleEvent{SessionID: sess.ID, AgentID: sess.AgentID, Reason: LifecycleReasonPruned})
		}
	}
	return events
}

type persistedStore struct {
	SavedAt  time.Time          `json:"savedAt"`
	Sessions []persistedSession `json:"sessions"`
}

type persistedSession struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agentId"`
	Label      string    `json:"label,omitempty"`
	Browser    string    `json:"browser,omitempty"`
	TokenHash  string    `json:"tokenHash"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time `json:"expiresAt,omitempty"`
	Status     string    `json:"status"`
	Grants     []string  `json:"grants,omitempty"`
}

// toPersisted maps an in-memory Session to its on-disk record.
func (sess *Session) toPersisted() persistedSession {
	return persistedSession{
		ID:         sess.ID,
		AgentID:    sess.AgentID,
		Label:      sess.Label,
		Browser:    sess.Browser,
		TokenHash:  hex.EncodeToString(sess.TokenHash[:]),
		CreatedAt:  sess.CreatedAt,
		LastSeenAt: sess.LastSeenAt,
		ExpiresAt:  sess.ExpiresAt,
		Status:     sess.Status,
		// Clone Grants: snapshots are marshalled outside s.mu, so the record must
		// not alias store-owned slices that a concurrent SetGrants could mutate.
		Grants: append([]string(nil), sess.Grants...),
	}
}

// toSession maps an on-disk record back to an in-memory Session, decoding and
// validating the token hash. ok=false means the record is malformed and should
// be skipped. idleTimeout is injected from store config (not persisted).
func (rec persistedSession) toSession(idleTimeout time.Duration) (*Session, bool) {
	tokenHash, err := hex.DecodeString(strings.TrimSpace(rec.TokenHash))
	if err != nil || len(tokenHash) != sha256.Size {
		return nil, false
	}
	var hash [32]byte
	copy(hash[:], tokenHash)

	return &Session{
		ID:          rec.ID,
		AgentID:     rec.AgentID,
		Label:       rec.Label,
		Browser:     rec.Browser,
		TokenHash:   hash,
		CreatedAt:   rec.CreatedAt,
		LastSeenAt:  rec.LastSeenAt,
		ExpiresAt:   rec.ExpiresAt,
		IdleTimeout: idleTimeout,
		Status:      rec.Status,
		Grants:      append([]string(nil), rec.Grants...),
	}, true
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
		sess, ok := rec.toSession(s.cfg.IdleTimeout)
		if !ok {
			continue
		}
		if sess.Status != StatusActive {
			continue
		}
		if s.isExpired(sess, now) {
			continue
		}
		s.sessions[sess.ID] = sess
		s.byTokenHash[sess.TokenHash] = sess
	}
}

// snapshotJob is a self-contained persistence snapshot stamped with a sequence,
// built under s.mu and written outside it.
type snapshotJob struct {
	snapshot persistedStore
	seq      uint64
}

// snapshotLocked builds a self-contained value-copy snapshot of every session
// and stamps it with a monotonic sequence. Caller must hold s.mu. ok=false when
// persistence is disabled (no PersistPath), in which case there is nothing to write.
func (s *Store) snapshotLocked() (snapshotJob, bool) {
	if s.cfg.PersistPath == "" {
		return snapshotJob{}, false
	}
	s.saveSeq++
	snapshot := persistedStore{
		SavedAt:  s.now().UTC(),
		Sessions: make([]persistedSession, 0, len(s.sessions)),
	}
	for _, sess := range s.sessions {
		snapshot.Sessions = append(snapshot.Sessions, sess.toPersisted())
	}
	return snapshotJob{snapshot: snapshot, seq: s.saveSeq}, true
}

// maybeSnapshotTouchLocked builds a snapshot for a LastSeen-only update at most
// once per touchPersistInterval. Caller must hold s.mu; the resulting write
// happens after the lock is released. The next real mutation's snapshot
// opportunistically flushes any debounced LastSeen for all sessions.
func (s *Store) maybeSnapshotTouchLocked(now time.Time) (snapshotJob, bool) {
	if now.Sub(s.lastTouchSave) < touchPersistInterval {
		return snapshotJob{}, false
	}
	s.lastTouchSave = now
	return s.snapshotLocked()
}

// writeSnapshot marshals and atomically writes a snapshot outside s.mu. Writers
// serialize on saveMu; a snapshot older than one already written is skipped so a
// stale snapshot can never clobber a fresher one.
func (s *Store) writeSnapshot(job snapshotJob) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if job.seq <= s.writtenSeq {
		return
	}
	s.writtenSeq = job.seq

	data, err := json.MarshalIndent(job.snapshot, "", "  ")
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
