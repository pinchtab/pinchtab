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
	"strings"
	"sync"
	"time"
)

const (
	defaultSessionIdleTimeout = 30 * time.Minute
	defaultQueryLimit         = 200
)

type Config struct {
	Enabled     bool
	SessionIdle time.Duration
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
	path             string
	sessionIdleLimit time.Duration

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
	return NewStore(stateDir, cfg.SessionIdle)
}

func NewStore(stateDir string, sessionIdle time.Duration) (*Store, error) {
	activityDir := filepath.Join(stateDir, "activity")
	if err := os.MkdirAll(activityDir, 0750); err != nil {
		return nil, fmt.Errorf("create activity dir: %w", err)
	}
	if sessionIdle <= 0 {
		sessionIdle = defaultSessionIdleTimeout
	}

	return &Store{
		path:             filepath.Join(activityDir, "events.jsonl"),
		sessionIdleLimit: sessionIdle,
		sessions:         make(map[string]sessionState),
	}, nil
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
	if evt.SessionID == "" {
		evt.SessionID = s.sessionIDLocked(evt)
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
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

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, fmt.Errorf("open activity log: %w", err)
	}
	defer func() { _ = f.Close() }()

	var events []Event
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
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan activity log: %w", err)
	}
	return events, nil
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

func randomID(prefix string) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s%x", prefix, time.Now().UnixNano()&0xffffffff)
	}
	return prefix + hex.EncodeToString(b)
}
