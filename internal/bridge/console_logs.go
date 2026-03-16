package bridge

import (
	"sync"
	"time"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Source    string    `json:"source,omitempty"`
}

type ErrorEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Type      string    `json:"type,omitempty"`
	URL       string    `json:"url,omitempty"`
	Line      int64     `json:"line,omitempty"`
	Column    int64     `json:"column,omitempty"`
	Stack     string    `json:"stack,omitempty"`
}

type TabLogs struct {
	Console []LogEntry
	Errors  []ErrorEntry
	mu      sync.RWMutex
}

type ConsoleLogStore struct {
	tabs     map[string]*TabLogs
	maxLines int
	mu       sync.RWMutex
}

func NewConsoleLogStore(maxLines int) *ConsoleLogStore {
	if maxLines <= 0 {
		maxLines = 1000 // default
	}
	return &ConsoleLogStore{
		tabs:     make(map[string]*TabLogs),
		maxLines: maxLines,
	}
}

func (s *ConsoleLogStore) getTab(tabID string) *TabLogs {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tabs[tabID]
	if !ok {
		t = &TabLogs{
			Console: make([]LogEntry, 0),
			Errors:  make([]ErrorEntry, 0),
		}
		s.tabs[tabID] = t
	}
	return t
}

func (s *ConsoleLogStore) AddConsoleLog(tabID string, entry LogEntry) {
	t := s.getTab(tabID)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Console = append(t.Console, entry)
	if len(t.Console) > s.maxLines {
		t.Console = t.Console[len(t.Console)-s.maxLines:]
	}
}

func (s *ConsoleLogStore) AddErrorLog(tabID string, entry ErrorEntry) {
	t := s.getTab(tabID)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Errors = append(t.Errors, entry)
	if len(t.Errors) > s.maxLines {
		t.Errors = t.Errors[len(t.Errors)-s.maxLines:]
	}
}

func (s *ConsoleLogStore) GetConsoleLogs(tabID string, limit int) []LogEntry {
	s.mu.RLock()
	t, ok := s.tabs[tabID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	start := 0
	if limit > 0 && len(t.Console) > limit {
		start = len(t.Console) - limit
	}
	res := make([]LogEntry, len(t.Console)-start)
	copy(res, t.Console[start:])
	return res
}

func (s *ConsoleLogStore) GetErrorLogs(tabID string, limit int) []ErrorEntry {
	s.mu.RLock()
	t, ok := s.tabs[tabID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	start := 0
	if limit > 0 && len(t.Errors) > limit {
		start = len(t.Errors) - limit
	}
	res := make([]ErrorEntry, len(t.Errors)-start)
	copy(res, t.Errors[start:])
	return res
}

func (s *ConsoleLogStore) ClearConsoleLogs(tabID string) {
	s.mu.RLock()
	t, ok := s.tabs[tabID]
	s.mu.RUnlock()
	if ok {
		t.mu.Lock()
		t.Console = make([]LogEntry, 0)
		t.mu.Unlock()
	}
}

func (s *ConsoleLogStore) ClearErrorLogs(tabID string) {
	s.mu.RLock()
	t, ok := s.tabs[tabID]
	s.mu.RUnlock()
	if ok {
		t.mu.Lock()
		t.Errors = make([]ErrorEntry, 0)
		t.mu.Unlock()
	}
}

func (s *ConsoleLogStore) RemoveTab(tabID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tabs, tabID)
}
