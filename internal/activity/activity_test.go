package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreRecordAndQuery(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1)
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

func TestStoreQueryFiltersByAgentID(t *testing.T) {
	store, err := NewStore(t.TempDir(), 1)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	now := time.Now().UTC()
	events := []Event{
		{Timestamp: now.Add(-2 * time.Minute), Source: "server", AgentID: "cli", Path: "/tabs/tab-1/text", Method: "GET", Status: 200},
		{Timestamp: now.Add(-1 * time.Minute), Source: "bridge", AgentID: "mcp", Path: "/tabs/tab-2/action", Method: "POST", Status: 200},
	}
	for _, evt := range events {
		if err := store.Record(evt); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	got, err := store.Query(Filter{AgentID: "cli", Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].AgentID != "cli" {
		t.Fatalf("AgentID = %q, want cli", got[0].AgentID)
	}
}

func TestStoreWritesJSONLFile(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, 1)
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

func TestStorePartitionsDashboardEventsOutsidePrimaryLog(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, 1)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	now := time.Now().UTC()
	if err := store.Record(Event{
		Timestamp: now,
		Source:    "dashboard",
		Method:    "GET",
		Path:      "/api/events",
		Status:    200,
	}); err != nil {
		t.Fatalf("Record dashboard: %v", err)
	}
	if err := store.Record(Event{
		Timestamp: now.Add(time.Second),
		Source:    "server",
		Method:    "GET",
		Path:      "/health",
		Status:    200,
	}); err != nil {
		t.Fatalf("Record server: %v", err)
	}

	mainPath := filepath.Join(root, "activity", "events-"+now.Format(time.DateOnly)+".jsonl")
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile main: %v", err)
	}
	if strings.Contains(string(mainData), "\"source\":\"dashboard\"") {
		t.Fatal("primary activity log should not include dashboard events")
	}
	if !strings.Contains(string(mainData), "\"source\":\"server\"") {
		t.Fatal("primary activity log should include server events")
	}

	dashboardPath := filepath.Join(root, "activity", "events-dashboard-"+now.Format(time.DateOnly)+".jsonl")
	dashboardData, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("ReadFile dashboard: %v", err)
	}
	if !strings.Contains(string(dashboardData), "\"source\":\"dashboard\"") {
		t.Fatal("dashboard activity log missing dashboard event")
	}

	gotAll, err := store.Query(Filter{Limit: 10})
	if err != nil {
		t.Fatalf("Query all: %v", err)
	}
	if len(gotAll) != 2 {
		t.Fatalf("unfiltered query = %d events, want 2 (server + dashboard)", len(gotAll))
	}

	gotDashboard, err := store.Query(Filter{Source: "dashboard", Limit: 10})
	if err != nil {
		t.Fatalf("Query dashboard: %v", err)
	}
	if len(gotDashboard) != 1 || gotDashboard[0].Source != "dashboard" {
		t.Fatalf("dashboard query = %#v, want dashboard event", gotDashboard)
	}

	gotServer, err := store.Query(Filter{Source: "server", Limit: 10})
	if err != nil {
		t.Fatalf("Query server: %v", err)
	}
	if len(gotServer) != 1 || gotServer[0].Source != "server" {
		t.Fatalf("server query = %#v, want one deduplicated server event", gotServer)
	}
}

func TestStoreWritesServerEventsToSourcePartitionToo(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, 1)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	now := time.Now().UTC()
	if err := store.Record(Event{
		Timestamp: now,
		Source:    "server",
		Method:    "POST",
		Path:      "/sessions",
		Status:    401,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	mainPath := filepath.Join(root, "activity", "events-"+now.Format(time.DateOnly)+".jsonl")
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile main: %v", err)
	}
	if !strings.Contains(string(mainData), "\"source\":\"server\"") {
		t.Fatal("primary activity log should include server events")
	}

	serverPath := filepath.Join(root, "activity", "events-server-"+now.Format(time.DateOnly)+".jsonl")
	serverData, err := os.ReadFile(serverPath)
	if err != nil {
		t.Fatalf("ReadFile server: %v", err)
	}
	if !strings.Contains(string(serverData), "\"source\":\"server\"") {
		t.Fatal("server partition log should include server events")
	}
}

func TestStoreWithEventsRecordsClientOnlyByDefaultPolicy(t *testing.T) {
	root := t.TempDir()
	store, err := NewStoreWithEvents(root, 1, EventSourceConfig{})
	if err != nil {
		t.Fatalf("NewStoreWithEvents: %v", err)
	}

	now := time.Now().UTC()
	for _, evt := range []Event{
		{Timestamp: now, Source: "client", Method: "GET", Path: "/text", Status: 200},
		{Timestamp: now.Add(time.Second), Source: "server", Method: "GET", Path: "/health", Status: 200},
		{Timestamp: now.Add(2 * time.Second), Source: "dashboard", Method: "GET", Path: "/api/events", Status: 200},
		{Timestamp: now.Add(3 * time.Second), Source: "orchestrator", Method: "GET", Path: "/instances", Status: 200},
	} {
		if err := store.Record(evt); err != nil {
			t.Fatalf("Record(%s): %v", evt.Source, err)
		}
	}

	gotAll, err := store.Query(Filter{Limit: 10})
	if err != nil {
		t.Fatalf("Query all: %v", err)
	}
	if len(gotAll) != 1 || gotAll[0].Source != "client" {
		t.Fatalf("unfiltered query = %#v, want single client event", gotAll)
	}

	clientPath := filepath.Join(root, "activity", "events-client-"+now.Format(time.DateOnly)+".jsonl")
	clientData, err := os.ReadFile(clientPath)
	if err != nil {
		t.Fatalf("ReadFile client: %v", err)
	}
	if !strings.Contains(string(clientData), "\"source\":\"client\"") {
		t.Fatal("client activity log should include client events")
	}
	if strings.Contains(string(clientData), "\"source\":\"server\"") ||
		strings.Contains(string(clientData), "\"source\":\"dashboard\"") ||
		strings.Contains(string(clientData), "\"source\":\"orchestrator\"") {
		t.Fatal("client activity log should exclude disabled non-client events")
	}

	for _, source := range []string{"dashboard", "server", "orchestrator"} {
		path := filepath.Join(root, "activity", "events-"+source+"-"+now.Format(time.DateOnly)+".jsonl")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s activity log should not exist, stat err = %v", source, err)
		}
	}
}

func TestStorePrunesExpiredDailyFiles(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, 1)
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

	if _, err := NewStore(root, 30); err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected expired activity file to be pruned on startup, stat err = %v", err)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected current activity file to remain, stat err = %v", err)
	}
}

func TestNewStorePrunesExpiredSourceSpecificDailyFilesOnStartup(t *testing.T) {
	root := t.TempDir()
	activityDir := filepath.Join(root, "activity")
	if err := os.MkdirAll(activityDir, 0750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	oldDay := time.Now().UTC().AddDate(0, 0, -31)
	oldPath := filepath.Join(activityDir, "events-dashboard-"+oldDay.Format(time.DateOnly)+".jsonl")
	if err := os.WriteFile(oldPath, []byte("{\"source\":\"dashboard\"}\n"), 0600); err != nil {
		t.Fatalf("WriteFile old: %v", err)
	}

	keepDay := time.Now().UTC()
	keepPath := filepath.Join(activityDir, "events-dashboard-"+keepDay.Format(time.DateOnly)+".jsonl")
	if err := os.WriteFile(keepPath, []byte("{\"source\":\"dashboard\"}\n"), 0600); err != nil {
		t.Fatalf("WriteFile keep: %v", err)
	}

	if _, err := NewStore(root, 30); err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected expired source-specific activity file to be pruned, stat err = %v", err)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected current source-specific activity file to remain, stat err = %v", err)
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
	if _, err := NewStore(t.TempDir(), 0); err == nil {
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
	store, err := NewStore(root, 1)
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

func TestNormalizeSourceName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "dashboard", want: "dashboard"},
		{in: " Dashboard UI ", want: "dashboard-ui"},
		{in: "mcp/agent", want: "mcp-agent"},
		{in: "___", want: ""},
	}

	for _, tt := range tests {
		if got := normalizeSourceName(tt.in); got != tt.want {
			t.Fatalf("normalizeSourceName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
