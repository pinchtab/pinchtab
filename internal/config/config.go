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

type RuntimeConfig struct {
	Bind              string
	Port              string
	InstancePortStart int // Starting port for instances (default 9868)
	InstancePortEnd   int // Ending port for instances (default 9968)
	CdpURL            string
	Token             string
	AllowEvaluate     bool
	StateDir          string
	Headless          bool
	NoRestore         bool
	ProfileDir        string
	ChromeVersion     string
	Timezone          string
	BlockImages       bool
	BlockMedia        bool
	BlockAds          bool
	MaxTabs           int
	ChromeBinary      string
	ChromeExtraFlags  string
	ExtensionPaths    []string
	UserAgent         string
	NoAnimations      bool
	StealthLevel      string
	ActionTimeout     time.Duration
	NavigateTimeout   time.Duration
	ShutdownTimeout   time.Duration
	WaitNavDelay      time.Duration
}

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

// splitCommaPaths splits a comma-separated string into non-empty trimmed paths.
func splitCommaPaths(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func envBoolOr(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envMigrate(newKey, oldKey string) string {
	if v := os.Getenv(newKey); v != "" {
		return v
	}
	if v := os.Getenv(oldKey); v != "" {
		slog.Warn("deprecated env var, use "+newKey+" instead", "var", oldKey)
		return v
	}
	return ""
}

func envOrMigrate(newKey, oldKey, fallback string) string {
	if v := envMigrate(newKey, oldKey); v != "" {
		return v
	}
	return fallback
}

func envIntOrMigrate(newKey, oldKey string, fallback int) int {
	v := envMigrate(newKey, oldKey)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func envBoolOrMigrate(newKey, oldKey string, fallback bool) bool {
	if v, ok := os.LookupEnv(newKey); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		default:
			return fallback
		}
	}
	if v, ok := os.LookupEnv(oldKey); ok {
		slog.Warn("deprecated env var, use "+newKey+" instead", "var", oldKey)
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		default:
			return fallback
		}
	}
	return fallback
}

func envMigrateIsSet(newKey, oldKey string) bool {
	if os.Getenv(newKey) != "" {
		return true
	}
	return os.Getenv(oldKey) != ""
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

type FileConfig struct {
	Port              string `json:"port"`
	InstancePortStart *int   `json:"instancePortStart,omitempty"`
	InstancePortEnd   *int   `json:"instancePortEnd,omitempty"`
	CdpURL            string `json:"cdpUrl,omitempty"`
	Token             string `json:"token,omitempty"`
	AllowEvaluate     *bool  `json:"allowEvaluate,omitempty"`
	StateDir          string `json:"stateDir"`
	ProfileDir        string `json:"profileDir"`
	Headless          *bool  `json:"headless,omitempty"`
	NoRestore         bool   `json:"noRestore"`
	MaxTabs           *int   `json:"maxTabs,omitempty"`
	TimeoutSec        int    `json:"timeoutSec,omitempty"`
	NavigateSec       int    `json:"navigateSec,omitempty"`
}

func Load() *RuntimeConfig {
	cfg := &RuntimeConfig{
		Bind:              envOrMigrate("PINCHTAB_BIND", "BRIDGE_BIND", "127.0.0.1"),
		Port:              envOrMigrate("PINCHTAB_PORT", "BRIDGE_PORT", "9867"),
		InstancePortStart: envIntOr("INSTANCE_PORT_START", 9868),
		InstancePortEnd:   envIntOr("INSTANCE_PORT_END", 9968),
		CdpURL:            os.Getenv("CDP_URL"),
		Token:             envMigrate("PINCHTAB_TOKEN", "BRIDGE_TOKEN"),
		AllowEvaluate:     envBoolOrMigrate("PINCHTAB_ALLOW_EVALUATE", "BRIDGE_ALLOW_EVALUATE", false),
		StateDir:          envOrMigrate("PINCHTAB_STATE_DIR", "BRIDGE_STATE_DIR", userConfigDir()),
		Headless:          envBoolOrMigrate("PINCHTAB_HEADLESS", "BRIDGE_HEADLESS", true),
		NoRestore:         envBoolOrMigrate("PINCHTAB_NO_RESTORE", "BRIDGE_NO_RESTORE", false),
		ProfileDir:        envOrMigrate("PINCHTAB_PROFILE_DIR", "BRIDGE_PROFILE", filepath.Join(userConfigDir(), "chrome-profile")),
		ChromeVersion:     envOrMigrate("PINCHTAB_CHROME_VERSION", "BRIDGE_CHROME_VERSION", "144.0.7559.133"),
		Timezone:          envMigrate("PINCHTAB_TIMEZONE", "BRIDGE_TIMEZONE"),
		BlockImages:       envBoolOrMigrate("PINCHTAB_BLOCK_IMAGES", "BRIDGE_BLOCK_IMAGES", false),
		BlockMedia:        envBoolOrMigrate("PINCHTAB_BLOCK_MEDIA", "BRIDGE_BLOCK_MEDIA", false),
		BlockAds:          envBoolOrMigrate("PINCHTAB_BLOCK_ADS", "BRIDGE_BLOCK_ADS", false),
		MaxTabs:           envIntOrMigrate("PINCHTAB_MAX_TABS", "BRIDGE_MAX_TABS", 20),
		ChromeBinary:      envOr("CHROME_BIN", os.Getenv("CHROME_BINARY")),
		ChromeExtraFlags:  os.Getenv("CHROME_FLAGS"),
		ExtensionPaths:    splitCommaPaths(os.Getenv("CHROME_EXTENSION_PATHS")),
		UserAgent:         envMigrate("PINCHTAB_USER_AGENT", "BRIDGE_USER_AGENT"),
		NoAnimations:      envBoolOrMigrate("PINCHTAB_NO_ANIMATIONS", "BRIDGE_NO_ANIMATIONS", false),
		StealthLevel:      envOrMigrate("PINCHTAB_STEALTH", "BRIDGE_STEALTH", "light"),
		ActionTimeout:     30 * time.Second,
		NavigateTimeout:   60 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		WaitNavDelay:      1 * time.Second,
	}

	configPath := envOrMigrate("PINCHTAB_CONFIG", "BRIDGE_CONFIG", filepath.Join(userConfigDir(), "config.json"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	var fc FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return cfg
	}

	if fc.Port != "" && !envMigrateIsSet("PINCHTAB_PORT", "BRIDGE_PORT") {
		cfg.Port = fc.Port
	}
	if fc.InstancePortStart != nil && os.Getenv("INSTANCE_PORT_START") == "" {
		cfg.InstancePortStart = *fc.InstancePortStart
	}
	if fc.InstancePortEnd != nil && os.Getenv("INSTANCE_PORT_END") == "" {
		cfg.InstancePortEnd = *fc.InstancePortEnd
	}
	if fc.CdpURL != "" && os.Getenv("CDP_URL") == "" {
		cfg.CdpURL = fc.CdpURL
	}
	if fc.Token != "" && !envMigrateIsSet("PINCHTAB_TOKEN", "BRIDGE_TOKEN") {
		cfg.Token = fc.Token
	}
	if fc.AllowEvaluate != nil && !envMigrateIsSet("PINCHTAB_ALLOW_EVALUATE", "BRIDGE_ALLOW_EVALUATE") {
		cfg.AllowEvaluate = *fc.AllowEvaluate
	}
	if fc.StateDir != "" && !envMigrateIsSet("PINCHTAB_STATE_DIR", "BRIDGE_STATE_DIR") {
		cfg.StateDir = fc.StateDir
	}
	if fc.ProfileDir != "" && !envMigrateIsSet("PINCHTAB_PROFILE_DIR", "BRIDGE_PROFILE") {
		cfg.ProfileDir = fc.ProfileDir
	}
	if fc.Headless != nil && !envMigrateIsSet("PINCHTAB_HEADLESS", "BRIDGE_HEADLESS") {
		cfg.Headless = *fc.Headless
	}
	if fc.NoRestore && !envMigrateIsSet("PINCHTAB_NO_RESTORE", "BRIDGE_NO_RESTORE") {
		cfg.NoRestore = true
	}
	if fc.MaxTabs != nil && !envMigrateIsSet("PINCHTAB_MAX_TABS", "BRIDGE_MAX_TABS") {
		cfg.MaxTabs = *fc.MaxTabs
	}
	if fc.TimeoutSec > 0 && !envMigrateIsSet("PINCHTAB_TIMEOUT", "BRIDGE_TIMEOUT") {
		cfg.ActionTimeout = time.Duration(fc.TimeoutSec) * time.Second
	}
	if fc.NavigateSec > 0 && !envMigrateIsSet("PINCHTAB_NAV_TIMEOUT", "BRIDGE_NAV_TIMEOUT") {
		cfg.NavigateTimeout = time.Duration(fc.NavigateSec) * time.Second
	}

	return cfg
}

func DefaultFileConfig() FileConfig {
	h := true
	start := 9868
	end := 9968
	return FileConfig{
		Port:              "9867",
		InstancePortStart: &start,
		InstancePortEnd:   &end,
		StateDir:          userConfigDir(),
		ProfileDir:        filepath.Join(userConfigDir(), "chrome-profile"),
		Headless:          &h,
		NoRestore:         false,
		TimeoutSec:        15,
		NavigateSec:       30,
	}
}

func HandleConfigCommand(cfg *RuntimeConfig) {
	if len(os.Args) < 3 {
		fmt.Println("Usage: pinchtab config <command>")
		fmt.Println("Commands:")
		fmt.Println("  init    - Create default config file")
		fmt.Println("  show    - Show current configuration")
		return
	}

	switch os.Args[2] {
	case "init":
		configPath := filepath.Join(homeDir(), ".pinchtab", "config.json")

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
		fmt.Println("\nExample with auth token:")
		fmt.Println(`{
  "port": "9867",
  "token": "your-secret-token",
  "headless": true,
  "stateDir": "` + fc.StateDir + `",
  "profileDir": "` + fc.ProfileDir + `"
}`)

	case "show":
		fmt.Println("Current configuration:")
		fmt.Printf("  Port:       %s\n", cfg.Port)
		fmt.Printf("  CDP URL:    %s\n", cfg.CdpURL)
		fmt.Printf("  Token:      %s\n", MaskToken(cfg.Token))
		fmt.Printf("  Eval JS:    %v\n", cfg.AllowEvaluate)
		fmt.Printf("  State Dir:  %s\n", cfg.StateDir)
		fmt.Printf("  Profile:    %s\n", cfg.ProfileDir)
		fmt.Printf("  Headless:   %v\n", cfg.Headless)
		fmt.Printf("  Max Tabs:   %d\n", cfg.MaxTabs)
		fmt.Printf("  No Restore: %v\n", cfg.NoRestore)
		fmt.Printf("  Extensions: %v\n", cfg.ExtensionPaths)
		fmt.Printf("  Timeouts:   action=%v navigate=%v\n", cfg.ActionTimeout, cfg.NavigateTimeout)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func MaskToken(t string) string {
	if t == "" {
		return "(none)"
	}
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "..." + t[len(t)-4:]
}
