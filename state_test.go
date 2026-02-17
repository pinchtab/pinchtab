package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkCleanExit_NoFile(t *testing.T) {
	origProfileDir := profileDir
	profileDir = t.TempDir()
	defer func() { profileDir = origProfileDir }()

	// Should not panic when file doesn't exist
	markCleanExit()
}

func TestMarkCleanExit_PatchesCrashed(t *testing.T) {
	origProfileDir := profileDir
	profileDir = t.TempDir()
	defer func() { profileDir = origProfileDir }()

	prefsDir := filepath.Join(profileDir, "Default")
	_ = os.MkdirAll(prefsDir, 0755)

	prefsPath := filepath.Join(prefsDir, "Preferences")
	content := `{"profile":{"exit_type":"Crashed","exited_cleanly":false}}`
	_ = os.WriteFile(prefsPath, []byte(content), 0644)

	markCleanExit()

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
	origProfileDir := profileDir
	profileDir = t.TempDir()
	defer func() { profileDir = origProfileDir }()

	prefsDir := filepath.Join(profileDir, "Default")
	_ = os.MkdirAll(prefsDir, 0755)

	prefsPath := filepath.Join(prefsDir, "Preferences")
	content := `{"profile":{"exit_type":"Normal","exited_cleanly":true}}`
	_ = os.WriteFile(prefsPath, []byte(content), 0644)

	markCleanExit()

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
	// Should not panic — just logs error and returns
	b.SaveState()
}

func TestRestoreState_NoFile(t *testing.T) {
	origStateDir := stateDir
	stateDir = t.TempDir()
	defer func() { stateDir = origStateDir }()

	b := newTestBridge()
	// Should not panic when file doesn't exist
	b.RestoreState()
}

func TestRestoreState_EmptyTabs(t *testing.T) {
	origStateDir := stateDir
	stateDir = t.TempDir()
	defer func() { stateDir = origStateDir }()

	state := SessionState{Tabs: []TabState{}, SavedAt: "2026-02-17T07:00:00Z"}
	data, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(stateDir, "sessions.json"), data, 0644)

	b := newTestBridge()
	// Should return early — no tabs to restore
	b.RestoreState()
}

func TestRestoreState_InvalidJSON(t *testing.T) {
	origStateDir := stateDir
	stateDir = t.TempDir()
	defer func() { stateDir = origStateDir }()

	_ = os.WriteFile(filepath.Join(stateDir, "sessions.json"), []byte("{broken"), 0644)

	b := newTestBridge()
	// Should not panic on invalid JSON
	b.RestoreState()
}

func TestWasUncleanExit_Crashed(t *testing.T) {
	tmp := t.TempDir()
	origProfile := profileDir
	profileDir = tmp
	defer func() { profileDir = origProfile }()

	defaultDir := filepath.Join(tmp, "Default")
	os.MkdirAll(defaultDir, 0755)
	os.WriteFile(filepath.Join(defaultDir, "Preferences"),
		[]byte(`{"profile":{"exit_type":"Crashed","exited_cleanly":false}}`), 0644)

	if !wasUncleanExit() {
		t.Error("expected wasUncleanExit to return true for Crashed exit_type")
	}
}

func TestWasUncleanExit_Normal(t *testing.T) {
	tmp := t.TempDir()
	origProfile := profileDir
	profileDir = tmp
	defer func() { profileDir = origProfile }()

	defaultDir := filepath.Join(tmp, "Default")
	os.MkdirAll(defaultDir, 0755)
	os.WriteFile(filepath.Join(defaultDir, "Preferences"),
		[]byte(`{"profile":{"exit_type":"Normal","exited_cleanly":true}}`), 0644)

	if wasUncleanExit() {
		t.Error("expected wasUncleanExit to return false for Normal exit_type")
	}
}

func TestWasUncleanExit_NoFile(t *testing.T) {
	tmp := t.TempDir()
	origProfile := profileDir
	profileDir = tmp
	defer func() { profileDir = origProfile }()

	if wasUncleanExit() {
		t.Error("expected wasUncleanExit to return false when no Preferences file exists")
	}
}

func TestClearChromeSessions(t *testing.T) {
	tmp := t.TempDir()
	origProfile := profileDir
	profileDir = tmp
	defer func() { profileDir = origProfile }()

	sessionsDir := filepath.Join(tmp, "Default", "Sessions")
	os.MkdirAll(sessionsDir, 0755)
	os.WriteFile(filepath.Join(sessionsDir, "Session_1"), []byte("data"), 0644)

	clearChromeSessions()

	if _, err := os.Stat(sessionsDir); !os.IsNotExist(err) {
		t.Error("expected Sessions dir to be removed")
	}
}

func TestClearChromeSessions_NoDir(t *testing.T) {
	tmp := t.TempDir()
	origProfile := profileDir
	profileDir = tmp
	defer func() { profileDir = origProfile }()

	// Should not panic when dir doesn't exist
	clearChromeSessions()
}
