package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestPrintConfigOverview(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	fc.Server.Token = "very-long-token-secret"
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	cfg := config.Load()
	output := captureStdout(t, func() {
		printConfigOverview(cfg)
	})

	required := []string{
		"Config",
		"strategy",
		"stealth level",
		"tab eviction",
		"file",
		"token",
		"dashboard",
		configPath,
		"very...cret",
		"Change config:",
		"pinchtab config set",
	}
	for _, needle := range required {
		if !strings.Contains(output, needle) {
			t.Fatalf("expected config overview to contain %q\n%s", needle, output)
		}
	}
}

func TestClipboardCommands(t *testing.T) {
	commands := clipboardCommands()
	if len(commands) == 0 {
		t.Fatal("expected clipboard commands")
	}
	for _, command := range commands {
		if command.name == "" {
			t.Fatalf("clipboard command missing name: %+v", command)
		}
	}
}

func TestEmitConfigTokenFailsLoudlyWhenClipboardUnavailable(t *testing.T) {
	t.Setenv("PATH", "")

	var err error
	stdout, _ := captureStdoutStderr(t, func() {
		err = emitConfigToken("very-secret-token-value", false)
	})

	if err == nil {
		t.Fatal("emitConfigToken() returned nil with no clipboard available; the command would exit 0 having done nothing")
	}
	if strings.Contains(err.Error(), "very-secret-token-value") {
		t.Errorf("the failure message leaks the token: %v", err)
	}
	if !strings.Contains(err.Error(), "--stdout") {
		t.Errorf("the failure names no supported way to get the token on a headless host: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty; a caller capturing $(...) must not receive prose", stdout)
	}
}

func TestConfigSetAllowsDashPrefixedValue(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "set", "browser.extraFlags", "--disable-gpu --ash-no-nudges"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if !strings.Contains(output, "Set browser.extraFlags = --disable-gpu --ash-no-nudges") {
		t.Fatalf("expected success output, got %q", output)
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Browser.BrowserExtraFlags != "--disable-gpu --ash-no-nudges" {
		t.Fatalf("BrowserExtraFlags = %q, want %q", saved.Browser.BrowserExtraFlags, "--disable-gpu --ash-no-nudges")
	}
}

func TestConfigSetRejectsUnsafeChromeExtraFlags(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	var execErr error
	stderr := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"config", "set", "browser.extraFlags", "--no-sandbox --disable-gpu"})
		execErr = rootCmd.Execute()
	})

	if execErr == nil {
		t.Fatalf("expected Execute() to return error for declined unsafe save")
	}
	if !strings.Contains(stderr, "browser.extraFlags") || !strings.Contains(stderr, "runtime compatibility") {
		t.Fatalf("expected unsafe flag warning on stderr, got %q", stderr)
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Browser.BrowserExtraFlags != "" {
		t.Fatalf("BrowserExtraFlags = %q, want empty string after declining unsafe save", saved.Browser.BrowserExtraFlags)
	}
}

func TestConfigShowLoadsLegacyFlatConfigWithoutToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	if err := os.WriteFile(configPath, []byte(`{
  "port": "8765",
  "headless": true,
  "maxTabs": 30
}`), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "show"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if !strings.Contains(output, "8765") {
		t.Fatalf("expected config show output to contain legacy port, got %q", output)
	}
	if !strings.Contains(output, "Current configuration") {
		t.Fatalf("expected config show header, got %q", output)
	}
}

func TestConfigShowIncludesTrustLoopbackProxy(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	fc := config.DefaultFileConfig()
	if fc.Security.TrustLoopbackProxy == nil {
		t.Fatal("default trustLoopbackProxy pointer is nil")
	}
	*fc.Security.TrustLoopbackProxy = true
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "show"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if !strings.Contains(output, "Trust Loopback Proxy: true") {
		t.Fatalf("expected config show output to include trust loopback proxy setting, got %q", output)
	}
}

func TestConfigShowIncludesIDPIAndAllowedDomains(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	fc := config.DefaultFileConfig()
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "show"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	// The browsing allowlist is the most security-relevant setting a user edits;
	// it must be visible in `config show` so an edit can be confirmed there.
	if !strings.Contains(output, "Allowed Domains:") {
		t.Errorf("config show output missing 'Allowed Domains:'; got %q", output)
	}
	if !strings.Contains(output, "IDPI:") {
		t.Errorf("config show output missing 'IDPI:'; got %q", output)
	}
	if !strings.Contains(output, "localhost") {
		t.Errorf("expected default allowlist (localhost) in output; got %q", output)
	}
}

func TestConfigGetMasksServerToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	fc := config.DefaultFileConfig()
	fc.Server.Token = "very-secret-token-value"
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "get", "server.token"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if strings.Contains(output, "very-secret-token-value") {
		t.Fatalf("expected token to stay masked, got %q", output)
	}
	if !strings.Contains(output, "very...alue") {
		t.Fatalf("expected masked token, got %q", output)
	}
}

func TestConfigTokenSubcommandCopiesToClipboard(t *testing.T) {
	originalLookPath := clipboardLookPath
	originalExecCommand := clipboardExecCommand
	clipboardLookPath = func(name string) (string, error) { return name, nil }
	clipboardExecCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	}
	t.Cleanup(func() {
		clipboardLookPath = originalLookPath
		clipboardExecCommand = originalExecCommand
	})

	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	fc := config.DefaultFileConfig()
	fc.Server.Token = "test-token-for-clipboard"
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	stdout, stderr := captureStdoutStderr(t, func() {
		rootCmd.SetArgs([]string{"config", "token"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if strings.Contains(stdout+stderr, "test-token-for-clipboard") {
		t.Fatalf("expected token to stay hidden, got stdout=%q stderr=%q", stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty; the human message belongs on stderr so $(...) captures nothing", stdout)
	}
	if !strings.Contains(stderr, "Token copied to clipboard") {
		t.Fatalf("expected clipboard success message on stderr, got %q", stderr)
	}
}

func TestCopyToClipboardTimesOut(t *testing.T) {
	originalLookPath := clipboardLookPath
	originalExecCommand := clipboardExecCommand
	originalTimeout := clipboardTimeout
	clipboardLookPath = func(name string) (string, error) { return name, nil }
	clipboardTimeout = 20 * time.Millisecond
	clipboardExecCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestClipboardBlockingHelper")
		cmd.Env = append(os.Environ(), "PINCHTAB_CLIPBOARD_BLOCKING_HELPER=1")
		return cmd
	}
	t.Cleanup(func() {
		clipboardLookPath = originalLookPath
		clipboardExecCommand = originalExecCommand
		clipboardTimeout = originalTimeout
	})

	started := time.Now()
	err := copyToClipboard("test-token")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("copyToClipboard() error = %v, want timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("copyToClipboard() took %s, want bounded completion", elapsed)
	}
}

func TestClipboardBlockingHelper(t *testing.T) {
	if os.Getenv("PINCHTAB_CLIPBOARD_BLOCKING_HELPER") != "1" {
		return
	}
	select {}
}

func TestConfigSchemaSubcommandPrintsURL(t *testing.T) {
	resetConfigSchemaPrintFlag(t)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetConfigSchemaPrintFlag(t)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "schema"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if got := strings.TrimSpace(output); got != config.CurrentConfigSchemaURL() {
		t.Fatalf("config schema output = %q, want %q", got, config.CurrentConfigSchemaURL())
	}
}

func TestConfigSchemaSubcommandPrintsBundledSchema(t *testing.T) {
	resetConfigSchemaPrintFlag(t)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetConfigSchemaPrintFlag(t)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "schema", "--print"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	var raw map[string]any
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		t.Fatalf("schema output is not valid JSON: %v\n%s", err, output)
	}
	if raw["$id"] != config.CurrentConfigSchemaURL() {
		t.Fatalf("schema $id = %q, want %q", raw["$id"], config.CurrentConfigSchemaURL())
	}
}

func TestEmitConfigTokenReturnsErrorWhenEmpty(t *testing.T) {
	err := emitConfigToken("", false)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty token error, got %q", err.Error())
	}
}

func resetConfigSchemaPrintFlag(t *testing.T) {
	t.Helper()

	cmd, _, err := rootCmd.Find([]string{"config", "schema"})
	if err != nil {
		t.Fatalf("find config schema command: %v", err)
	}
	if err := cmd.Flags().Set("print", "false"); err != nil {
		t.Fatalf("reset schema print flag: %v", err)
	}
}

// restartHintForMode names `pinchtab server restart` only for the server/daemon
// front door ("dashboard"); a bridge (or an unknown mode) gets a mode-neutral
// instruction, because that command would stop a bridge. This is the config-set
// path counterpart of the capability-gate remedy test.
func TestRestartHintForMode(t *testing.T) {
	dashboard := restartHintForMode("dashboard")
	if !strings.Contains(dashboard, "pinchtab server restart") {
		t.Errorf("dashboard hint should name the server restart command: %q", dashboard)
	}
	for _, mode := range []string{"bridge", "server", ""} {
		hint := restartHintForMode(mode)
		if strings.Contains(hint, "pinchtab server restart") {
			t.Errorf("mode %q hint names `pinchtab server restart`, which is wrong off the dashboard: %q", mode, hint)
		}
		if !strings.Contains(strings.ToLower(hint), "restart") {
			t.Errorf("mode %q hint does not tell the caller to restart: %q", mode, hint)
		}
	}
}

func TestConfigSetHintsRestartWhenServerRunning(t *testing.T) {
	// Spin up a fake dashboard health endpoint so hintRestartIfRunning detects a
	// running server (mode "dashboard"), the surface where `pinchtab server restart`
	// is the correct command.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok","mode":"dashboard"}`))
	}))
	defer srv.Close()

	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]

	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	// Write a minimal valid config pointing at the fake server port.
	configJSON := []byte(`{"configVersion":"0.8.0","server":{"port":"` + port + `","token":"test-token-for-restart-hint-00000"}}`)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			rootCmd.SetArgs([]string{"config", "set", "security.allowScreencast", "true"})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "restart") {
		t.Fatalf("expected restart hint on stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "pinchtab server restart") {
		t.Fatalf("expected 'pinchtab server restart' in hint, got %q", stderr)
	}
}

// AC: config set of a capability while a BRIDGE is running must not print a hint
// that names `pinchtab server restart` — running it would kill the bridge.
func TestConfigSetHintDoesNotNameServerRestartForBridge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok","mode":"bridge"}`))
	}))
	defer srv.Close()

	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]

	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	configJSON := []byte(`{"configVersion":"0.8.0","server":{"port":"` + port + `","token":"test-token-for-restart-hint-00000"}}`)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			rootCmd.SetArgs([]string{"config", "set", "security.allowScreencast", "true"})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})
	})

	if strings.Contains(stderr, "pinchtab server restart") {
		t.Fatalf("bridge-mode config set named `pinchtab server restart`, which would kill the bridge: %q", stderr)
	}
	if !strings.Contains(stderr, "restart") {
		t.Fatalf("expected a mode-neutral restart hint on stderr, got %q", stderr)
	}
}

// A running instance whose token the CLI cannot present answers /health with
// 401/403. It is still running and still needs a restart to apply the change, so
// the hint must print — mode-neutral, since the mode is hidden. This pins the
// regression where switching off CheckPinchTabRunning (any 200) to a strict probe
// left a protected instance with no hint at all.
func TestConfigSetHintsRestartForProtectedInstance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]

	configPath := filepath.Join(t.TempDir(), "pinchtab", "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	configJSON := []byte(`{"configVersion":"0.8.0","server":{"port":"` + port + `","token":"test-token-for-restart-hint-00000"}}`)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, configJSON, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	stderr := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			rootCmd.SetArgs([]string{"config", "set", "security.allowScreencast", "true"})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "restart") {
		t.Fatalf("a protected running instance got no restart hint: %q", stderr)
	}
	if strings.Contains(stderr, "pinchtab server restart") {
		t.Fatalf("a protected instance's mode is unknown, so the hint must stay neutral: %q", stderr)
	}
}

func TestIsSensitiveConfigPath(t *testing.T) {
	cases := map[string]bool{
		"server.token":                            true,
		"instanceDefaults.proxy.password":         true,
		"autosolver.credentials.capsolver.apiKey": true,
		"cloak.fontsDir":                          false,
		"strategy":                                false,
		"":                                        false,
	}
	for path, want := range cases {
		if got := isSensitiveConfigPath(path); got != want {
			t.Errorf("isSensitiveConfigPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestConfigSetMasksServerTokenInOutput(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fc := config.DefaultFileConfig()
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "set", "server.token", "very-secret-token-value"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if strings.Contains(output, "very-secret-token-value") {
		t.Fatalf("expected token to stay masked, got %q", output)
	}
	if !strings.Contains(output, "very...alue") {
		t.Fatalf("expected masked token in success output, got %q", output)
	}
}
