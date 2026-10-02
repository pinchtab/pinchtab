package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNavPolicyDenialExitsOne(t *testing.T) {
	if os.Getenv("PINCHTAB_NAV_POLICY_HELPER") == "1" {
		rootCmd.SetArgs([]string{"--server", os.Getenv("PINCHTAB_NAV_POLICY_SERVER"), "nav", "https://example.com"})
		if err := rootCmd.Execute(); err != nil {
			os.Exit(commandExitCode(err))
		}
		return
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if r.URL.Path != "/navigate" {
			t.Errorf("path = %q, want /navigate", r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"navigation blocked by IDPI","code":"idpi_domain_blocked"}`))
	}))
	t.Cleanup(srv.Close)

	child := exec.Command(os.Args[0], "-test.run=^TestNavPolicyDenialExitsOne$", "-test.timeout=30s") // #nosec G204 -- re-executes this test binary with fixed arguments.
	child.Env = append(os.Environ(),
		"PINCHTAB_NAV_POLICY_HELPER=1",
		"PINCHTAB_NAV_POLICY_SERVER="+srv.URL,
		"HOME="+t.TempDir(),
		"XDG_STATE_HOME="+t.TempDir(),
	)
	out, err := child.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("nav policy denial exited with %v, want exit 1; output:\n%s", err, out)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", exitErr.ExitCode(), out)
	}
	if !strings.Contains(string(out), "Error 403: navigation blocked by IDPI") {
		t.Fatalf("output did not report the policy 403:\n%s", out)
	}
}

func TestTabHandoffFamilyRefusesLocallyOnAnEmptyTabID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a request was issued for an empty tab id: %s %s — the refusal must happen before the wire, or the mux 404s /tabs//<verb> and the CLI misdiagnoses the server as outdated", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PINCHTAB_SERVER", srv.URL)
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	orig := resolveTabStateEndpoint
	resolveTabStateEndpoint = func() (string, string) { return srv.URL, "" }
	t.Cleanup(func() { resolveTabStateEndpoint = orig })

	for name, build := range map[string]func() *cobra.Command{
		"handoff":        newTabHandoffCmd,
		"resume":         newTabResumeCmd,
		"handoff-status": newTabHandoffStatusCmd,
	} {
		t.Run(name, func(t *testing.T) {
			cmd := build()
			cmd.SetArgs([]string{})
			err := cmd.Execute()
			if !errors.Is(err, errNoCurrentTab) {
				t.Fatalf("err = %v, want the local no-current-tab refusal", err)
			}
			for _, lever := range []string{"tab id", "pinchtab nav"} {
				if !strings.Contains(err.Error(), lever) {
					t.Errorf("refusal %q does not name the %q lever", err, lever)
				}
			}
		})
	}
}

func TestMustResolveTabArgPassesAnExplicitOrCachedID(t *testing.T) {
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	orig := resolveTabStateEndpoint
	resolveTabStateEndpoint = func() (string, string) { return "http://127.0.0.1:19999", "" }
	t.Cleanup(func() { resolveTabStateEndpoint = orig })

	if id, err := mustResolveTabArg([]string{"TAB123"}); err != nil || id != "TAB123" {
		t.Fatalf("explicit id: got %q, %v", id, err)
	}

	defaultTabState.write("CACHED42")
	if id, err := mustResolveTabArg(nil); err != nil || id != "CACHED42" {
		t.Fatalf("cached id: got %q, %v — the refusal must not swallow the cache fallback", id, err)
	}
}

func TestTabsCommandStarsTheCachedTabOnlyForAnonymousCallers(t *testing.T) {
	if os.Getenv("PINCHTAB_TABS_STAR_HELPER") == "1" {
		agentID := os.Getenv("PINCHTAB_AGENT_ID")
		_ = os.Unsetenv("PINCHTAB_AGENT_ID")
		WriteTabStateFile("TAB_B")
		_ = os.Setenv("PINCHTAB_AGENT_ID", agentID)
		rootCmd.SetArgs([]string{"tabs"})
		if err := rootCmd.Execute(); err != nil {
			os.Exit(commandExitCode(err))
		}
		return
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/tabs" {
			_, _ = w.Write([]byte(`{"tabs":[{"id":"TAB_A","url":"https://a.example/","title":"A"},{"id":"TAB_B","url":"https://b.example/","title":"B"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name, agentID, want string
	}{
		{"anonymous", "", "TAB_A\thttps://a.example/\tA\n*TAB_B\thttps://b.example/\tB\n"},
		{"identified", "agent-env", "TAB_A\thttps://a.example/\tA\nTAB_B\thttps://b.example/\tB\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=^TestTabsCommandStarsTheCachedTabOnlyForAnonymousCallers$", "-test.timeout=30s") // #nosec G204 -- re-executes this test binary with fixed arguments.
			child.Env = append(os.Environ(),
				"PINCHTAB_TABS_STAR_HELPER=1",
				"PINCHTAB_SERVER="+srv.URL,
				"PINCHTAB_TOKEN=x",
				"PINCHTAB_SESSION=",
				"PINCHTAB_AGENT_ID="+tc.agentID,
				"HOME="+t.TempDir(),
				"XDG_STATE_HOME="+t.TempDir(),
			)
			out, err := child.Output()
			if err != nil {
				t.Fatalf("tabs exited with %v; stdout:\n%s", err, out)
			}
			if !strings.HasPrefix(string(out), tc.want) {
				t.Fatalf("tabs stdout = %q, want it to start with %q", out, tc.want)
			}
		})
	}
}
