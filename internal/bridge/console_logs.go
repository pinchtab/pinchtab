package bridge

import (
	"sync"
	"time"
)

// LogEntry represents a single console log entry.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Source    string    `json:"source,omitempty"`
}

// ErrorEntry represents an uncaught error/exception.
type ErrorEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Type      string    `json:"type,omitempty"`
	URL       string    `json:"url,omitempty"`
	Line      int64     `json:"line,omitempty"`
	Column    int64     `json:"column,omitempty"`
	Stack     string    `json:"stack,omitempty"`
}

// TabLogs holds console and error logs for a single tab.
type TabLogs struct {
	Console []LogEntry
	Errors  []ErrorEntry
	mu      sync.RWMutex
}

// ConsoleLogStore manages console/error logs across all tabs.
type ConsoleLogStore struct {
	tabs     map[string]*TabLogs
	maxLines int
	mu       sync.RWMutex
}

// NewConsoleLogStore creates a new log store with the given max lines per tab.
func NewConsoleLogStore(maxLines int) *ConsoleLogStore {
	if maxLines <= 0 {
		maxLines = 1000
	}
	return &ConsoleLogStore{
		tabs:     make(map[string]*TabLogs),
		maxLines: maxLines,
	}
}

func (s *ConsoleLogStore) getOrCreateTab(tabID string) *TabLogs {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tabs[tabID]; ok {
		return t
	}
	t := &TabLogs{
		Console: make([]LogEntry, 0),
		Errors:  make([]ErrorEntry, 0),
	}
	s.tabs[tabID] = t
	return t
}

// AddConsoleLog adds a console log entry for a tab.
func (s *ConsoleLogStore) AddConsoleLog(tabID string, entry LogEntry) {
	t := s.getOrCreateTab(tabID)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Console = append(t.Console, entry)
	if len(t.Console) > s.maxLines {
		t.Console = t.Console[len(t.Console)-s.maxLines:]
	}
}

// AddErrorLog adds an error log entry for a tab.
func (s *ConsoleLogStore) AddErrorLog(tabID string, entry ErrorEntry) {
	t := s.getOrCreateTab(tabID)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Errors = append(t.Errors, entry)
	if len(t.Errors) > s.maxLines {
		t.Errors = t.Errors[len(t.Errors)-s.maxLines:]
	}
}

// GetConsoleLogs returns console logs for a tab. If limit > 0, returns at most limit entries.
func (s *ConsoleLogStore) GetConsoleLogs(tabID string, limit int) []LogEntry {
	s.mu.RLock()
	t, ok := s.tabs[tabID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	count := len(t.Console)
	if limit <= 0 || limit > count {
		limit = count
	}
	if limit > s.maxLines {
		limit = s.maxLines
	}
	// Return most recent entries
	start := count - limit
	result := make([]LogEntry, limit)
	copy(result, t.Console[start:])
	return result
}

// GetErrorLogs returns error logs for a tab. If limit > 0, returns at most limit entries.
func (s *ConsoleLogStore) GetErrorLogs(tabID string, limit int) []ErrorEntry {
	s.mu.RLock()
	t, ok := s.tabs[tabID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	count := len(t.Errors)
	if limit <= 0 || limit > count {
		limit = count
	}
	if limit > s.maxLines {
		limit = s.maxLines
	}
	start := count - limit
	result := make([]ErrorEntry, limit)
	copy(result, t.Errors[start:])
	return result
}

// ClearConsoleLogs clears console logs for a tab.
func (s *ConsoleLogStore) ClearConsoleLogs(tabID string) {
	s.mu.RLock()
	t, ok := s.tabs[tabID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	t.mu.Lock()
	t.Console = t.Console[:0]
	t.mu.Unlock()
}

// ClearErrorLogs clears error logs for a tab.
func (s *ConsoleLogStore) ClearErrorLogs(tabID string) {
	s.mu.RLock()
	t, ok := s.tabs[tabID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	t.mu.Lock()
	t.Errors = t.Errors[:0]
	t.mu.Unlock()
}

// RemoveTab removes all logs for a tab (call on tab close).
func (s *ConsoleLogStore) RemoveTab(tabID string) {
	s.mu.Lock()
	delete(s.tabs, tabID)
	s.mu.Unlock()
}
