package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RuntimeConfig holds all runtime settings used throughout the application.
// This is the single source of truth for configuration at runtime.
type RuntimeConfig struct {
	// Server settings
	Bind              string
	Port              string
	InstancePortStart int // Starting port for instances (default 9868)
	InstancePortEnd   int // Ending port for instances (default 9968)
	Token             string
	StateDir          string

	// Security settings
	AllowEvaluate   bool
	AllowMacro      bool
	AllowScreencast bool
	AllowDownload   bool
	AllowUpload     bool

	// Browser/instance settings
	Headless          bool
	NoRestore         bool
	ProfileDir        string
	ProfilesBaseDir   string
	DefaultProfile    string
	ChromeVersion     string
	Timezone          string
	BlockImages       bool
	BlockMedia        bool
	BlockAds          bool
	MaxTabs           int
	MaxParallelTabs   int // 0 = auto-detect from runtime.NumCPU
	ChromeBinary      string
	ChromeExtraFlags  string
	ExtensionPaths    []string
	UserAgent         string
	NoAnimations      bool
	StealthLevel      string
	TabEvictionPolicy string // "reject" (default), "close_oldest", "close_lru"

	// Timeout settings
	ActionTimeout   time.Duration
	NavigateTimeout time.Duration
	ShutdownTimeout time.Duration
	WaitNavDelay    time.Duration

	// Orchestrator settings (dashboard mode only)
	Strategy         string // "simple" (default), "explicit", or "simple-autorestart"
	AllocationPolicy string // "fcfs" (default), "round_robin", "random"

	// Attach settings
	AttachEnabled      bool
	AttachAllowHosts   []string
	AttachAllowSchemes []string

	// IDPI (Indirect Prompt Injection defense) settings
	IDPI IDPIConfig

	// Scheduler settings (dashboard mode only)
	Scheduler SchedulerConfig
}

// SchedulerConfig holds task scheduler settings.
type SchedulerConfig struct {
	Enabled           bool   `json:"enabled,omitempty"`
	Strategy          string `json:"strategy,omitempty"`
	MaxQueueSize      int    `json:"maxQueueSize,omitempty"`
	MaxPerAgent       int    `json:"maxPerAgent,omitempty"`
	MaxInflight       int    `json:"maxInflight,omitempty"`
	MaxPerAgentFlight int    `json:"maxPerAgentInflight,omitempty"`
	ResultTTLSec      int    `json:"resultTTLSec,omitempty"`
	WorkerCount       int    `json:"workerCount,omitempty"`
}

// --- Nested FileConfig structure (PR #91 model) ---

// FileConfig is the persistent configuration written to disk.
// Uses nested sections that match actual runtime ownership.
type FileConfig struct {
	Server           ServerConfig           `json:"server,omitempty"`
	Browser          BrowserConfig          `json:"browser,omitempty"`
	InstanceDefaults InstanceDefaultsConfig `json:"instanceDefaults,omitempty"`
	Security         SecurityConfig         `json:"security,omitempty"`
	Profiles         ProfilesConfig         `json:"profiles,omitempty"`
	MultiInstance    MultiInstanceConfig    `json:"multiInstance,omitempty"`
	Attach           AttachConfig           `json:"attach,omitempty"`
	Timeouts         TimeoutsConfig         `json:"timeouts,omitempty"`
	Scheduler        SchedulerFileConfig    `json:"scheduler,omitempty"`
}

// ServerConfig holds server/network settings.
type ServerConfig struct {
	Port     string `json:"port,omitempty"`
	Bind     string `json:"bind,omitempty"`
	Token    string `json:"token,omitempty"`
	StateDir string `json:"stateDir,omitempty"`
}

// BrowserConfig holds Chrome executable/runtime wiring.
type BrowserConfig struct {
	ChromeVersion    string   `json:"version,omitempty"`
	ChromeBinary     string   `json:"binary,omitempty"`
	ChromeExtraFlags string   `json:"extraFlags,omitempty"`
	ExtensionPaths   []string `json:"extensionPaths,omitempty"`
}

// InstanceDefaultsConfig holds the default behavior for a launched browser instance.
type InstanceDefaultsConfig struct {
	Mode              string `json:"mode,omitempty"`
	NoRestore         *bool  `json:"noRestore,omitempty"`
	Timezone          string `json:"timezone,omitempty"`
	BlockImages       *bool  `json:"blockImages,omitempty"`
	BlockMedia        *bool  `json:"blockMedia,omitempty"`
	BlockAds          *bool  `json:"blockAds,omitempty"`
	MaxTabs           *int   `json:"maxTabs,omitempty"`
	MaxParallelTabs   *int   `json:"maxParallelTabs,omitempty"`
	UserAgent         string `json:"userAgent,omitempty"`
	NoAnimations      *bool  `json:"noAnimations,omitempty"`
	StealthLevel      string `json:"stealthLevel,omitempty"`
	TabEvictionPolicy string `json:"tabEvictionPolicy,omitempty"`
}

// ProfilesConfig holds profile storage defaults.
type ProfilesConfig struct {
	BaseDir        string `json:"baseDir,omitempty"`
	DefaultProfile string `json:"defaultProfile,omitempty"`
}

// SecurityConfig holds security/permission settings.
type SecurityConfig struct {
	AllowEvaluate   *bool      `json:"allowEvaluate,omitempty"`
	AllowMacro      *bool      `json:"allowMacro,omitempty"`
	AllowScreencast *bool      `json:"allowScreencast,omitempty"`
	AllowDownload   *bool      `json:"allowDownload,omitempty"`
	AllowUpload     *bool      `json:"allowUpload,omitempty"`
	IDPI            IDPIConfig `json:"idpi,omitempty"`
}

// IDPIConfig holds the configuration for the Indirect Prompt Injection (IDPI)
// defense layer. All fields default to zero/false, making the feature fully
// opt-in with no change to existing behaviour when Enabled is false.
type IDPIConfig struct {
	// Enabled is the master switch. No IDPI checks are performed when false.
	Enabled bool `json:"enabled,omitempty"`

	// AllowedDomains is a whitelist of permitted navigation targets.
	// An empty list means all domains are allowed (no restriction).
	// Patterns support exact matches ("example.com") and single-level wildcard
	// prefixes ("*.example.com"). The special value "*" allows everything.
	AllowedDomains []string `json:"allowedDomains,omitempty"`

	// StrictMode controls the response when a threat is detected.
	// true  → block the request (HTTP 403 for navigation; scan refuses response).
	// false → allow the request but emit an X-IDPI-Warning response header.
	StrictMode bool `json:"strictMode,omitempty"`

	// ScanContent enables keyword-pattern scanning on page content returned by
	// /snapshot and /text. When a known injection phrase is detected the
	// response is annotated (warn mode) or refused (strict mode).
	ScanContent bool `json:"scanContent,omitempty"`

	// WrapContent wraps plain-text output from /text in
	// <untrusted_web_content> XML delimiters and prepends a safety advisory
	// so downstream LLMs treat the content as data, not as instructions.
	WrapContent bool `json:"wrapContent,omitempty"`

	// CustomPatterns is a user-extensible list of additional injection-detection
	// phrases. Matched case-insensitively, same as the built-in patterns.
	CustomPatterns []string `json:"customPatterns,omitempty"`
}

// MultiInstanceConfig holds multi-instance orchestration settings.
type MultiInstanceConfig struct {
	Strategy          string `json:"strategy,omitempty"`
	AllocationPolicy  string `json:"allocationPolicy,omitempty"`
	InstancePortStart *int   `json:"instancePortStart,omitempty"`
	InstancePortEnd   *int   `json:"instancePortEnd,omitempty"`
}

// AttachConfig holds policy for attaching to externally managed Chrome instances.
type AttachConfig struct {
	Enabled      *bool    `json:"enabled,omitempty"`
	AllowHosts   []string `json:"allowHosts,omitempty"`
	AllowSchemes []string `json:"allowSchemes,omitempty"`
}

// TimeoutsConfig holds timeout settings.
type TimeoutsConfig struct {
	ActionSec   int `json:"actionSec,omitempty"`
	NavigateSec int `json:"navigateSec,omitempty"`
	ShutdownSec int `json:"shutdownSec,omitempty"`
	WaitNavMs   int `json:"waitNavMs,omitempty"`
}

// SchedulerFileConfig holds scheduler settings in the config file.
type SchedulerFileConfig struct {
	Enabled           *bool  `json:"enabled,omitempty"`
	Strategy          string `json:"strategy,omitempty"`
	MaxQueueSize      *int   `json:"maxQueueSize,omitempty"`
	MaxPerAgent       *int   `json:"maxPerAgent,omitempty"`
	MaxInflight       *int   `json:"maxInflight,omitempty"`
	MaxPerAgentFlight *int   `json:"maxPerAgentInflight,omitempty"`
	ResultTTLSec      *int   `json:"resultTTLSec,omitempty"`
	WorkerCount       *int   `json:"workerCount,omitempty"`
}

// --- Legacy flat FileConfig for backward compatibility ---

// legacyFileConfig is the old flat structure for backward compatibility.
type legacyFileConfig struct {
	Port              string `json:"port"`
	InstancePortStart *int   `json:"instancePortStart,omitempty"`
	InstancePortEnd   *int   `json:"instancePortEnd,omitempty"`
	Token             string `json:"token,omitempty"`
	AllowEvaluate     *bool  `json:"allowEvaluate,omitempty"`
	AllowMacro        *bool  `json:"allowMacro,omitempty"`
	AllowScreencast   *bool  `json:"allowScreencast,omitempty"`
	AllowDownload     *bool  `json:"allowDownload,omitempty"`
	AllowUpload       *bool  `json:"allowUpload,omitempty"`
	StateDir          string `json:"stateDir"`
	ProfileDir        string `json:"profileDir"`
	Headless          *bool  `json:"headless,omitempty"`
	NoRestore         bool   `json:"noRestore"`
	MaxTabs           *int   `json:"maxTabs,omitempty"`
	TimeoutSec        int    `json:"timeoutSec,omitempty"`
	NavigateSec       int    `json:"navigateSec,omitempty"`
}

// convertLegacyConfig converts flat config to nested structure.
func convertLegacyConfig(lc *legacyFileConfig) *FileConfig {
	fc := &FileConfig{}

	// Server
	fc.Server.Port = lc.Port
	fc.Server.Token = lc.Token
	fc.Server.StateDir = lc.StateDir

	// Browser / instance defaults
	if lc.Headless != nil {
		if *lc.Headless {
			fc.InstanceDefaults.Mode = "headless"
		} else {
			fc.InstanceDefaults.Mode = "headed"
		}
	}
	fc.InstanceDefaults.MaxTabs = lc.MaxTabs
	if lc.NoRestore {
		b := true
		fc.InstanceDefaults.NoRestore = &b
	}

	// Profiles
	if lc.ProfileDir != "" {
		fc.Profiles.BaseDir = filepath.Dir(lc.ProfileDir)
		fc.Profiles.DefaultProfile = filepath.Base(lc.ProfileDir)
	}

	// Security
	fc.Security.AllowEvaluate = lc.AllowEvaluate
	fc.Security.AllowMacro = lc.AllowMacro
	fc.Security.AllowScreencast = lc.AllowScreencast
	fc.Security.AllowDownload = lc.AllowDownload
	fc.Security.AllowUpload = lc.AllowUpload

	// Timeouts
	fc.Timeouts.ActionSec = lc.TimeoutSec
	fc.Timeouts.NavigateSec = lc.NavigateSec

	// Multi-instance
	fc.MultiInstance.InstancePortStart = lc.InstancePortStart
	fc.MultiInstance.InstancePortEnd = lc.InstancePortEnd

	return fc
}

// isLegacyConfig detects if JSON is flat (legacy) or nested (new).
// Returns true if it looks like legacy format.
func isLegacyConfig(data []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}

	// If any new nested keys exist, it's new format
	if _, hasServer := probe["server"]; hasServer {
		return false
	}
	if _, hasBrowser := probe["browser"]; hasBrowser {
		return false
	}
	if _, hasInstanceDefaults := probe["instanceDefaults"]; hasInstanceDefaults {
		return false
	}
	if _, hasProfiles := probe["profiles"]; hasProfiles {
		return false
	}
	if _, hasMultiInstance := probe["multiInstance"]; hasMultiInstance {
		return false
	}
	if _, hasSecurity := probe["security"]; hasSecurity {
		return false
	}
	if _, hasAttach := probe["attach"]; hasAttach {
		return false
	}
	if _, hasTimeouts := probe["timeouts"]; hasTimeouts {
		return false
	}

	// If "port" or "headless" exist at top level, it's legacy
	if _, hasPort := probe["port"]; hasPort {
		return true
	}
	if _, hasHeadless := probe["headless"]; hasHeadless {
		return true
	}

	// Default to new format for empty/unknown
	return false
}

// --- Environment variable helpers ---

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

// homeDir returns the user's home directory, checking $HOME first for container compatibility
func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	h, _ := os.UserHomeDir()
	return h
}

// userConfigDir returns the OS-appropriate app config directory:
// - macOS: ~/Library/Application Support/pinchtab
// - Linux: ~/.config/pinchtab (or $XDG_CONFIG_HOME/pinchtab)
// - Windows: %APPDATA%\pinchtab
//
// For backwards compatibility, if ~/.pinchtab exists and the new location
// doesn't, it returns ~/.pinchtab (allowing seamless migration).
func userConfigDir() string {
	home := homeDir()
	legacyPath := filepath.Join(home, ".pinchtab")

	// Try to get OS-appropriate config directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		// Fallback to legacy location if UserConfigDir fails
		return legacyPath
	}

	newPath := filepath.Join(configDir, "pinchtab")

	// Backwards compatibility: if legacy location exists and new doesn't, use legacy
	legacyExists := dirExists(legacyPath)
	newExists := dirExists(newPath)

	if legacyExists && !newExists {
		return legacyPath
	}

	return newPath
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func (c *RuntimeConfig) ListenAddr() string {
	return c.Bind + ":" + c.Port
}

func modeToHeadless(mode string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return fallback
	case "headless":
		return true
	case "headed":
		return false
	default:
		return fallback
	}
}

func finalizeProfileConfig(cfg *RuntimeConfig) {
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = "default"
	}
	if cfg.ProfilesBaseDir == "" {
		cfg.ProfilesBaseDir = filepath.Join(cfg.StateDir, "profiles")
	}
	if cfg.ProfileDir == "" {
		cfg.ProfileDir = filepath.Join(cfg.ProfilesBaseDir, cfg.DefaultProfile)
	}
}

// Load returns the RuntimeConfig with precedence: env vars > config file > defaults.
func Load() *RuntimeConfig {
	cfg := &RuntimeConfig{
		// Server defaults + selected env vars
		Bind:              envOr("PINCHTAB_BIND", "127.0.0.1"),
		Port:              envOr("PINCHTAB_PORT", "9867"),
		InstancePortStart: 9868,
		InstancePortEnd:   9968,
		Token:             os.Getenv("PINCHTAB_TOKEN"),
		StateDir:          userConfigDir(),

		// Security defaults
		AllowEvaluate:   false,
		AllowMacro:      false,
		AllowScreencast: false,
		AllowDownload:   false,
		AllowUpload:     false,

		// Browser / instance defaults
		Headless:          true,
		NoRestore:         false,
		ProfileDir:        "",
		ProfilesBaseDir:   "",
		DefaultProfile:    "default",
		ChromeVersion:     "144.0.7559.133",
		Timezone:          "",
		BlockImages:       false,
		BlockMedia:        false,
		BlockAds:          false,
		MaxTabs:           20,
		MaxParallelTabs:   0,
		ChromeBinary:      os.Getenv("CHROME_BIN"),
		ChromeExtraFlags:  "",
		ExtensionPaths:    nil,
		UserAgent:         "",
		NoAnimations:      false,
		StealthLevel:      "light",
		TabEvictionPolicy: "reject",

		// Timeout defaults
		ActionTimeout:   30 * time.Second,
		NavigateTimeout: 60 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		WaitNavDelay:    1 * time.Second,

		// Orchestrator defaults
		Strategy:         "simple",
		AllocationPolicy: "fcfs",

		// Attach defaults
		AttachEnabled:      false,
		AttachAllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
		AttachAllowSchemes: []string{"ws", "wss"},
	}
	finalizeProfileConfig(cfg)

	// Load config file (supports both legacy flat and new nested format)
	configPath := envOr("PINCHTAB_CONFIG", filepath.Join(userConfigDir(), "config.json"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read config file", "path", configPath, "error", err)
		}
		return cfg
	}

	slog.Debug("loading config file", "path", configPath)

	var fc *FileConfig

	if isLegacyConfig(data) {
		var lc legacyFileConfig
		if err := json.Unmarshal(data, &lc); err != nil {
			slog.Warn("failed to parse legacy config", "path", configPath, "error", err)
			return cfg
		}
		fc = convertLegacyConfig(&lc)
		slog.Info("loaded legacy flat config, consider migrating to nested format", "path", configPath)
	} else {
		fc = &FileConfig{}
		if err := json.Unmarshal(data, fc); err != nil {
			slog.Warn("failed to parse config", "path", configPath, "error", err)
			return cfg
		}
	}

	// Validate file config and log warnings
	if errs := ValidateFileConfig(fc); len(errs) > 0 {
		for _, e := range errs {
			slog.Warn("config validation error", "path", configPath, "error", e)
		}
	}

	// Apply file config (only if env var NOT set)
	applyFileConfig(cfg, fc)
	finalizeProfileConfig(cfg)

	return cfg
}

// applyFileConfig merges FileConfig values into RuntimeConfig.
// Only applies values where the corresponding env var is NOT set.
func applyFileConfig(cfg *RuntimeConfig, fc *FileConfig) {
	// Server
	if fc.Server.Port != "" && os.Getenv("PINCHTAB_PORT") == "" {
		cfg.Port = fc.Server.Port
	}
	if fc.Server.Bind != "" && os.Getenv("PINCHTAB_BIND") == "" {
		cfg.Bind = fc.Server.Bind
	}
	if fc.Server.Token != "" && os.Getenv("PINCHTAB_TOKEN") == "" {
		cfg.Token = fc.Server.Token
	}
	if fc.Server.StateDir != "" {
		cfg.StateDir = fc.Server.StateDir
	}
	// Security
	if fc.Security.AllowEvaluate != nil {
		cfg.AllowEvaluate = *fc.Security.AllowEvaluate
	}
	if fc.Security.AllowMacro != nil {
		cfg.AllowMacro = *fc.Security.AllowMacro
	}
	if fc.Security.AllowScreencast != nil {
		cfg.AllowScreencast = *fc.Security.AllowScreencast
	}
	if fc.Security.AllowDownload != nil {
		cfg.AllowDownload = *fc.Security.AllowDownload
	}
	if fc.Security.AllowUpload != nil {
		cfg.AllowUpload = *fc.Security.AllowUpload
	}
	// IDPI – copy the whole struct; individual fields have safe zero-value defaults.
	cfg.IDPI = fc.Security.IDPI

	// Browser
	if fc.Browser.ChromeVersion != "" {
		cfg.ChromeVersion = fc.Browser.ChromeVersion
	}
	if fc.Browser.ChromeBinary != "" && os.Getenv("CHROME_BIN") == "" {
		cfg.ChromeBinary = fc.Browser.ChromeBinary
	}
	if fc.Browser.ChromeExtraFlags != "" {
		cfg.ChromeExtraFlags = fc.Browser.ChromeExtraFlags
	}
	if len(fc.Browser.ExtensionPaths) > 0 {
		cfg.ExtensionPaths = fc.Browser.ExtensionPaths
	}

	// Instance defaults
	if fc.InstanceDefaults.Mode != "" {
		cfg.Headless = modeToHeadless(fc.InstanceDefaults.Mode, cfg.Headless)
	}
	if fc.InstanceDefaults.NoRestore != nil {
		cfg.NoRestore = *fc.InstanceDefaults.NoRestore
	}
	if fc.InstanceDefaults.Timezone != "" {
		cfg.Timezone = fc.InstanceDefaults.Timezone
	}
	if fc.InstanceDefaults.BlockImages != nil {
		cfg.BlockImages = *fc.InstanceDefaults.BlockImages
	}
	if fc.InstanceDefaults.BlockMedia != nil {
		cfg.BlockMedia = *fc.InstanceDefaults.BlockMedia
	}
	if fc.InstanceDefaults.BlockAds != nil {
		cfg.BlockAds = *fc.InstanceDefaults.BlockAds
	}
	if fc.InstanceDefaults.MaxTabs != nil {
		cfg.MaxTabs = *fc.InstanceDefaults.MaxTabs
	}
	if fc.InstanceDefaults.MaxParallelTabs != nil {
		cfg.MaxParallelTabs = *fc.InstanceDefaults.MaxParallelTabs
	}
	if fc.InstanceDefaults.UserAgent != "" {
		cfg.UserAgent = fc.InstanceDefaults.UserAgent
	}
	if fc.InstanceDefaults.NoAnimations != nil {
		cfg.NoAnimations = *fc.InstanceDefaults.NoAnimations
	}
	if fc.InstanceDefaults.StealthLevel != "" {
		cfg.StealthLevel = fc.InstanceDefaults.StealthLevel
	}
	if fc.InstanceDefaults.TabEvictionPolicy != "" {
		cfg.TabEvictionPolicy = fc.InstanceDefaults.TabEvictionPolicy
	}

	// Profiles
	if fc.Profiles.BaseDir != "" {
		cfg.ProfilesBaseDir = fc.Profiles.BaseDir
	}
	if fc.Profiles.DefaultProfile != "" {
		cfg.DefaultProfile = fc.Profiles.DefaultProfile
	}
	cfg.ProfileDir = ""

	// Multi-instance
	if fc.MultiInstance.Strategy != "" {
		cfg.Strategy = fc.MultiInstance.Strategy
	}
	if fc.MultiInstance.AllocationPolicy != "" {
		cfg.AllocationPolicy = fc.MultiInstance.AllocationPolicy
	}
	if fc.MultiInstance.InstancePortStart != nil {
		cfg.InstancePortStart = *fc.MultiInstance.InstancePortStart
	}
	if fc.MultiInstance.InstancePortEnd != nil {
		cfg.InstancePortEnd = *fc.MultiInstance.InstancePortEnd
	}

	// Attach
	if fc.Attach.Enabled != nil {
		cfg.AttachEnabled = *fc.Attach.Enabled
	}
	if len(fc.Attach.AllowHosts) > 0 {
		cfg.AttachAllowHosts = append([]string(nil), fc.Attach.AllowHosts...)
	}
	if len(fc.Attach.AllowSchemes) > 0 {
		cfg.AttachAllowSchemes = append([]string(nil), fc.Attach.AllowSchemes...)
	}

	// Timeouts
	if fc.Timeouts.ActionSec > 0 {
		cfg.ActionTimeout = time.Duration(fc.Timeouts.ActionSec) * time.Second
	}
	if fc.Timeouts.NavigateSec > 0 {
		cfg.NavigateTimeout = time.Duration(fc.Timeouts.NavigateSec) * time.Second
	}
	if fc.Timeouts.ShutdownSec > 0 {
		cfg.ShutdownTimeout = time.Duration(fc.Timeouts.ShutdownSec) * time.Second
	}
	if fc.Timeouts.WaitNavMs > 0 {
		cfg.WaitNavDelay = time.Duration(fc.Timeouts.WaitNavMs) * time.Millisecond
	}

	// Scheduler
	if fc.Scheduler.Enabled != nil {
		cfg.Scheduler.Enabled = *fc.Scheduler.Enabled
	}
	if fc.Scheduler.Strategy != "" {
		cfg.Scheduler.Strategy = fc.Scheduler.Strategy
	}
	if fc.Scheduler.MaxQueueSize != nil {
		cfg.Scheduler.MaxQueueSize = *fc.Scheduler.MaxQueueSize
	}
	if fc.Scheduler.MaxPerAgent != nil {
		cfg.Scheduler.MaxPerAgent = *fc.Scheduler.MaxPerAgent
	}
	if fc.Scheduler.MaxInflight != nil {
		cfg.Scheduler.MaxInflight = *fc.Scheduler.MaxInflight
	}
	if fc.Scheduler.MaxPerAgentFlight != nil {
		cfg.Scheduler.MaxPerAgentFlight = *fc.Scheduler.MaxPerAgentFlight
	}
	if fc.Scheduler.ResultTTLSec != nil {
		cfg.Scheduler.ResultTTLSec = *fc.Scheduler.ResultTTLSec
	}
	if fc.Scheduler.WorkerCount != nil {
		cfg.Scheduler.WorkerCount = *fc.Scheduler.WorkerCount
	}
}

// ApplyFileConfigToRuntime merges file configuration into an existing runtime
// config and refreshes derived profile paths for long-running processes.
func ApplyFileConfigToRuntime(cfg *RuntimeConfig, fc *FileConfig) {
	if cfg == nil || fc == nil {
		return
	}

	applyFileConfig(cfg, fc)
	finalizeProfileConfig(cfg)
}

// DefaultFileConfig returns a FileConfig with sensible defaults (nested format).
func DefaultFileConfig() FileConfig {
	start := 9868
	end := 9968
	maxTabs := 20
	return FileConfig{
		Server: ServerConfig{
			Port:     "9867",
			Bind:     "127.0.0.1",
			StateDir: userConfigDir(),
		},
		Browser: BrowserConfig{
			ChromeVersion: "144.0.7559.133",
		},
		InstanceDefaults: InstanceDefaultsConfig{
			Mode:              "headless",
			MaxTabs:           &maxTabs,
			StealthLevel:      "light",
			TabEvictionPolicy: "reject",
		},
		Profiles: ProfilesConfig{
			BaseDir:        filepath.Join(userConfigDir(), "profiles"),
			DefaultProfile: "default",
		},
		MultiInstance: MultiInstanceConfig{
			Strategy:          "simple",
			AllocationPolicy:  "fcfs",
			InstancePortStart: &start,
			InstancePortEnd:   &end,
		},
		Attach: AttachConfig{
			AllowHosts:   []string{"127.0.0.1", "localhost", "::1"},
			AllowSchemes: []string{"ws", "wss"},
		},
		Timeouts: TimeoutsConfig{
			ActionSec:   30,
			NavigateSec: 60,
			ShutdownSec: 10,
			WaitNavMs:   1000,
		},
	}
}

// FileConfigFromRuntime converts the effective runtime configuration back into a
// nested file configuration shape. This is primarily used when orchestrator-launched
// child instances need a generated config file.
func FileConfigFromRuntime(cfg *RuntimeConfig) FileConfig {
	if cfg == nil {
		return DefaultFileConfig()
	}

	noRestore := cfg.NoRestore
	blockImages := cfg.BlockImages
	blockMedia := cfg.BlockMedia
	blockAds := cfg.BlockAds
	maxTabs := cfg.MaxTabs
	maxParallelTabs := cfg.MaxParallelTabs
	noAnimations := cfg.NoAnimations
	allowEvaluate := cfg.AllowEvaluate
	allowMacro := cfg.AllowMacro
	allowScreencast := cfg.AllowScreencast
	allowDownload := cfg.AllowDownload
	allowUpload := cfg.AllowUpload
	attachEnabled := cfg.AttachEnabled
	start := cfg.InstancePortStart
	end := cfg.InstancePortEnd

	mode := "headless"
	if !cfg.Headless {
		mode = "headed"
	}

	fc := FileConfig{
		Server: ServerConfig{
			Port:     cfg.Port,
			Bind:     cfg.Bind,
			Token:    cfg.Token,
			StateDir: cfg.StateDir,
		},
		Browser: BrowserConfig{
			ChromeVersion:    cfg.ChromeVersion,
			ChromeBinary:     cfg.ChromeBinary,
			ChromeExtraFlags: cfg.ChromeExtraFlags,
			ExtensionPaths:   append([]string(nil), cfg.ExtensionPaths...),
		},
		InstanceDefaults: InstanceDefaultsConfig{
			Mode:              mode,
			NoRestore:         &noRestore,
			Timezone:          cfg.Timezone,
			BlockImages:       &blockImages,
			BlockMedia:        &blockMedia,
			BlockAds:          &blockAds,
			MaxTabs:           &maxTabs,
			MaxParallelTabs:   &maxParallelTabs,
			UserAgent:         cfg.UserAgent,
			NoAnimations:      &noAnimations,
			StealthLevel:      cfg.StealthLevel,
			TabEvictionPolicy: cfg.TabEvictionPolicy,
		},
		Security: SecurityConfig{
			AllowEvaluate:   &allowEvaluate,
			AllowMacro:      &allowMacro,
			AllowScreencast: &allowScreencast,
			AllowDownload:   &allowDownload,
			AllowUpload:     &allowUpload,
		},
		Profiles: ProfilesConfig{
			BaseDir:        cfg.ProfilesBaseDir,
			DefaultProfile: cfg.DefaultProfile,
		},
		MultiInstance: MultiInstanceConfig{
			Strategy:          cfg.Strategy,
			AllocationPolicy:  cfg.AllocationPolicy,
			InstancePortStart: &start,
			InstancePortEnd:   &end,
		},
		Attach: AttachConfig{
			Enabled:      &attachEnabled,
			AllowHosts:   append([]string(nil), cfg.AttachAllowHosts...),
			AllowSchemes: append([]string(nil), cfg.AttachAllowSchemes...),
		},
		Timeouts: TimeoutsConfig{
			ActionSec:   int(cfg.ActionTimeout / time.Second),
			NavigateSec: int(cfg.NavigateTimeout / time.Second),
			ShutdownSec: int(cfg.ShutdownTimeout / time.Second),
			WaitNavMs:   int(cfg.WaitNavDelay / time.Millisecond),
		},
	}

	return fc
}

// HandleConfigCommand handles `pinchtab config <subcommand>`.
func HandleConfigCommand(cfg *RuntimeConfig) {
	if len(os.Args) < 3 {
		printConfigUsage()
		return
	}

	switch os.Args[2] {
	case "init":
		handleConfigInit()
	case "show":
		handleConfigShow(cfg)
	case "path":
		handleConfigPath()
	case "validate":
		handleConfigValidate()
	case "get":
		handleConfigGet()
	case "set":
		handleConfigSet()
	case "patch":
		handleConfigPatch()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[2])
		printConfigUsage()
		os.Exit(1)
	}
}

func printConfigUsage() {
	fmt.Println("Usage: pinchtab config <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init              Create default config file")
	fmt.Println("  show              Show current configuration")
	fmt.Println("  path              Show config file path")
	fmt.Println("  validate          Validate config file")
	fmt.Println("  get <path>        Get a config value (e.g., server.port)")
	fmt.Println("  set <path> <val>  Set a config value (e.g., server.port 8080)")
	fmt.Println("  patch <json>      Merge JSON into config")
}

func handleConfigGet() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: pinchtab config get <path>")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  pinchtab config get server.port")
		fmt.Println("  pinchtab config get instanceDefaults.mode")
		fmt.Println("  pinchtab config get attach.allowHosts")
		fmt.Println()
		fmt.Println("Sections: server, browser, instanceDefaults, security, profiles, multiInstance, attach, timeouts")
		os.Exit(1)
	}

	path := os.Args[3]

	fc, _, err := LoadFileConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	value, err := GetConfigValue(fc, path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(value)
}

func handleConfigInit() {
	configPath := filepath.Join(userConfigDir(), "config.json")

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config file already exists at %s\n", configPath)
		fmt.Print("Overwrite? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			return
		}
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		os.Exit(1)
	}

	fc := DefaultFileConfig()
	data, _ := json.MarshalIndent(fc, "", "  ")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		fmt.Printf("Error writing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config file created at %s\n", configPath)
	fmt.Println()
	fmt.Println("Example with auth token:")
	fmt.Println(`{
  "server": {
    "port": "9867",
    "token": "your-secret-token"
  },
  "instanceDefaults": {
    "mode": "headless",
    "maxTabs": 20
  },
  "profiles": {
    "defaultProfile": "default"
  }
}`)
}

func handleConfigShow(cfg *RuntimeConfig) {
	fmt.Println("Current configuration (env > file > defaults):")
	fmt.Println()
	fmt.Println("Server:")
	fmt.Printf("  Port:           %s\n", cfg.Port)
	fmt.Printf("  Bind:           %s\n", cfg.Bind)
	fmt.Printf("  Token:          %s\n", MaskToken(cfg.Token))
	fmt.Printf("  State Dir:      %s\n", cfg.StateDir)
	fmt.Printf("  Instance Ports: %d-%d\n", cfg.InstancePortStart, cfg.InstancePortEnd)
	fmt.Println()
	fmt.Println("Security:")
	fmt.Printf("  Evaluate:       %v\n", cfg.AllowEvaluate)
	fmt.Printf("  Macro:          %v\n", cfg.AllowMacro)
	fmt.Printf("  Screencast:     %v\n", cfg.AllowScreencast)
	fmt.Printf("  Download:       %v\n", cfg.AllowDownload)
	fmt.Printf("  Upload:         %v\n", cfg.AllowUpload)
	fmt.Println()
	fmt.Println("Browser / Instance Defaults:")
	fmt.Printf("  Headless:       %v\n", cfg.Headless)
	fmt.Printf("  No Restore:     %v\n", cfg.NoRestore)
	fmt.Printf("  Profile Dir:    %s\n", cfg.ProfileDir)
	fmt.Printf("  Profiles Dir:   %s\n", cfg.ProfilesBaseDir)
	fmt.Printf("  Default Profile: %s\n", cfg.DefaultProfile)
	fmt.Printf("  Max Tabs:       %d\n", cfg.MaxTabs)
	fmt.Printf("  Stealth:        %s\n", cfg.StealthLevel)
	fmt.Printf("  Tab Eviction:   %s\n", cfg.TabEvictionPolicy)
	fmt.Printf("  Extensions:     %v\n", cfg.ExtensionPaths)
	fmt.Println()
	fmt.Println("Multi-Instance:")
	fmt.Printf("  Strategy:       %s\n", cfg.Strategy)
	fmt.Printf("  Allocation:     %s\n", cfg.AllocationPolicy)
	fmt.Println()
	fmt.Println("Attach:")
	fmt.Printf("  Enabled:        %v\n", cfg.AttachEnabled)
	fmt.Printf("  Allow Hosts:    %v\n", cfg.AttachAllowHosts)
	fmt.Printf("  Allow Schemes:  %v\n", cfg.AttachAllowSchemes)
	fmt.Println()
	fmt.Println("Timeouts:")
	fmt.Printf("  Action:         %v\n", cfg.ActionTimeout)
	fmt.Printf("  Navigate:       %v\n", cfg.NavigateTimeout)
	fmt.Printf("  Shutdown:       %v\n", cfg.ShutdownTimeout)
}

func handleConfigPath() {
	configPath := envOr("PINCHTAB_CONFIG", filepath.Join(userConfigDir(), "config.json"))
	fmt.Println(configPath)
}

func handleConfigSet() {
	if len(os.Args) < 5 {
		fmt.Println("Usage: pinchtab config set <path> <value>")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  pinchtab config set server.port 8080")
		fmt.Println("  pinchtab config set instanceDefaults.mode headed")
		fmt.Println("  pinchtab config set multiInstance.strategy explicit")
		fmt.Println()
		fmt.Println("Sections: server, browser, instanceDefaults, security, profiles, multiInstance, attach, timeouts")
		os.Exit(1)
	}

	path := os.Args[3]
	value := os.Args[4]

	fc, configPath, err := LoadFileConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := SetConfigValue(fc, path, value); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Validate before saving
	if errs := ValidateFileConfig(fc); len(errs) > 0 {
		fmt.Printf("Warning: new value causes validation error(s):\n")
		for _, e := range errs {
			fmt.Printf("  - %v\n", e)
		}
		fmt.Print("Save anyway? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted.")
			return
		}
	}

	if err := SaveFileConfig(fc, configPath); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Set %s = %s\n", path, value)
}

func handleConfigPatch() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: pinchtab config patch '<json>'")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println(`  pinchtab config patch '{"server": {"port": "8080"}}'`)
		fmt.Println(`  pinchtab config patch '{"instanceDefaults": {"mode": "headed", "maxTabs": 50}}'`)
		os.Exit(1)
	}

	jsonPatch := os.Args[3]

	fc, configPath, err := LoadFileConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := PatchConfigJSON(fc, jsonPatch); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Validate before saving
	if errs := ValidateFileConfig(fc); len(errs) > 0 {
		fmt.Printf("Warning: patch causes validation error(s):\n")
		for _, e := range errs {
			fmt.Printf("  - %v\n", e)
		}
		fmt.Print("Save anyway? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Aborted.")
			return
		}
	}

	if err := SaveFileConfig(fc, configPath); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Config patched successfully")
}

func handleConfigValidate() {
	configPath := envOr("PINCHTAB_CONFIG", filepath.Join(userConfigDir(), "config.json"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		os.Exit(1)
	}

	var fc *FileConfig

	if isLegacyConfig(data) {
		var lc legacyFileConfig
		if err := json.Unmarshal(data, &lc); err != nil {
			fmt.Printf("Error parsing config file: %v\n", err)
			os.Exit(1)
		}
		fc = convertLegacyConfig(&lc)
		fmt.Println("Note: Config file uses legacy flat format. Consider migrating to nested format.")
		fmt.Println()
	} else {
		fc = &FileConfig{}
		if err := json.Unmarshal(data, fc); err != nil {
			fmt.Printf("Error parsing config file: %v\n", err)
			os.Exit(1)
		}
	}

	errs := ValidateFileConfig(fc)
	if len(errs) == 0 {
		fmt.Printf("✓ Config file is valid: %s\n", configPath)
		return
	}

	fmt.Printf("✗ Config file has %d error(s):\n", len(errs))
	for _, e := range errs {
		fmt.Printf("  - %v\n", e)
	}
	os.Exit(1)
}

// MaskToken masks a token for display (shows first/last 4 chars).
func MaskToken(t string) string {
	if t == "" {
		return "(none)"
	}
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}
