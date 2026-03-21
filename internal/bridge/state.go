package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
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
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type SessionState struct {
	Tabs    []TabState `json:"tabs"`
	SavedAt string     `json:"savedAt"`
}

// IsTransientURL returns true for URLs that should not be shown in the UI
// or persisted to session state (about:blank, chrome://, etc.).
func IsTransientURL(url string) bool {
	switch url {
	case "about:blank", "chrome://newtab/", "chrome://new-tab-page/":
		return true
	}
	return strings.HasPrefix(url, "chrome://") ||
		strings.HasPrefix(url, "chrome-extension://") ||
		strings.HasPrefix(url, "devtools://") ||
		strings.HasPrefix(url, "file://") ||
		strings.Contains(url, "localhost:")
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
	if patched != string(data) {
		if err := os.WriteFile(prefsPath, []byte(patched), 0644); err != nil {
			slog.Error("patch prefs", "err", err)
		}
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

var sessionRestoreFiles = []string{
	"Current Session",
	"Current Tabs",
	"Last Session",
	"Last Tabs",
}

func ClearChromeSessions(profileDir string) {
	sessionsDir := filepath.Join(profileDir, "Default", "Sessions")

	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		return
	}

	var failed []string
	for _, name := range sessionRestoreFiles {
		p := filepath.Join(sessionsDir, name)
		if err := retryRemove(p, 3); err != nil {
			failed = append(failed, name)
			slog.Warn("failed to remove session file", "file", name, "err", err)
		}
	}

	if len(failed) == 0 {
		slog.Info("cleared Chrome session restore files")
	}
}

func retryRemove(path string, maxRetries int) error {
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(50*(1<<uint(attempt))) * time.Millisecond) // 100ms, 200ms, ...
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
	targets, err := b.ListTargets()
	if err != nil {
		slog.Error("save state: list targets", "err", err)
		return
	}

	accessed := b.AccessedTabIDs()
	tabs := make([]TabState, 0, len(targets))
	seen := make(map[string]bool, len(targets))
	for _, t := range targets {
		if t.URL == "" || IsTransientURL(t.URL) {
			continue
		}
		if seen[t.URL] {
			continue
		}
		if !accessed[string(t.TargetID)] {
			continue
		}
		seen[t.URL] = true
		tabs = append(tabs, TabState{
			ID:    string(t.TargetID),
			URL:   t.URL,
			Title: t.Title,
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
	} else {
		slog.Info("saved tabs", "count", len(tabs), "path", path)
	}
}

func (b *Bridge) RestoreState() {
	path := filepath.Join(b.Config.StateDir, "sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	if len(state.Tabs) == 0 {
		return
	}

	const maxConcurrentTabs = 3
	const maxConcurrentNavs = 2

	tabSem := make(chan struct{}, maxConcurrentTabs)
	navSem := make(chan struct{}, maxConcurrentNavs)

	restored := 0
	for _, tab := range state.Tabs {
		if strings.Contains(tab.URL, "/sorry/") || strings.Contains(tab.URL, "about:blank") {
			continue
		}

		tabSem <- struct{}{}

		if restored > 0 {
			time.Sleep(200 * time.Millisecond)
		}

		ctx, cancel := chromedp.NewContext(b.BrowserCtx)

		if err := chromedp.Run(ctx); err != nil {
			cancel()
			<-tabSem
			attrs := []any{"err", err}
			if host := safeURLHostForLog(tab.URL); host != "" {
				attrs = append(attrs, "host", host)
			}
			slog.Warn("restore tab failed", attrs...)
			continue
		}

		newID := string(chromedp.FromContext(ctx).Target.TargetID)
		b.tabSetup(ctx)
		b.mu.Lock()
		b.tabs[newID] = &TabEntry{Ctx: ctx, Cancel: cancel}
		b.accessed[newID] = true
		b.mu.Unlock()
		restored++

		go func(tabCtx context.Context, url string) {
			defer func() { <-tabSem }()

			navSem <- struct{}{}
			defer func() { <-navSem }()

			tCtx, tCancel := context.WithTimeout(tabCtx, 15*time.Second)
			defer tCancel()
			_ = chromedp.Run(tCtx, chromedp.ActionFunc(func(ctx context.Context) error {
				p := map[string]any{"url": url}
				return chromedp.FromContext(ctx).Target.Execute(ctx, "Page.navigate", p, nil)
			}))
		}(ctx, tab.URL)
	}
	if restored > 0 {
		slog.Info("restored tabs", "count", restored, "concurrent_limit", maxConcurrentTabs)
	}
}
