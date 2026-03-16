package bridge

import (
	"testing"
	"time"
)

func TestConsoleLogStore(t *testing.T) {
	store := NewConsoleLogStore(5) // Max 5 lines

	// Add 10 logs (should keep last 5)
	for i := 0; i < 10; i++ {
		store.AddConsoleLog("test_tab", LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "log " + string(rune(i)),
		})
	}

	logs := store.GetConsoleLogs("test_tab", 0)
	if len(logs) != 5 {
		t.Fatalf("expected 5 logs, got %d", len(logs))
	}

	// Limit parameter tests
	logs = store.GetConsoleLogs("test_tab", 2)
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs via limit param, got %d", len(logs))
	}

	// Add errors limit test
	for i := 0; i < 7; i++ {
		store.AddErrorLog("test_tab", ErrorEntry{
			Timestamp: time.Now(),
			Message:   "error " + string(rune(i)),
		})
	}

	errors := store.GetErrorLogs("test_tab", 0)
	if len(errors) != 5 { // Since max was initialized to 5
		t.Fatalf("expected 5 errors, got %d", len(errors))
	}

	store.ClearConsoleLogs("test_tab")
	logs = store.GetConsoleLogs("test_tab", 0)
	if len(logs) != 0 {
		t.Fatalf("expected 0 logs after clear, got %d", len(logs))
	}

	store.RemoveTab("test_tab")
	logs = store.GetConsoleLogs("test_tab", 0)
	if logs != nil {
		t.Fatalf("expected nil list for removed tab")
	}
}
