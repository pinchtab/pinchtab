package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

var crashedPrefsReplacer = strings.NewReplacer(
	`"exit_type":"Crashed"`, `"exit_type":"Normal"`,
	`"exit_type": "Crashed"`, `"exit_type": "Normal"`,
	`"exited_cleanly":false`, `"exited_cleanly":true`,
	`"exited_cleanly": false`, `"exited_cleanly": true`,
)

type TabState struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Title         string `json:"title"`
	Status        string `json:"status,omitempty"`
	HandoffReason string `json:"handoffReason,omitempty"`
}

type SessionState struct {
	Tabs    []TabState `json:"tabs"`
	SavedAt string     `json:"savedAt"`
}

var sessionRestoreFiles = []string{
	"Current Session",
	"Current Tabs",
	"Last Session",
	"Last Tabs",
}

// IsTransientURL returns true for URLs that should not be shown in the UI
// or persisted to session state.
func IsTransientURL(rawURL, serverPort string) bool {
	switch rawURL {
	case "about:blank", "chrome://newtab/", "chrome://new-tab-page/":
		return true
	}
	return strings.HasPrefix(rawURL, "chrome://") ||
		strings.HasPrefix(rawURL, "chrome-extension://") ||
		strings.HasPrefix(rawURL, "devtools://") ||
		strings.HasPrefix(rawURL, "file://") ||
		(serverPort != "" && strings.Contains(rawURL, "localhost:"+serverPort))
}

func safeURLHostForLog(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func MarkCleanExit(profileDir string) {
	prefsPath := filepath.Join(profileDir, "Default", "Preferences")
	data, err := os.ReadFile(prefsPath)
	if err != nil {
		return
	}
	patched := crashedPrefsReplacer.Replace(string(data))
	if patched == string(data) {
		return
	}
	if err := os.WriteFile(prefsPath, []byte(patched), 0644); err != nil {
		slog.Error("patch prefs", "err", err)
	}
}

func WasUncleanExit(profileDir string) bool {
	prefsPath := filepath.Join(profileDir, "Default", "Preferences")
	data, err := os.ReadFile(prefsPath)
	if err != nil {
		return false
	}
	prefs := string(data)
	return strings.Contains(prefs, `"exit_type":"Crashed"`) || strings.Contains(prefs, `"exit_type": "Crashed"`)
}

// isSessionRestoreFile reports whether Chrome replays name on its next launch.
// Current Chrome writes timestamped Session_<ts> and Tabs_<ts> files; the fixed
// names are what older builds used.
func isSessionRestoreFile(name string) bool {
	return slices.Contains(sessionRestoreFiles, name) ||
		strings.HasPrefix(name, "Session_") || strings.HasPrefix(name, "Tabs_")
}

func ClearChromeSessions(profileDir string) {
	sessionsDir := filepath.Join(profileDir, "Default", "Sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !isSessionRestoreFile(entry.Name()) {
			continue
		}
		if err := retryRemove(filepath.Join(sessionsDir, entry.Name()), 3); err != nil {
			slog.Warn("failed to remove session file", "file", entry.Name(), "err", err)
			continue
		}
		removed++
	}
	if removed > 0 {
		slog.Info("cleared Chrome session restore files", "count", removed)
	}
}

func retryRemove(path string, maxRetries int) error {
	var err error
	for attempt := range maxRetries {
		if attempt > 0 {
			time.Sleep(time.Duration(50*(1<<uint(attempt))) * time.Millisecond)
		}
		err = os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		if !isLockError(err) {
			return err
		}
		slog.Debug("file locked, retrying remove", "path", filepath.Base(path), "attempt", attempt+1)
	}
	return fmt.Errorf("still locked after %d attempts: %w", maxRetries, err)
}

func (b *Bridge) SaveState() {
	if b == nil || b.Config == nil || b.Config.StateDir == "" || b.TabManager == nil {
		return
	}

	targets, err := b.ListTargets()
	if err != nil {
		slog.Error("save state: list targets", "err", err)
		return
	}

	accessed := b.AccessedTabIDs()
	tabs := make([]TabState, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	for _, t := range targets {
		if t.URL == "" || IsTransientURL(t.URL, b.Config.Port) {
			continue
		}
		if seen[t.URL] || !accessed[string(t.TargetID)] {
			continue
		}
		seen[t.URL] = true

		status := "active"
		handoffReason := ""
		if hs, ok := b.TabHandoffState(string(t.TargetID)); ok {
			status = hs.Status
			handoffReason = hs.Reason
		}
		tabs = append(tabs, TabState{
			ID:            string(t.TargetID),
			URL:           t.URL,
			Title:         t.Title,
			Status:        status,
			HandoffReason: handoffReason,
		})
	}

	state := SessionState{
		Tabs:    tabs,
		SavedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		slog.Error("save state: marshal", "err", err)
		return
	}
	if err := os.MkdirAll(b.Config.StateDir, 0755); err != nil {
		slog.Error("save state: mkdir", "err", err)
		return
	}
	path := filepath.Join(b.Config.StateDir, "sessions.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Error("save state: write", "err", err)
		return
	}
	slog.Info("saved tabs", "count", len(tabs), "path", path)
}

func (b *Bridge) ClearSavedState() {
	if b == nil || b.Config == nil || b.Config.StateDir == "" {
		return
	}
	path := filepath.Join(b.Config.StateDir, "sessions.json")
	backupPath := path + ".bak"
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("clear saved state backup", "path", backupPath, "err", err)
	}
	if err := os.Rename(path, backupPath); err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("backup saved state", "path", path, "backup", backupPath, "err", err)
		}
		return
	}
	slog.Info("backed up saved state", "path", path, "backup", backupPath)
}

func (b *Bridge) CleanupSavedStateBackup() {
	if b == nil || b.Config == nil || b.Config.StateDir == "" {
		return
	}
	backupPath := filepath.Join(b.Config.StateDir, "sessions.json.bak")
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("cleanup saved state backup", "path", backupPath, "err", err)
	}
}

func (b *Bridge) RestoreState() {
	if b == nil || b.Config == nil || b.Config.StateDir == "" || b.BrowserCtx == nil || b.TabManager == nil {
		return
	}

	path := filepath.Join(b.Config.StateDir, "sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil || len(state.Tabs) == 0 {
		return
	}

	restored := 0
	for _, tab := range state.Tabs {
		if tab.URL == "" || IsTransientURL(tab.URL, b.Config.Port) || strings.Contains(tab.URL, "/sorry/") {
			continue
		}
		if _, _, _, err := b.CreateTab(tab.URL); err != nil {
			attrs := []any{"err", err}
			if host := safeURLHostForLog(tab.URL); host != "" {
				attrs = append(attrs, "host", host)
			}
			slog.Warn("restore tab failed", attrs...)
			continue
		}
		restored++
		if restored > 0 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if restored > 0 {
		slog.Info("restored tabs", "count", restored)
	}
}

func (b *Bridge) GetDocumentReadyState(tabID string) (string, error) {
	tabCtx, _, err := b.TabContext(tabID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(tabCtx, 2*time.Second)
	defer cancel()
	var readyState string
	err = chromedp.Run(ctx, chromedp.Evaluate(`document.readyState`, &readyState))
	if err != nil {
		return "", err
	}
	return readyState, nil
}

func (b *Bridge) IsNetworkIdle(tabID string) (bool, bool) {
	if b.netMonitor == nil {
		return false, false
	}
	return b.netMonitor.IsTabIdle(tabID)
}
