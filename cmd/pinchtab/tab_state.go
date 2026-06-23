package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// tabStateStore owns the local "current tab" state for anonymous CLI calls:
// state-file path selection, freshness/TTL tracking, on-disk reads/writes, and
// the authenticated existence probe against a running server. Centralizing it
// here keeps these filesystem/network side effects out of command registration
// (cmd_cli_register.go), which now only wires the --tab flag onto commands and
// no longer has to thread tab-cache policy through the CLI wiring.
type tabStateStore struct{}

// defaultTabState is the process-wide tab-state service used by CLI wiring and
// the thin package-level helpers below.
var defaultTabState = tabStateStore{}

const tabProbeTTL = 30 * time.Second

// useLocal reports whether anonymous local tab-state should be used. Identified
// callers (PINCHTAB_SESSION, --agent-id, or PINCHTAB_AGENT_ID) defer to the
// server-side scoped current-tab store instead.
func (tabStateStore) useLocal() bool {
	if strings.TrimSpace(os.Getenv("PINCHTAB_SESSION")) != "" {
		return false
	}
	return resolveCLIAgentID() == ""
}

func (tabStateStore) path() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir + "/pinchtab/current-tab"
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home + "/.local/state/pinchtab/current-tab"
	}
	return "/tmp/pinchtab-current-tab"
}

func (s tabStateStore) read() string {
	data, err := os.ReadFile(s.path())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s tabStateStore) write(tabID string) {
	if tabID == "" || !s.useLocal() {
		return
	}
	path := s.path()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, []byte(tabID+"\n"), 0644)
}

func (s tabStateStore) clearIfCurrent(tabID string) {
	if tabID == "" || !s.useLocal() || s.read() != tabID {
		return
	}
	_ = os.Remove(s.path())
}

// recentlyValidated reports whether the saved-tab file was written or validated
// within tabProbeTTL, letting back-to-back commands skip the probe.
func (s tabStateStore) recentlyValidated() bool {
	info, err := os.Stat(s.path())
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < tabProbeTTL
}

// touch bumps the state file's mtime to mark it freshly validated.
func (s tabStateStore) touch() {
	now := time.Now()
	_ = os.Chtimes(s.path(), now, now)
}

// resolveArg returns the explicit positional tab argument when present, else the
// locally cached current tab (for anonymous single-agent workflows).
func (s tabStateStore) resolveArg(args []string) string {
	if len(args) > 0 && args[0] != "" {
		return args[0]
	}
	if !s.useLocal() {
		return ""
	}
	return s.read()
}

// applyDefaultFlag fills an unset --tab from the cached current tab, probing the
// server for existence only when the cache is stale (older than tabProbeTTL).
func (s tabStateStore) applyDefaultFlag(cmd *cobra.Command) {
	if cmd == nil || !s.useLocal() {
		return
	}
	flag := cmd.Flags().Lookup("tab")
	if flag == nil || flag.Changed || flag.Value.String() != "" {
		return
	}
	tabID := s.read()
	if tabID == "" {
		return
	}
	if !s.recentlyValidated() {
		if !s.probeExists(tabID) {
			_ = os.Remove(s.path())
			return
		}
		s.touch() // validated now → trusted for the next tabProbeTTL
	}
	_ = cmd.Flags().Set("tab", tabID)
	flag.Changed = false
}

// probeExists checks whether a cached tab ID still exists on the server.
// Returns true if the tab is valid, the server is unreachable (it may auto-start
// later), or the check is inconclusive. Returns false only on a definitive 404.
func (tabStateStore) probeExists(tabID string) bool {
	base := resolveBaseURL("http://127.0.0.1:9867")
	token := resolveToken()

	// Fast path: if the port isn't listening, skip the HTTP probe entirely.
	// This avoids a 2s timeout on every CLI command when the server is down.
	if !portIsListening(base) {
		return true
	}

	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest("GET", base+"/tabs/"+tabID+"/title", nil)
	if err != nil {
		return true
	}
	req.Header.Set("X-PinchTab-Source", "client")
	if token != "" {
		if strings.HasPrefix(token, "ses_") {
			req.Header.Set("Authorization", "Session "+token)
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return true
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode != http.StatusNotFound
}

// The package-level helpers below preserve the existing CLI/test API by
// delegating to defaultTabState; command files call these directly while the
// service owns the filesystem/network logic.

func resolveTabArg(args []string) string { return defaultTabState.resolveArg(args) }

func defaultTabFlagFromState(cmd *cobra.Command) { defaultTabState.applyDefaultFlag(cmd) }

func useLocalTabStateFile() bool { return defaultTabState.useLocal() }

func tabStateFile() string { return defaultTabState.path() }

// WriteTabStateFile persists the current tab for anonymous local workflows.
func WriteTabStateFile(tabID string) { defaultTabState.write(tabID) }

// ClearTabStateFileIfCurrent removes the cached tab only if it matches tabID.
func ClearTabStateFileIfCurrent(tabID string) { defaultTabState.clearIfCurrent(tabID) }

func tabStateRecentlyValidated() bool { return defaultTabState.recentlyValidated() }

func touchTabStateFile() { defaultTabState.touch() }
