package activity

import (
	"bufio"
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
	defaultQueryLimit    = 200
	maxQueryLimit        = 1000
	defaultRetentionDays = 1
)

type Config struct {
	Enabled       bool
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

type Recorder interface {
	Enabled() bool
	Record(Event) error
	Query(Filter) ([]Event, error)
}

type Store struct {
	dir           string
	retentionDays int

	mu sync.Mutex
}

type noopRecorder struct{}

func NewRecorder(cfg Config, stateDir string) (Recorder, error) {
	if !cfg.Enabled {
		return noopRecorder{}, nil
	}
	return NewStore(stateDir, cfg.RetentionDays)
}

func NewStore(stateDir string, retentionDays int) (*Store, error) {
	activityDir := filepath.Join(stateDir, "activity")
	if err := os.MkdirAll(activityDir, 0750); err != nil {
		return nil, fmt.Errorf("create activity dir: %w", err)
	}
	if retentionDays <= 0 {
		return nil, fmt.Errorf("activity retentionDays must be > 0 (got %d)", retentionDays)
	}

	store := &Store{
		dir:           activityDir,
		retentionDays: retentionDays,
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

	if err := s.pruneExpiredFilesLocked(evt.Timestamp); err != nil {
		return err
	}

	line, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal activity event: %w", err)
	}
	if shouldWritePrimaryLog(evt.Source) {
		if err := appendJSONL(s.filePathFor(evt.Timestamp), line); err != nil {
			return err
		}
	}
	if sourcePath := s.sourceFilePathFor(evt.Source, evt.Timestamp); sourcePath != "" {
		if err := appendJSONL(sourcePath, line); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Query(filter Filter) ([]Event, error) {
	if s == nil {
		return nil, nil
	}

	limit := clampQueryLimit(filter.Limit)

	var events []Event
	seen := make(map[string]struct{})
	for _, path := range s.queryFiles(filter.Source) {
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
			key := eventDedupKey(evt)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
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

func (s *Store) sourceFilePathFor(source string, ts time.Time) string {
	source = normalizeSourceName(source)
	if source == "" {
		return ""
	}
	return filepath.Join(s.dir, fmt.Sprintf("events-%s-%s.jsonl", source, ts.UTC().Format(time.DateOnly)))
}

func (s *Store) queryFiles(source string) []string {
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

	source = normalizeSourceName(source)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isActivityLogFile(name) {
			continue
		}
		if source == "" && !isPrimaryDailyActivityLog(name) {
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
		if !isActivityLogFile(name) {
			continue
		}
		day, ok := activityLogDay(name)
		if !ok {
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

func appendJSONL(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open activity log: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write activity event: %w", err)
	}
	return nil
}

func shouldWritePrimaryLog(source string) bool {
	switch normalizeSourceName(source) {
	case "", "server", "bridge":
		return true
	default:
		return false
	}
}

func normalizeSourceName(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(source))
	lastDash := false
	for _, r := range source {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func isActivityLogFile(name string) bool {
	return name != "events.jsonl" && strings.HasPrefix(name, "events-") && strings.HasSuffix(name, ".jsonl")
}

func isPrimaryDailyActivityLog(name string) bool {
	if !isActivityLogFile(name) {
		return false
	}
	middle := strings.TrimSuffix(strings.TrimPrefix(name, "events-"), ".jsonl")
	return len(middle) == len(time.DateOnly)
}

func activityLogDay(name string) (string, bool) {
	if !isActivityLogFile(name) {
		return "", false
	}
	middle := strings.TrimSuffix(strings.TrimPrefix(name, "events-"), ".jsonl")
	if len(middle) < len(time.DateOnly) {
		return "", false
	}
	day := middle[len(middle)-len(time.DateOnly):]
	if len(day) != len(time.DateOnly) {
		return "", false
	}
	return day, true
}

func eventDedupKey(evt Event) string {
	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Sprintf("%s|%s|%s|%s|%d", evt.Timestamp.UTC().Format(time.RFC3339Nano), evt.Source, evt.Method, evt.Path, evt.Status)
	}
	return string(data)
}
