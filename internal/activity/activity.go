package activity

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultSessionIdleTimeout = 30 * time.Minute
	defaultQueryLimit         = 200
	maxQueryLimit             = 1000
	defaultRetentionDays      = 30
)

type Config struct {
	Enabled       bool
	SessionIdle   time.Duration
	RetentionDays int
}

type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	Source      string    `json:"source"`
	RequestID   string    `json:"requestId,omitempty"`
	SessionID   string    `json:"sessionId,omitempty"`
	ActorID     string    `json:"actorId,omitempty"`
	AgentID     string    `json:"agentId,omitempty"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Status      int       `json:"status"`
	DurationMs  int64     `json:"durationMs"`
	RemoteAddr  string    `json:"remoteAddr,omitempty"`
	InstanceID  string    `json:"instanceId,omitempty"`
	ProfileID   string    `json:"profileId,omitempty"`
	ProfileName string    `json:"profileName,omitempty"`
	TabID       string    `json:"tabId,omitempty"`
	URL         string    `json:"url,omitempty"`
	Action      string    `json:"action,omitempty"`
	Engine      string    `json:"engine,omitempty"`
	Ref         string    `json:"ref,omitempty"`
}

type Filter struct {
	Source      string
	RequestID   string
	SessionID   string
	ActorID     string
	AgentID     string
	InstanceID  string
	ProfileID   string
	ProfileName string
	TabID       string
	Action      string
	Engine      string
	PathPrefix  string
	Since       time.Time
	Until       time.Time
	Limit       int
}

type sessionState struct {
	SessionID string
	LastSeen  time.Time
}

type Recorder interface {
	Enabled() bool
	Record(Event) error
	Query(Filter) ([]Event, error)
}

type Store struct {
	dir              string
	sessionIdleLimit time.Duration
	retentionDays    int

	mu       sync.Mutex
	sessions map[string]sessionState
}

type noopRecorder struct{}

func NewRecorder(cfg Config, stateDir string) (Recorder, error) {
	if !cfg.Enabled {
		return noopRecorder{}, nil
	}
	if cfg.SessionIdle <= 0 {
		cfg.SessionIdle = defaultSessionIdleTimeout
	}
	return NewStore(stateDir, cfg.SessionIdle, cfg.RetentionDays)
}

func NewStore(stateDir string, sessionIdle time.Duration, retentionDays int) (*Store, error) {
	activityDir := filepath.Join(stateDir, "activity")
	if err := os.MkdirAll(activityDir, 0750); err != nil {
		return nil, fmt.Errorf("create activity dir: %w", err)
	}
	if sessionIdle <= 0 {
		sessionIdle = defaultSessionIdleTimeout
	}
	if retentionDays <= 0 {
		return nil, fmt.Errorf("activity retentionDays must be > 0 (got %d)", retentionDays)
	}

	store := &Store{
		dir:              activityDir,
		sessionIdleLimit: sessionIdle,
		retentionDays:    retentionDays,
		sessions:         make(map[string]sessionState),
	}
	if err := store.pruneExpiredFiles(time.Now().UTC()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Enabled() bool {
	return s != nil
}

func (s *Store) Record(evt Event) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	} else {
		evt.Timestamp = evt.Timestamp.UTC()
	}
	evt.URL = sanitizeActivityURL(evt.URL)
	if evt.SessionID == "" {
		evt.SessionID = s.sessionIDLocked(evt)
	}

	if err := s.pruneExpiredFilesLocked(evt.Timestamp); err != nil {
		return err
	}

	f, err := os.OpenFile(s.filePathFor(evt.Timestamp), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open activity log: %w", err)
	}
	defer func() { _ = f.Close() }()

	line, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal activity event: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write activity event: %w", err)
	}
	return nil
}

func (s *Store) Query(filter Filter) ([]Event, error) {
	if s == nil {
		return nil, nil
	}

	limit := clampQueryLimit(filter.Limit)

	var events []Event
	for _, path := range s.queryFiles() {
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("open activity log: %w", err)
		}

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			var evt Event
			if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
				continue
			}
			if !filter.matches(evt) {
				continue
			}
			if len(events) < limit {
				events = append(events, evt)
				continue
			}
			copy(events, events[1:])
			events[len(events)-1] = evt
		}
		closeErr := f.Close()
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scan activity log: %w", err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close activity log: %w", closeErr)
		}
	}
	return events, nil
}

func clampQueryLimit(limit int) int {
	if limit <= 0 {
		return defaultQueryLimit
	}
	if limit > maxQueryLimit {
		return maxQueryLimit
	}
	return limit
}

func (noopRecorder) Enabled() bool {
	return false
}

func (noopRecorder) Record(Event) error {
	return nil
}

func (noopRecorder) Query(Filter) ([]Event, error) {
	return []Event{}, nil
}

func (f Filter) matches(evt Event) bool {
	if f.Source != "" && evt.Source != f.Source {
		return false
	}
	if f.RequestID != "" && evt.RequestID != f.RequestID {
		return false
	}
	if f.SessionID != "" && evt.SessionID != f.SessionID {
		return false
	}
	if f.ActorID != "" && evt.ActorID != f.ActorID {
		return false
	}
	if f.AgentID != "" && evt.AgentID != f.AgentID {
		return false
	}
	if f.InstanceID != "" && evt.InstanceID != f.InstanceID {
		return false
	}
	if f.ProfileID != "" && evt.ProfileID != f.ProfileID {
		return false
	}
	if f.ProfileName != "" && evt.ProfileName != f.ProfileName {
		return false
	}
	if f.TabID != "" && evt.TabID != f.TabID {
		return false
	}
	if f.Action != "" && evt.Action != f.Action {
		return false
	}
	if f.Engine != "" && evt.Engine != f.Engine {
		return false
	}
	if f.PathPrefix != "" && !strings.HasPrefix(evt.Path, f.PathPrefix) {
		return false
	}
	if !f.Since.IsZero() && evt.Timestamp.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && evt.Timestamp.After(f.Until) {
		return false
	}
	return true
}

func (s *Store) sessionIDLocked(evt Event) string {
	key := evt.ActorID
	if key == "" {
		key = "agent:" + evt.AgentID
	}
	if key == "" || key == "agent:" {
		return ""
	}

	now := evt.Timestamp
	prev, ok := s.sessions[key]
	if ok && now.Sub(prev.LastSeen) <= s.sessionIdleLimit {
		prev.LastSeen = now
		s.sessions[key] = prev
		return prev.SessionID
	}

	sessionID := randomID("ses_")
	s.sessions[key] = sessionState{
		SessionID: sessionID,
		LastSeen:  now,
	}
	return sessionID
}

func FingerprintToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return "tok_" + hex.EncodeToString(sum[:6])
}

func (s *Store) filePathFor(ts time.Time) string {
	return filepath.Join(s.dir, fmt.Sprintf("events-%s.jsonl", ts.UTC().Format(time.DateOnly)))
}

func (s *Store) queryFiles() []string {
	_ = s.pruneExpiredFiles(time.Now().UTC())

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}

	files := make([]string, 0, len(entries)+1)
	legacyPath := filepath.Join(s.dir, "events.jsonl")
	if _, err := os.Stat(legacyPath); err == nil {
		files = append(files, legacyPath)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "events-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		files = append(files, filepath.Join(s.dir, name))
	}

	sort.Strings(files)
	return files
}

func (s *Store) pruneExpiredFiles(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pruneExpiredFilesLocked(now)
}

func (s *Store) pruneExpiredFilesLocked(now time.Time) error {
	if s.retentionDays <= 0 {
		return nil
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read activity dir: %w", err)
	}

	keepFrom := now.UTC().AddDate(0, 0, -(s.retentionDays - 1)).Format(time.DateOnly)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "events.jsonl" {
			info, err := entry.Info()
			if err == nil && info.ModTime().UTC().Format(time.DateOnly) < keepFrom {
				if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("remove expired legacy activity log: %w", err)
				}
			}
			continue
		}
		if !strings.HasPrefix(name, "events-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, "events-"), ".jsonl")
		if len(day) != len(time.DateOnly) {
			continue
		}
		if day < keepFrom {
			if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove expired activity log: %w", err)
			}
		}
	}

	return nil
}

func randomID(prefix string) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s%x", prefix, time.Now().UnixNano()&0xffffffff)
	}
	return prefix + hex.EncodeToString(b)
}
