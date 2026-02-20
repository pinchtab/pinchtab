package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestMarkCleanExit_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	MarkCleanExit(tmpDir)
}

func TestMarkCleanExit_PatchesCrashed(t *testing.T) {
	tmpDir := t.TempDir()
	prefsDir := filepath.Join(tmpDir, "Default")
	_ = os.MkdirAll(prefsDir, 0755)

	prefsPath := filepath.Join(prefsDir, "Preferences")
	content := `{"profile":{"exit_type":"Crashed","exited_cleanly":false}}`
	_ = os.WriteFile(prefsPath, []byte(content), 0644)

	MarkCleanExit(tmpDir)

	data, err := os.ReadFile(prefsPath)
	if err != nil {
		t.Fatalf("failed to read patched prefs: %v", err)
	}
	s := string(data)
	if s != `{"profile":{"exit_type":"Normal","exited_cleanly":true}}` {
		t.Errorf("prefs not properly patched: %s", s)
	}
}

func TestMarkCleanExit_NoPatch(t *testing.T) {
	tmpDir := t.TempDir()
	prefsDir := filepath.Join(tmpDir, "Default")
	_ = os.MkdirAll(prefsDir, 0755)

	prefsPath := filepath.Join(prefsDir, "Preferences")
	content := `{"profile":{"exit_type":"Normal","exited_cleanly":true}}`
	_ = os.WriteFile(prefsPath, []byte(content), 0644)

	MarkCleanExit(tmpDir)

	data, _ := os.ReadFile(prefsPath)
	if string(data) != content {
		t.Error("prefs should not have been modified")
	}
}

func TestSessionState_Marshal(t *testing.T) {
	state := SessionState{
		Tabs: []TabState{
			{ID: "tab1", URL: "https://example.com", Title: "Example"},
			{ID: "tab2", URL: "https://google.com", Title: "Google"},
		},
		SavedAt: "2026-02-17T07:00:00Z",
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded SessionState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(decoded.Tabs) != 2 {
		t.Errorf("expected 2 tabs, got %d", len(decoded.Tabs))
	}
	if decoded.Tabs[0].URL != "https://example.com" {
		t.Errorf("expected example.com, got %s", decoded.Tabs[0].URL)
	}
}

func TestSaveState_NoBrowser(t *testing.T) {
	b := newTestBridge()
	b.Config = &config.RuntimeConfig{StateDir: t.TempDir()}
	b.SaveState()
}

func TestRestoreState_NoFile(t *testing.T) {
	b := newTestBridge()
	b.Config = &config.RuntimeConfig{StateDir: t.TempDir()}
	b.RestoreState()
}

func TestRestoreState_EmptyTabs(t *testing.T) {
	tmpDir := t.TempDir()
	state := SessionState{Tabs: []TabState{}, SavedAt: "2026-02-17T07:00:00Z"}
	data, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(tmpDir, "sessions.json"), data, 0644)

	b := newTestBridge()
	b.Config = &config.RuntimeConfig{StateDir: tmpDir}
	b.RestoreState()
}

func TestWasUncleanExit_Crashed(t *testing.T) {
	tmp := t.TempDir()
	defaultDir := filepath.Join(tmp, "Default")
	_ = os.MkdirAll(defaultDir, 0755)
	_ = os.WriteFile(filepath.Join(defaultDir, "Preferences"),
		[]byte(`{"profile":{"exit_type":"Crashed","exited_cleanly":false}}`), 0644)

	if !WasUncleanExit(tmp) {
		t.Error("expected WasUncleanExit to return true for Crashed exit_type")
	}
}

func TestWasUncleanExit_Normal(t *testing.T) {
	tmp := t.TempDir()
	defaultDir := filepath.Join(tmp, "Default")
	_ = os.MkdirAll(defaultDir, 0755)
	_ = os.WriteFile(filepath.Join(defaultDir, "Preferences"),
		[]byte(`{"profile":{"exit_type":"Normal","exited_cleanly":true}}`), 0644)

	if WasUncleanExit(tmp) {
		t.Error("expected WasUncleanExit to return false for Normal exit_type")
	}
}

func TestClearChromeSessions(t *testing.T) {
	tmp := t.TempDir()
	sessionsDir := filepath.Join(tmp, "Default", "Sessions")
	_ = os.MkdirAll(sessionsDir, 0755)
	_ = os.WriteFile(filepath.Join(sessionsDir, "Session_1"), []byte("data"), 0644)

	ClearChromeSessions(tmp)

	if _, err := os.Stat(sessionsDir); !os.IsNotExist(err) {
		t.Error("expected Sessions dir to be removed")
	}
}
