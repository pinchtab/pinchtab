package bridge

import (
	"sync"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/sanitize"
)

const (
	maxConsoleLogLines     = 1000
	maxConsoleLevelBytes   = 32
	maxConsoleMessageBytes = 4 * 1024
	maxConsoleSourceBytes  = 512
	maxErrorMessageBytes   = 4 * 1024
	maxErrorTypeBytes      = 128
	maxErrorURLBytes       = 2 * 1024
	maxErrorStackBytes     = 8 * 1024
)

type logBuffer[T any] struct {
	mu       sync.RWMutex
	entries  []T
	maxLines int
}

func (b *logBuffer[T]) add(entry T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, entry)
	if len(b.entries) > b.maxLines {
		b.entries = b.entries[len(b.entries)-b.maxLines:]
	}
}

func (b *logBuffer[T]) tail(limit int) []T {
	b.mu.RLock()
	defer b.mu.RUnlock()
	count := len(b.entries)
	if limit <= 0 || limit > count {
		limit = count
	}
	result := make([]T, limit)
	copy(result, b.entries[count-limit:])
	return result
}

func (b *logBuffer[T]) clear() {
	b.mu.Lock()
	b.entries = b.entries[:0]
	b.mu.Unlock()
}

type tabLogs struct {
	console logBuffer[types.ConsoleLogEntry]
	errors  logBuffer[types.ErrorLogEntry]
}

type ConsoleLogStore struct {
	tabs     map[string]*tabLogs
	maxLines int
	mu       sync.RWMutex
}

func NewConsoleLogStore(maxLines int) *ConsoleLogStore {
	if maxLines <= 0 || maxLines > maxConsoleLogLines {
		maxLines = maxConsoleLogLines
	}
	return &ConsoleLogStore{
		tabs:     make(map[string]*tabLogs),
		maxLines: maxLines,
	}
}

func (s *ConsoleLogStore) getOrCreateTab(tabID string) *tabLogs {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tabs[tabID]; ok {
		return t
	}
	t := &tabLogs{}
	t.console.maxLines = s.maxLines
	t.errors.maxLines = s.maxLines
	s.tabs[tabID] = t
	return t
}

func (s *ConsoleLogStore) tab(tabID string) (*tabLogs, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tabs[tabID]
	return t, ok
}

func (s *ConsoleLogStore) AddConsoleLog(tabID string, entry types.ConsoleLogEntry) {
	s.getOrCreateTab(tabID).console.add(normalizeConsoleLogEntry(entry))
}

func (s *ConsoleLogStore) AddErrorLog(tabID string, entry types.ErrorLogEntry) {
	s.getOrCreateTab(tabID).errors.add(normalizeErrorLogEntry(entry))
}

func (s *ConsoleLogStore) GetConsoleLogs(tabID string, limit int) []types.ConsoleLogEntry {
	if t, ok := s.tab(tabID); ok {
		return t.console.tail(limit)
	}
	return nil
}

func (s *ConsoleLogStore) GetErrorLogs(tabID string, limit int) []types.ErrorLogEntry {
	if t, ok := s.tab(tabID); ok {
		return t.errors.tail(limit)
	}
	return nil
}

func (s *ConsoleLogStore) ClearConsoleLogs(tabID string) {
	if t, ok := s.tab(tabID); ok {
		t.console.clear()
	}
}

func (s *ConsoleLogStore) ClearErrorLogs(tabID string) {
	if t, ok := s.tab(tabID); ok {
		t.errors.clear()
	}
}

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
