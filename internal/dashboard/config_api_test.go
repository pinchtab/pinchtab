package dashboard

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/pinchtab/pinchtab/internal/browsers/all"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
)

type stubAgentCounter struct {
	count int
}

func (s stubAgentCounter) AgentCount() int { return s.count }

func TestNewConfigAPISnapshotsBootConfigFromFile(t *testing.T) {
	defaults := config.DefaultFileConfig()
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	fileConfig := map[string]any{
		"configVersion": defaults.ConfigVersion,
		"server": map[string]any{
			"port":     defaults.Server.Port,
			"bind":     defaults.Server.Bind,
			"stateDir": defaults.Server.StateDir,
		},
		"profiles": map[string]any{
			"baseDir":        defaults.Profiles.BaseDir,
			"defaultProfile": defaults.Profiles.DefaultProfile,
		},
		"multiInstance": map[string]any{
			"strategy": defaults.MultiInstance.Strategy,
			"restart": map[string]any{
				"maxRestarts":    nil,
				"initBackoffSec": nil,
				"maxBackoffSec":  nil,
				"stableAfterSec": nil,
			},
		},
	}

	data, err := json.Marshal(fileConfig)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runtime := config.Load()
	api := NewConfigAPI(runtime, nil, nil, nil, nil, "test", time.Now())

	if api.boot.MultiInstance.Restart.MaxRestarts != nil {
		t.Fatalf("boot restart maxRestarts = %v, want nil from file snapshot", *api.boot.MultiInstance.Restart.MaxRestarts)
	}

	_, path, restartReasons, err := api.currentConfig()
	if err != nil {
		t.Fatalf("currentConfig() error = %v", err)
	}
	if path != configPath {
		t.Fatalf("currentConfig() path = %q, want %q", path, configPath)
	}
	if len(restartReasons) != 0 {
		t.Fatalf("currentConfig() restartReasons = %v, want none", restartReasons)
	}
}

// TestCurrentConfigCachesByMtime verifies currentConfig serves the cached snapshot
// while the file mtime is unchanged and reloads only when the mtime advances.
func TestCurrentConfigCachesByMtime(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)

	if err := os.WriteFile(configPath, []byte(`{"server":{"port":"8888"}}`), 0644); err != nil {
		t.Fatalf("WriteFile A: %v", err)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	origMtime := info.ModTime()

	api := NewConfigAPI(config.Load(), nil, nil, nil, nil, "test", time.Now())

	cfg, _, _, err := api.currentConfig()
	if err != nil {
		t.Fatalf("currentConfig A: %v", err)
	}
	if cfg.Server.Port != "8888" {
		t.Fatalf("port = %q, want 8888", cfg.Server.Port)
	}

	// Rewrite with different content but FORCE the same mtime: the cache must serve
	// the stale snapshot (proving it did not re-read the file).
	if err := os.WriteFile(configPath, []byte(`{"server":{"port":"9999"}}`), 0644); err != nil {
		t.Fatalf("WriteFile B: %v", err)
	}
	if err := os.Chtimes(configPath, origMtime, origMtime); err != nil {
		t.Fatalf("Chtimes same: %v", err)
	}
	cfg, _, _, err = api.currentConfig()
	if err != nil {
		t.Fatalf("currentConfig cached: %v", err)
	}
	if cfg.Server.Port != "8888" {
		t.Fatalf("port = %q, want 8888 (cached; same mtime should not reload)", cfg.Server.Port)
	}

	// Advance the mtime: the cache must invalidate and reload the new content.
	later := origMtime.Add(2 * time.Second)
	if err := os.Chtimes(configPath, later, later); err != nil {
		t.Fatalf("Chtimes later: %v", err)
	}
	cfg, _, _, err = api.currentConfig()
	if err != nil {
		t.Fatalf("currentConfig reload: %v", err)
	}
	if cfg.Server.Port != "9999" {
		t.Fatalf("port = %q, want 9999 (mtime changed → reload)", cfg.Server.Port)
	}
}

func TestRestartReasonsIncludeStealthLevel(t *testing.T) {
	cfg := config.DefaultFileConfig()
	api := NewConfigAPI(config.Load(), nil, nil, nil, nil, "test", time.Now())
	api.boot = cfg

	next := cfg
	next.InstanceDefaults.StealthLevel = "full"

	reasons := api.restartReasonsFor(next)
	if !slices.Contains(reasons, "Stealth level") {
		t.Fatalf("restartReasonsFor() = %v, want Stealth level", reasons)
	}
}

func TestHandleGetConfigRedactsToken(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	stateKey := "state-secret"
	fc.Security.StateEncryptionKey = &stateKey
	fc.AutoSolver.External.CapsolverKey = "capsolver-secret"
	fc.AutoSolver.External.TwoCaptchaKey = "twocaptcha-secret"

	api := newConfigAPITestAPI(t, fc)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	api.HandleGetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandleGetConfig() status = %d, want %d", w.Code, http.StatusOK)
	}

	env := decodeConfigEnvelope(t, w)
	if env.Config.Server.Token != "" {
		t.Fatalf("config token = %q, want redacted empty string", env.Config.Server.Token)
	}
	if env.Config.Security.StateEncryptionKey != nil {
		t.Fatalf("config stateEncryptionKey = %v, want nil", env.Config.Security.StateEncryptionKey)
	}
	if env.Config.AutoSolver.External.CapsolverKey != "" {
		t.Fatalf("config capsolverKey = %q, want redacted empty string", env.Config.AutoSolver.External.CapsolverKey)
	}
	if env.Config.AutoSolver.External.TwoCaptchaKey != "" {
		t.Fatalf("config twoCaptchaKey = %q, want redacted empty string", env.Config.AutoSolver.External.TwoCaptchaKey)
	}
	if !env.TokenConfigured {
		t.Fatal("tokenConfigured = false, want true")
	}
}

func TestHandlePutConfigPreservesExistingToken(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	stateKey := "state-secret"
	fc.Security.StateEncryptionKey = &stateKey
	fc.AutoSolver.External.CapsolverKey = "capsolver-secret"
	fc.AutoSolver.External.TwoCaptchaKey = "twocaptcha-secret"

	api := newConfigAPITestAPI(t, fc)

	payload := config.DefaultFileConfig()
	payload.Server.Port = "9999"

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandlePutConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandlePutConfig() status = %d, want %d", w.Code, http.StatusOK)
	}

	env := decodeConfigEnvelope(t, w)
	if env.Config.Server.Token != "" {
		t.Fatalf("response token = %q, want redacted empty string", env.Config.Server.Token)
	}
	if !env.TokenConfigured {
		t.Fatal("tokenConfigured = false, want true")
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Server.Token != "secret-token" {
		t.Fatalf("saved token = %q, want existing token preserved", saved.Server.Token)
	}
	if saved.Security.StateEncryptionKey == nil || *saved.Security.StateEncryptionKey != stateKey {
		t.Fatalf("saved stateEncryptionKey = %v, want existing key preserved", saved.Security.StateEncryptionKey)
	}
	if saved.AutoSolver.External.CapsolverKey != "capsolver-secret" {
		t.Fatalf("saved capsolverKey = %q, want existing key preserved", saved.AutoSolver.External.CapsolverKey)
	}
	if saved.AutoSolver.External.TwoCaptchaKey != "twocaptcha-secret" {
		t.Fatalf("saved twoCaptchaKey = %q, want existing key preserved", saved.AutoSolver.External.TwoCaptchaKey)
	}
	if saved.Server.Port != "9999" {
		t.Fatalf("saved port = %q, want %q", saved.Server.Port, "9999")
	}
}

func TestHandlePutConfigPreservesWriteOnlySecretsFromRedactedGetPayload(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	stateKey := "state-secret"
	fc.Security.StateEncryptionKey = &stateKey
	fc.AutoSolver.External.CapsolverKey = "capsolver-secret"
	fc.AutoSolver.External.TwoCaptchaKey = "twocaptcha-secret"

	api := newConfigAPITestAPI(t, fc)
	sessions := browsersession.NewManager(browsersession.Config{ElevationWindow: time.Minute})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	api.SetSessionManager(sessions)

	getReq := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	getRes := httptest.NewRecorder()
	api.HandleGetConfig(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("HandleGetConfig() status = %d, want %d", getRes.Code, http.StatusOK)
	}

	env := decodeConfigEnvelope(t, getRes)
	env.Config.Server.Port = "9898"

	body, err := json.Marshal(env.Config)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	putReq := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	putReq.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	putRes := httptest.NewRecorder()
	api.HandlePutConfig(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("HandlePutConfig() status = %d, want %d", putRes.Code, http.StatusOK)
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Server.Token != "secret-token" {
		t.Fatalf("saved token = %q, want existing token preserved", saved.Server.Token)
	}
	if saved.Security.StateEncryptionKey == nil || *saved.Security.StateEncryptionKey != stateKey {
		t.Fatalf("saved stateEncryptionKey = %v, want existing key preserved", saved.Security.StateEncryptionKey)
	}
	if saved.AutoSolver.External.CapsolverKey != "capsolver-secret" {
		t.Fatalf("saved capsolverKey = %q, want existing key preserved", saved.AutoSolver.External.CapsolverKey)
	}
	if saved.AutoSolver.External.TwoCaptchaKey != "twocaptcha-secret" {
		t.Fatalf("saved twoCaptchaKey = %q, want existing key preserved", saved.AutoSolver.External.TwoCaptchaKey)
	}
	if saved.Server.Port != "9898" {
		t.Fatalf("saved port = %q, want %q", saved.Server.Port, "9898")
	}
}

func TestHandlePutConfigPreservesUnspecifiedAttachSettings(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	attachEnabled := true
	fc.Security.Attach.Enabled = &attachEnabled
	fc.Security.Attach.AllowHosts = []string{"127.0.0.1", "pinchtab-bridge"}
	fc.Security.Attach.AllowSchemes = []string{"http", "https"}

	api := newConfigAPITestAPI(t, fc)

	body := []byte(`{"server":{"trustProxyHeaders":true}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandlePutConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandlePutConfig() status = %d, want %d", w.Code, http.StatusOK)
	}

	saved, _, err := config.LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v", err)
	}
	if saved.Security.Attach.Enabled == nil || !*saved.Security.Attach.Enabled {
		t.Fatalf("saved attach.enabled = %v, want true", saved.Security.Attach.Enabled)
	}
	if len(saved.Security.Attach.AllowHosts) != 2 || saved.Security.Attach.AllowHosts[1] != "pinchtab-bridge" {
		t.Fatalf("saved attach.allowHosts = %v, want preserved hosts", saved.Security.Attach.AllowHosts)
	}
	if len(saved.Security.Attach.AllowSchemes) != 2 || saved.Security.Attach.AllowSchemes[0] != "http" {
		t.Fatalf("saved attach.allowSchemes = %v, want preserved schemes", saved.Security.Attach.AllowSchemes)
	}
}

func TestHandlePutConfigRejectsWriteOnlyTokenField(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"

	api := newConfigAPITestAPI(t, fc)

	payload := config.DefaultFileConfig()
	payload.Server.Token = "replacement-token"

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandlePutConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("HandlePutConfig() status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "token_write_only") {
		t.Fatalf("response = %q, want token_write_only error", w.Body.String())
	}
}

func TestHandlePutConfigRequiresElevationForProxyChangeWithDashboardCookie(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	api := newConfigAPITestAPI(t, fc)
	sessions := browsersession.NewManager(browsersession.Config{ElevationWindow: time.Minute})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	api.SetSessionManager(sessions)

	payload := config.DefaultFileConfig()
	payload.Browser.Proxy = config.BrowserProxyConfig{
		Server:   "http://proxy.example.com:8080",
		Username: "alice",
		Password: "secret",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	w := httptest.NewRecorder()
	api.HandlePutConfig(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("HandlePutConfig() status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "session_elevation_required") {
		t.Fatalf("response = %q, want session_elevation_required", w.Body.String())
	}
}

func TestHandlePutConfigAllowsElevatedProxyChange(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	api := newConfigAPITestAPI(t, fc)
	sessions := browsersession.NewManager(browsersession.Config{ElevationWindow: time.Minute})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	if !sessions.Elevate(sessionID, "secret-token") {
		t.Fatal("Elevate() = false, want true")
	}
	api.SetSessionManager(sessions)

	payload := config.DefaultFileConfig()
	payload.Browser.Proxy = config.BrowserProxyConfig{
		Server:   "http://proxy.example.com:8080",
		Username: "alice",
		Password: "secret",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	w := httptest.NewRecorder()
	api.HandlePutConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandlePutConfig() status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandlePutConfigAuditsProxyChangeServers(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "secret-token"
	api := newConfigAPITestAPI(t, fc)
	sessions := browsersession.NewManager(browsersession.Config{ElevationWindow: time.Minute})
	sessionID, err := sessions.Create("secret-token")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	if !sessions.Elevate(sessionID, "secret-token") {
		t.Fatal("Elevate() = false, want true")
	}
	api.SetSessionManager(sessions)

	payload := config.DefaultFileConfig()
	payload.Browser.Proxy = config.BrowserProxyConfig{
		Server:   "http://proxy.example.com:8080",
		Username: "alice",
		Password: "secret",
	}
	payload.Browser.Targets = config.BrowserTargetsConfig{
		"proxy-target": {
			Provider: config.BrowserChrome,
			Proxy: config.BrowserProxyConfig{
				Server:   "socks5://10.0.0.1:1080",
				Username: "bob",
				Password: "target-secret",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
	w := httptest.NewRecorder()
	logs := captureDashboardSlog(t, func() {
		api.HandlePutConfig(w, req)
	})

	if w.Code != http.StatusOK {
		t.Fatalf("HandlePutConfig() status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	for _, needle := range []string{
		`"event":"config.proxy_changed"`,
		`"scope":"browser.proxy"`,
		`"server":"http://proxy.example.com:8080"`,
		`"scope":"browser.targets.proxy-target.proxy"`,
		`"server":"socks5://10.0.0.1:1080"`,
	} {
		if !strings.Contains(logs, needle) {
			t.Fatalf("expected audit log to contain %q\n%s", needle, logs)
		}
	}
	for _, secret := range []string{"secret-token", "secret", "target-secret"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("audit log leaked secret %q\n%s", secret, logs)
		}
	}
}

func TestHandleHealthIncludesAgentCount(t *testing.T) {
	fc := config.DefaultFileConfig()
	api := newConfigAPITestAPI(t, fc)
	api.agents = stubAgentCounter{count: 3}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	api.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandleHealth() status = %d, want %d", w.Code, http.StatusOK)
	}

	var health healthEnvelope
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if health.Agents != 3 {
		t.Fatalf("health agents = %d, want 3", health.Agents)
	}
}

func captureDashboardSlog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(old)
	fn()
	return buf.String()
}

func TestHandleHealthSecurityVisibilityByAuthMethod(t *testing.T) {
	fc := config.DefaultFileConfig()
	api := newConfigAPITestAPI(t, fc)

	tests := []struct {
		name         string
		authHeader   string
		cookie       bool
		wantSecurity bool
	}{
		{name: "bearer", authHeader: "Bearer secret-token", wantSecurity: true},
		{name: "dashboard cookie", cookie: true, wantSecurity: true},
		{name: "agent session", authHeader: "Session ses_test", wantSecurity: false},
		{name: "no auth context", wantSecurity: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.cookie {
				req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: "dashboard-session"})
			}
			w := httptest.NewRecorder()
			api.HandleHealth(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("HandleHealth() status = %d, want %d", w.Code, http.StatusOK)
			}
			var health healthEnvelope
			if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if got := health.Security != nil; got != tt.wantSecurity {
				t.Fatalf("health.Security present = %v, want %v", got, tt.wantSecurity)
			}
		})
	}
}

func newConfigAPITestAPI(t *testing.T, fc config.FileConfig) *ConfigAPI {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	if err := config.SaveFileConfig(&fc, configPath); err != nil {
		t.Fatalf("SaveFileConfig() error = %v", err)
	}

	return NewConfigAPI(config.Load(), nil, nil, nil, nil, "test", time.Now())
}

func decodeConfigEnvelope(t *testing.T, w *httptest.ResponseRecorder) configEnvelope {
	t.Helper()

	var env configEnvelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return env
}

// TestRedactTokenCoversAllSensitiveFields uses reflection to find fields that look
// like secrets (containing "key", "token", "secret", "password", "credential" in their
// name) and verifies they are all redacted. This test will fail if a new sensitive
// field is added to FileConfig without updating redactToken().
func TestRedactTokenCoversAllSensitiveFields(t *testing.T) {
	fc := config.DefaultFileConfig()
	fc.Server.Token = "test-token"
	encKey := "test-encryption-key"
	fc.Security.StateEncryptionKey = &encKey
	fc.AutoSolver.External.CapsolverKey = "test-capsolver-key"
	fc.AutoSolver.External.TwoCaptchaKey = "test-twocaptcha-key"
	fc.Browser.Proxy = config.BrowserProxyConfig{
		Server:   "http://proxy.example.com:8080",
		Username: "alice",
		Password: "raw-proxy-password",
	}
	fc.Browser.Targets = config.BrowserTargetsConfig{
		"with-proxy": {
			Provider: config.BrowserChrome,
			Proxy: config.BrowserProxyConfig{
				Server:   "socks5://10.0.0.1:1080",
				Username: "bob",
				Password: "target-proxy-password",
			},
		},
	}

	redacted := redactToken(fc)

	// Masked to "***" (not empty) so the dashboard knows credentials are configured.
	if redacted.Browser.Proxy.Password != "***" {
		t.Errorf("Browser.Proxy.Password not redacted: got %q", redacted.Browser.Proxy.Password)
	}
	if redacted.Browser.Proxy.Server != fc.Browser.Proxy.Server {
		t.Errorf("Browser.Proxy.Server should be preserved, got %q", redacted.Browser.Proxy.Server)
	}
	if tp := redacted.Browser.Targets["with-proxy"].Proxy.Password; tp != "***" {
		t.Errorf("per-target proxy password not redacted: got %q", tp)
	}
	if fc.Browser.Proxy.Password != "raw-proxy-password" {
		t.Errorf("redactToken mutated source Browser.Proxy.Password: %q", fc.Browser.Proxy.Password)
	}
	if fc.Browser.Targets["with-proxy"].Proxy.Password != "target-proxy-password" {
		t.Errorf("redactToken mutated source target proxy password")
	}

	var unredacted []string
	findSensitiveFields(reflect.ValueOf(redacted), "", &unredacted)

	if len(unredacted) > 0 {
		t.Fatalf("redactToken() did not redact sensitive fields: %v\n"+
			"Add these to redactToken() in config_api.go", unredacted)
	}
}

// findSensitiveFields recursively scans a struct for fields with names suggesting
// they contain secrets, and reports any that have non-zero values.
func findSensitiveFields(v reflect.Value, path string, unredacted *[]string) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	sensitivePatterns := []string{"key", "token", "secret", "password", "credential"}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldPath := field.Name
		if path != "" {
			fieldPath = path + "." + field.Name
		}

		fieldVal := v.Field(i)

		nameLower := strings.ToLower(field.Name)
		isSensitive := false
		for _, pattern := range sensitivePatterns {
			if strings.Contains(nameLower, pattern) {
				isSensitive = true
				break
			}
		}

		if isSensitive && !isZeroValue(fieldVal) && !isMaskedString(fieldVal) {
			*unredacted = append(*unredacted, fieldPath)
		}

		if fieldVal.Kind() == reflect.Struct || (fieldVal.Kind() == reflect.Ptr && fieldVal.Elem().Kind() == reflect.Struct) {
			findSensitiveFields(fieldVal, fieldPath, unredacted)
		}
	}
}

func isMaskedString(v reflect.Value) bool {
	if v.Kind() == reflect.String {
		return v.String() == "***"
	}
	return false
}

func isZeroValue(v reflect.Value) bool {
	if v.Kind() == reflect.Ptr {
		return v.IsNil()
	}
	return v.IsZero()
}
