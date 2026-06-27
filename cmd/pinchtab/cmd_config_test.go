package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		"allocation policy",
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

func TestCopyConfigTokenDoesNotPrintTokenWhenClipboardUnavailable(t *testing.T) {
	t.Setenv("PATH", "")

	output := captureStdout(t, func() {
		if err := copyConfigToken("very-secret-token-value"); err != nil {
			t.Fatalf("copyConfigToken() error = %v", err)
		}
	})

	if strings.Contains(output, "very-secret-token-value") {
		t.Fatalf("expected token to stay hidden, got %q", output)
	}
	if !strings.Contains(output, "token not shown for safety") {
		t.Fatalf("expected safe fallback message, got %q", output)
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

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"config", "token"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if strings.Contains(output, "test-token-for-clipboard") {
		t.Fatalf("expected token to stay hidden, got %q", output)
	}
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

func TestCopyConfigTokenReturnsErrorWhenEmpty(t *testing.T) {
	err := copyConfigToken("")
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

func TestConfigSetHintsRestartWhenServerRunning(t *testing.T) {
	// Spin up a fake health endpoint so hintRestartIfRunning detects a running server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
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
