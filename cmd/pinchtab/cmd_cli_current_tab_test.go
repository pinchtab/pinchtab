package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestUseLocalTabStateFileDisabledForIdentifiedCallers(t *testing.T) {
	oldAgentID := cliAgentID
	defer func() { cliAgentID = oldAgentID }()

	t.Run("session", func(t *testing.T) {
		cliAgentID = ""
		t.Setenv("PINCHTAB_AGENT_ID", "")
		t.Setenv("PINCHTAB_SESSION", "ses_test")
		if useLocalTabStateFile() {
			t.Fatal("session-authenticated callers should not use local current-tab state")
		}
	})

	t.Run("agent flag", func(t *testing.T) {
		cliAgentID = "agent-flag"
		t.Setenv("PINCHTAB_AGENT_ID", "")
		t.Setenv("PINCHTAB_SESSION", "")
		if useLocalTabStateFile() {
			t.Fatal("--agent-id callers should not use local current-tab state")
		}
	})

	t.Run("agent env", func(t *testing.T) {
		cliAgentID = ""
		t.Setenv("PINCHTAB_AGENT_ID", "agent-env")
		t.Setenv("PINCHTAB_SESSION", "")
		if useLocalTabStateFile() {
			t.Fatal("PINCHTAB_AGENT_ID callers should not use local current-tab state")
		}
	})
}

func TestDefaultTabFlagFromStateOnlyForAnonymousCallers(t *testing.T) {
	oldAgentID := cliAgentID
	defer func() { cliAgentID = oldAgentID }()

	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("PINCHTAB_AGENT_ID", "")
	t.Setenv("PINCHTAB_SESSION", "")
	cliAgentID = ""

	// Pin the endpoint the probe resolves. Without this the probe reaches whatever
	// server the developer's own config points at, asks it about "tab-local", is
	// correctly told no such tab, and clears the cache — so this test would assert
	// flag-filling policy while measuring the machine.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	oldResolve := resolveTabStateEndpoint
	resolveTabStateEndpoint = func() (string, string) { return srv.URL, "" }
	t.Cleanup(func() { resolveTabStateEndpoint = oldResolve })

	WriteTabStateFile("tab-local")

	anonymous := &cobra.Command{Use: "anonymous"}
	anonymous.Flags().String("tab", "", "Tab ID")
	defaultTabFlagFromState(anonymous)
	if got, _ := anonymous.Flags().GetString("tab"); got != "tab-local" {
		t.Fatalf("anonymous tab flag = %q, want tab-local", got)
	}
	if anonymous.Flags().Changed("tab") {
		t.Fatal("state-backed tab default should not mark --tab as explicitly changed")
	}

	t.Setenv("PINCHTAB_SESSION", "ses_test")
	sessionScoped := &cobra.Command{Use: "session"}
	sessionScoped.Flags().String("tab", "", "Tab ID")
	defaultTabFlagFromState(sessionScoped)
	if got, _ := sessionScoped.Flags().GetString("tab"); got != "" {
		t.Fatalf("session tab flag = %q, want empty", got)
	}

	t.Setenv("PINCHTAB_SESSION", "")
	cliAgentID = "agent-flag"
	agentScoped := &cobra.Command{Use: "agent"}
	agentScoped.Flags().String("tab", "", "Tab ID")
	defaultTabFlagFromState(agentScoped)
	if got, _ := agentScoped.Flags().GetString("tab"); got != "" {
		t.Fatalf("agent tab flag = %q, want empty", got)
	}
}

// The resolver the probe shares with the command, exercised for real rather than
// through the test seam: with no env vars set, both the port and the token must come
// from the config file. The old probe read env only and fell back to a hardcoded
// 9867, so a configured port sent it to a server that was not there — and a
// configured token meant it sent no credential at all.
func TestResolveTabStateEndpointReadsTheConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"server":{"port":"19771","token":"file-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)
	t.Setenv("PINCHTAB_SERVER", "")
	t.Setenv("PINCHTAB_TOKEN", "")
	t.Setenv("PINCHTAB_SESSION", "")
	oldServerURL := serverURL
	serverURL = ""
	t.Cleanup(func() { serverURL = oldServerURL })

	base, token := resolveTabStateEndpoint()

	if want := "http://127.0.0.1:19771"; base != want {
		t.Errorf("base = %q, want the configured port %q", base, want)
	}
	if token != "file-token" {
		t.Errorf("token = %q, want the config-file token", token)
	}

	// An explicit env var still wins, as it does for the command itself.
	t.Setenv("PINCHTAB_SERVER", "http://127.0.0.1:18000")
	t.Setenv("PINCHTAB_TOKEN", "env-token")
	if base, token = resolveTabStateEndpoint(); base != "http://127.0.0.1:18000" || token != "env-token" {
		t.Errorf("env override = (%q, %q), want the env values", base, token)
	}
}

func TestTabTargetIsTheCachedTabOnlyForAnonymousCallers(t *testing.T) {
	oldAgentID := cliAgentID
	defer func() { cliAgentID = oldAgentID }()
	cliAgentID = ""
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")
	oldResolve := resolveTabStateEndpoint
	resolveTabStateEndpoint = func() (string, string) { return "http://127.0.0.1:19999", "" }
	t.Cleanup(func() { resolveTabStateEndpoint = oldResolve })

	WriteTabStateFile("TAB_B")
	if got := defaultTabState.target(); got != "TAB_B" {
		t.Fatalf("anonymous target = %q, want TAB_B", got)
	}

	t.Setenv("PINCHTAB_AGENT_ID", "agent-env")
	if got := defaultTabState.target(); got != "" {
		t.Fatalf("identified target = %q, want empty: the next command uses the server-scoped tab", got)
	}
}
