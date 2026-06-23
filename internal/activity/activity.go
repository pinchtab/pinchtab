package activity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/browserops"
)

const (
	defaultQueryLimit    = 200
	maxQueryLimit        = 1000
	defaultRetentionDays = 30
)

type Config struct {
	Enabled       bool
	RetentionDays int
	Events        EventSourceConfig
}

type EventSourceConfig struct {
	Dashboard    bool
	Server       bool
	Bridge       bool
	Orchestrator bool
	Scheduler    bool
	MCP          bool
	Other        bool
}

type Event struct {
	Timestamp   time.Time                 `json:"timestamp"`
	Source      string                    `json:"source"`
	RequestID   string                    `json:"requestId,omitempty"`
	SessionID   string                    `json:"sessionId,omitempty"`
	AgentID     string                    `json:"agentId,omitempty"`
	Method      string                    `json:"method"`
	Path        string                    `json:"path"`
	Status      int                       `json:"status"`
	DurationMs  int64                     `json:"durationMs"`
	RemoteAddr  string                    `json:"remoteAddr,omitempty"`
	InstanceID  string                    `json:"instanceId,omitempty"`
	ProfileID   string                    `json:"profileId,omitempty"`
	ProfileName string                    `json:"profileName,omitempty"`
	TabID       string                    `json:"tabId,omitempty"`
	URL         string                    `json:"url,omitempty"`
	Action      string                    `json:"action,omitempty"`
	Route       *browserops.RouteMetadata `json:"route,omitempty"`
	Ref         string                    `json:"ref,omitempty"`
}

type Filter struct {
	Source      string
	RequestID   string
	SessionID   string
	AgentID     string
	AgentIDLike string
	InstanceID  string
	ProfileID   string
	ProfileName string
	TabID       string
	Action      string
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
	events        EventSourceConfig

	mu            sync.Mutex
	lastPruneTime time.Time
}

// TailReader provides an incremental read interface that only scans new lines
// appended since the last call, avoiding a full-file rescan on each poll.
type TailReader struct {
	store  *Store
	source string
	now    func() time.Time // injectable clock for rollover tests; defaults to time.Now().UTC

	mu     sync.Mutex // guards the cursor below
	file   string
	offset int64
}

// NewTailReader creates a reader that efficiently tails new events for a given source.
func (s *Store) NewTailReader(source string) *TailReader {
	return &TailReader{
		store:  s,
		source: source,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// Read returns events appended since the last Read call. It seeks to the
// last known file offset rather than scanning from the beginning.
func (tr *TailReader) Read(limit int) ([]Event, error) {
	if tr.store == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultQueryLimit
	}

	source := normalizeSourceName(tr.source)
	todayPath := tr.store.sourceFilePathFor(source, tr.now())
	if todayPath == "" {
		return nil, nil
	}

	tr.mu.Lock()
	cursor := tr.file
	offset := tr.offset
	tr.mu.Unlock()
	if cursor == "" {
		// First read: tail from today's file (historical days are not back-filled).
		cursor = todayPath
		offset = 0
	}

	var events []Event
	for {
		evs, newOffset, reachedEOF, err := readLinesFrom(cursor, offset, limit-len(events), source)
		if err != nil {
			return events, err
		}
		events = append(events, evs...)
		offset = newOffset

		if len(events) >= limit {
			break // cap hit; resume on this cursor next poll
		}
		if !reachedEOF {
			break // partial/in-progress trailing line; stay on this file
		}
		if cursor == todayPath {
			break // current file is today and fully drained; caught up
		}
		// A previous day's file is fully drained — advance toward today so its
		// unread tail (and any intermediate gap days) is never skipped on rollover.
		next := tr.nextSourceFilePath(cursor, todayPath, source)
		if next == "" {
			break
		}
		cursor = next
		offset = 0
	}

	tr.mu.Lock()
	tr.file = cursor
	tr.offset = offset
	tr.mu.Unlock()

	return events, nil
}

// readLinesFrom reads complete JSONL records from path starting at startOffset,
// returning matched events (capped at limit), the new byte offset, and whether the
// file end was reached with no partial line pending. A missing file is treated as
// empty + EOF so a drained or absent day advances. Partial trailing lines are not
// consumed (re-read once complete); malformed/filtered lines are consumed.
func readLinesFrom(path string, startOffset int64, limit int, source string) (events []Event, newOffset int64, reachedEOF bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, startOffset, true, nil
		}
		return nil, startOffset, false, err
	}
	defer func() { _ = f.Close() }()

	if startOffset > 0 {
		if _, seekErr := f.Seek(startOffset, 0); seekErr != nil {
			_, _ = f.Seek(0, 0)
			startOffset = 0
		}
	}

	var consumed int64
	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			// No trailing '\n' (EOF / append in progress): `line` is a partial
			// record. Do NOT consume it, so it is re-read once complete.
			reachedEOF = true
			break
		}
		consumed += int64(len(line)) // complete line, including the '\n'
		var evt Event
		if jsonErr := json.Unmarshal([]byte(line), &evt); jsonErr != nil {
			continue // malformed line still consumed (don't re-read it)
		}
		if source != "" && evt.Source != source {
			continue // filtered line still consumed
		}
		events = append(events, evt)
		if limit > 0 && len(events) >= limit {
			// Cap reached: more may remain, so do not report EOF.
			break
		}
	}

	return events, startOffset + consumed, reachedEOF, nil
}

// nextSourceFilePath returns the next source log file to drain after cursorPath,
// walking forward one day at a time toward today. It returns the first existing
// intermediate day's file, or todayPath once the walk reaches today's day (even if
// today's file doesn't exist yet). Returns "" if cursorPath has no parseable day.
func (tr *TailReader) nextSourceFilePath(cursorPath, todayPath, source string) string {
	cursorDay, ok := activityLogDay(filepath.Base(cursorPath))
	if !ok {
		return ""
	}
	day, err := time.Parse(time.DateOnly, cursorDay)
	if err != nil {
		return ""
	}
	todayDay := tr.now().UTC().Format(time.DateOnly)

	for {
		day = day.AddDate(0, 0, 1)
		d := day.Format(time.DateOnly)
		if d >= todayDay {
			return todayPath
		}
		p := tr.store.sourceFilePathFor(source, day)
		if _, statErr := os.Stat(p); statErr == nil {
			return p // an intermediate day with recorded events
		}
		// missing intermediate day → keep walking
	}
}

type noopRecorder struct{}

func NewRecorder(cfg Config, stateDir string) (Recorder, error) {
	if !cfg.Enabled {
		return noopRecorder{}, nil
	}
	return NewStoreWithEvents(stateDir, cfg.RetentionDays, cfg.Events)
}

func NewStore(stateDir string, retentionDays int) (*Store, error) {
	return NewStoreWithEvents(stateDir, retentionDays, EventSourceConfig{
		Dashboard:    true,
		Server:       true,
		Bridge:       true,
		Orchestrator: true,
		Scheduler:    true,
		MCP:          true,
		Other:        true,
	})
}

func NewStoreWithEvents(stateDir string, retentionDays int, events EventSourceConfig) (*Store, error) {
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
		events:        events,
		lastPruneTime: time.Now().UTC(),
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

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	} else {
		evt.Timestamp = evt.Timestamp.UTC()
	}
	if !s.shouldRecordSource(evt.Source) {
		return nil
	}
	evt.URL = sanitizeActivityURL(evt.URL)

	if err := s.maybePrune(evt.Timestamp); err != nil {
		return err
	}

	// Marshal and append outside the lock: the only shared mutable state is the
	// prune bookkeeping (handled above); appendJSONL uses O_APPEND single writes,
	// which are atomic, so concurrent appenders cannot interleave. This keeps a
	// slow disk on one request path from head-of-line blocking other recorders.
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

// maybePrune holds s.mu only for the retention bookkeeping: it updates
// lastPruneTime and runs the throttled (~hourly/daily) prune under the lock so
// pruners stay serialized, while leaving the per-event marshal+append unlocked.
func (s *Store) maybePrune(ts time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ts.Sub(s.lastPruneTime) > 1*time.Hour ||
		ts.UTC().Format(time.DateOnly) != s.lastPruneTime.Format(time.DateOnly) {
		s.lastPruneTime = ts
		return s.pruneExpiredFilesLocked(ts)
	}
	return nil
}

func (s *Store) Query(filter Filter) ([]Event, error) {
	if s == nil {
		return nil, nil
	}

	limit := clampQueryLimit(filter.Limit)

	var events []Event
	head := 0 // index of the oldest event once the window is full
	full := false
	seen := make(map[string]struct{})
	for _, path := range s.queryFiles(filter) {
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
			if !full {
				events = append(events, evt)
				if len(events) == limit {
					full = true // events now holds exactly `limit`; head stays 0 until first overflow
				}
				continue
			}
			events[head] = evt
			head = (head + 1) % limit
		}
		closeErr := f.Close()
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scan activity log: %w", err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close activity log: %w", closeErr)
		}
	}
	if !full || head == 0 {
		return events, nil // not wrapped → already oldest→newest
	}
	ordered := make([]Event, limit)
	for i := 0; i < limit; i++ {
		ordered[i] = events[(head+i)%limit]
	}
	return ordered, nil
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

func (s *Store) shouldRecordSource(source string) bool {
	switch normalizeSourceName(source) {
	case "client":
		return true
	case "dashboard":
		return s.events.Dashboard
	case "server":
		return s.events.Server
	case "bridge":
		return s.events.Bridge
	case "orchestrator":
		return s.events.Orchestrator
	case "scheduler":
		return s.events.Scheduler
	case "mcp":
		return s.events.MCP
	default:
		return s.events.Other
	}
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

// dayInRange reports whether a file's day (YYYY-MM-DD) overlaps the [sinceDay,
// untilDay] window. An empty bound is unbounded; day strings sort chronologically.
func dayInRange(day, sinceDay, untilDay string) bool {
	if sinceDay != "" && day < sinceDay {
		return false
	}
	if untilDay != "" && day > untilDay {
		return false
	}
	return true
}

func (s *Store) queryFiles(filter Filter) []string {
	now := time.Now().UTC()
	s.mu.Lock()
	if now.Sub(s.lastPruneTime) > 1*time.Hour {
		s.lastPruneTime = now
		s.mu.Unlock()
		_ = s.pruneExpiredFiles(now)
	} else {
		s.mu.Unlock()
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}

	// Coarse day prefilter: a file for day d holds events in [d 00:00, d 23:59:59],
	// so it can only contain a match when sinceDay <= d <= untilDay. The per-event
	// filter.matches still applies the exact Since/Until check.
	var sinceDay, untilDay string
	if !filter.Since.IsZero() {
		sinceDay = filter.Since.UTC().Format(time.DateOnly)
	}
	if !filter.Until.IsZero() {
		untilDay = filter.Until.UTC().Format(time.DateOnly)
	}

	files := make([]string, 0, len(entries)+1)
	legacyPath := filepath.Join(s.dir, "events.jsonl")
	if _, err := os.Stat(legacyPath); err == nil {
		// Legacy log has no encoded day; include it conservatively.
		files = append(files, legacyPath)
	}

	source := normalizeSourceName(filter.Source)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isActivityLogFile(name) {
			continue
		}
		if source != "" && !isSourceLogFile(name, source) {
			continue
		}
		if day, ok := activityLogDay(name); ok && !dayInRange(day, sinceDay, untilDay) {
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

func isSourceLogFile(name, source string) bool {
	prefix := "events-" + source + "-"
	return strings.HasPrefix(name, prefix)
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

// eventDedupKey builds a cheap composite key that collapses the byte-identical
// primary + per-source dual-write copies of an event, without json.Marshal-ing the
// whole struct. A false collision would require two genuinely-distinct events
// sharing a nanosecond timestamp AND every other field — effectively impossible.
func eventDedupKey(evt Event) string {
	return strings.Join([]string{
		evt.Timestamp.UTC().Format(time.RFC3339Nano),
		evt.Source,
		evt.RequestID,
		evt.Method,
		evt.Path,
		strconv.Itoa(evt.Status),
		evt.TabID,
	}, "|")
}
