package bridge

import (
	"sync"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/sanitize"
)

const (
	defaultConsoleLogMaxLines = 1000
	maxConsoleLogStoreLines   = 1000
	maxConsoleLevelBytes      = 32
	maxConsoleMessageBytes    = 4 * 1024
	maxConsoleSourceBytes     = 512
	maxErrorMessageBytes      = 4 * 1024
	maxErrorTypeBytes         = 128
	maxErrorURLBytes          = 2 * 1024
	maxErrorStackBytes        = 8 * 1024
)

// TabLogs holds console and error logs for a single tab.
type TabLogs struct {
	Console []types.ConsoleLogEntry
	Errors  []types.ErrorLogEntry
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
		maxLines = defaultConsoleLogMaxLines
	} else if maxLines > maxConsoleLogStoreLines {
		maxLines = maxConsoleLogStoreLines
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
		Console: make([]types.ConsoleLogEntry, 0),
		Errors:  make([]types.ErrorLogEntry, 0),
	}
	s.tabs[tabID] = t
	return t
}

// AddConsoleLog adds a console log entry for a tab.
func (s *ConsoleLogStore) AddConsoleLog(tabID string, entry types.ConsoleLogEntry) {
	t := s.getOrCreateTab(tabID)
	entry = normalizeConsoleLogEntry(entry)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Console = append(t.Console, entry)
	if len(t.Console) > s.maxLines {
		t.Console = t.Console[len(t.Console)-s.maxLines:]
	}
}

// AddErrorLog adds an error log entry for a tab.
func (s *ConsoleLogStore) AddErrorLog(tabID string, entry types.ErrorLogEntry) {
	t := s.getOrCreateTab(tabID)
	entry = normalizeErrorLogEntry(entry)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Errors = append(t.Errors, entry)
	if len(t.Errors) > s.maxLines {
		t.Errors = t.Errors[len(t.Errors)-s.maxLines:]
	}
}

// GetConsoleLogs returns console logs for a tab. If limit > 0, returns at most limit entries.
func (s *ConsoleLogStore) GetConsoleLogs(tabID string, limit int) []types.ConsoleLogEntry {
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
	start := count - limit
	result := make([]types.ConsoleLogEntry, limit)
	copy(result, t.Console[start:])
	return result
}

// GetErrorLogs returns error logs for a tab. If limit > 0, returns at most limit entries.
func (s *ConsoleLogStore) GetErrorLogs(tabID string, limit int) []types.ErrorLogEntry {
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
	result := make([]types.ErrorLogEntry, limit)
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

func normalizeConsoleLogEntry(entry types.ConsoleLogEntry) types.ConsoleLogEntry {
	entry.Level = sanitize.TruncateUTF8BytesWithEllipsis(entry.Level, maxConsoleLevelBytes)
	entry.Message = sanitize.TruncateUTF8BytesWithEllipsis(entry.Message, maxConsoleMessageBytes)
	entry.Source = sanitize.TruncateUTF8BytesWithEllipsis(entry.Source, maxConsoleSourceBytes)
	return entry
}

func normalizeErrorLogEntry(entry types.ErrorLogEntry) types.ErrorLogEntry {
	entry.Message = sanitize.TruncateUTF8BytesWithEllipsis(entry.Message, maxErrorMessageBytes)
	entry.Type = sanitize.TruncateUTF8BytesWithEllipsis(entry.Type, maxErrorTypeBytes)
	entry.URL = sanitize.TruncateUTF8BytesWithEllipsis(entry.URL, maxErrorURLBytes)
	entry.Stack = sanitize.TruncateUTF8BytesWithEllipsis(entry.Stack, maxErrorStackBytes)
	return entry
}
