package bridge

import (
	"context"
	"encoding/json"
	"log/slog"
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

func isTransientURL(url string) bool {
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

func ClearChromeSessions(profileDir string) {
	sessionsDir := filepath.Join(profileDir, "Default", "Sessions")

	// Retry with backoff on Windows where file locks may persist after Chrome exit
	const maxRetries = 3
	const retryDelayMs = 100

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Small delay before retry to allow file handles to be released
			time.Sleep(time.Duration(retryDelayMs) * time.Millisecond)
		}

		err = os.RemoveAll(sessionsDir)
		if err == nil {
			slog.Info("cleared Chrome sessions dir (prevent tab restore hang)")
			return
		}

		if attempt < maxRetries-1 {
			slog.Debug("failed to clear Chrome sessions dir, retrying", "attempt", attempt+1, "err", err)
		}
	}

	// Log final error if all retries failed
	slog.Warn("failed to clear Chrome sessions dir after retries", "err", err)
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
		if t.URL == "" || isTransientURL(t.URL) {
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
			slog.Warn("restore tab failed", "url", tab.URL, "err", err)
			continue
		}

		newID := string(chromedp.FromContext(ctx).Target.TargetID)
		b.tabSetup(ctx)
		b.mu.Lock()
		b.tabs[newID] = &TabEntry{Ctx: ctx, Cancel: cancel}
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
