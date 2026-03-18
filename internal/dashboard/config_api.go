package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type profileLister interface {
	List() ([]bridge.ProfileInfo, error)
}

type runtimeConfigApplier interface {
	ApplyRuntimeConfig(*config.RuntimeConfig)
}

type ConfigAPI struct {
	runtime   *config.RuntimeConfig
	instances InstanceLister
	profiles  profileLister
	applier   runtimeConfigApplier
	version   string
	startedAt time.Time
	boot      config.FileConfig
	mu        sync.RWMutex
}

type configEnvelope struct {
	Config          config.FileConfig `json:"config"`
	ConfigPath      string            `json:"configPath"`
	TokenConfigured bool              `json:"tokenConfigured"`
	RestartRequired bool              `json:"restartRequired"`
	RestartReasons  []string          `json:"restartReasons,omitempty"`
}

type healthInstanceInfo struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type healthEnvelope struct {
	Status          string              `json:"status"`
	Mode            string              `json:"mode"`
	Version         string              `json:"version"`
	Uptime          int64               `json:"uptime"`
	AuthRequired    bool                `json:"authRequired"`
	Profiles        int                 `json:"profiles"`
	Instances       int                 `json:"instances"`
	DefaultInstance *healthInstanceInfo `json:"defaultInstance,omitempty"`
	Agents          int                 `json:"agents"`
	RestartRequired bool                `json:"restartRequired"`
	RestartReasons  []string            `json:"restartReasons,omitempty"`
}

func NewConfigAPI(
	runtime *config.RuntimeConfig,
	instances InstanceLister,
	profiles profileLister,
	applier runtimeConfigApplier,
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
		version:   version,
		startedAt: startedAt,
		boot:      boot,
	}
}

func (c *ConfigAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config", c.HandleGetConfig)
	mux.HandleFunc("PUT /api/config", c.HandlePutConfig)
}

func (c *ConfigAPI) HandleHealth(w http.ResponseWriter, r *http.Request) {
	info, err := c.healthInfo()
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
	normalized := config.DefaultFileConfig()
	if err := json.NewDecoder(r.Body).Decode(&normalized); err != nil {
		httpx.ErrorCode(w, 400, "bad_config_json", "invalid config payload", false, nil)
		return
	}

	if errs := config.ValidateFileConfig(&normalized); len(errs) > 0 {
		messages := make([]string, 0, len(errs))
		for _, validationErr := range errs {
			messages = append(messages, validationErr.Error())
		}
		httpx.ErrorCode(w, 400, "invalid_config", "config validation failed", false, map[string]any{
			"errors": messages,
		})
		return
	}
	if strings.TrimSpace(normalized.Server.Token) != "" {
		httpx.ErrorCode(w, 400, "token_write_only", "manage the API token outside the dashboard", false, nil)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	current, path, err := config.LoadFileConfig()
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	normalized.Server.Token = current.Server.Token
	if err := config.SaveFileConfig(&normalized, path); err != nil {
		httpx.Error(w, 500, err)
		return
	}

	config.ApplyFileConfigToRuntime(c.runtime, &normalized)
	if c.applier != nil {
		c.applier.ApplyRuntimeConfig(c.runtime)
	}

	restartReasons := c.restartReasonsFor(normalized)
	authn.AuditLog(r, "config.updated",
		"restartRequired", len(restartReasons) > 0,
		"restartReasons", restartReasons,
	)
	httpx.JSON(w, 200, c.configEnvelopeFor(normalized, path, restartReasons))
}

func (c *ConfigAPI) healthInfo() (healthEnvelope, error) {
	_, _, restartReasons, err := c.currentConfig()
	if err != nil {
		return healthEnvelope{}, err
	}

	profileCount := 0
	if c.profiles != nil {
		profiles, err := c.profiles.List()
		if err == nil {
			profileCount = len(profiles)
		}
	}

	instanceCount := 0
	var defaultInst *healthInstanceInfo
	if c.instances != nil {
		instances := c.instances.List()
		instanceCount = len(instances)
		if len(instances) > 0 {
			defaultInst = &healthInstanceInfo{
				ID:     instances[0].ID,
				Status: instances[0].Status,
			}
		}
	}
	return healthEnvelope{
		Status:          "ok",
		Mode:            "dashboard",
		Version:         c.version,
		Uptime:          int64(time.Since(c.startedAt).Milliseconds()),
		AuthRequired:    strings.TrimSpace(c.runtime.Token) != "",
		Profiles:        profileCount,
		Instances:       instanceCount,
		DefaultInstance: defaultInst,
		Agents:          0,
		RestartRequired: len(restartReasons) > 0,
		RestartReasons:  restartReasons,
	}, nil
}

func (c *ConfigAPI) currentConfig() (config.FileConfig, string, []string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	fc, path, err := config.LoadFileConfig()
	if err != nil {
		return config.FileConfig{}, "", nil, err
	}
	restartReasons := c.restartReasonsFor(*fc)
	return *fc, path, restartReasons, nil
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

func (c *ConfigAPI) tokenConfigured(cfg config.FileConfig) bool {
	if c != nil && c.runtime != nil && strings.TrimSpace(c.runtime.Token) != "" {
		return true
	}
	return strings.TrimSpace(cfg.Server.Token) != ""
}

func redactToken(cfg config.FileConfig) config.FileConfig {
	cfg.Server.Token = ""
	return cfg
}

func (c *ConfigAPI) restartReasonsFor(next config.FileConfig) []string {
	reasons := make([]string, 0, 5)

	if c.boot.Server.Port != next.Server.Port || c.boot.Server.Bind != next.Server.Bind {
		reasons = append(reasons, "Server address")
	}
	if c.boot.Profiles.BaseDir != next.Profiles.BaseDir {
		reasons = append(reasons, "Profiles directory")
	}
	if c.boot.MultiInstance.Strategy != next.MultiInstance.Strategy {
		reasons = append(reasons, "Routing strategy")
	}
	if c.boot.InstanceDefaults.StealthLevel != next.InstanceDefaults.StealthLevel {
		reasons = append(reasons, "Stealth level")
	}
	if !sameIntPtr(c.boot.MultiInstance.Restart.MaxRestarts, next.MultiInstance.Restart.MaxRestarts) ||
		!sameIntPtr(c.boot.MultiInstance.Restart.InitBackoffSec, next.MultiInstance.Restart.InitBackoffSec) ||
		!sameIntPtr(c.boot.MultiInstance.Restart.MaxBackoffSec, next.MultiInstance.Restart.MaxBackoffSec) ||
		!sameIntPtr(c.boot.MultiInstance.Restart.StableAfterSec, next.MultiInstance.Restart.StableAfterSec) {
		reasons = append(reasons, "Restart policy")
	}

	return reasons
}

func sameIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
