package dashboard

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type profileLister interface {
	List() ([]bridge.ProfileInfo, error)
}

type runtimeConfigApplier interface {
	ApplyRuntimeConfig(*config.RuntimeConfig)
}

type agentCounter interface {
	AgentCount() int
}

type ConfigAPI struct {
	runtime   *config.RuntimeConfig
	instances InstanceLister
	profiles  profileLister
	applier   runtimeConfigApplier
	agents    agentCounter
	sessions  *browsersession.Manager
	version   string
	startedAt time.Time
	boot      config.FileConfig
	mu        sync.RWMutex

	// Config-file snapshot cached by mtime so read-only health/config polls answer
	// from memory instead of re-reading + parsing the file every request. Guarded
	// by mu; invalidated on write (persistAndApply) and on any mtime change.
	cfgCacheValid   bool
	cfgCachePath    string
	cfgCacheMtime   time.Time
	cfgCacheFC      config.FileConfig
	cfgCacheReasons []string
}

type configEnvelope struct {
	Config          config.FileConfig `json:"config"`
	ConfigPath      string            `json:"configPath"`
	TokenConfigured bool              `json:"tokenConfigured"`
	RestartRequired bool              `json:"restartRequired"`
	RestartReasons  []string          `json:"restartReasons,omitempty"`
}

func NewConfigAPI(
	runtime *config.RuntimeConfig,
	instances InstanceLister,
	profiles profileLister,
	applier runtimeConfigApplier,
	agents agentCounter,
	version string,
	startedAt time.Time,
) *ConfigAPI {
	boot := config.DefaultFileConfig()
	// Snapshot the on-disk file config at boot so restart detection compares
	// file-at-boot against the current file, not a lossy runtime reconstruction.
	if fc, _, err := config.LoadFileConfig(); err == nil && fc != nil {
		boot = *fc
	}
	return &ConfigAPI{
		runtime:   runtime,
		instances: instances,
		profiles:  profiles,
		applier:   applier,
		agents:    agents,
		version:   version,
		startedAt: startedAt,
		boot:      boot,
	}
}

func (c *ConfigAPI) SetSessionManager(sessions *browsersession.Manager) {
	if c == nil {
		return
	}
	c.sessions = sessions
}

func (c *ConfigAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config", c.HandleGetConfig)
	mux.HandleFunc("PUT /api/config", c.HandlePutConfig)
}

func (c *ConfigAPI) HandleHealth(w http.ResponseWriter, r *http.Request) {
	info, err := c.healthInfo(healthSecurityVisibleTo(r))
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	httpx.JSON(w, 200, info)
}

func (c *ConfigAPI) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, path, restartReasons, err := c.currentConfig()
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	httpx.JSON(w, 200, c.configEnvelopeFor(cfg, path, restartReasons))
}

func (c *ConfigAPI) HandlePutConfig(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	current, path, err := config.LoadFileConfig()
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	normalized, ok := c.parseConfigUpdate(w, r, current)
	if !ok {
		return
	}
	changes, ok := c.authorizeSensitiveChanges(w, r, current, &normalized)
	if !ok {
		return
	}
	if !c.persistAndApply(w, &normalized, path) {
		return
	}
	c.respondConfigUpdated(w, r, normalized, path, changes)
}

// parseConfigUpdate reads the PUT body, rejects write-only token changes, decodes it onto a
// copy of the current config, restores redacted secrets, and validates the result.
func (c *ConfigAPI) parseConfigUpdate(w http.ResponseWriter, r *http.Request, current *config.FileConfig) (config.FileConfig, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.ErrorCode(w, 400, "bad_config_json", "invalid config payload", false, nil)
		return config.FileConfig{}, false
	}

	var tokenProbe struct {
		Server struct {
			Token *string `json:"token"`
		} `json:"server"`
	}
	if err := json.Unmarshal(body, &tokenProbe); err != nil {
		httpx.ErrorCode(w, 400, "bad_config_json", "invalid config payload", false, nil)
		return config.FileConfig{}, false
	}
	if tokenProbe.Server.Token != nil && strings.TrimSpace(*tokenProbe.Server.Token) != "" {
		httpx.ErrorCode(w, 400, "token_write_only", "manage the API token outside the dashboard", false, nil)
		return config.FileConfig{}, false
	}

	normalized := *current
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&normalized); err != nil {
		httpx.ErrorCode(w, 400, "bad_config_json", "invalid config payload", false, nil)
		return config.FileConfig{}, false
	}
	config.NormalizeFileConfigAliasesFromJSON(&normalized, body)
	preserveWriteOnlyConfigFields(&normalized, current)

	if errs := config.ValidateFileConfig(&normalized); len(errs) > 0 {
		messages := make([]string, 0, len(errs))
		for _, validationErr := range errs {
			messages = append(messages, validationErr.Error())
		}
		httpx.ErrorCode(w, 400, "invalid_config", "config validation failed", false, map[string]any{
			"errors": messages,
		})
		return config.FileConfig{}, false
	}
	return normalized, true
}

// authorizeSensitiveChanges computes the sensitive change set and rejects proxy/security
// changes that require elevation the caller doesn't have.
func (c *ConfigAPI) authorizeSensitiveChanges(w http.ResponseWriter, r *http.Request, current, normalized *config.FileConfig) (sensitiveConfigChangeSet, bool) {
	changes := sensitiveConfigChanges(current, normalized)
	if changes.requiresElevation && !c.hasConfigWriteElevation(r) {
		authn.AuditWarn(r, "config.update_elevation_required", "changes", changes.names)
		httpx.ErrorCode(w, http.StatusForbidden, "session_elevation_required", "re-enter the API token before changing proxy or security settings", false, nil)
		return changes, false
	}
	return changes, true
}

// persistAndApply writes the normalized config to disk and applies it to the runtime,
// session manager, and applier.
func (c *ConfigAPI) persistAndApply(w http.ResponseWriter, normalized *config.FileConfig, path string) bool {
	if err := config.SaveFileConfig(normalized, path); err != nil {
		httpx.Error(w, 500, err)
		return false
	}
	// Invalidate the mtime cache (caller holds c.mu): covers the rare case of two
	// writes landing in the same filesystem mtime tick.
	c.cfgCacheValid = false

	config.ApplyFileConfigToRuntime(c.runtime, normalized)
	if c.sessions != nil {
		c.sessions.UpdateConfig(BrowserSessionConfig(c.runtime))
	}
	if c.applier != nil {
		c.applier.ApplyRuntimeConfig(c.runtime)
	}
	return true
}

// respondConfigUpdated audits the change and writes the updated config envelope.
func (c *ConfigAPI) respondConfigUpdated(w http.ResponseWriter, r *http.Request, normalized config.FileConfig, path string, changes sensitiveConfigChangeSet) {
	restartReasons := c.restartReasonsFor(normalized)
	if changes.proxyChanged {
		authn.AuditLog(r, "config.proxy_changed",
			"scopes", changes.proxyScopes,
			"proxies", changes.proxyAudit,
		)
	}
	authn.AuditLog(r, "config.updated",
		"restartRequired", len(restartReasons) > 0,
		"restartReasons", restartReasons,
	)
	httpx.JSON(w, 200, c.configEnvelopeFor(normalized, path, restartReasons))
}

func (c *ConfigAPI) hasConfigWriteElevation(r *http.Request) bool {
	if c == nil || c.runtime == nil || strings.TrimSpace(c.runtime.Token) == "" {
		return true
	}
	creds := authn.CredentialsFromRequest(r)
	if creds.Method != authn.MethodCookie {
		return true
	}
	return c.sessions != nil && c.sessions.IsElevated(creds.Value, c.runtime.Token)
}

func (c *ConfigAPI) currentConfig() (config.FileConfig, string, []string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	path := config.ConfigFilePath()
	var mtime time.Time
	if info, err := os.Stat(path); err == nil {
		mtime = info.ModTime()
	}
	// Missing file → zero mtime; LoadFileConfig returns defaults; caching that is
	// fine and a later file creation (nonzero mtime) invalidates the cache.

	if c.cfgCacheValid && c.cfgCachePath == path && c.cfgCacheMtime.Equal(mtime) {
		return c.cfgCacheFC, path, append([]string(nil), c.cfgCacheReasons...), nil
	}

	fc, loadedPath, err := config.LoadFileConfig()
	if err != nil {
		// Do not poison the cache on a transient load error; leave any prior
		// valid snapshot intact and propagate the error.
		return config.FileConfig{}, "", nil, err
	}
	reasons := c.restartReasonsFor(*fc)

	c.cfgCacheValid = true
	c.cfgCachePath = path
	c.cfgCacheMtime = mtime
	c.cfgCacheFC = *fc
	c.cfgCacheReasons = reasons
	return *fc, loadedPath, append([]string(nil), reasons...), nil
}

func (c *ConfigAPI) configEnvelopeFor(cfg config.FileConfig, path string, restartReasons []string) configEnvelope {
	return configEnvelope{
		Config:          redactToken(cfg),
		ConfigPath:      path,
		TokenConfigured: c.tokenConfigured(cfg),
		RestartRequired: len(restartReasons) > 0,
		RestartReasons:  restartReasons,
	}
}
