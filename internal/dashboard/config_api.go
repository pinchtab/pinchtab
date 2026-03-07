package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/web"
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
	RestartRequired bool              `json:"restartRequired"`
	RestartReasons  []string          `json:"restartReasons,omitempty"`
}

type healthEnvelope struct {
	Status          string   `json:"status"`
	Mode            string   `json:"mode"`
	Version         string   `json:"version"`
	Uptime          int64    `json:"uptime"`
	Profiles        int      `json:"profiles"`
	Instances       int      `json:"instances"`
	Agents          int      `json:"agents"`
	RestartRequired bool     `json:"restartRequired"`
	RestartReasons  []string `json:"restartReasons,omitempty"`
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
	if runtime != nil {
		boot = config.FileConfigFromRuntime(runtime)
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
		web.Error(w, 500, err)
		return
	}
	web.JSON(w, 200, info)
}

func (c *ConfigAPI) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, path, restartReasons, err := c.currentConfig()
	if err != nil {
		web.Error(w, 500, err)
		return
	}
	web.JSON(w, 200, configEnvelope{
		Config:          cfg,
		ConfigPath:      path,
		RestartRequired: len(restartReasons) > 0,
		RestartReasons:  restartReasons,
	})
}

func (c *ConfigAPI) HandlePutConfig(w http.ResponseWriter, r *http.Request) {
	var incoming config.FileConfig
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		web.ErrorCode(w, 400, "bad_config_json", "invalid config payload", false, nil)
		return
	}

	normalized, err := normalizeFileConfig(&incoming)
	if err != nil {
		web.Error(w, 400, err)
		return
	}

	if errs := config.ValidateFileConfig(&normalized); len(errs) > 0 {
		messages := make([]string, 0, len(errs))
		for _, validationErr := range errs {
			messages = append(messages, validationErr.Error())
		}
		web.ErrorCode(w, 400, "invalid_config", "config validation failed", false, map[string]any{
			"errors": messages,
		})
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	_, path, err := config.LoadFileConfig()
	if err != nil {
		web.Error(w, 500, err)
		return
	}
	if err := config.SaveFileConfig(&normalized, path); err != nil {
		web.Error(w, 500, err)
		return
	}

	config.ApplyFileConfigToRuntime(c.runtime, &normalized)
	if c.applier != nil {
		c.applier.ApplyRuntimeConfig(c.runtime)
	}

	restartReasons := c.restartReasonsFor(normalized)
	web.JSON(w, 200, configEnvelope{
		Config:          normalized,
		ConfigPath:      path,
		RestartRequired: len(restartReasons) > 0,
		RestartReasons:  restartReasons,
	})
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
	if c.instances != nil {
		instanceCount = len(c.instances.List())
	}
	return healthEnvelope{
		Status:          "ok",
		Mode:            "dashboard",
		Version:         c.version,
		Uptime:          int64(time.Since(c.startedAt).Milliseconds()),
		Profiles:        profileCount,
		Instances:       instanceCount,
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
	normalized, err := normalizeFileConfig(fc)
	if err != nil {
		return config.FileConfig{}, "", nil, err
	}
	restartReasons := c.restartReasonsFor(normalized)
	return normalized, path, restartReasons, nil
}

func (c *ConfigAPI) restartReasonsFor(next config.FileConfig) []string {
	reasons := make([]string, 0, 4)

	if c.boot.Server.Port != next.Server.Port || c.boot.Server.Bind != next.Server.Bind {
		reasons = append(reasons, "Server address")
	}
	if c.boot.Profiles.BaseDir != next.Profiles.BaseDir {
		reasons = append(reasons, "Profiles directory")
	}
	if c.boot.MultiInstance.Strategy != next.MultiInstance.Strategy {
		reasons = append(reasons, "Routing strategy")
	}

	return reasons
}

func normalizeFileConfig(fc *config.FileConfig) (config.FileConfig, error) {
	base := config.DefaultFileConfig()
	if fc == nil {
		return base, nil
	}

	data, err := json.Marshal(fc)
	if err != nil {
		return config.FileConfig{}, err
	}
	patch := strings.TrimSpace(string(data))
	if patch == "" || patch == "null" || patch == "{}" {
		return base, nil
	}

	if err := config.PatchConfigJSON(&base, patch); err != nil {
		return config.FileConfig{}, err
	}
	return base, nil
}
