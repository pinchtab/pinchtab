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
			{ID: "tab1", URL: "https://pinchtab.com", Title: "Example"},
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
	if decoded.Tabs[0].URL != "https://pinchtab.com" {
		t.Errorf("expected pinchtab.com, got %s", decoded.Tabs[0].URL)
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

func TestIsTransientURL(t *testing.T) {
	const serverPort = "9867"

	transient := []string{
		"about:blank",
		"chrome://newtab/",
		"chrome://new-tab-page/",
		"chrome://settings/",
		"chrome-extension://abc/popup.html",
		"devtools://devtools/inspector.html",
		"file:///tmp/test.html",
		"http://localhost:9867/welcome",
	}
	for _, u := range transient {
		if !IsTransientURL(u, serverPort) {
			t.Errorf("expected transient: %s", u)
		}
	}

	persistent := []string{
		"https://pinchtab.com",
		"https://github.com/pinchtab/pinchtab",
		"https://www.google.com/search?q=test",
		"https://httpbin.org/get",
		"http://localhost:3000/dashboard",
		"http://localhost:3001/en/access",
	}
	for _, u := range persistent {
		if IsTransientURL(u, serverPort) {
			t.Errorf("expected persistent: %s", u)
		}
	}
}

func TestSafeURLHostForLog(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "https query", raw: "https://example.com/reset?token=secret", want: "example.com"},
		{name: "subdomain with port", raw: "https://app.example.com:8443/path?q=1", want: "app.example.com"},
		{name: "malformed", raw: "://bad-url", want: ""},
		{name: "empty", raw: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeURLHostForLog(tt.raw); got != tt.want {
				t.Fatalf("safeURLHostForLog(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestClearChromeSessions(t *testing.T) {
	tmp := t.TempDir()
	sessionsDir := filepath.Join(tmp, "Default", "Sessions")
	_ = os.MkdirAll(sessionsDir, 0755)

	// Create the specific session restore files
	for _, name := range sessionRestoreFiles {
		_ = os.WriteFile(filepath.Join(sessionsDir, name), []byte("data"), 0644)
	}
	// Also create an unrelated file that should NOT be deleted
	_ = os.WriteFile(filepath.Join(sessionsDir, "Session_1"), []byte("other"), 0644)

	ClearChromeSessions(tmp)

	// Session restore files should be gone
	for _, name := range sessionRestoreFiles {
		if _, err := os.Stat(filepath.Join(sessionsDir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", name)
		}
	}

	// Unrelated files should still exist
	if _, err := os.Stat(filepath.Join(sessionsDir, "Session_1")); err != nil {
		t.Error("expected unrelated Session_1 file to remain")
	}
}

func TestClearChromeSessions_MissingDir(t *testing.T) {
	tmp := t.TempDir()
	sessionsDir := filepath.Join(tmp, "Default", "Sessions")
	// Don't create the directory

	ClearChromeSessions(tmp)

	// Should not panic, and Sessions dir should still not exist
	if _, err := os.Stat(sessionsDir); !os.IsNotExist(err) {
		t.Error("expected Sessions dir to not exist")
	}
}

func TestRetryRemove_NonExistent(t *testing.T) {
	err := retryRemove("/tmp/does-not-exist-at-all", 3)
	if err != nil {
		t.Errorf("expected nil for non-existent file, got: %v", err)
	}
}

func TestRetryRemove_Success(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "testfile")
	_ = os.WriteFile(p, []byte("data"), 0644)

	err := retryRemove(p, 3)
	if err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}
}

func TestClearSavedState_BackupsSessionsFile(t *testing.T) {
	tmp := t.TempDir()
	sessionsPath := filepath.Join(tmp, "sessions.json")
	if err := os.WriteFile(sessionsPath, []byte(`{"tabs":[]}`), 0644); err != nil {
		t.Fatalf("write sessions: %v", err)
	}

	b := newTestBridge()
	b.Config = &config.RuntimeConfig{StateDir: tmp}
	b.ClearSavedState()

	if _, err := os.Stat(sessionsPath); !os.IsNotExist(err) {
		t.Fatalf("expected sessions.json to be renamed away, got err=%v", err)
	}
	backupPath := sessionsPath + ".bak"
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != `{"tabs":[]}` {
		t.Fatalf("backup contents = %q", string(data))
	}
}

func TestCleanupSavedStateBackup_RemovesBackupFile(t *testing.T) {
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "sessions.json.bak")
	if err := os.WriteFile(backupPath, []byte(`{"tabs":[]}`), 0644); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	b := newTestBridge()
	b.Config = &config.RuntimeConfig{StateDir: tmp}
	b.CleanupSavedStateBackup()

	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("expected backup to be removed, got err=%v", err)
	}
}
