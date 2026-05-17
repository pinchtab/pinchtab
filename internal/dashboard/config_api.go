package dashboard

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/cli/report"
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

type healthSecurityInfo struct {
	Level                     string   `json:"level"`
	Bind                      string   `json:"bind"`
	AllowedDomains            []string `json:"allowedDomains"`
	IDPIEnabled               bool     `json:"idpiEnabled"`
	EnabledSensitiveEndpoints []string `json:"enabledSensitiveEndpoints"`
	GuardsDown                bool     `json:"guardsDown"`
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
	Security        *healthSecurityInfo `json:"security,omitempty"`
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

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.ErrorCode(w, 400, "bad_config_json", "invalid config payload", false, nil)
		return
	}

	var tokenProbe struct {
		Server struct {
			Token *string `json:"token"`
		} `json:"server"`
	}
	if err := json.Unmarshal(body, &tokenProbe); err != nil {
		httpx.ErrorCode(w, 400, "bad_config_json", "invalid config payload", false, nil)
		return
	}
	if tokenProbe.Server.Token != nil && strings.TrimSpace(*tokenProbe.Server.Token) != "" {
		httpx.ErrorCode(w, 400, "token_write_only", "manage the API token outside the dashboard", false, nil)
		return
	}

	normalized := *current
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&normalized); err != nil {
		httpx.ErrorCode(w, 400, "bad_config_json", "invalid config payload", false, nil)
		return
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
		return
	}
	changes := sensitiveConfigChanges(current, &normalized)
	if changes.requiresElevation && !c.hasConfigWriteElevation(r) {
		authn.AuditWarn(r, "config.update_elevation_required", "changes", changes.names)
		httpx.ErrorCode(w, http.StatusForbidden, "session_elevation_required", "re-enter the API token before changing proxy or security settings", false, nil)
		return
	}
	if err := config.SaveFileConfig(&normalized, path); err != nil {
		httpx.Error(w, 500, err)
		return
	}

	config.ApplyFileConfigToRuntime(c.runtime, &normalized)
	if c.sessions != nil {
		c.sessions.UpdateConfig(BrowserSessionConfig(c.runtime))
	}
	if c.applier != nil {
		c.applier.ApplyRuntimeConfig(c.runtime)
	}

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

type sensitiveConfigChangeSet struct {
	requiresElevation bool
	proxyChanged      bool
	names             []string
	proxyScopes       []string
	proxyAudit        []proxyAuditChange
}

type proxyAuditChange struct {
	Scope  string `json:"scope"`
	Server string `json:"server"`
}

func sensitiveConfigChanges(current, next *config.FileConfig) sensitiveConfigChangeSet {
	var out sensitiveConfigChangeSet
	if current == nil || next == nil {
		return out
	}
	if !reflect.DeepEqual(current.Security, next.Security) {
		out.requiresElevation = true
		out.names = append(out.names, "security")
	}
	if !reflect.DeepEqual(current.Browser.Proxy, next.Browser.Proxy) {
		out.requiresElevation = true
		out.proxyChanged = true
		out.names = append(out.names, "browser.proxy")
		out.proxyScopes = append(out.proxyScopes, "browser.proxy")
		out.proxyAudit = append(out.proxyAudit, proxyAuditChange{
			Scope:  "browser.proxy",
			Server: next.Browser.Proxy.Redacted().Server,
		})
	}
	for _, name := range changedTargetProxyNames(current.Browser.Targets, next.Browser.Targets) {
		out.requiresElevation = true
		out.proxyChanged = true
		field := "browser.targets." + name + ".proxy"
		out.names = append(out.names, field)
		out.proxyScopes = append(out.proxyScopes, field)
		out.proxyAudit = append(out.proxyAudit, proxyAuditChange{
			Scope:  field,
			Server: next.Browser.Targets[name].Proxy.Redacted().Server,
		})
	}
	return out
}

func changedTargetProxyNames(current, next config.BrowserTargetsConfig) []string {
	seen := make(map[string]struct{}, len(current)+len(next))
	names := make([]string, 0, len(current)+len(next))
	for name := range current {
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	for name := range next {
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	var changed []string
	for _, name := range names {
		if !reflect.DeepEqual(current[name].Proxy, next[name].Proxy) {
			changed = append(changed, name)
		}
	}
	return changed
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

func (c *ConfigAPI) healthInfo(includeSecurity bool) (healthEnvelope, error) {
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
	agentCount := 0
	if c.agents != nil {
		agentCount = c.agents.AgentCount()
	}
	out := healthEnvelope{
		Status:          "ok",
		Mode:            "dashboard",
		Version:         c.version,
		Uptime:          int64(time.Since(c.startedAt).Milliseconds()),
		AuthRequired:    strings.TrimSpace(c.runtime.Token) != "",
		Profiles:        profileCount,
		Instances:       instanceCount,
		DefaultInstance: defaultInst,
		Agents:          agentCount,
		RestartRequired: len(restartReasons) > 0,
		RestartReasons:  restartReasons,
	}
	if includeSecurity {
		security := runtimeSecurityInfo(c.runtime)
		out.Security = &security
	}
	return out, nil
}

func healthSecurityVisibleTo(r *http.Request) bool {
	switch authn.CredentialsFromRequest(r).Method {
	case authn.MethodHeader, authn.MethodCookie:
		return true
	default:
		return false
	}
}

func runtimeSecurityInfo(cfg *config.RuntimeConfig) healthSecurityInfo {
	if cfg == nil {
		return healthSecurityInfo{Level: "UNKNOWN"}
	}
	posture := report.AssessSecurityPosture(cfg)
	enabled := append([]string(nil), cfg.EnabledSensitiveEndpoints()...)
	domains := append([]string(nil), cfg.AllowedDomains...)
	return healthSecurityInfo{
		Level:                     posture.Level,
		Bind:                      cfg.Bind,
		AllowedDomains:            domains,
		IDPIEnabled:               cfg.IDPI.Enabled,
		EnabledSensitiveEndpoints: enabled,
		GuardsDown:                isGuardsDownPosture(cfg),
	}
}

// isGuardsDownPosture reports whether the runtime config matches the
// guards-down preset signature (all sensitive endpoints + attach + IDPI off).
func isGuardsDownPosture(cfg *config.RuntimeConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.AllowEvaluate &&
		cfg.AllowMacro &&
		cfg.AllowScreencast &&
		cfg.AllowDownload &&
		cfg.AllowCookies &&
		cfg.AllowUpload &&
		cfg.AllowNetworkIntercept &&
		cfg.AttachEnabled &&
		!cfg.IDPI.Enabled
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
	cfg.Security.StateEncryptionKey = nil
	cfg.AutoSolver.External.CapsolverKey = ""
	cfg.AutoSolver.External.TwoCaptchaKey = ""
	cfg.AutoSolver.Credentials = config.AutoSolverCredentialsConf{}
	cfg.Browser.Proxy = cfg.Browser.Proxy.Redacted()
	if len(cfg.Browser.Targets) > 0 {
		// Copy before mutating: maps are reference types.
		copied := make(config.BrowserTargetsConfig, len(cfg.Browser.Targets))
		for name, t := range cfg.Browser.Targets {
			t.Proxy = t.Proxy.Redacted()
			copied[name] = t
		}
		cfg.Browser.Targets = copied
	}
	return cfg
}

func preserveWriteOnlyConfigFields(dst, src *config.FileConfig) {
	if dst == nil || src == nil {
		return
	}
	dst.Server.Token = src.Server.Token
	dst.Security.StateEncryptionKey = src.Security.StateEncryptionKey
	dst.AutoSolver.External.CapsolverKey = src.AutoSolver.External.CapsolverKey
	dst.AutoSolver.External.TwoCaptchaKey = src.AutoSolver.External.TwoCaptchaKey
	// Credentials are write-only: when the dashboard PUTs config without a
	// credential field (because GET redacted them), keep the value already
	// on disk. Per-field so a deliberate set-to-blank still wins.
	preserveCredString(&dst.AutoSolver.Credentials.Login.User, src.AutoSolver.Credentials.Login.User)
	preserveCredString(&dst.AutoSolver.Credentials.Login.Password, src.AutoSolver.Credentials.Login.Password)
	preserveCredString(&dst.AutoSolver.Credentials.Signup.Name, src.AutoSolver.Credentials.Signup.Name)
	preserveCredString(&dst.AutoSolver.Credentials.Signup.Email, src.AutoSolver.Credentials.Signup.Email)
	preserveCredString(&dst.AutoSolver.Credentials.Signup.Password, src.AutoSolver.Credentials.Signup.Password)
	preserveCredString(&dst.AutoSolver.Credentials.Form.Field1, src.AutoSolver.Credentials.Form.Field1)
	preserveCredString(&dst.AutoSolver.Credentials.Form.Field2, src.AutoSolver.Credentials.Form.Field2)
	preserveCredString(&dst.AutoSolver.Credentials.Form.Email, src.AutoSolver.Credentials.Form.Email)

	// Restore the on-disk proxy password when the dashboard echoes back the redaction mask.
	preserveProxyPassword(&dst.Browser.Proxy, src.Browser.Proxy)
	for name, t := range dst.Browser.Targets {
		if srcT, ok := src.Browser.Targets[name]; ok {
			preserveProxyPassword(&t.Proxy, srcT.Proxy)
			dst.Browser.Targets[name] = t
		}
	}
}

// preserveProxyPassword keeps the on-disk password when the inbound PUT is blank or the "***" mask.
func preserveProxyPassword(dst *config.BrowserProxyConfig, src config.BrowserProxyConfig) {
	if dst == nil {
		return
	}
	if dst.Password == "" || dst.Password == "***" {
		dst.Password = src.Password
	}
}

// preserveCredString keeps the existing src value when dst is empty (i.e. the
// PUT didn't include this field because GET redacted it). A deliberate
// blank from PUT is indistinguishable from "not provided" in JSON without
// pointer types — given these are credentials, the safer default is to
// preserve. Callers can clear by writing the JSON file directly.
func preserveCredString(dst *string, src string) {
	if dst == nil {
		return
	}
	if *dst == "" {
		*dst = src
	}
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
