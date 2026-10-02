package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

type exitPanic struct{ code int }

func interceptExit(t *testing.T) {
	t.Helper()
	orig := osExit
	osExit = func(code int) { panic(exitPanic{code}) }
	t.Cleanup(func() { osExit = orig })
}

func expectRefusalExit(t *testing.T, fn func()) string {
	t.Helper()
	return captureStderr(t, func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("the path completed without refusing")
			} else if _, ok := r.(exitPanic); !ok {
				panic(r)
			}
		}()
		fn()
	})
}

// failOnAnyRequest listens on loopback while the CLI is pointed at the SAME port via
// 0.0.0.0, which the predicate classifies as non-loopback but the OS routes locally —
// so a guard that fails to refuse produces an observable request instead of a timeout.
func failOnAnyRequest(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a request reached the host: %s %s Authorization=%q — the refusal must happen before anything is sent", r.Method, r.URL.Path, r.Header.Get("Authorization"))
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return "http://0.0.0.0:" + u.Port()
}

func clearCredentialEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PINCHTAB_SESSION", "")
	t.Setenv("PINCHTAB_TOKEN", "")
	t.Setenv("PINCHTAB_AGENT_ID", "")
}

func TestARemoteServerWithNoEnvCredentialRefusesBeforeAnyRequest(t *testing.T) {
	clearCredentialEnv(t)
	remote := failOnAnyRequest(t)
	t.Setenv("PINCHTAB_SERVER", remote)
	interceptExit(t)
	cfg := &config.RuntimeConfig{Port: "9867", Token: "local-secret"}

	stderr := expectRefusalExit(t, func() { newCLIConn(cfg) })

	for _, want := range []string{remote, "PINCHTAB_TOKEN", "server.token"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("refusal %q does not name %q; a typo'd host must be visible in the message", stderr, want)
		}
	}
	if strings.Contains(stderr, "local-secret") {
		t.Errorf("refusal %q echoes the secret it protects", stderr)
	}
}

func TestTheMCPPathRefusesARemoteHostWithoutACredential(t *testing.T) {
	clearCredentialEnv(t)
	remote := failOnAnyRequest(t)
	t.Setenv("PINCHTAB_SERVER", remote)
	interceptExit(t)
	cfg := &config.RuntimeConfig{Port: "9867", Token: "local-secret"}

	stderr := expectRefusalExit(t, func() { runMCP(cfg) })

	if !strings.Contains(stderr, remote) || !strings.Contains(stderr, "PINCHTAB_TOKEN") {
		t.Errorf("mcp refusal %q must name the host and PINCHTAB_TOKEN — this is the flow docs/mcp.md recommends", stderr)
	}
}

func TestTheTabProbeNeverSendsTheConfigTokenToARemoteHost(t *testing.T) {
	clearCredentialEnv(t)
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"server":{"token":"local-secret"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", configPath)

	t.Setenv("PINCHTAB_SERVER", "http://203.0.113.9:9867")
	if base, token := resolveTabStateEndpoint(); token != "" {
		t.Errorf("probe endpoint %s carries token %q; the probe must degrade to unauthenticated rather than leak the config secret", base, token)
	}

	t.Setenv("PINCHTAB_SERVER", "http://127.0.0.1:19999")
	if _, token := resolveTabStateEndpoint(); token != "local-secret" {
		t.Errorf("loopback probe token = %q, want the config token; the guard must not break the local probe", token)
	}
}

func TestTheConfigTokenStillServesEveryLoopbackShape(t *testing.T) {
	clearCredentialEnv(t)
	cfg := &config.RuntimeConfig{Port: "9867", Token: "local-secret"}

	for name, base := range map[string]string{
		"the default base":         "http://127.0.0.1:9867",
		"loopback on another port": "http://127.0.0.1:9999",
		"localhost by name":        "http://localhost:9868",
		"ipv6 loopback":            "http://[::1]:9867",
		"other 127/8 loopback":     "http://127.0.0.2:9867",
	} {
		t.Run(name, func(t *testing.T) {
			token, _, err := resolveCLIToken(cfg, base)
			if err != nil || token != "local-secret" {
				t.Fatalf("token = %q, err = %v; a loopback destination keeps today's behaviour exactly", token, err)
			}
		})
	}
}

// With no server.token there is nothing to withhold, so refusing would name a secret that
// does not exist AND break a legitimate flow — a tokenless server on a LAN or in Docker,
// which internal/cli/report/security.go treats as supported-but-warned rather than
// impossible. Nothing leaves the machine either way, so the refusal buys nothing: exactly
// the test the loopback predicate was chosen by. The remote answers its own honest 401.
//
// This was invisible because every other refusal test sets a config token.
func TestAnEmptyConfigTokenIsNotRefused(t *testing.T) {
	clearCredentialEnv(t)
	cfg := &config.RuntimeConfig{Port: "9867"}

	token, _, err := resolveCLIToken(cfg, "http://203.0.113.9:9867")
	if err != nil {
		t.Fatalf("a tokenless config was refused against a remote host: %v — nothing would have been sent, so this is breakage with no security gain", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty so the remote answers its own 401", token)
	}
}

// The refusal survives for the case it exists for: the same remote host WITH a config token.
// Paired with the test above so a fix that simply deleted the guard cannot pass both.
func TestAPresentConfigTokenIsStillRefusedForTheSameHost(t *testing.T) {
	clearCredentialEnv(t)
	cfg := &config.RuntimeConfig{Port: "9867", Token: "local-secret"}

	token, _, err := resolveCLIToken(cfg, "http://203.0.113.9:9867")
	if err == nil {
		t.Fatal("a config token was handed to a remote host; that is the leak this card closes")
	}
	if token != "" {
		t.Errorf("token = %q returned alongside the refusal", token)
	}
	if !strings.Contains(err.Error(), "server.token") {
		t.Errorf("refusal %q no longer names what it is withholding", err)
	}
}

func TestAnEnvCredentialTravelsToAnyHost(t *testing.T) {
	clearCredentialEnv(t)
	t.Setenv("PINCHTAB_TOKEN", "remote-secret")
	cfg := &config.RuntimeConfig{Port: "9867", Token: "local-secret"}

	token, _, err := resolveCLIToken(cfg, "http://203.0.113.9:9867")
	if err != nil || token != "remote-secret" {
		t.Fatalf("token = %q, err = %v; an explicit env credential is the deliberate way to authenticate to a named host", token, err)
	}
}

func TestEveryCredentialBranchRecordsTheBaseItSendsTo(t *testing.T) {
	cfg := &config.RuntimeConfig{Port: "9867", Token: "local-secret"}

	for _, tc := range []struct {
		name, env, base, wantSource string
	}{
		{"session env", "PINCHTAB_SESSION", "http://203.0.113.9:9001", "PINCHTAB_SESSION"},
		{"token env", "PINCHTAB_TOKEN", "http://203.0.113.9:9002", "PINCHTAB_TOKEN"},
		{"config token on loopback", "", "http://127.0.0.1:9003", "server.token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearCredentialEnv(t)
			if tc.env != "" {
				t.Setenv(tc.env, "env-secret")
			}

			_, source, err := resolveCLIToken(cfg, tc.base)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(source, tc.wantSource) {
				t.Errorf("returned source = %q, want it to name %s; a 401 hint would name the wrong credential", source, tc.wantSource)
			}
		})
	}
}

func TestSubcommandHelpListsTheGlobalFlags(t *testing.T) {
	target, _, err := rootCmd.Find([]string{"tabs"})
	if err != nil {
		t.Fatal(err)
	}
	usage := target.UsageString()
	for _, want := range []string{"Global Flags", "--server", "--agent-id"} {
		if !strings.Contains(usage, want) {
			t.Errorf("`pinchtab tabs --help` output lacks %q; --server redirects every command and must be discoverable from the command being run\n%s", want, usage)
		}
	}
}

func TestTheConfigTokenIsRefusedForALocalhostSubdomain(t *testing.T) {
	clearCredentialEnv(t)
	cfg := &config.RuntimeConfig{Port: "9867", Token: "local-secret"}

	if token, _, err := resolveCLIToken(cfg, "http://foo.localhost:9867"); err == nil || token != "" {
		t.Fatalf("token = %q, err = %v; a *.localhost name can resolve off-host on pure-Go builds", token, err)
	}
}
