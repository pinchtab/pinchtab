package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRecordAndQuery(t *testing.T) {
	store, err := NewStore(t.TempDir(), 30*time.Minute, 1)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	now := time.Now().UTC()
	events := []Event{
		{Timestamp: now.Add(-2 * time.Minute), Source: "server", AgentID: "cli", TabID: "tab-1", Path: "/tabs/tab-1/text", Method: "GET", Status: 200},
		{Timestamp: now.Add(-1 * time.Minute), Source: "bridge", AgentID: "mcp", TabID: "tab-2", Path: "/tabs/tab-2/action", Method: "POST", Status: 200},
	}
	for _, evt := range events {
		if err := store.Record(evt); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	got, err := store.Query(Filter{TabID: "tab-2", Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].AgentID != "mcp" {
		t.Fatalf("AgentID = %q, want mcp", got[0].AgentID)
	}
}

func TestStoreWritesJSONLFile(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, 30*time.Minute, 1)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Record(Event{
		Timestamp: now,
		Source:    "server",
		Method:    "GET",
		Path:      "/health",
		Status:    200,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	path := filepath.Join(root, "activity", "events-"+now.Format(time.DateOnly)+".jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("activity log missing: %v", err)
	}
}

func TestStorePrunesExpiredDailyFiles(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, 30*time.Minute, 1)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	oldDay := time.Now().UTC().AddDate(0, 0, -1)
	if err := store.Record(Event{
		Timestamp: oldDay,
		Source:    "server",
		Method:    "GET",
		Path:      "/old",
		Status:    200,
	}); err != nil {
		t.Fatalf("Record old: %v", err)
	}
	if err := store.Record(Event{
		Timestamp: time.Now().UTC(),
		Source:    "server",
		Method:    "GET",
		Path:      "/new",
		Status:    200,
	}); err != nil {
		t.Fatalf("Record new: %v", err)
	}

	oldPath := filepath.Join(root, "activity", "events-"+oldDay.Format(time.DateOnly)+".jsonl")
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old activity file to be pruned, stat err = %v", err)
	}
}

func TestNewStorePrunesExpiredDailyFilesOnStartup(t *testing.T) {
	root := t.TempDir()
	activityDir := filepath.Join(root, "activity")
	if err := os.MkdirAll(activityDir, 0750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	oldDay := time.Now().UTC().AddDate(0, 0, -31)
	oldPath := filepath.Join(activityDir, "events-"+oldDay.Format(time.DateOnly)+".jsonl")
	if err := os.WriteFile(oldPath, []byte("{\"path\":\"/old\"}\n"), 0600); err != nil {
		t.Fatalf("WriteFile old: %v", err)
	}

	keepDay := time.Now().UTC()
	keepPath := filepath.Join(activityDir, "events-"+keepDay.Format(time.DateOnly)+".jsonl")
	if err := os.WriteFile(keepPath, []byte("{\"path\":\"/new\"}\n"), 0600); err != nil {
		t.Fatalf("WriteFile keep: %v", err)
	}

	if _, err := NewStore(root, 30*time.Minute, 30); err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected expired activity file to be pruned on startup, stat err = %v", err)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected current activity file to remain, stat err = %v", err)
	}
}

func TestNewRecorderDisabledReturnsNoop(t *testing.T) {
	rec, err := NewRecorder(Config{}, t.TempDir())
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	if rec.Enabled() {
		t.Fatal("expected disabled recorder")
	}
}

func TestNewStoreRejectsZeroRetentionDays(t *testing.T) {
	if _, err := NewStore(t.TempDir(), 30*time.Minute, 0); err == nil {
		t.Fatal("expected NewStore to reject zero retentionDays")
	}
}

func TestClampQueryLimit(t *testing.T) {
	if got := clampQueryLimit(0); got != defaultQueryLimit {
		t.Fatalf("clampQueryLimit(0) = %d, want %d", got, defaultQueryLimit)
	}
	if got := clampQueryLimit(maxQueryLimit + 1); got != maxQueryLimit {
		t.Fatalf("clampQueryLimit(max+1) = %d, want %d", got, maxQueryLimit)
	}
	if got := clampQueryLimit(25); got != 25 {
		t.Fatalf("clampQueryLimit(25) = %d, want 25", got)
	}
}

func TestStoreRecord_SanitizesURLBeforePersisting(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, 30*time.Minute, 1)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	now := time.Now().UTC()
	if err := store.Record(Event{
		Timestamp: now,
		Source:    "server",
		Method:    "GET",
		Path:      "/navigate",
		Status:    200,
		URL:       "https://user:pass@example.com/callback?code=secret#done",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	path := filepath.Join(root, "activity", "events-"+now.Format(time.DateOnly)+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var evt Event
	if err := json.Unmarshal(data[:len(data)-1], &evt); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if evt.URL != "https://example.com/callback" {
		t.Fatalf("evt.URL = %q, want sanitized URL", evt.URL)
	}
}
