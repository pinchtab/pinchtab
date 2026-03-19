package bridge

import (
	"testing"
	"time"
)

func TestConsoleLogStore_AddAndGet(t *testing.T) {
	store := NewConsoleLogStore(100)

	// Add console logs
	store.AddConsoleLog("tab1", LogEntry{
		Timestamp: time.Now(),
		Level:     "log",
		Message:   "Hello world",
	})
	store.AddConsoleLog("tab1", LogEntry{
		Timestamp: time.Now(),
		Level:     "warn",
		Message:   "Warning message",
	})

	logs := store.GetConsoleLogs("tab1", 0)
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}
	if logs[0].Level != "log" {
		t.Errorf("expected first log level 'log', got '%s'", logs[0].Level)
	}
}

func TestConsoleLogStore_AddAndGetErrors(t *testing.T) {
	store := NewConsoleLogStore(100)

	store.AddErrorLog("tab1", ErrorEntry{
		Timestamp: time.Now(),
		Message:   "Uncaught ReferenceError: x is not defined",
		URL:       "http://example.com/script.js",
		Line:      42,
		Column:    10,
	})

	errors := store.GetErrorLogs("tab1", 0)
	if len(errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(errors))
	}
	if errors[0].Line != 42 {
		t.Errorf("expected line 42, got %d", errors[0].Line)
	}
}

func TestConsoleLogStore_Limit(t *testing.T) {
	store := NewConsoleLogStore(5)

	// Add more than max
	for i := 0; i < 10; i++ {
		store.AddConsoleLog("tab1", LogEntry{
			Timestamp: time.Now(),
			Level:     "log",
			Message:   string(rune('0' + i)),
		})
	}

	logs := store.GetConsoleLogs("tab1", 0)
	if len(logs) != 5 {
		t.Errorf("expected 5 logs (maxLines), got %d", len(logs))
	}
	// Should have the last 5 entries (5,6,7,8,9)
	if logs[0].Message != "5" {
		t.Errorf("expected first message '5', got '%s'", logs[0].Message)
	}
}

func TestConsoleLogStore_GetWithLimit(t *testing.T) {
	store := NewConsoleLogStore(100)

	for i := 0; i < 10; i++ {
		store.AddConsoleLog("tab1", LogEntry{
			Timestamp: time.Now(),
			Level:     "log",
			Message:   string(rune('0' + i)),
		})
	}

	// Get only last 3
	logs := store.GetConsoleLogs("tab1", 3)
	if len(logs) != 3 {
		t.Errorf("expected 3 logs, got %d", len(logs))
	}
	// Should be 7, 8, 9
	if logs[0].Message != "7" {
		t.Errorf("expected first message '7', got '%s'", logs[0].Message)
	}
}

func TestConsoleLogStore_Clear(t *testing.T) {
	store := NewConsoleLogStore(100)

	store.AddConsoleLog("tab1", LogEntry{Level: "log", Message: "test"})
	store.AddErrorLog("tab1", ErrorEntry{Message: "error"})

	store.ClearConsoleLogs("tab1")
	logs := store.GetConsoleLogs("tab1", 0)
	if len(logs) != 0 {
		t.Errorf("expected 0 console logs after clear, got %d", len(logs))
	}

	// Errors should still be there
	errors := store.GetErrorLogs("tab1", 0)
	if len(errors) != 1 {
		t.Errorf("expected 1 error (not cleared), got %d", len(errors))
	}

	store.ClearErrorLogs("tab1")
	errors = store.GetErrorLogs("tab1", 0)
	if len(errors) != 0 {
		t.Errorf("expected 0 errors after clear, got %d", len(errors))
	}
}

func TestConsoleLogStore_RemoveTab(t *testing.T) {
	store := NewConsoleLogStore(100)

	store.AddConsoleLog("tab1", LogEntry{Level: "log", Message: "test"})
	store.AddConsoleLog("tab2", LogEntry{Level: "log", Message: "other"})

	store.RemoveTab("tab1")

	logs1 := store.GetConsoleLogs("tab1", 0)
	logs2 := store.GetConsoleLogs("tab2", 0)

	if logs1 != nil {
		t.Errorf("expected nil for removed tab, got %v", logs1)
	}
	if len(logs2) != 1 {
		t.Errorf("expected 1 log for tab2, got %d", len(logs2))
	}
}

func TestConsoleLogStore_NonexistentTab(t *testing.T) {
	store := NewConsoleLogStore(100)

	logs := store.GetConsoleLogs("nonexistent", 0)
	if logs != nil {
		t.Errorf("expected nil for nonexistent tab, got %v", logs)
	}

	errors := store.GetErrorLogs("nonexistent", 0)
	if errors != nil {
		t.Errorf("expected nil for nonexistent tab errors, got %v", errors)
	}

	// Should not panic
	store.ClearConsoleLogs("nonexistent")
	store.ClearErrorLogs("nonexistent")
	store.RemoveTab("nonexistent")
}
