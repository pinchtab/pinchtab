package activity_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/config"
)

// Loading again models the process restart promised by the config-set refusal:
// the new runtime must derive the activity sink from the saved server.stateDir.
func TestRestartMovesActivityRecordsWithServerStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state-moved")
	fc := config.DefaultFileConfig()
	fc.Server.StateDir = stateDir
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	restarted := config.Load()
	wantDir := filepath.Join(stateDir, "activity")
	if got := restarted.ActivityLogDir(); got != wantDir {
		t.Fatalf("ActivityLogDir() after restart = %q, want %q", got, wantDir)
	}
	store, err := activity.NewStore(restarted.ActivityLogDir(), 1)
	if err != nil {
		t.Fatalf("NewStore() after restart: %v", err)
	}
	if err := store.Record(activity.Event{Source: "server", Path: "/after-restart"}); err != nil {
		t.Fatalf("Record() after restart: %v", err)
	}
	entries, err := os.ReadDir(wantDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", wantDir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("activity record was not written under restarted state dir %q", wantDir)
	}
}
